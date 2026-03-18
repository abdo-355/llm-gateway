package services

import "time"

const (
	defaultRequestTimeout          = 5 * time.Minute
	defaultRequestTimeoutMs        = int(defaultRequestTimeout / time.Millisecond)
	defaultStreamInactivityTimeout = defaultRequestTimeout
)
