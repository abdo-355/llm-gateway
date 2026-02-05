package lib

import (
	"log/slog"
	"os"

	"github.com/abdo-355/llm-gateway/internal/config"
)

type Logger struct {
	*slog.Logger
}

func NewLogger(env *config.EnvConfig) *Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug, // Allow all log levels through
	}

	if env.Environment == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return &Logger{Logger: slog.New(handler)}
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}

var logger *Logger

func InitLogger(env *config.EnvConfig) {
	logger = NewLogger(env)
}

func Info(msg string, args ...any) {
	if logger != nil {
		logger.Info(msg, args...)
	}
}

func Error(msg string, args ...any) {
	if logger != nil {
		logger.Error(msg, args...)
	}
}

func Warn(msg string, args ...any) {
	if logger != nil {
		logger.Warn(msg, args...)
	}
}

func Debug(msg string, args ...any) {
	if logger != nil {
		logger.Debug(msg, args...)
	}
}
