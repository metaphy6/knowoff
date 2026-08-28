package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestConsoleHandlerFormatsSeverityAndDependency(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewConsoleHandler(&output, slog.LevelDebug))
	logger.LogAttrs(
		context.Background(),
		slog.LevelWarn,
		"dependency degraded",
		slog.String("dependency", "postgres"),
		slog.String("error", "connection refused\nretrying"),
	)

	line := output.String()
	for _, expected := range []string{
		"WARN",
		"⚠️",
		"🐘",
		"dependency degraded",
		"dependency=postgres",
		"error=connection refused retrying",
	} {
		if !strings.Contains(line, expected) {
			t.Errorf("log line %q does not contain %q", line, expected)
		}
	}
	if strings.Count(line, "\n") != 2 {
		t.Errorf("log record must have one message and one attribute line, got %q", line)
	}
}

func TestConsoleHandlerRedactsSensitiveFields(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewConsoleHandler(&output, slog.LevelDebug))
	logger.Info("session started", "access_token", "private-value")

	if strings.Contains(output.String(), "private-value") {
		t.Fatalf("sensitive value was logged: %q", output.String())
	}
	if !strings.Contains(output.String(), "access_token=[REDACTED]") {
		t.Fatalf("sensitive field was not marked redacted: %q", output.String())
	}
}

func TestConsoleHandlerFiltersBelowConfiguredLevel(t *testing.T) {
	var output bytes.Buffer
	handler := NewConsoleHandler(&output, slog.LevelInfo)
	record := slog.NewRecord(time.Now(), slog.LevelDebug, "not emitted", 0)
	if handler.Enabled(context.Background(), record.Level) {
		t.Fatal("debug record must be disabled at info level")
	}
}
