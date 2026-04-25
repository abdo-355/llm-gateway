package verification

import (
	"io"
	"time"

	"github.com/abdo-355/llm-gateway/internal/types"
)

const (
	DefaultProbeMaxTokens = 1024
	DefaultRetries        = 3
	DefaultRequestTimeout = 5 * time.Minute
)

type Config struct {
	Provider       string
	Model          string
	Timeout        time.Duration
	FailFast       bool
	Progress       io.Writer
	ProbeMaxTokens int
	Retries        int
	RequestTimeout time.Duration
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
	Provider      string
	Model         string
	Endpoint      string
	Probe         string
	Fields        []string
	Status        string
	Retries       int
	Latency       time.Duration
	HTTPStatus    int
	TokensUsed    string
	Failure       string
	Phase         string
	StartedAt     time.Time
	EndedAt       time.Time
	Attempts      int
	TransientHits int
}

type AttemptLog struct {
	Provider   string
	Model      string
	Endpoint   string
	Probe      string
	Phase      string
	Attempt    int
	StartedAt  time.Time
	EndedAt    time.Time
	Latency    time.Duration
	HTTPStatus int
	TokensUsed string
	Status     string
	Failure    string
}

type ModelOutcome struct {
	Provider            string
	Model               string
	Endpoint            string
	FinalStatus         string
	Deferred            bool
	Recovered           bool
	RecoveryAttempted   bool
	RecoverySucceeded   bool
	TransientFailures   int
	CompletedProbes     int
	FailedProbes        int
	SkippedProbes       int
	RemainingProbeNames []string
}

type Report struct {
	StartedAt     time.Time
	EndedAt       time.Time
	Results       []ProbeResult
	AttemptLogs   []AttemptLog
	ModelOutcomes []ModelOutcome
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
