package metrics

import (
	"context"
	"strings"
)

type ctxKey string

const (
	tierKey     ctxKey = "tier"
	strategyKey ctxKey = "strategy"
)

const defaultStrategy = "default"

var supportedStrategies = map[string]struct{}{
	"balanced":            {},
	"cheap_fast":          {},
	"reliable_structured": {},
}

func SetTier(ctx context.Context, tier string) context.Context {
	return context.WithValue(ctx, tierKey, tier)
}

func GetTier(ctx context.Context) string {
	if v, ok := ctx.Value(tierKey).(string); ok {
		return v
	}
	return "unknown"
}

func SetStrategy(ctx context.Context, strategy string) context.Context {
	return context.WithValue(ctx, strategyKey, normalizeStrategy(strategy))
}

func GetStrategy(ctx context.Context) string {
	if v, ok := ctx.Value(strategyKey).(string); ok {
		return v
	}
	return defaultStrategy
}

func normalizeStrategy(strategy string) string {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if _, ok := supportedStrategies[strategy]; ok {
		return strategy
	}
	return defaultStrategy
}
