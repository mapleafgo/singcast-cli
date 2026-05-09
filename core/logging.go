package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

const maxCoreLogEntries = 500

// Log level constants. Lower number = higher severity.
const (
	LogLevelError int32 = 2
	LogLevelWarn  int32 = 3
	LogLevelInfo  int32 = 4
	LogLevelDebug int32 = 5
	LogLevelTrace int32 = 6
)

var (
	coreLogMu sync.Mutex

	// Fixed-size ring buffer avoids slice-shift memory waste.
	coreLogRing [maxCoreLogEntries]LogEntry
	coreLogPos  int // next write position
	coreLogLen  int // current length (0..maxCoreLogEntries)

	// Runtime log level filter. Lower number = higher severity.
	// Error=2, Warn=3, Info=4, Debug=5, Trace=6.
	coreLogLevel atomic.Int32
)

func init() {
	coreLogLevel.Store(LogLevelInfo)
	slog.SetDefault(slog.New(&coreLogHandler{}))
}

// SetLogLevel sets the minimum log level (2=Error, 3=Warn, 4=Info, 5=Debug, 6=Trace).
func SetLogLevel(level int32) { coreLogLevel.Store(level) }

// GetLogLevel returns the current minimum log level.
func GetLogLevel() int32 { return coreLogLevel.Load() }

// coreLogHandler implements slog.Handler, routing log entries to an in-memory
// ring buffer and the event callback. No file I/O is performed; the frontend
// handles all log persistence.
type coreLogHandler struct {
	extra string // accumulated key=value pairs from WithAttrs
}

func (h *coreLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return slogToCoreLevel(level) <= coreLogLevel.Load()
}

func (h *coreLogHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	if h.extra != "" {
		b.WriteString(h.extra)
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})

	// Always mirror to stderr for debugging (gomobile forwards to Android logcat).
	fmt.Fprintln(os.Stderr, b.String())

	entry := LogEntry{
		Level:     slogToCoreLevel(r.Level),
		Message:   b.String(),
		Timestamp: r.Time.UnixMilli(),
	}

	coreLogMu.Lock()
	coreLogRing[coreLogPos] = entry
	coreLogPos = (coreLogPos + 1) % maxCoreLogEntries
	if coreLogLen < maxCoreLogEntries {
		coreLogLen++
	}
	coreLogMu.Unlock()
	return nil
}

func (h *coreLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var b strings.Builder
	b.WriteString(h.extra)
	for _, a := range attrs {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
	}
	return &coreLogHandler{extra: b.String()}
}

func (h *coreLogHandler) WithGroup(name string) slog.Handler { return h }

// parseLogLevel converts a log level string to its numeric value.
// Returns (0, false) for unrecognized strings.
func parseLogLevel(s string) (int32, bool) {
	switch s {
	case "trace":
		return LogLevelTrace, true
	case "debug":
		return LogLevelDebug, true
	case "info":
		return LogLevelInfo, true
	case "warn":
		return LogLevelWarn, true
	case "error":
		return LogLevelError, true
	default:
		return 0, false
	}
}

func slogToCoreLevel(level slog.Level) int32 {
	switch {
	case level >= slog.LevelError:
		return LogLevelError
	case level >= slog.LevelWarn:
		return LogLevelWarn
	case level >= slog.LevelInfo:
		return LogLevelInfo
	case level >= slog.LevelDebug:
		return LogLevelDebug
	default:
		return LogLevelTrace
	}
}

// syncLogLevelFromConfig parses log.level from sing-box JSON config and
// sets coreLogLevel to match, so kernel diagnostic logs follow the config.
func syncLogLevelFromConfig(jsonContent string) {
	var cfg struct {
		Log struct {
			Level string `json:"level"`
		} `json:"log"`
	}
	if err := json.Unmarshal([]byte(jsonContent), &cfg); err != nil {
		slog.Debug("[syncLogLevel] parse config failed", "error", err)
		return
	}
	newLevel, ok := parseLogLevel(cfg.Log.Level)
	if !ok {
		return
	}
	oldLevel := GetLogLevel()
	slog.Info("[syncLogLevel] syncing log level", "configLevel", cfg.Log.Level, "oldLevel", oldLevel, "newLevel", newLevel)
	SetLogLevel(newLevel)
}

func queryCoreLogs() string {
	entries := queryCoreLogEntries()
	if len(entries) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(entries)
	return string(data)
}

func queryCoreLogEntries() []LogEntry {
	coreLogMu.Lock()
	defer coreLogMu.Unlock()
	if coreLogLen == 0 {
		return nil
	}
	entries := make([]LogEntry, 0, coreLogLen)
	start := (coreLogPos - coreLogLen + maxCoreLogEntries) % maxCoreLogEntries
	for i := range coreLogLen {
		entries = append(entries, coreLogRing[(start+i)%maxCoreLogEntries])
	}
	return entries
}
