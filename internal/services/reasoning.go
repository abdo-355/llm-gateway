package services

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/abdo-355/llm-gateway/internal/types"
)

// Canonical reasoning effort levels accepted on the reasoning_effort field.
const (
	ReasoningDisabled = "disabled"
	ReasoningMinimal  = "minimal"
	ReasoningLow      = "low"
	ReasoningMedium   = "medium"
	ReasoningHigh     = "high"
	ReasoningXHigh    = "xhigh"
	ReasoningMax      = "max"
)

// Default thinking budgets per canonical effort level, mirroring litellm's
// ladder. Overridable via REASONING_BUDGET_<LEVEL> env vars.
const (
	defaultBudgetMinimal = 128
	defaultBudgetLow     = 1024
	defaultBudgetMedium  = 2048
	defaultBudgetHigh    = 4096
	defaultBudgetXHigh   = 8192
	defaultBudgetMax     = 16384

	// maxTokensInflation reserves room for the visible answer above a
	// synthesized thinking budget when the caller omitted max_tokens.
	maxTokensInflation = 4096

	// defaultReasoningLevels is the capability set assumed when a provider or
	// model declares Reasoning without an explicit ReasoningLevels list. The
	// exotic tiers (xhigh/max) stay opt-in via explicit lists.
	defaultReasoningLevels = ReasoningMinimal + "," + ReasoningLow + "," + ReasoningMedium + "," + ReasoningHigh
)

// ResolvedReasoning is the single internal representation every provider
// mapper consumes, regardless of which request dialect the client used.
type ResolvedReasoning struct {
	Present      bool   // any reasoning param appeared in the request
	Disabled     bool   // caller explicitly turned reasoning off
	Level        string // canonical effort level (empty when budget-driven only)
	BudgetTokens int    // >0 when an explicit token budget was supplied
}

var budgetLadderOnce sync.Once
var budgetLadder map[string]int

func getBudgetLadder() map[string]int {
	budgetLadderOnce.Do(func() {
		ladder := map[string]int{
			ReasoningMinimal: defaultBudgetMinimal,
			ReasoningLow:     defaultBudgetLow,
			ReasoningMedium:  defaultBudgetMedium,
			ReasoningHigh:    defaultBudgetHigh,
			ReasoningXHigh:   defaultBudgetXHigh,
			ReasoningMax:     defaultBudgetMax,
		}
		for level, current := range ladder {
			if raw := strings.TrimSpace(os.Getenv("REASONING_BUDGET_" + strings.ToUpper(level))); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
					current = n
				}
			}
			ladder[level] = current
		}
		budgetLadder = ladder
	})
	return budgetLadder
}

// CanonicalizeReasoningEffort normalizes client-supplied effort vocabulary to a
// canonical level. It returns "" for unrecognized input so callers can decide
// between dropping and erroring. Aliases none/disable/off collapse to disabled;
// true is treated as medium (the litellm convention for boolean thinking).
func CanonicalizeReasoningEffort(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "":
		return ""
	case "none", "disable", "disabled", "off":
		return ReasoningDisabled
	case "minimal":
		return ReasoningMinimal
	case "true", "medium", "default":
		return ReasoningMedium
	case "false":
		return ReasoningDisabled
	case "low":
		return ReasoningLow
	case "high":
		return ReasoningHigh
	case "xhigh":
		return ReasoningXHigh
	case "max":
		return ReasoningMax
	default:
		return ""
	}
}

// EffortToBudget maps a canonical level onto its ladder budget. Unknown levels
// fall back to medium; disabled maps to zero.
func EffortToBudget(level string) int {
	if level == ReasoningDisabled || level == "" {
		return 0
	}
	if budget, ok := getBudgetLadder()[level]; ok {
		return budget
	}
	return getBudgetLadder()[ReasoningMedium]
}

// BudgetToEffort buckets a token budget back onto the canonical level scale.
// Thresholds mirror the ladder so effort→budget→effort round-trips exactly for
// low/medium/high and degrades predictably for arbitrary budgets.
func BudgetToEffort(budget int) string {
	ladder := getBudgetLadder()
	switch {
	case budget <= 0:
		return ReasoningDisabled
	case budget < ladder[ReasoningLow]:
		return ReasoningMinimal
	case budget < ladder[ReasoningMedium]:
		return ReasoningLow
	case budget < ladder[ReasoningHigh]:
		return ReasoningMedium
	case budget < ladder[ReasoningXHigh]:
		return ReasoningHigh
	case budget < ladder[ReasoningMax]:
		return ReasoningXHigh
	default:
		return ReasoningMax
	}
}

// NormalizeReasoningParams collapses the request's reasoning dialects into one
// resolved form. Precedence: explicit thinking.budget_tokens wins over
// reasoning_effort; thinking.type=disabled always disables.
func NormalizeReasoningParams(req types.ChatCompletionRequest) ResolvedReasoning {
	resolved := ResolvedReasoning{}

	if req.Thinking != nil && req.Thinking.Type == "enabled" && req.Thinking.BudgetTokens != nil && *req.Thinking.BudgetTokens > 0 {
		resolved.Present = true
		resolved.BudgetTokens = *req.Thinking.BudgetTokens
		resolved.Level = BudgetToEffort(resolved.BudgetTokens)
		return resolved
	}

	if req.Thinking != nil {
		resolved.Present = true
		if req.Thinking.Type == "disabled" {
			resolved.Disabled = true
			resolved.Level = ReasoningDisabled
			return resolved
		}
	}

	if req.ReasoningEffort != nil {
		level := CanonicalizeReasoningEffort(*req.ReasoningEffort)
		if level != "" {
			resolved.Present = true
			resolved.Level = level
			resolved.Disabled = level == ReasoningDisabled
		}
	}

	return resolved
}

// SupportsReasoningLevel reports whether a capability set accepts a given
// canonical level. Common tiers pass unless explicitly excluded; exotic tiers
// (xhigh, max) require explicit inclusion — litellm's two gating postures.
func SupportsReasoningLevel(caps types.ProviderCapabilities, level string) bool {
	if !caps.Reasoning {
		return false
	}
	if level == "" || level == ReasoningDisabled {
		return true
	}

	var allowed []string
	if len(caps.ReasoningLevels) > 0 {
		allowed = caps.ReasoningLevels
	} else {
		allowed = strings.Split(defaultReasoningLevels, ",")
	}

	exotic := level == ReasoningXHigh || level == ReasoningMax
	for _, candidate := range allowed {
		canonical := CanonicalizeReasoningEffort(candidate)
		if canonical == level {
			return true
		}
	}
	if exotic {
		return false
	}

	// Unlisted common levels degrade rather than hard-fail unless the model
	// explicitly narrowed its list.
	return len(caps.ReasoningLevels) == 0
}

// InflatedMaxTokens returns max_tokens raised to cover a synthesized thinking
// budget plus answer room when the caller did not set one. Returns 0 when no
// inflation is needed (caller set max_tokens themselves).
func InflatedMaxTokens(req types.ChatCompletionRequest, budget int) int {
	if budget <= 0 {
		return 0
	}
	if req.MaxTokens != nil || req.MaxCompletionTokens != nil {
		return 0
	}
	return budget + maxTokensInflation
}
