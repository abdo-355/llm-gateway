package metrics

import "context"

type ctxKey string

const (
	tierKey     ctxKey = "tier"
	strategyKey ctxKey = "strategy"
)

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
	return context.WithValue(ctx, strategyKey, strategy)
}

func GetStrategy(ctx context.Context) string {
	if v, ok := ctx.Value(strategyKey).(string); ok {
		return v
	}
	return "default"
}
