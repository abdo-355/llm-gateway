package verification

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abdo-355/llm-gateway/internal/types"
)

const (
	mainAttemptTimeout     = 5 * time.Minute
	recoveryAttemptTimeout = 2 * time.Minute
	retryDelay             = 10 * time.Second
	maxTransientHits       = 3
	minProviderSpacing     = 1 * time.Second
)

type Runner struct {
	config        Config
	client        probeClient
	ctx           context.Context
	phase         string
	timeout       time.Duration
	maxAttempts   int
	attemptNumber int
	scheduler     *requestScheduler
	attempts      *attemptCollector
}

type attemptCollector struct {
	mu   sync.Mutex
	logs []AttemptLog
}

func (c *attemptCollector) Add(log AttemptLog) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logs = append(c.logs, log)
}

func (c *attemptCollector) Snapshot() []AttemptLog {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]AttemptLog, len(c.logs))
	copy(out, c.logs)
	return out
}

type requestScheduler struct {
	mu        sync.Mutex
	providers map[string]*providerSchedule
}

type providerSchedule struct {
	nextStart time.Time
	interval  time.Duration
	models    map[string]*modelSchedule
}

type modelSchedule struct {
	nextStart time.Time
	interval  time.Duration
}

type modelRunState struct {
	completed         []ProbeResult
	remaining         []Probe
	transientHits     int
	deferred          bool
	recoveryAttempt   bool
	recoverySucceeded bool
}

func Run(ctx context.Context, cfg Config) (*Report, error) {
	return runWithClient(ctx, cfg, newClient(cfg))
}

func runWithClient(ctx context.Context, cfg Config, probeClient probeClient) (*Report, error) {
	cfg = normalizeConfig(cfg)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	combos := EnumerateCombos(cfg)
	if len(combos) == 0 {
		return nil, fmt.Errorf("no provider/model combinations matched the requested filters")
	}

	report := &Report{StartedAt: time.Now().UTC()}

	scheduler := newRequestScheduler(combos)
	attempts := &attemptCollector{}

	mainRunner := &Runner{
		config:      cfg,
		client:      probeClient,
		ctx:         runCtx,
		phase:       "main",
		timeout:     cfg.Timeout,
		maxAttempts: cfg.Retries,
		scheduler:   scheduler,
		attempts:    attempts,
	}

	modelResults := make(chan modelExecutionResult, len(combos))
	var wg sync.WaitGroup

	for _, combo := range combos {
		wg.Add(1)
		go func(combo Combo) {
			defer wg.Done()
			modelResults <- runModel(mainRunner, combo, BuildProbes(cfg))
		}(combo)
	}

	go func() {
		wg.Wait()
		close(modelResults)
	}()

	mainExecutions := make([]modelExecutionResult, 0, len(combos))
	for execution := range modelResults {
		mainExecutions = append(mainExecutions, execution)
		report.Results = append(report.Results, execution.Results...)
		report.ModelOutcomes = append(report.ModelOutcomes, execution.Outcome)
		if cfg.FailFast && hasFailure(execution.Results) {
			cancel()
		}
	}

	recoveryTargets := make([]modelExecutionResult, 0)
	for _, execution := range mainExecutions {
		if execution.Deferred && len(execution.Remaining) > 0 {
			recoveryTargets = append(recoveryTargets, execution)
		}
	}

	if len(recoveryTargets) > 0 {
		recoveryRunner := &Runner{
			config:      cfg,
			client:      probeClient,
			ctx:         runCtx,
			phase:       "recovery",
			timeout:     recoveryAttemptTimeout,
			maxAttempts: 1,
			scheduler:   scheduler,
			attempts:    attempts,
		}

		recoveryResults := make(chan modelExecutionResult, len(recoveryTargets))
		var recoveryWG sync.WaitGroup

		for _, target := range recoveryTargets {
			recoveryWG.Add(1)
			go func(target modelExecutionResult) {
				defer recoveryWG.Done()
				recoveryResults <- recoverModel(recoveryRunner, target)
			}(target)
		}

		go func() {
			recoveryWG.Wait()
			close(recoveryResults)
		}()

		recoveryByKey := make(map[string]modelExecutionResult, len(recoveryTargets))
		for execution := range recoveryResults {
			recoveryByKey[execution.Combo.Key()] = execution
			report.Results = append(report.Results, execution.Results...)
		}

		for i := range report.ModelOutcomes {
			key := report.ModelOutcomes[i].Provider + "/" + report.ModelOutcomes[i].Model
			if recovery, ok := recoveryByKey[key]; ok {
				report.ModelOutcomes[i] = recovery.Outcome
			}
		}
	}

	report.AttemptLogs = attempts.Snapshot()
	report.EndedAt = time.Now().UTC()

	sortResults(report.Results)
	sortAttemptLogs(report.AttemptLogs)
	sortModelOutcomes(report.ModelOutcomes)

	return report, nil
}

