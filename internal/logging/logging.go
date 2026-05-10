package logging

import (
	"io"
	"log/slog"
	"os"
)

// Level represents logging severity.
type Level slog.Level

const (
	LevelDebug Level = Level(slog.LevelDebug)
	LevelInfo  Level = Level(slog.LevelInfo)
	LevelWarn  Level = Level(slog.LevelWarn)
	LevelError Level = Level(slog.LevelError)
)

type Logger struct {
	inner *slog.Logger
	level *slog.LevelVar
}

var defaultLogger *Logger

func init() {
	defaultLogger = NewLogger(LevelInfo, false, os.Stderr)
}

// NewLogger creates a structured logger. Set json=true for JSON output.
func NewLogger(level Level, json bool, w io.Writer) *Logger {
	lvl := &slog.LevelVar{}
	lvl.Set(slog.Level(level))

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: lvl}
	if json {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return &Logger{inner: slog.New(handler), level: lvl}
}

// Package-level convenience functions.

func Debug(msg string, args ...any) {
	defaultLogger.inner.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	defaultLogger.inner.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	defaultLogger.inner.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	defaultLogger.inner.Error(msg, args...)
}
