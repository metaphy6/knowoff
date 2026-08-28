// Package logging provides safe, human-readable development log output.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ConsoleHandler writes one sanitized, readable line per slog record.
type ConsoleHandler struct {
	writer    io.Writer
	minLevel  slog.Level
	attrs     []slog.Attr
	groups    []string
	addSource bool
	mu        *sync.Mutex
}

// NewConsoleHandler creates a text handler suitable for terminals and Docker logs.
func NewConsoleHandler(writer io.Writer, minLevel slog.Level) *ConsoleHandler {
	return &ConsoleHandler{writer: writer, minLevel: minLevel, mu: &sync.Mutex{}}
}

// NewConsoleHandlerWithOptions preserves the slog text-handler configuration.
func NewConsoleHandlerWithOptions(writer io.Writer, options *slog.HandlerOptions) *ConsoleHandler {
	return &ConsoleHandler{
		writer:    writer,
		minLevel:  options.Level.Level(),
		addSource: options.AddSource,
		mu:        &sync.Mutex{},
	}
}

func (handler *ConsoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= handler.minLevel
}

func (handler *ConsoleHandler) Handle(_ context.Context, record slog.Record) error {
	attributes := append([]slog.Attr{}, handler.attrs...)
	record.Attrs(func(attribute slog.Attr) bool {
		attributes = append(attributes, attribute)
		return true
	})

	parts := make([]string, 0, len(attributes))
	topic := "🧩"
	for _, attribute := range attributes {
		if attribute.Equal(slog.Attr{}) {
			continue
		}
		key := strings.Join(append(handler.groups, attribute.Key), ".")
		value := attribute.Value.Resolve()
		if key == "dependency" {
			topic = topicEmoji(value.String())
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, formatValue(key, value)))
	}

	line := fmt.Sprintf(
		"%s  %s %s [%s]  %s",
		record.Time.Format(time.TimeOnly),
		severityEmoji(record.Level),
		topic,
		record.Level.String(),
		sanitize(record.Message),
	)
	if len(parts) > 0 {
		line += "\n    " + strings.Join(parts, "  ")
	}
	if handler.addSource && record.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{record.PC})
		frame, _ := frames.Next()
		line += fmt.Sprintf("\n    source=%s:%d", filepath.Base(frame.File), frame.Line)
	}
	line += "\n"

	handler.mu.Lock()
	defer handler.mu.Unlock()
	_, err := io.WriteString(handler.writer, line)
	return err
}

func (handler *ConsoleHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	clone := *handler
	clone.attrs = append(append([]slog.Attr{}, handler.attrs...), attributes...)
	return &clone
}

func (handler *ConsoleHandler) WithGroup(name string) slog.Handler {
	clone := *handler
	if name != "" {
		clone.groups = append(append([]string{}, handler.groups...), name)
	}
	return &clone
}

func severityEmoji(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "🚨"
	case level >= slog.LevelWarn:
		return "⚠️"
	case level <= slog.LevelDebug:
		return "🔎"
	default:
		return "ℹ️"
	}
}

func topicEmoji(topic string) string {
	switch strings.ToLower(topic) {
	case "postgres", "postgresql", "database":
		return "🐘"
	case "redis":
		return "⚡"
	case "storage", "minio":
		return "🗄️"
	default:
		return "🧩"
	}
}

func formatValue(key string, value slog.Value) string {
	if isSensitive(key) {
		return "[REDACTED]"
	}
	if value.Kind() == slog.KindString {
		return sanitize(value.String())
	}
	return sanitize(value.String())
}

func isSensitive(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "password") || strings.Contains(key, "secret") ||
		strings.Contains(key, "token") || strings.Contains(key, "authorization") ||
		strings.Contains(key, "cookie")
}

func sanitize(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(value)
}