type modelExecutionResult struct {
	Combo     Combo
	Results   []ProbeResult
	Remaining []Probe
	Deferred  bool
	Outcome   ModelOutcome
}

func normalizeConfig(cfg Config) Config {
	if cfg.Timeout <= 0 {
		cfg.Timeout = mainAttemptTimeout
	}
	if cfg.ProbeMaxTokens <= 0 {
		cfg.ProbeMaxTokens = DefaultProbeMaxTokens
	}
	if cfg.Retries <= 0 {
		cfg.Retries = 3
	}
	return cfg
}

func runModel(runner *Runner, combo Combo, probes []Probe) modelExecutionResult {
	state := modelRunState{
		completed: make([]ProbeResult, 0, len(probes)),
		remaining: make([]Probe, 0),
	}

	requestedProbes := parseProbeList(runner.config.Probe)
	for _, probe := range probes {
		if len(requestedProbes) > 0 && !containsProbe(requestedProbes, probe.Name) {
			continue
		}

		if state.deferred {
			state.remaining = append(state.remaining, probe)
			state.completed = append(state.completed, deferredSkip(combo, probe))
			continue
		}

		if probe.Applicable != nil && !probe.Applicable(combo) {
			state.completed = append(state.completed, notApplicableSkip(combo, probe, runner.phase))
			continue
		}

		result := runner.runProbeWithPolicy(combo, probe)
		state.completed = append(state.completed, result)
		state.transientHits += result.TransientHits

		if state.transientHits >= maxTransientHits {
			state.deferred = true
		}
	}

	return modelExecutionResult{
		Combo:     combo,
		Results:   state.completed,
		Remaining: state.remaining,
		Deferred:  state.deferred,
		Outcome:   buildOutcome(combo, state.completed, state.remaining, state.deferred, false, false),
	}
}

func recoverModel(runner *Runner, original modelExecutionResult) modelExecutionResult {
	results := make([]ProbeResult, 0, len(original.Remaining)+1)

	recoveryProbe := Probe{
		Name:   "final_recovery_check",
		Fields: []string{"messages", "max_tokens"},
		Run: func(r *Runner, combo Combo) ProbeResult {
			req := types.ChatCompletionRequest{
				Model:     combo.Model,
				Messages:  basicMessages("Reply with OK only."),
				MaxTokens: probeTokenPtr(r.config, 8),
			}
			return r.runJSONProbe(combo, "final_recovery_check", []string{"messages", "max_tokens"}, req, validateNonEmptyChatMessage)
		},
	}

	recoveryResult := runner.runProbeWithPolicy(original.Combo, recoveryProbe)
	results = append(results, recoveryResult)

	recovered := recoveryResult.Status == "PASS"
	remaining := make([]Probe, 0)

	if recovered {
		replayRunner := *runner
		replayRunner.phase = "recovery_replay"

		for _, probe := range original.Remaining {
			if probe.Applicable != nil && !probe.Applicable(original.Combo) {
				results = append(results, notApplicableSkip(original.Combo, probe, "recovery_replay"))
				continue
			}
			replayed := replayRunner.runProbeWithPolicy(original.Combo, probe)
			results = append(results, replayed)
		}
	}

	if !recovered {
		remaining = append(remaining, original.Remaining...)
	}

	merged := make([]ProbeResult, 0, len(original.Results)+len(results))
	merged = append(merged, original.Results...)
	merged = append(merged, results...)

	return modelExecutionResult{
		Combo:     original.Combo,
		Results:   results,
		Remaining: remaining,
		Deferred:  !recovered,
		Outcome:   buildOutcome(original.Combo, merged, remaining, !recovered, true, recovered),
	}
}

