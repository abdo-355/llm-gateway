import { ScoredCandidate } from './score';
import { RouterHints } from './types';

export interface RoutingPlan {
  attempts: RoutingAttempt[];
  maxAttempts: number;
  hardTimeoutMs: number | undefined;
  retryOn429: boolean;
  retryOnTimeout: boolean;
  retryOn5xx: boolean;
}

export interface RoutingAttempt {
  providerId: string;
  model: string;
  baseUrl: string;
  apiKey: string | undefined;
  score: number;
  timeoutMs: number;
}

export function compilePlan(
  candidates: ScoredCandidate[],
  routerHints: RouterHints | undefined,
  getApiKey: (providerId: string) => string | undefined
): RoutingPlan {
  const maxAttempts = routerHints?.fallback?.max_attempts ?? 3;
  const hardTimeoutMs = routerHints?.slo?.hard_timeout_ms;
  
  const retryOn429 = routerHints?.fallback?.on_429 ?? true;
  const retryOnTimeout = routerHints?.fallback?.on_timeout ?? true;
  const retryOn5xx = routerHints?.fallback?.on_5xx ?? true;

  const attempts: RoutingAttempt[] = candidates.slice(0, maxAttempts).map(candidate => {
    const perAttemptTimeout = routerHints?.slo?.max_latency_ms ?? 30000;
    
    return {
      providerId: candidate.provider.id,
      model: candidate.model,
      baseUrl: candidate.provider.base_url,
      apiKey: getApiKey(candidate.provider.id),
      score: candidate.score,
      timeoutMs: perAttemptTimeout,
    };
  });

  return {
    attempts,
    maxAttempts,
    hardTimeoutMs,
    retryOn429,
    retryOnTimeout,
    retryOn5xx,
  };
}
