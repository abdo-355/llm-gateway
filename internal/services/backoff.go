package services

import (
	"math/rand"
	"time"
)

// BackoffStrategy calculates backoff durations
type BackoffStrategy interface {
	CalculateBackoff(attempt int) time.Duration
	GetStrategyName() string
}

// ExponentialBackoff implements exponential backoff with jitter
type ExponentialBackoff struct {
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         float64
}

// NewExponentialBackoff creates exponential backoff strategy
func NewExponentialBackoff(initial, max time.Duration, multiplier, jitter float64) *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialBackoff: initial,
		MaxBackoff:     max,
		Multiplier:     multiplier,
		Jitter:         jitter,
	}
}

func (e *ExponentialBackoff) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return e.InitialBackoff
	}

	// Exponential: initial * multiplier^attempt
	backoff := float64(e.InitialBackoff)
	for i := 0; i < attempt; i++ {
		backoff *= e.Multiplier
	}

	// Cap at max
	if backoff > float64(e.MaxBackoff) {
		backoff = float64(e.MaxBackoff)
	}

	// Apply jitter
	backoff = applyJitter(backoff, e.Jitter)

	return time.Duration(backoff)
}

func (e *ExponentialBackoff) GetStrategyName() string {
	return "exponential"
}

// LinearBackoff implements linear backoff with jitter
type LinearBackoff struct {
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Increment      time.Duration
	Jitter         float64
}

func (l *LinearBackoff) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return l.InitialBackoff
	}

	backoff := float64(l.InitialBackoff) + float64(l.Increment)*float64(attempt)

	// Cap at max
	if backoff > float64(l.MaxBackoff) {
		backoff = float64(l.MaxBackoff)
	}

	// Apply jitter
	backoff = applyJitter(backoff, l.Jitter)

	return time.Duration(backoff)
}

func (l *LinearBackoff) GetStrategyName() string {
	return "linear"
}

// applyJitter adds randomness to prevent thundering herd
func applyJitter(baseValue float64, jitterFactor float64) float64 {
	if jitterFactor <= 0 {
		return baseValue
	}

	// Random factor: 1.0 ± jitterFactor
	// E.g., jitterFactor=0.5 means backoff is in [0.5*base, 1.5*base]
	randomFactor := 1.0 + (rand.Float64()*2-1.0)*jitterFactor
	return baseValue * randomFactor
}

// DefaultBackoffStrategy returns the default backoff strategy
func DefaultBackoffStrategy() BackoffStrategy {
	return NewExponentialBackoff(
		250*time.Millisecond,  // initial
		8000*time.Millisecond, // max
		2.0,                   // multiplier
		0.5,                   // jitter
	)
}
