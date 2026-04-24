package verification

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/abdo-355/llm-gateway/internal/types"
)

type Runner struct {
	config Config
	client probeClient
	ctx    context.Context
}

func Run(ctx context.Context, cfg Config) (*Report, error) {
	return runWithClient(ctx, cfg, newClient(cfg))
}

func runWithClient(ctx context.Context, cfg Config, probeClient probeClient) (*Report, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.ProbeMaxTokens <= 0 {
		cfg.ProbeMaxTokens = DefaultProbeMaxTokens
	}
	if cfg.Retries <= 0 {
		cfg.Retries = 3
	}

	combos := EnumerateCombos(cfg)
	if len(combos) == 0 {
		return nil, fmt.Errorf("no provider/model combinations matched the requested filters")
	}

	report := &Report{StartedAt: time.Now().UTC()}

	byProvider := groupCombosByProvider(combos)
	probes := BuildProbes(cfg)

	var wg sync.WaitGroup
	results := make(chan ProbeResult, 0)

	for providerID, providerCombos := range byProvider {
		wg.Add(1)
		go runProviderGoroutine(providerID, providerCombos, probes, &Runner{
			config: cfg,
			client: probeClient,
			ctx:    ctx,
		}, results, &wg)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		report.Results = append(report.Results, result)
	}

	report.EndedAt = time.Now().UTC()
	sortResults(report.Results)

	return report, nil
}

func groupCombosByProvider(combos []Combo) map[string][]Combo {
	byProvider := make(map[string][]Combo)
	for _, combo := range combos {
		byProvider[combo.Provider.ID] = append(byProvider[combo.Provider.ID], combo)
	}
	return byProvider
}

func runProviderGoroutine(providerID string, combos []Combo, probes []Probe, runner *Runner, results chan<- ProbeResult, wg *sync.WaitGroup) {
	defer wg.Done()

	limiter := newProviderLimiter()
	rateLimited := make(map[string]string)

	for _, combo := range combos {
		for _, probe := range probes {
			if msg, ok := rateLimited[combo.Model]; ok {
				result := rateLimitedSkip(combo, probe, msg)
				results <- result
				continue
			}

			result := runProbeWithRetry(runner, limiter, combo, probe)
			results <- result

			if result.HTTPStatus == 429 {
				rateLimited[combo.Model] = result.Failure
			}

			if runner.config.FailFast && result.Status == "FAIL" {
				return
			}
		}
	}
}

func rateLimitedSkip(combo Combo, probe Probe, failureMsg string) ProbeResult {
	if failureMsg == "" {
		failureMsg = "rate_limited: earlier probe returned 429"
	}
	return ProbeResult{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      probe.Name,
		Fields:     probe.Fields,
		Status:     "SKIP",
		HTTPStatus: 429,
		Failure:    failureMsg,
	}
}

func runProbeWithRetry(runner *Runner, limiter *providerLimiter, combo Combo, probe Probe) ProbeResult {
	var lastResult ProbeResult
	retries := runner.config.Retries

	for attempt := 0; attempt <= retries; attempt++ {
		result := runSingleProbe(runner, combo, probe)
		result.Retries = attempt

		if result.Status == "PASS" {
			return result
		}

		if result.HTTPStatus == 429 || result.HTTPStatus == 0 {
			lastResult = result
			break
		}

		lastResult = result

		if attempt < retries && isRetryable(result) {
			backoff := calculateBackoff(attempt)
			runner.logProgress("retry %d/%d for %s %s %s after %s (failed: %s)\n",
				attempt+1, retries, combo.Provider.ID, combo.Model, probe.Name,
				backoff.Round(time.Second), result.Failure)
			time.Sleep(backoff)
			continue
		}
		break
	}

	return lastResult
}

func runSingleProbe(runner *Runner, combo Combo, probe Probe) ProbeResult {
	if probe.Applicable != nil && !probe.Applicable(combo) {
		return ProbeResult{
			Provider:   combo.Provider.ID,
			Model:      combo.Model,
			Endpoint:   combo.Endpoint,
			Probe:      probe.Name,
			Fields:     probe.Fields,
			Status:     "SKIP",
			Latency:    0,
			HTTPStatus: 0,
			Failure:    "not applicable for configured provider capabilities",
		}
	}

	result := probe.Run(runner, combo)
	return result
}

func isRetryable(result ProbeResult) bool {
	if result.HTTPStatus == 0 {
		return true
	}
	switch result.HTTPStatus {
	case 408, 429, 500, 502, 503, 504:
		return true
	}
	return false
}

func calculateBackoff(attempt int) time.Duration {
	base := 1 * time.Second
	maxBackoff := 30 * time.Second

	backoff := time.Duration(math.Min(
		float64(base)*math.Pow(2, float64(attempt)),
		float64(maxBackoff),
	))

	jitter := time.Duration(float64(backoff) * 0.1 * (float64(attempt)*0.5 + 0.5))

	return backoff + jitter
}

