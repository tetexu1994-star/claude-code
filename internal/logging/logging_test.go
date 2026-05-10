package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	t.Parallel()

	t.Run("text format", func(t *testing.T) {
		var buf bytes.Buffer
		logger := NewLogger(LevelInfo, false, &buf)
		if logger == nil {
			t.Fatal("expected logger, got nil")
		}
		if logger.inner == nil {
			t.Fatal("expected inner logger, got nil")
		}
		logger.inner.Info("test message")
		output := buf.String()
		if !strings.Contains(output, "test message") {
			t.Errorf("expected 'test message' in output, got %q", output)
		}
		if !strings.Contains(output, "INFO") {
			t.Errorf("expected 'INFO' level, got %q", output)
		}
	})

	t.Run("json format", func(t *testing.T) {
		var buf bytes.Buffer
		logger := NewLogger(LevelWarn, true, &buf)
		if logger == nil {
			t.Fatal("expected logger, got nil")
		}
		logger.inner.Warn("json test", "key", "value")
		output := buf.String()
		if !strings.Contains(output, "\"msg\":\"json test\"") {
			t.Errorf("expected JSON msg field, got %q", output)
		}
		if !strings.Contains(output, "\"level\":\"WARN\"") {
			t.Errorf("expected JSON WARN level, got %q", output)
		}
	})

	t.Run("debug level filtered out at info", func(t *testing.T) {
		var buf bytes.Buffer
		logger := NewLogger(LevelInfo, false, &buf)
		logger.inner.Debug("should not appear")
		output := buf.String()
		if output != "" {
			t.Errorf("expected empty output, got %q", output)
		}
	})

	t.Run("debug level passes at debug", func(t *testing.T) {
		var buf bytes.Buffer
		logger := NewLogger(LevelDebug, false, &buf)
		logger.inner.Debug("debug output")
		output := buf.String()
		if !strings.Contains(output, "debug output") {
			t.Errorf("expected debug output, got %q", output)
		}
	})

	t.Run("error level", func(t *testing.T) {
		var buf bytes.Buffer
		logger := NewLogger(LevelError, false, &buf)
		logger.inner.Error("error occurred", "code", 500)
		output := buf.String()
		if !strings.Contains(output, "error occurred") {
			t.Errorf("expected error message, got %q", output)
		}
		if !strings.Contains(output, "ERROR") {
			t.Errorf("expected ERROR level, got %q", output)
		}
	})
}

func TestLevelConstants(t *testing.T) {
	t.Parallel()

	if LevelDebug != -4 {
		t.Errorf("expected LevelDebug -4, got %d", LevelDebug)
	}
	if LevelInfo != 0 {
		t.Errorf("expected LevelInfo 0, got %d", LevelInfo)
	}
	if LevelWarn != 4 {
		t.Errorf("expected LevelWarn 4, got %d", LevelWarn)
	}
	if LevelError != 8 {
		t.Errorf("expected LevelError 8, got %d", LevelError)
	}
}

func TestPackageLevelLogging(t *testing.T) {
	// Replace the default logger with a buffer-backed one for testing.
	var buf bytes.Buffer
	orig := defaultLogger
	defaultLogger = NewLogger(LevelDebug, false, &buf)
	defer func() { defaultLogger = orig }()

	Info("test info", "key", "val")
	output := buf.String()
	if !strings.Contains(output, "test info") {
		t.Errorf("expected 'test info', got %q", output)
	}

	buf.Reset()
	Warn("test warning")
	output = buf.String()
	if !strings.Contains(output, "test warning") {
		t.Errorf("expected 'test warning', got %q", output)
	}

	buf.Reset()
	Error("test error")
	output = buf.String()
	if !strings.Contains(output, "test error") {
		t.Errorf("expected 'test error', got %q", output)
	}

	buf.Reset()
	Debug("test debug")
	output = buf.String()
	if !strings.Contains(output, "test debug") {
		t.Errorf("expected 'test debug', got %q", output)
	}
}

func TestInitDefaultLogger(t *testing.T) {
	// The init function runs at package load time and creates a non-nil default logger.
	if defaultLogger == nil {
		t.Fatal("expected defaultLogger to be initialized by init()")
	}
	if defaultLogger.inner == nil {
		t.Fatal("expected defaultLogger.inner to be non-nil")
	}
}

func TestLoggerStruct(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLogger(LevelInfo, false, &buf)
	if logger.inner == nil {
		t.Fatal("expected inner logger")
	}
	if logger.level == nil {
		t.Fatal("expected level var")
	}
}
