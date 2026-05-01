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
	coreLogBuf []LogEntry
	coreLogMu  sync.Mutex
	coreLogCb  func(eventType int, jsonPayload string)
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
	if len(coreLogBuf) >= maxCoreLogEntries {
		coreLogBuf = coreLogBuf[1:]
	}
	coreLogBuf = append(coreLogBuf, entry)
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

func setLogCallback(fn func(int, string)) {
	coreLogMu.Lock()
	coreLogCb = fn
	coreLogMu.Unlock()
}

func queryCoreLogs() string {
	coreLogMu.Lock()
	defer coreLogMu.Unlock()
	if len(coreLogBuf) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(coreLogBuf)
	return string(data)
}