func buildOutcome(combo Combo, results []ProbeResult, remaining []Probe, deferred, recoveryAttempted, recoverySucceeded bool) ModelOutcome {
	outcome := ModelOutcome{
		Provider:          combo.Provider.ID,
		Model:             combo.Model,
		Endpoint:          combo.Endpoint,
		Deferred:          deferred,
		Recovered:         recoverySucceeded,
		RecoveryAttempted: recoveryAttempted,
		RecoverySucceeded: recoverySucceeded,
		FinalStatus:       "completed",
	}

	if deferred {
		outcome.FinalStatus = "deferred_timeout"
	}
	if recoverySucceeded {
		outcome.FinalStatus = "recovered"
	}
	if recoveryAttempted && !recoverySucceeded {
		outcome.FinalStatus = "deferred_after_recovery"
	}

	for _, result := range results {
		outcome.TransientFailures += result.TransientHits
		switch result.Status {
		case "PASS":
			outcome.CompletedProbes++
		case "FAIL":
			outcome.FailedProbes++
		case "SKIP":
			outcome.SkippedProbes++
		}
	}

	if len(remaining) > 0 {
		outcome.RemainingProbeNames = make([]string, 0, len(remaining))
		for _, probe := range remaining {
			outcome.RemainingProbeNames = append(outcome.RemainingProbeNames, probe.Name)
		}
	}

	return outcome
}

func deferredSkip(combo Combo, probe Probe) ProbeResult {
	now := time.Now().UTC()
	return ProbeResult{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      probe.Name,
		Fields:     probe.Fields,
		Status:     "SKIP",
		Failure:    "deferred_after_transient_failures",
		Phase:      "main",
		StartedAt:  now,
		EndedAt:    now,
		HTTPStatus: 0,
	}
}

func notApplicableSkip(combo Combo, probe Probe, phase string) ProbeResult {
	now := time.Now().UTC()
	return ProbeResult{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      probe.Name,
		Fields:     probe.Fields,
		Status:     "SKIP",
		Failure:    "not applicable for configured provider capabilities",
		Phase:      phase,
		StartedAt:  now,
		EndedAt:    now,
		HTTPStatus: 0,
	}
}

func (r *Runner) runProbeWithPolicy(combo Combo, probe Probe) ProbeResult {
	var last ProbeResult
	transientHits := 0

	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		attemptRunner := *r
		attemptRunner.attemptNumber = attempt

		result := probe.Run(&attemptRunner, combo)
		result.Phase = r.phase
		result.Attempts = attempt
		result.Retries = attempt - 1

		if result.Status == "PASS" {
			result.TransientHits = transientHits
			return result
		}

		if isTransientProbeFailure(result) {
			transientHits++
			last = result
			if attempt < r.maxAttempts {
				r.logProgress("retrying %s/%s %s in %s after transient failure: %s\n",
					combo.Provider.ID, combo.Model, probe.Name, retryDelay, result.Failure)
				select {
				case <-time.After(retryDelay):
				case <-r.ctx.Done():
					cancelled := result
					cancelled.Status = "FAIL"
					cancelled.Failure = "request cancelled"
					cancelled.TransientHits = transientHits
					return cancelled
				}
				continue
			}
			last.TransientHits = transientHits
			return last
		}

		result.TransientHits = transientHits
		return result
	}

	last.TransientHits = transientHits
	return last
}

func isTransientProbeFailure(result ProbeResult) bool {
	if result.Status == "PASS" {
		return false
	}
	if result.HTTPStatus == 408 || result.HTTPStatus == 429 {
		return true
	}
	return strings.Contains(strings.ToLower(result.Failure), "timeout")
}

