package metrics

import "context"

type ctxKey string

const (
	logicalModelKey  ctxKey = "logical_model"
	routerProfileKey ctxKey = "router_profile"
)

func SetLogicalModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, logicalModelKey, model)
}

func GetLogicalModel(ctx context.Context) string {
	if v, ok := ctx.Value(logicalModelKey).(string); ok {
		return v
	}
	return "unknown"
}

func SetRouterProfile(ctx context.Context, profile string) context.Context {
	return context.WithValue(ctx, routerProfileKey, profile)
}

func GetRouterProfile(ctx context.Context) string {
	if v, ok := ctx.Value(routerProfileKey).(string); ok {
		return v
	}
	return "default"
}
