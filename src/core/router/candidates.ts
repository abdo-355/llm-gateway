import { ProviderConfig } from '../../providers/types';
import { Certification } from '../../config/certifications';
import { RouterHints } from './types';
import { DerivedRequirements } from './types';

export interface Candidate {
  provider: ProviderConfig;
  model: string;
  isCertifiedForStrictSchema: boolean;
}

export interface FilterResult {
  candidates: Candidate[];
  filtered: Array<{ provider: string; model?: string; reason: string }>;
}

export function generateCandidates(
  providers: ProviderConfig[],
  certifications: Certification[]
): Candidate[] {
  const candidates: Candidate[] = [];

  for (const provider of providers) {
    const allowedModels = getAllowedModels(provider);
    
    for (const model of allowedModels) {
      const isCertified = isCertifiedForStrictSchema(certifications, provider.id, model);
      candidates.push({
        provider,
        model,
        isCertifiedForStrictSchema: isCertified,
      });
    }
  }

  return candidates;
}

function getAllowedModels(provider: ProviderConfig): string[] {
  if (provider.models.mode === 'allowlist') {
    return provider.models.allow;
  } else if (provider.models.mode === 'denylist') {
    // For denylist mode, we'd need to fetch available models first
    // For now, return empty array as we'd need discovery
    return [];
  } else {
    // Discovery mode would need to fetch from provider
    // For now, return empty array
    return [];
  }
}

function isCertifiedForStrictSchema(
  certifications: Certification[],
  providerId: string,
  model: string
): boolean {
  const cert = certifications.find(
    c => c.provider === providerId && c.model === model
  );
  return cert?.json_schema_strict === true;
}

export function filterCandidates(
  candidates: Candidate[],
  requirements: DerivedRequirements,
  routerHints: RouterHints | undefined,
  quotaState: Map<string, { remaining: number; headroomScore: number }>,
  circuitBreakerState: Map<string, boolean>
): FilterResult {
  const result: Candidate[] = [];
  const filtered: Array<{ provider: string; model?: string; reason: string }> = [];

  for (const candidate of candidates) {
    const providerId = candidate.provider.id;
    const model = candidate.model;

    // Check allow/deny lists
    if (routerHints?.providers?.allow) {
      if (!routerHints.providers.allow.includes(providerId)) {
        filtered.push({ provider: providerId, model, reason: 'not_in_allowlist' });
        continue;
      }
    }

    if (routerHints?.providers?.deny) {
      if (routerHints.providers.deny.includes(providerId)) {
        filtered.push({ provider: providerId, model, reason: 'in_denylist' });
        continue;
      }
    }

    // Check budget mode
    if (routerHints?.budget?.mode === 'free_only') {
      const isFreeTier = candidate.provider.routing?.tags?.includes('free-tier') ||
                         candidate.provider.routing?.tags?.includes('local') ||
                         candidate.provider.routing?.tags?.includes('free');
      if (!isFreeTier) {
        filtered.push({ provider: providerId, model, reason: 'not_free_tier' });
        continue;
      }
    }

    // Check strict schema requirement (hard filter)
    if (requirements.output === 'json_schema_strict') {
      if (!candidate.isCertifiedForStrictSchema) {
        // Check if provider declares capability at all
        const capability = candidate.provider.capabilities.structured_outputs.json_schema_strict;
        if (capability !== 'json_schema_strict' && capability !== 'model_dependent') {
          filtered.push({ provider: providerId, model, reason: 'not_certified_for_json_schema' });
          continue;
        }
        // If model_dependent, we must check certification
        if (capability === 'model_dependent' && !candidate.isCertifiedForStrictSchema) {
          filtered.push({ provider: providerId, model, reason: 'not_certified_for_json_schema' });
          continue;
        }
      }
    }

    // Check streaming requirement
    if (requirements.streaming === 'required' && !candidate.provider.capabilities.streaming) {
      filtered.push({ provider: providerId, model, reason: 'streaming_not_supported' });
      continue;
    }

    // Check tools requirement
    if (requirements.tools === 'required' && !candidate.provider.capabilities.tools) {
      filtered.push({ provider: providerId, model, reason: 'tools_not_supported' });
      continue;
    }

    // Check circuit breaker
    const isCircuitOpen = circuitBreakerState.get(providerId) ?? false;
    if (isCircuitOpen) {
      filtered.push({ provider: providerId, model, reason: 'circuit_breaker_open' });
      continue;
    }

    // Check quota
    const quota = quotaState.get(`${providerId}:${model}`);
    if (quota && quota.remaining <= 0) {
      filtered.push({ provider: providerId, model, reason: 'quota_exceeded' });
      continue;
    }

    result.push(candidate);
  }

  return { candidates: result, filtered };
}
