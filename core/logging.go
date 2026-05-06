package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

const maxCoreLogEntries = 500

var (
	coreLogMu sync.Mutex
	coreLogCb func(eventType int32, jsonPayload string)

	// Fixed-size ring buffer avoids slice-shift memory waste.
	coreLogRing [maxCoreLogEntries]LogEntry
	coreLogPos  int // next write position
	coreLogLen  int // current length (0..maxCoreLogEntries)

	// Runtime log level filter. Lower number = higher severity.
	// Error=2, Warn=3, Info=4, Debug=5, Trace=6.
	coreLogLevel atomic.Int32
)

func init() {
	coreLogLevel.Store(4) // Info
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

	entry := LogEntry{
		Level:   slogToCoreLevel(r.Level),
		Message: b.String(),
	}

	coreLogMu.Lock()
	coreLogRing[coreLogPos] = entry
	coreLogPos = (coreLogPos + 1) % maxCoreLogEntries
	if coreLogLen < maxCoreLogEntries {
		coreLogLen++
	}
	cb := coreLogCb
	coreLogMu.Unlock()

	if cb != nil {
		data, _ := json.Marshal([]LogEntry{entry})
		cb(EventCoreLog, string(data))
	}
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

func slogToCoreLevel(level slog.Level) int32 {
	switch {
	case level >= slog.LevelError:
		return 2 // Error
	case level >= slog.LevelWarn:
		return 3 // Warn
	case level >= slog.LevelInfo:
		return 4 // Info
	case level >= slog.LevelDebug:
		return 5 // Debug
	default:
		return 6 // Trace
	}
}

func setLogCallback(fn func(int32, string)) {
	coreLogMu.Lock()
	coreLogCb = fn
	coreLogMu.Unlock()
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
		return
	}
	switch cfg.Log.Level {
	case "trace":
		SetLogLevel(6)
	case "debug":
		SetLogLevel(5)
	case "info":
		SetLogLevel(4)
	case "warn":
		SetLogLevel(3)
	case "error":
		SetLogLevel(2)
	default:
		return
	}
	slog.Debug("[syncLogLevel] synced from config", "configLevel", cfg.Log.Level, "coreLevel", GetLogLevel())
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
	for i := 0; i < coreLogLen; i++ {
		entries = append(entries, coreLogRing[(start+i)%maxCoreLogEntries])
	}
	return entries
}
