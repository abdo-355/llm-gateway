package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetStrategyNormalizesMetricLabel(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
		want     string
	}{
		{name: "supported", strategy: "balanced", want: "balanced"},
		{name: "normalizes case and whitespace", strategy: "  CHEAP_FAST ", want: "cheap_fast"},
		{name: "supported structured strategy", strategy: "reliable_structured", want: "reliable_structured"},
		{name: "unknown falls back", strategy: "user-controlled-value", want: "default"},
		{name: "empty falls back", strategy: "", want: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := SetStrategy(context.Background(), tt.strategy)
			assert.Equal(t, tt.want, GetStrategy(ctx))
		})
	}
}
