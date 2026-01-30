import { z } from 'zod';

export const StructuredOutputLevelSchema = z.enum([
  'none',
  'json_object',
  'json_schema_strict',
  'model_dependent',
  'unknown',
]);
export type StructuredOutputLevel = z.infer<typeof StructuredOutputLevelSchema>;

export const AuthConfigSchema = z.union([
  z.object({
    type: z.literal('none'),
  }),
  z.object({
    type: z.literal('bearer_env'),
    env: z.string(),
  }),
  z.object({
    type: z.literal('header_env'),
    header: z.string(),
    env: z.string(),
  }),
]);

export const ModelsConfigSchema = z.union([
  z.object({
    mode: z.literal('allowlist'),
    allow: z.array(z.string()),
  }),
  z.object({
    mode: z.literal('denylist'),
    deny: z.array(z.string()),
  }),
  z.object({
    mode: z.literal('discovery'),
    cache_ttl_ms: z.number().default(300000),
  }),
]);

export const ProviderConfigSchema = z.object({
  id: z.string(),
  kind: z.enum(['openai_compatible', 'anthropic', 'custom']),
  base_url: z.string().url(),
  auth: AuthConfigSchema,
  models: ModelsConfigSchema,
  capabilities: z.object({
    chat_completions: z.boolean(),
    streaming: z.boolean(),
    tools: z.boolean(),
    structured_outputs: z.object({
      json_schema_strict: StructuredOutputLevelSchema,
      json_object: z.boolean(),
    }),
  }),
  limits: z.object({
    daily_requests: z.number().int().positive().optional(),
    rpm: z.number().int().positive().optional(),
    tpm: z.number().int().positive().optional(),
    max_concurrency: z.number().int().positive().optional(),
  }).optional(),
  routing: z.object({
    base_weight: z.number().min(0).max(10).default(1.0),
    tags: z.array(z.string()).default([]),
  }).optional(),
});

export const ProvidersConfigSchema = z.object({
  providers: z.array(ProviderConfigSchema),
});

export type ProviderConfig = z.infer<typeof ProviderConfigSchema>;
export type ProvidersConfig = z.infer<typeof ProvidersConfigSchema>;
