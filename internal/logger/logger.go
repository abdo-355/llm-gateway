package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var log *zerolog.Logger

func Init(serviceName, env, level string) {
	isProduction := strings.EqualFold(env, "production")

	lvl := parseLevel(level)

	var l zerolog.Logger

	if isProduction {
		zerolog.TimeFieldFormat = time.RFC3339Nano
		l = zerolog.New(os.Stdout).Level(lvl).With().
			Timestamp().
			Str("service", serviceName).
			Str("env", strings.ToLower(env)).
			Logger()
	} else {
		l = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).Level(lvl).With().
			Timestamp().
			Str("service", serviceName).
			Str("env", strings.ToLower(env)).
			Logger()
	}

	log = &l
}

func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

var noopLogger = zerolog.Nop()

func Info() *zerolog.Event {
	if log == nil {
		return noopLogger.Info()
	}
	return log.Info()
}

func Error() *zerolog.Event {
	if log == nil {
		return noopLogger.Error()
	}
	return log.Error()
}

func Debug() *zerolog.Event {
	if log == nil {
		return noopLogger.Debug()
	}
	return log.Debug()
}

func Warn() *zerolog.Event {
	if log == nil {
		return noopLogger.Warn()
	}
	return log.Warn()
}
