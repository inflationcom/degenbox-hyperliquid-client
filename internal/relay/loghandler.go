package relay

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type LogSender interface {
	SendLog(msg clientLogMsg)
}

type WSLogHandler struct {
	level  slog.Leveler
	sender LogSender
	attrs  []slog.Attr
	mu     sync.Mutex
}

func NewWSLogHandler(level slog.Leveler, sender LogSender) *WSLogHandler {
	return &WSLogHandler{level: level, sender: sender}
}

func (h *WSLogHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *WSLogHandler) Handle(_ context.Context, r slog.Record) error {
	level := "info"
	switch {
	case r.Level >= slog.LevelError:
		level = "error"
	case r.Level >= slog.LevelWarn:
		level = "warn"
	case r.Level >= slog.LevelInfo:
		level = "info"
	default:
		level = "debug"
	}

	attrs := make(map[string]string)
	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})

	var attrMap map[string]string
	if len(attrs) > 0 {
		attrMap = attrs
	}

	h.sender.SendLog(clientLogMsg{
		relayMsg: relayMsg{Type: "client_log", Timestamp: time.Now().UnixMilli()},
		Level:    level,
		Message:  r.Message,
		Attrs:    attrMap,
	})

	return nil
}

func (h *WSLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &WSLogHandler{level: h.level, sender: h.sender, attrs: newAttrs}
}

func (h *WSLogHandler) WithGroup(_ string) slog.Handler {
	return h
}