func (r *Runner) runJSONProbe(combo Combo, name string, fields []string, req types.ChatCompletionRequest, validate func(*types.ChatCompletionResponse) error) ProbeResult {
	started := time.Now().UTC()

	if err := r.scheduler.WaitTurn(r.ctx, combo); err != nil {
		ended := time.Now().UTC()
		result := ProbeResult{
			Provider:  combo.Provider.ID,
			Model:     combo.Model,
			Endpoint:  combo.Endpoint,
			Probe:     name,
			Fields:    fields,
			Status:    "FAIL",
			Failure:   err.Error(),
			Phase:     r.phase,
			StartedAt: started,
			EndedAt:   ended,
			Latency:   ended.Sub(started),
		}
		r.attempts.Add(AttemptLog{
			Provider:  combo.Provider.ID,
			Model:     combo.Model,
			Endpoint:  combo.Endpoint,
			Probe:     name,
			Phase:     r.phase,
			Attempt:   r.attemptNumber,
			StartedAt: started,
			EndedAt:   ended,
			Latency:   ended.Sub(started),
			Status:    "FAIL",
			Failure:   err.Error(),
		})
		return result
	}

	ctx, cancel := context.WithTimeout(r.ctx, r.timeout)
	defer cancel()

	call := r.client.call(ctx, combo, req)
	ended := time.Now().UTC()

	result := ProbeResult{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      name,
		Fields:     fields,
		Latency:    call.Latency,
		HTTPStatus: call.HTTPStatus,
		TokensUsed: call.TokensUsed,
		Phase:      r.phase,
		StartedAt:  started,
		EndedAt:    ended,
	}

	attemptStatus := "FAIL"
	if call.Failure != "" {
		if call.HTTPStatus == 429 {
			result.Status = "FAIL"
			result.Failure = call.Failure
			attemptStatus = "RATE_LIMITED"
		} else if call.HTTPStatus == 408 || strings.Contains(strings.ToLower(call.Failure), "timeout") {
			result.Status = "FAIL"
			result.Failure = call.Failure
			attemptStatus = "TIMEOUT"
		} else {
			result.Status = "FAIL"
			result.Failure = call.Failure
		}
		r.attempts.Add(AttemptLog{
			Provider:   combo.Provider.ID,
			Model:      combo.Model,
			Endpoint:   combo.Endpoint,
			Probe:      name,
			Phase:      r.phase,
			Attempt:    r.attemptNumber,
			StartedAt:  started,
			EndedAt:    ended,
			Latency:    call.Latency,
			HTTPStatus: call.HTTPStatus,
			TokensUsed: call.TokensUsed,
			Status:     attemptStatus,
			Failure:    call.Failure,
		})
		return result
	}

	if validate != nil {
		if err := validate(call.Response); err != nil {
			result.Status = "FAIL"
			result.Failure = err.Error()
			r.attempts.Add(AttemptLog{
				Provider:   combo.Provider.ID,
				Model:      combo.Model,
				Endpoint:   combo.Endpoint,
				Probe:      name,
				Phase:      r.phase,
				Attempt:    r.attemptNumber,
				StartedAt:  started,
				EndedAt:    ended,
				Latency:    call.Latency,
				HTTPStatus: call.HTTPStatus,
				TokensUsed: call.TokensUsed,
				Status:     "FAIL",
				Failure:    err.Error(),
			})
			return result
		}
	}

	result.Status = "PASS"
	r.attempts.Add(AttemptLog{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      name,
		Phase:      r.phase,
		Attempt:    r.attemptNumber,
		StartedAt:  started,
		EndedAt:    ended,
		Latency:    call.Latency,
		HTTPStatus: call.HTTPStatus,
		TokensUsed: call.TokensUsed,
		Status:     "PASS",
	})
	return result
}

