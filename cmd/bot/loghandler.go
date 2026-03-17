package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type logMsg string

type TUILogHandler struct {
	level   slog.Leveler
	program *tea.Program
	mu      sync.Mutex
	attrs   []slog.Attr
	buf     []string // buffer for pre-program log lines
}

func NewTUILogHandler(level slog.Leveler) *TUILogHandler {
	return &TUILogHandler{level: level}
}

func (h *TUILogHandler) SetProgram(p *tea.Program) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.program = p
	// Don't flush here - p.Send before p.Run is dropped by bubbletea.
	// Buffered lines are flushed via FlushCmd() in the model's Init().
}

// FlushCmd returns a tea.Cmd that sends all buffered log lines to the TUI.
// Call this from Init() after the program is running.
func (h *TUILogHandler) FlushCmd() tea.Cmd {
	h.mu.Lock()
	lines := h.buf
	h.buf = nil
	h.mu.Unlock()

	if len(lines) == 0 {
		return nil
	}

	return func() tea.Msg {
		return flushLogsMsg(lines)
	}
}

type flushLogsMsg []string

func (h *TUILogHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *TUILogHandler) Handle(_ context.Context, r slog.Record) error {
	ts := styleLogTime.Render(r.Time.Format(time.TimeOnly))

	lvl, lvlRender := levelStyle(r.Level)
	tag := lvlRender(fmt.Sprintf("%-5s", lvl))

	var side string
	var parts []string
	writeAttr := func(a slog.Attr) {
		if a.Key == "side" {
			side = a.Value.String()
		}
		parts = append(parts, styleLogAttr.Render(a.Key+"=")+a.Value.String())
	}
	for _, a := range h.attrs {
		writeAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(a)
		return true
	})

	msg := r.Message
	if side == "BUY" {
		msg = styleGreen.Render(msg)
	} else if side == "SELL" {
		msg = styleRed.Render(msg)
	}

	line := fmt.Sprintf(" %s %s %s", ts, tag, msg)
	if len(parts) > 0 {
		line += "  " + strings.Join(parts, "  ")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.program != nil {
		h.program.Send(logMsg(line))
	} else {
		h.buf = append(h.buf, line)
	}

	return nil
}

func (h *TUILogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	prog := h.program
	h.mu.Unlock()

	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &TUILogHandler{
		level:   h.level,
		program: prog,
		attrs:   newAttrs,
	}
}

func (h *TUILogHandler) WithGroup(_ string) slog.Handler {
	return h
}

func levelStyle(l slog.Level) (string, func(...string) string) {
	switch {
	case l >= slog.LevelError:
		return "ERROR", styleLogError.Render
	case l >= slog.LevelWarn:
		return "WARN", styleLogWarn.Render
	case l >= slog.LevelInfo:
		return "INFO", styleLogInfo.Render
	default:
		return "DEBUG", styleLogDebug.Render
	}
}

type MultiHandler struct {
	mu       sync.RWMutex
	handlers []slog.Handler
}

func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

func (m *MultiHandler) Add(h slog.Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, h)
}

func (m *MultiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r)
		}
	}
	return nil
}

func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

func (m *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}
