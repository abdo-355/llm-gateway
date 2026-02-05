package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var log zerolog.Logger

func Init(serviceName, env string) {
	isProduction := strings.EqualFold(env, "production")

	if isProduction {
		zerolog.TimeFieldFormat = time.RFC3339Nano
		log = zerolog.New(os.Stdout).With().
			Str("service", serviceName).
			Str("env", strings.ToLower(env)).
			Logger()
	} else {
		log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().
			Str("service", serviceName).
			Str("env", strings.ToLower(env)).
			Logger()
	}
}

// Get returns the logger instance (for internal use)
func Get() zerolog.Logger {
	return log
}

// Convenience methods for clean API
func Info() *zerolog.Event {
	return log.Info()
}

func Error() *zerolog.Event {
	return log.Error()
}

func Debug() *zerolog.Event {
	return log.Debug()
}

func Warn() *zerolog.Event {
	return log.Warn()
}