func (r *Runner) runStreamProbe(combo Combo, name string, fields []string, req types.ChatCompletionRequest) ProbeResult {
	started := time.Now().UTC()

	if err := r.scheduler.WaitTurn(r.ctx, combo); err != nil {
		ended := time.Now().UTC()
		result := ProbeResult{
			Provider:  combo.Provider.ID,
			Model:     combo.Model,
			Endpoint:  combo.Endpoint,
			Probe:     name,
			Fields:    fields,
			Status:    "FAIL",
			Failure:   err.Error(),
			Phase:     r.phase,
			StartedAt: started,
			EndedAt:   ended,
			Latency:   ended.Sub(started),
		}
		r.attempts.Add(AttemptLog{
			Provider:  combo.Provider.ID,
			Model:     combo.Model,
			Endpoint:  combo.Endpoint,
			Probe:     name,
			Phase:     r.phase,
			Attempt:   r.attemptNumber,
			StartedAt: started,
			EndedAt:   ended,
			Latency:   ended.Sub(started),
			Status:    "FAIL",
			Failure:   err.Error(),
		})
		return result
	}

	ctx, cancel := context.WithTimeout(r.ctx, r.timeout)
	defer cancel()

	call := r.client.stream(ctx, combo, req)
	ended := time.Now().UTC()

	result := ProbeResult{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      name,
		Fields:     fields,
		Latency:    call.Latency,
		HTTPStatus: call.HTTPStatus,
		TokensUsed: call.TokensUsed,
		Phase:      r.phase,
		StartedAt:  started,
		EndedAt:    ended,
	}

	attemptStatus := "FAIL"
	if call.Failure != "" {
		if call.HTTPStatus == 429 {
			result.Status = "FAIL"
			result.Failure = call.Failure
			attemptStatus = "RATE_LIMITED"
		} else if call.HTTPStatus == 408 || strings.Contains(strings.ToLower(call.Failure), "timeout") {
			result.Status = "FAIL"
			result.Failure = call.Failure
			attemptStatus = "TIMEOUT"
		} else {
			result.Status = "FAIL"
			result.Failure = call.Failure
		}
		r.attempts.Add(AttemptLog{
			Provider:   combo.Provider.ID,
			Model:      combo.Model,
			Endpoint:   combo.Endpoint,
			Probe:      name,
			Phase:      r.phase,
			Attempt:    r.attemptNumber,
			StartedAt:  started,
			EndedAt:    ended,
			Latency:    call.Latency,
			HTTPStatus: call.HTTPStatus,
			TokensUsed: call.TokensUsed,
			Status:     attemptStatus,
			Failure:    call.Failure,
		})
		return result
	}

	if !call.Done {
		result.Status = "FAIL"
		result.Failure = "stream_failed: stream did not complete with [DONE]"
		r.attempts.Add(AttemptLog{
			Provider:   combo.Provider.ID,
			Model:      combo.Model,
			Endpoint:   combo.Endpoint,
			Probe:      name,
			Phase:      r.phase,
			Attempt:    r.attemptNumber,
			StartedAt:  started,
			EndedAt:    ended,
			Latency:    call.Latency,
			HTTPStatus: call.HTTPStatus,
			TokensUsed: call.TokensUsed,
			Status:     "FAIL",
			Failure:    result.Failure,
		})
		return result
	}
	if len(call.Chunks) == 0 {
		result.Status = "FAIL"
		result.Failure = "stream_failed: no chunks received"
		r.attempts.Add(AttemptLog{
			Provider:   combo.Provider.ID,
			Model:      combo.Model,
			Endpoint:   combo.Endpoint,
			Probe:      name,
			Phase:      r.phase,
			Attempt:    r.attemptNumber,
			StartedAt:  started,
			EndedAt:    ended,
			Latency:    call.Latency,
			HTTPStatus: call.HTTPStatus,
			TokensUsed: call.TokensUsed,
			Status:     "FAIL",
			Failure:    result.Failure,
		})
		return result
	}
	if !hasUsageChunk(call.Chunks) {
		result.Status = "FAIL"
		result.Failure = "stream_failed: include_usage requested but no chunk contained usage"
		r.attempts.Add(AttemptLog{
			Provider:   combo.Provider.ID,
			Model:      combo.Model,
			Endpoint:   combo.Endpoint,
			Probe:      name,
			Phase:      r.phase,
			Attempt:    r.attemptNumber,
			StartedAt:  started,
			EndedAt:    ended,
			Latency:    call.Latency,
			HTTPStatus: call.HTTPStatus,
			TokensUsed: call.TokensUsed,
			Status:     "FAIL",
			Failure:    result.Failure,
		})
		return result
	}

	result.Status = "PASS"
	r.attempts.Add(AttemptLog{
		Provider:   combo.Provider.ID,
		Model:      combo.Model,
		Endpoint:   combo.Endpoint,
		Probe:      name,
		Phase:      r.phase,
		Attempt:    r.attemptNumber,
		StartedAt:  started,
		EndedAt:    ended,
		Latency:    call.Latency,
		HTTPStatus: call.HTTPStatus,
		TokensUsed: call.TokensUsed,
		Status:     "PASS",
	})
	return result
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

func newRequestScheduler(combos []Combo) *requestScheduler {
	s := &requestScheduler{
		providers: make(map[string]*providerSchedule),
	}

	for _, combo := range combos {
		schedule, ok := s.providers[combo.Provider.ID]
		if !ok {
			schedule = &providerSchedule{
				interval: effectiveProviderInterval(combo),
				models:   make(map[string]*modelSchedule),
			}
			s.providers[combo.Provider.ID] = schedule
		}
		if _, exists := schedule.models[combo.Model]; !exists {
			schedule.models[combo.Model] = &modelSchedule{
				interval: effectiveModelInterval(combo),
			}
		}
	}

	return s
}

func (s *requestScheduler) WaitTurn(ctx context.Context, combo Combo) error {
	s.mu.Lock()
	schedule, ok := s.providers[combo.Provider.ID]
	if !ok {
		schedule = &providerSchedule{
			interval: effectiveProviderInterval(combo),
			models:   make(map[string]*modelSchedule),
		}
		s.providers[combo.Provider.ID] = schedule
	}
	modelSlot, ok := schedule.models[combo.Model]
	if !ok {
		modelSlot = &modelSchedule{
			interval: effectiveModelInterval(combo),
		}
		schedule.models[combo.Model] = modelSlot
	}

	now := time.Now()
	startAt := now
	if schedule.nextStart.After(startAt) {
		startAt = schedule.nextStart
	}
	if modelSlot.nextStart.After(startAt) {
		startAt = modelSlot.nextStart
	}

	schedule.nextStart = startAt.Add(schedule.interval)
	modelSlot.nextStart = startAt.Add(modelSlot.interval)
	s.mu.Unlock()

	if wait := time.Until(startAt); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return fmt.Errorf("request cancelled")
		}
	}

	return nil
}

