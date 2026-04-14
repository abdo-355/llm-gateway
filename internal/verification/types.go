package verification

import (
	"io"
	"time"

	"github.com/abdo-355/llm-gateway/internal/types"
)

const DefaultProbeMaxTokens = 1024

type Config struct {
	Provider       string
	Model          string
	Timeout        time.Duration
	FailFast       bool
	Progress       io.Writer
	ProbeMaxTokens int
}

type Combo struct {
	Provider            types.ProviderConfig
	Model               string
	Limits              types.ModelLimits
	Endpoint            string
	StrictJSONCertified bool
}

func (c Combo) Key() string {
	return c.Provider.ID + "/" + c.Model
}

type Probe struct {
	Name       string
	Fields     []string
	Applicable func(combo Combo) bool
	Run        func(r *Runner, combo Combo) ProbeResult
}

type ProbeResult struct {
	Provider   string
	Model      string
	Endpoint   string
	Probe      string
	Fields     []string
	Status     string
	Latency    time.Duration
	HTTPStatus int
	TokensUsed string
	Failure    string
}

type Report struct {
	StartedAt time.Time
	EndedAt   time.Time
	Results   []ProbeResult
}

type providerSummary struct {
	Provider string
	Models   int
	Passed   int
	Failed   int
	Skipped  int
	Total    int
}

type featureSummary struct {
	Probe   string
	Passed  int
	Failed  int
	Skipped int
	Total   int
}

type requestResult struct {
	Latency    time.Duration
	HTTPStatus int
	TokensUsed string
	Failure    string
	Response   *types.ChatCompletionResponse
	Chunks     []types.SSEChunk
	Done       bool
	Attempted  bool
}