func (r *Runner) runJSONProbe(combo Combo, name string, fields []string, req types.ChatCompletionRequest, validate func(*types.ChatCompletionResponse) error) ProbeResult {
	if cancelled := r.waitForSlot(combo, name); cancelled != nil {
		cancelled.Fields = fields
		return *cancelled
	}

	ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
	defer cancel()

	call := r.client.call(ctx, combo, req)
	result := ProbeResult{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      name,
		Fields:     fields,
		Latency:    call.Latency,
		HTTPStatus: call.HTTPStatus,
		TokensUsed: call.TokensUsed,
	}
	if call.Failure != "" {
		if call.HTTPStatus == 429 {
			result.Status = "SKIP"
		} else {
			result.Status = "FAIL"
		}
		result.Failure = call.Failure
		return result
	}
	if validate != nil {
		if err := validate(call.Response); err != nil {
			result.Status = "FAIL"
			result.Failure = err.Error()
			return result
		}
	}
	result.Status = "PASS"
	return result
}

func (r *Runner) runStreamProbe(combo Combo, name string, fields []string, req types.ChatCompletionRequest) ProbeResult {
	if cancelled := r.waitForSlot(combo, name); cancelled != nil {
		cancelled.Fields = fields
		return *cancelled
	}

	ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
	defer cancel()

	call := r.client.stream(ctx, combo, req)
	result := ProbeResult{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      name,
		Fields:     fields,
		Latency:    call.Latency,
		HTTPStatus: call.HTTPStatus,
		TokensUsed: call.TokensUsed,
	}
	if call.Failure != "" {
		if call.HTTPStatus == 429 {
			result.Status = "SKIP"
		} else {
			result.Status = "FAIL"
		}
		result.Failure = call.Failure
		return result
	}
	if !call.Done {
		result.Status = "FAIL"
		result.Failure = "stream_failed: stream did not complete with [DONE]"
		return result
	}
	if len(call.Chunks) == 0 {
		result.Status = "FAIL"
		result.Failure = "stream_failed: no chunks received"
		return result
	}
	if !hasUsageChunk(call.Chunks) {
		result.Status = "FAIL"
		result.Failure = "stream_failed: include_usage requested but no chunk contained usage"
		return result
	}
	result.Status = "PASS"
	return result
}

func (r *Runner) waitForSlot(combo Combo, probe string) *ProbeResult {
	limiter := getProviderLimiter(combo.Provider.ID)
	if limiter != nil {
		if wait := limiter.WaitDuration(combo); wait > 0 {
			r.logProgress("waiting %s to respect RPM for %s\n", wait.Round(time.Second), combo.Provider.ID)
			select {
			case <-time.After(wait):
			case <-r.ctx.Done():
				return &ProbeResult{
					Provider: combo.Provider.ID,
					Model:    combo.Model,
					Endpoint: combo.Endpoint,
					Probe:    probe,
					Status:   "FAIL",
					Failure:  "request cancelled",
				}
			}
		}
	}
	return nil
}

func (r *Runner) logProgress(format string, args ...interface{}) {
	if r.config.Progress == nil {
		return
	}
	_, _ = fmt.Fprintf(r.config.Progress, format, args...)
}

func hasUsageChunk(chunks []types.SSEChunk) bool {
	for _, chunk := range chunks {
		if chunk.Usage != nil {
			return true
		}
	}
	return false
}

func sortResults(results []ProbeResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		if results[i].Model != results[j].Model {
			return results[i].Model < results[j].Model
		}
		return results[i].Probe < results[j].Probe
	})
}

type providerLimiter struct {
	mu       sync.Mutex
	rpmLimit int
	history  map[string][]time.Time
}

var (
	globalLimiters     = make(map[string]*providerLimiter)
	globalLimitersLock sync.Mutex
)

func newProviderLimiter() *providerLimiter {
	return &providerLimiter{
		history: make(map[string][]time.Time),
	}
}

func getProviderLimiter(providerID string) *providerLimiter {
	globalLimitersLock.Lock()
	defer globalLimitersLock.Unlock()

	if limiter, exists := globalLimiters[providerID]; exists {
		return limiter
	}

	limiter := &providerLimiter{
		history: make(map[string][]time.Time),
	}
	globalLimiters[providerID] = limiter
	return limiter
}

func (l *providerLimiter) WaitDuration(combo Combo) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.rpmLimit == 0 {
		l.rpmLimit = comboRPMLimit(combo)
		if l.rpmLimit == 0 && combo.Provider.Limits.Rpm != nil {
			l.rpmLimit = *combo.Provider.Limits.Rpm
		}
	}

	if l.rpmLimit <= 0 {
		return 0
	}

	now := time.Now()
	history := l.trimModel(combo.Model, now)
	if len(history) < l.rpmLimit {
		return 0
	}

	oldestAllowed := history[0].Add(time.Minute)
	if wait := time.Until(oldestAllowed); wait > 0 {
		return wait
	}
	return 0
}

func (l *providerLimiter) Record(combo Combo) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.history[combo.Model] = append(l.trimModel(combo.Model, now), now)
}

func (l *providerLimiter) trimModel(model string, now time.Time) []time.Time {
	history := l.history[model]
	trimmed := history[:0]
	for _, ts := range history {
		if now.Sub(ts) < time.Minute {
			trimmed = append(trimmed, ts)
		}
	}
	l.history[model] = trimmed
	return trimmed
}

func comboRPMLimit(combo Combo) int {
	if combo.Limits.Rpm != nil && *combo.Limits.Rpm > 0 {
		return *combo.Limits.Rpm
	}
	if combo.Provider.Limits.Rpm != nil && *combo.Provider.Limits.Rpm > 0 {
		return *combo.Provider.Limits.Rpm
	}
	return 0
}