func effectiveProviderInterval(combo Combo) time.Duration {
	halfRPM := halfLimit(providerRPMLimit(combo))
	if halfRPM <= 0 {
		return minProviderSpacing
	}

	interval := time.Minute / time.Duration(halfRPM)
	if interval < minProviderSpacing {
		return minProviderSpacing
	}
	return interval
}

func halfLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	half := limit / 2
	if half < 1 {
		return 1
	}
	return half
}

func effectiveModelInterval(combo Combo) time.Duration {
	halfRPM := halfLimit(modelRPMLimit(combo))
	if halfRPM <= 0 {
		return minProviderSpacing
	}

	interval := time.Minute / time.Duration(halfRPM)
	if interval < minProviderSpacing {
		return minProviderSpacing
	}
	return interval
}

func modelRPMLimit(combo Combo) int {
	if combo.Limits.Rpm != nil && *combo.Limits.Rpm > 0 {
		return *combo.Limits.Rpm
	}
	if combo.Limits.Rph != nil && *combo.Limits.Rph > 0 {
		return *combo.Limits.Rph / 60
	}
	return 0
}

func providerRPMLimit(combo Combo) int {
	if combo.Provider.Limits.Rpm != nil && *combo.Provider.Limits.Rpm > 0 {
		return *combo.Provider.Limits.Rpm
	}
	if combo.Provider.Limits.Rph != nil && *combo.Provider.Limits.Rph > 0 {
		return *combo.Provider.Limits.Rph / 60
	}
	return 0
}

func comboRPMLimit(combo Combo) int {
	modelRPM := modelRPMLimit(combo)
	providerRPM := providerRPMLimit(combo)

	if modelRPM > 0 && providerRPM > 0 {
		if modelRPM < providerRPM {
			return modelRPM
		}
		return providerRPM
	}
	if modelRPM > 0 {
		return modelRPM
	}
	if providerRPM > 0 {
		return providerRPM
	}
	return 0
}

func sortResults(results []ProbeResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		if results[i].Model != results[j].Model {
			return results[i].Model < results[j].Model
		}
		if results[i].Phase != results[j].Phase {
			return results[i].Phase < results[j].Phase
		}
		return results[i].Probe < results[j].Probe
	})
}

func sortAttemptLogs(logs []AttemptLog) {
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].Provider != logs[j].Provider {
			return logs[i].Provider < logs[j].Provider
		}
		if logs[i].Model != logs[j].Model {
			return logs[i].Model < logs[j].Model
		}
		if logs[i].StartedAt.Equal(logs[j].StartedAt) {
			if logs[i].Probe != logs[j].Probe {
				return logs[i].Probe < logs[j].Probe
			}
			return logs[i].Attempt < logs[j].Attempt
		}
		return logs[i].StartedAt.Before(logs[j].StartedAt)
	})
}

func sortModelOutcomes(outcomes []ModelOutcome) {
	sort.Slice(outcomes, func(i, j int) bool {
		if outcomes[i].Provider != outcomes[j].Provider {
			return outcomes[i].Provider < outcomes[j].Provider
		}
		return outcomes[i].Model < outcomes[j].Model
	})
}

func hasFailure(results []ProbeResult) bool {
	for _, result := range results {
		if result.Status == "FAIL" {
			return true
		}
	}
	return false
}

func parseProbeList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func containsProbe(list []string, name string) bool {
	for _, entry := range list {
		if entry == name {
			return true
		}
	}
	return false
}
