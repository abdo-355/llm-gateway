// Package lib provides shared utilities.
package lib

import (
	"log/slog"
	"os"
)

// Logger wraps slog.Logger with additional functionality
type Logger struct {
	*slog.Logger
}

// NewLogger creates a new logger instance
func NewLogger() *Logger {
	logLevel := getLogLevel()

	var handler slog.Handler
	if os.Getenv("NODE_ENV") == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

func getLogLevel() slog.Level {
	level := os.Getenv("LOG_LEVEL")
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithContext returns a logger with context fields
func (l *Logger) WithContext(args ...interface{}) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
	}
}

// WithRequestID returns a logger with request ID
func (l *Logger) WithRequestID(requestID string) *Logger {
	return l.WithContext("request_id", requestID)
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.Logger.Warn(msg, args...)
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}

// global logger instance
var globalLogger = NewLogger()

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	return globalLogger
}
