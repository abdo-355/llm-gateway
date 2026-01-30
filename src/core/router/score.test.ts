import { scoreCandidates } from '../core/router/score';
import { Candidate } from '../core/router/candidates';
import { RouterHints } from '../core/router/types';
import { ProviderConfig } from '../providers/types';

describe('scoreCandidates', () => {
  const createMockCandidate = (
    id: string,
    model: string,
    baseWeight: number,
    tags: string[],
    strictSchemaSupport: 'json_schema_strict' | 'model_dependent' | 'unknown' | 'none' = 'model_dependent'
  ): Candidate => ({
    provider: {
      id,
      kind: 'openai_compatible',
      base_url: `https://${id}.com`,
      auth: { type: 'none' },
      models: { mode: 'allowlist', allow: [model] },
      capabilities: {
        chat_completions: true,
        streaming: true,
        tools: true,
        structured_outputs: {
          json_schema_strict: strictSchemaSupport,
          json_object: true,
        },
      },
      routing: {
        base_weight: baseWeight,
        tags,
      },
    },
    model,
    isCertifiedForStrictSchema: strictSchemaSupport === 'json_schema_strict' || strictSchemaSupport === 'model_dependent',
  });

  it('should score based on base weight', () => {
    const candidates: Candidate[] = [
      createMockCandidate('provider-a', 'model-a', 1.0, []),
      createMockCandidate('provider-b', 'model-b', 2.0, []),
    ];

    const scored = scoreCandidates(candidates, undefined, new Map(), new Map(), new Map());

    expect(scored[0].provider.id).toBe('provider-b');
    expect(scored[1].provider.id).toBe('provider-a');
    expect(scored[0].score).toBeGreaterThan(scored[1].score);
  });

  it('should give preference bonus based on prefer list', () => {
    const candidates: Candidate[] = [
      createMockCandidate('provider-a', 'model-a', 1.0, []),
      createMockCandidate('provider-b', 'model-b', 1.0, []),
    ];

    const hints: RouterHints = {
      providers: {
        prefer: ['provider-a', 'provider-b'],
      },
    };

    const scored = scoreCandidates(candidates, hints, new Map(), new Map(), new Map());

    // provider-a should score higher because it's first in prefer list
    expect(scored[0].provider.id).toBe('provider-a');
    expect(scored[0].scoreBreakdown.prefer).toBeGreaterThan(scored[1].scoreBreakdown.prefer);
  });

  it('should give capability bonus for full strict schema support', () => {
    const candidates: Candidate[] = [
      createMockCandidate('provider-a', 'model-a', 1.0, [], 'unknown'),
      createMockCandidate('provider-b', 'model-b', 1.0, [], 'model_dependent'),
      createMockCandidate('provider-c', 'model-c', 1.0, [], 'json_schema_strict'),
    ];

    const scored = scoreCandidates(candidates, undefined, new Map(), new Map(), new Map());

    // Should be ordered by capability bonus
    expect(scored[0].provider.id).toBe('provider-c'); // Full support
    expect(scored[0].scoreBreakdown.capability).toBeGreaterThan(scored[1].scoreBreakdown.capability);
    expect(scored[1].provider.id).toBe('provider-b'); // Model dependent
    expect(scored[2].provider.id).toBe('provider-a'); // Unknown (no bonus)
  });

  it('should give cost bonus for free tier providers', () => {
    const candidates: Candidate[] = [
      createMockCandidate('paid', 'model-a', 1.0, ['paid']),
      createMockCandidate('free-tier', 'model-b', 1.0, ['free-tier']),
      createMockCandidate('local', 'model-c', 1.0, ['local', 'free']),
    ];

    const scored = scoreCandidates(candidates, undefined, new Map(), new Map(), new Map());

    // Free providers should have cost bonus
    const freeTier = scored.find(s => s.provider.id === 'free-tier');
    const local = scored.find(s => s.provider.id === 'local');
    const paid = scored.find(s => s.provider.id === 'paid');

    expect(freeTier?.scoreBreakdown.cost).toBeGreaterThan(0);
    expect(local?.scoreBreakdown.cost).toBeGreaterThan(0);
    expect(paid?.scoreBreakdown.cost).toBe(0);
  });

  it('should factor in quota headroom', () => {
    const candidates: Candidate[] = [
      createMockCandidate('provider-a', 'model-a', 1.0, []),
    ];

    const quotaState = new Map([
      ['provider-a:model-a', { remaining: 10, headroomScore: 0.9 }],
    ]);

    const scored = scoreCandidates(candidates, undefined, quotaState, new Map(), new Map());

    expect(scored[0].scoreBreakdown.quota).toBeGreaterThan(0);
  });

  it('should factor in health scores', () => {
    const candidates: Candidate[] = [
      createMockCandidate('provider-a', 'model-a', 1.0, []),
      createMockCandidate('provider-b', 'model-b', 1.0, []),
    ];

    const healthScores = new Map([
      ['provider-a', 1.0], // Healthy
      ['provider-b', 0.5], // Degraded
    ]);

    const scored = scoreCandidates(candidates, undefined, new Map(), healthScores, new Map());

    expect(scored[0].provider.id).toBe('provider-a');
    expect(scored[0].scoreBreakdown.health).toBeGreaterThan(scored[1].scoreBreakdown.health);
  });

  it('should factor in latency scores (lower is better)', () => {
    const candidates: Candidate[] = [
      createMockCandidate('provider-a', 'model-a', 1.0, []),
      createMockCandidate('provider-b', 'model-b', 1.0, []),
    ];

    const latencyScores = new Map([
      ['provider-a:model-a', 500],  // Fast
      ['provider-b:model-b', 3000], // Slow
    ]);

    const scored = scoreCandidates(candidates, undefined, new Map(), new Map(), latencyScores);

    expect(scored[0].provider.id).toBe('provider-a');
    expect(scored[0].scoreBreakdown.latency).toBeGreaterThan(scored[1].scoreBreakdown.latency);
  });

  it('should combine multiple factors', () => {
    const candidates: Candidate[] = [
      createMockCandidate('preferred-paid', 'model-a', 1.0, ['paid']), // Preferred but paid
      createMockCandidate('free', 'model-b', 0.8, ['free']), // Not preferred but free
    ];

    const hints: RouterHints = {
      providers: {
        prefer: ['preferred-paid'],
      },
    };

    const scored = scoreCandidates(candidates, hints, new Map(), new Map(), new Map());

    // The preferred provider should still win due to high preference bonus
    expect(scored[0].provider.id).toBe('preferred-paid');
  });
});
