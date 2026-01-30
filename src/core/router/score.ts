import { Candidate } from './candidates';
import { RouterHints } from './types';

export interface ScoredCandidate extends Candidate {
  score: number;
  scoreBreakdown: Record<string, number>;
}

// Weight configuration (should be configurable via env vars)
const WEIGHTS = {
  base: 1.0,
  prefer: 0.5,
  quota: 0.3,
  latency: 0.4,
  health: 0.5,
  capability: 0.2,
  cost: 0.1,
};

export function scoreCandidates(
  candidates: Candidate[],
  routerHints: RouterHints | undefined,
  quotaState: Map<string, { remaining: number; headroomScore: number }>,
  healthScores: Map<string, number>,
  latencyScores: Map<string, number>
): ScoredCandidate[] {
  const scored: ScoredCandidate[] = candidates.map(candidate => {
    const providerId = candidate.provider.id;
    const model = candidate.model;
    const key = `${providerId}:${model}`;
    
    const breakdown: Record<string, number> = {};

    // Base weight from provider config
    breakdown.base = (candidate.provider.routing?.base_weight ?? 1.0) * WEIGHTS.base;

    // Preference bonus
    const preferList = routerHints?.providers?.prefer || [];
    const preferIndex = preferList.indexOf(providerId);
    if (preferIndex !== -1) {
      // Higher score for earlier in prefer list
      breakdown.prefer = (1 - (preferIndex / preferList.length)) * WEIGHTS.prefer;
    } else {
      breakdown.prefer = 0;
    }

    // Quota headroom score
    const quota = quotaState.get(key);
    if (quota) {
      breakdown.quota = quota.headroomScore * WEIGHTS.quota;
    } else {
      // No quota tracking yet, assume full
      breakdown.quota = 1.0 * WEIGHTS.quota;
    }

    // Health score
    const health = healthScores.get(providerId) ?? 1.0;
    breakdown.health = health * WEIGHTS.health;

    // Latency score (lower is better, so invert)
    const latency = latencyScores.get(key);
    if (latency !== undefined && latency > 0) {
      // Normalize: assume good latency is < 1000ms
      const latencyScore = Math.max(0, 1 - (latency / 5000));
      breakdown.latency = latencyScore * WEIGHTS.latency;
    } else {
      // No latency data yet, assume average
      breakdown.latency = 0.5 * WEIGHTS.latency;
    }

    // Capability bonus for providers with full strict schema support
    if (candidate.provider.capabilities.structured_outputs.json_schema_strict === 'json_schema_strict') {
      breakdown.capability = WEIGHTS.capability;
    } else if (candidate.provider.capabilities.structured_outputs.json_schema_strict === 'model_dependent') {
      breakdown.capability = WEIGHTS.capability * 0.5;
    } else {
      breakdown.capability = 0;
    }

    // Cost penalty (free tier gets bonus)
    const isFree = candidate.provider.routing?.tags?.includes('free-tier') ||
                   candidate.provider.routing?.tags?.includes('local') ||
                   candidate.provider.routing?.tags?.includes('free');
    if (isFree) {
      breakdown.cost = WEIGHTS.cost; // Bonus
    } else {
      breakdown.cost = 0;
    }

    // Calculate total score
    const totalScore = Object.values(breakdown).reduce((sum, val) => sum + val, 0);

    return {
      ...candidate,
      score: totalScore,
      scoreBreakdown: breakdown,
    };
  });

  // Sort by score descending
  scored.sort((a, b) => b.score - a.score);

  return scored;
}
