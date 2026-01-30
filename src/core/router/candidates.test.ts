import { filterCandidates, generateCandidates, Candidate } from './candidates';
import { ProviderConfig, StructuredOutputLevel } from '../../providers/types';
import { Certification } from '../../config/certifications';
import { DerivedRequirements } from '../types';

describe('generateCandidates', () => {
  const mockProvider: ProviderConfig = {
    id: 'groq',
    kind: 'openai_compatible',
    base_url: 'https://api.groq.com',
    auth: { type: 'bearer_env', env: 'GROQ_API_KEY' },
    models: { mode: 'allowlist', allow: ['llama-3.1-70b', 'llama-3.1-8b'] },
    capabilities: {
      chat_completions: true,
      streaming: true,
      tools: true,
      structured_outputs: {
        json_schema_strict: 'model_dependent' as StructuredOutputLevel,
        json_object: true,
      },
    },
  };

  const mockCertifications: Certification[] = [
    { provider: 'groq', model: 'llama-3.1-70b', json_schema_strict: true, tested_at: '2026-01-29' },
  ];

  it('should generate candidates for all models', () => {
    const candidates = generateCandidates([mockProvider], mockCertifications);
    
    expect(candidates).toHaveLength(2);
    expect(candidates[0].provider.id).toBe('groq');
    expect(candidates[0].model).toBe('llama-3.1-70b');
    expect(candidates[1].model).toBe('llama-3.1-8b');
  });

  it('should mark certified models correctly', () => {
    const candidates = generateCandidates([mockProvider], mockCertifications);
    
    const certified = candidates.find(c => c.model === 'llama-3.1-70b');
    const notCertified = candidates.find(c => c.model === 'llama-3.1-8b');
    
    expect(certified?.isCertifiedForStrictSchema).toBe(true);
    expect(notCertified?.isCertifiedForStrictSchema).toBe(false);
  });
});

describe('filterCandidates', () => {
  const createMockCandidate = (
    id: string,
    model: string,
    capabilities: Partial<ProviderConfig['capabilities']> = {},
    tags: string[] = []
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
          json_schema_strict: 'model_dependent' as StructuredOutputLevel,
          json_object: true,
        },
        ...capabilities,
      },
      routing: { base_weight: 1.0, tags },
    },
    model,
    isCertifiedForStrictSchema: false,
  });

  it('should filter by allow list', () => {
    const candidates: Candidate[] = [
      createMockCandidate('groq', 'model-a'),
      createMockCandidate('openai', 'model-b'),
    ];

    const requirements: DerivedRequirements = {
      output: 'text',
      streaming: 'preferred',
      tools: 'forbidden',
    };

    const { candidates: filtered } = filterCandidates(
      candidates,
      requirements,
      { providers: { allow: ['groq'] } },
      new Map(),
      new Map()
    );

    expect(filtered).toHaveLength(1);
    expect(filtered[0].provider.id).toBe('groq');
  });

  it('should filter by budget mode', () => {
    const candidates: Candidate[] = [
      createMockCandidate('groq', 'model-a', {}, ['free-tier']),
      createMockCandidate('openai', 'model-b', {}, ['paid']),
    ];

    const requirements: DerivedRequirements = {
      output: 'text',
      streaming: 'preferred',
      tools: 'forbidden',
    };

    const { candidates: filtered, filtered: removed } = filterCandidates(
      candidates,
      requirements,
      { budget: { mode: 'free_only' } },
      new Map(),
      new Map()
    );

    expect(filtered).toHaveLength(1);
    expect(filtered[0].provider.id).toBe('groq');
    expect(removed.some(r => r.provider === 'openai' && r.reason === 'not_free_tier')).toBe(true);
  });
});
