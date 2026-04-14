package verification

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/abdo-355/llm-gateway/internal/types"
)

type Runner struct {
	config  Config
	client  *client
	limiter *rpmLimiter
	ctx     context.Context
}

func Run(ctx context.Context, cfg Config) (*Report, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.ProbeMaxTokens <= 0 {
		cfg.ProbeMaxTokens = DefaultProbeMaxTokens
	}

	combos := EnumerateCombos(cfg)
	if len(combos) == 0 {
		return nil, fmt.Errorf("no provider/model combinations matched the requested filters")
	}

	runner := &Runner{
		config:  cfg,
		client:  newClient(cfg),
		limiter: newRPMLimiter(),
		ctx:     ctx,
	}

	probes := BuildProbes(cfg)
	report := &Report{StartedAt: time.Now().UTC()}

	for _, combo := range combos {
		for _, probe := range probes {
			result := runner.runProbe(combo, probe)
			report.Results = append(report.Results, result)
			if runner.config.FailFast && result.Status == "FAIL" {
				report.EndedAt = time.Now().UTC()
				return report, fmt.Errorf("probe failed for %s %s %s: %s", combo.Provider.ID, combo.Model, probe.Name, result.Failure)
			}
		}
	}

	report.EndedAt = time.Now().UTC()
	sort.Slice(report.Results, func(i, j int) bool {
		if report.Results[i].Provider != report.Results[j].Provider {
			return report.Results[i].Provider < report.Results[j].Provider
		}
		if report.Results[i].Model != report.Results[j].Model {
			return report.Results[i].Model < report.Results[j].Model
		}
		return report.Results[i].Probe < report.Results[j].Probe
	})

	return report, nil
}

func (r *Runner) runProbe(combo Combo, probe Probe) ProbeResult {
	if probe.Applicable != nil && !probe.Applicable(combo) {
		return ProbeResult{
			Provider: combo.Provider.ID,
			Model:    combo.Model,
			Endpoint: combo.Endpoint,
			Probe:    probe.Name,
			Fields:   probe.Fields,
			Status:   "SKIP",
			Failure:  "not applicable for configured provider capabilities",
		}
	}
	return probe.Run(r, combo)
}

func (r *Runner) runJSONProbe(combo Combo, name string, fields []string, req types.ChatCompletionRequest, validate func(*types.ChatCompletionResponse) error) ProbeResult {
	if cancelled := r.waitForSlot(combo, name); cancelled != nil {
		cancelled.Fields = fields
		return *cancelled
	}

	ctx, cancel := context.WithTimeout(r.ctx, r.config.Timeout)
	defer cancel()

	call := r.client.call(ctx, combo, req)
	if call.Attempted {
		r.limiter.Record(combo)
	}
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
		result.Status = "FAIL"
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
	if call.Attempted {
		r.limiter.Record(combo)
	}
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
		result.Status = "FAIL"
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
	if wait := r.limiter.WaitDuration(combo); wait > 0 {
		r.logProgress("waiting %s to respect RPM for %s\n", wait.Round(time.Second), combo.Key())
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
	return nil
}

func (r *Runner) logProgress(format string, args ...any) {
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

type rpmLimiter struct {
	history map[string][]time.Time
}

func newRPMLimiter() *rpmLimiter {
	return &rpmLimiter{history: make(map[string][]time.Time)}
}

func (l *rpmLimiter) WaitDuration(combo Combo) time.Duration {
	limit := comboRPMLimit(combo)
	if limit <= 0 {
		return 0
	}

	now := time.Now()
	history := l.trim(combo.Key(), now)
	if len(history) < limit {
		return 0
	}

	oldestAllowed := history[0].Add(time.Minute)
	if wait := time.Until(oldestAllowed); wait > 0 {
		return wait
	}
	return 0
}

func (l *rpmLimiter) Record(combo Combo) {
	key := combo.Key()
	now := time.Now()
	l.history[key] = append(l.trim(key, now), now)
}

func (l *rpmLimiter) trim(key string, now time.Time) []time.Time {
	history := l.history[key]
	trimmed := history[:0]
	for _, ts := range history {
		if now.Sub(ts) < time.Minute {
			trimmed = append(trimmed, ts)
		}
	}
	l.history[key] = trimmed
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
