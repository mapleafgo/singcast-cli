package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

const maxCoreLogEntries = 500

var (
	coreLogMu sync.Mutex
	coreLogCb func(eventType int32, jsonPayload string)

	// Fixed-size ring buffer avoids slice-shift memory waste.
	coreLogRing [maxCoreLogEntries]LogEntry
	coreLogPos  int // next write position
	coreLogLen  int // current length (0..maxCoreLogEntries)
)

func init() {
	slog.SetDefault(slog.New(&coreLogHandler{}))
}

// coreLogHandler implements slog.Handler, routing log entries to an in-memory
// ring buffer and the event callback. No file I/O is performed; the frontend
// handles all log persistence.
type coreLogHandler struct {
	extra string // accumulated key=value pairs from WithAttrs
}

func (h *coreLogHandler) Enabled(context.Context, slog.Level) bool { return true }

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

func queryCoreLogs() string {
	coreLogMu.Lock()
	defer coreLogMu.Unlock()
	if coreLogLen == 0 {
		return "[]"
	}
	entries := make([]LogEntry, 0, coreLogLen)
	start := (coreLogPos - coreLogLen + maxCoreLogEntries) % maxCoreLogEntries
	for i := 0; i < coreLogLen; i++ {
		entries = append(entries, coreLogRing[(start+i)%maxCoreLogEntries])
	}
	data, _ := json.Marshal(entries)
	return string(data)
}
