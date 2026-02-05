package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var log *zerolog.Logger

func Init(serviceName, env string) {
	isProduction := strings.EqualFold(env, "production")

	l := zerolog.New(os.Stdout).With().
		Str("service", serviceName).
		Str("env", strings.ToLower(env)).
		Logger()

	if isProduction {
		zerolog.TimeFieldFormat = time.RFC3339Nano
	} else {
		l = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().
			Str("service", serviceName).
			Str("env", strings.ToLower(env)).
			Logger()
	}

	log = &l
}

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
