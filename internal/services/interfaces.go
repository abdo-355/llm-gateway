package services

import (
	"context"
	"net/http"

	"github.com/abdo-355/llm-gateway/internal/types"
)

type QuotaChecker interface {
	EstimateTokens(req types.ChatCompletionRequest) int
	CheckModelQuota(ctx context.Context, providerID, model string, limits types.ModelLimits, estimatedTokens int) error
	RecordModelUsage(ctx context.Context, providerID, model string, tokensUsed int) error
	HandleProviderRateLimit(ctx context.Context, providerID, model string, resp *http.Response) RateLimitInfo
	AcquireConcurrencySlot(ctx context.Context, providerID, model string, maxConcurrent int) error
	ReleaseConcurrencySlot(ctx context.Context, providerID, model string)
	CheckConcurrencyLimit(ctx context.Context, providerID, model string, maxConcurrent int) bool
	GetModelQuotaStatus(ctx context.Context, providerID, model string, limits *types.ModelLimits) QuotaStatus
}

type ConcurrencyUsageReader interface {
	GetConcurrencyUsage(ctx context.Context, providerID, model string) (int, error)
}

type ProviderQuotaSyncer interface {
	SyncProviderQuotaLimit(ctx context.Context, providerID, model, limitType string, limit int) error
}

type QuotaReservationService interface {
	CheckAndReserveQuota(ctx context.Context, providerID, model string, limits types.ModelLimits, estimatedTokens int) (*QuotaReservation, error)
	ReleaseQuotaReservation(ctx context.Context, reservation *QuotaReservation) error
	// ReleaseTokenReservation drops only the token estimates from a reservation,
	// keeping the request-count entries (RPM member + RPH/RPD counters) so failed
	// attempts still count toward upstream request windows.
	ReleaseTokenReservation(ctx context.Context, reservation *QuotaReservation) error
	RecordTokenUsage(ctx context.Context, reservation *QuotaReservation, actualTokens int) error
}

type HealthChecker interface {
	CanExecute(ctx context.Context, providerID, model string) bool
	GetCircuitState(ctx context.Context, providerID, model string) CircuitState
	CheckCircuitBreaker(ctx context.Context, providerID, model string) error
	RecordSuccess(ctx context.Context, providerID, model string, latencyMs int)
	RecordFailure(ctx context.Context, providerID, model string)
	GetHealthMetrics(ctx context.Context, providerID, model string) HealthMetrics
	GetAllHealthMetrics(ctx context.Context) []HealthMetrics
}

type ProviderCaller interface {
	CallProvider(baseURL, apiKey, model string, request types.ChatCompletionRequest, timeoutMs int, ctx context.Context, providerType string, auth types.ProviderAuth, requestID string) (*types.ChatCompletionResponse, error)
	StreamProviderChannel(baseURL, apiKey, model string, request types.ChatCompletionRequest, timeoutMs int, ctx context.Context, providerType string, auth types.ProviderAuth, requestID string) types.StreamResult
}

type RouterHandler interface {
	DeriveRequirements(req types.ChatCompletionRequest, hints *types.RouterHints) types.DerivedRequirements
	GenerateCandidates() []types.RoutingCandidate
	GenerateCandidatesForTier(tier types.Tier) []types.RoutingCandidate
	FilterCandidates(ctx context.Context, candidates []types.RoutingCandidate, requirements types.DerivedRequirements, req types.ChatCompletionRequest, hints *types.RouterHints) ([]types.RoutingCandidate, map[string]string)
	ScoreCandidates(ctx context.Context, candidates []types.RoutingCandidate, requirements types.DerivedRequirements, hints *types.RouterHints) []types.RoutingCandidate
	CompilePlan(candidates []types.RoutingCandidate, hints *types.RouterHints, tierSLO *types.TierSLO) types.RoutingPlan
	Execute(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) (*types.ExecutionResult, error)
	ExecuteStream(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) types.StreamResult
}
