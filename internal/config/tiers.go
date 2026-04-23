package config

import "github.com/abdo-355/llm-gateway/internal/types"

var tierRegistry = map[types.Tier]types.TierConfig{
	types.TierLite: {
		Tier: types.TierLite,
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(15000),
			MaxAttempts:  intPtr(2),
		},
	},
	types.TierDefault: {
		Tier: types.TierDefault,
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(30000),
			MaxAttempts:  intPtr(3),
		},
	},
	types.TierStrong: {
		Tier: types.TierStrong,
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(45000),
			MaxAttempts:  intPtr(3),
		},
	},
	types.TierFrontier: {
		Tier: types.TierFrontier,
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(60000),
			MaxAttempts:  intPtr(3),
		},
	},
	types.TierDeepThink: {
		Tier: types.TierDeepThink,
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(120000),
			MaxAttempts:  intPtr(4),
		},
	},
}

func GetTierConfig(tier types.Tier) *types.TierConfig {
	config, ok := tierRegistry[tier]
	if !ok {
		return nil
	}
	return &config
}

func intPtr(i int) *int {
	return &i
}
