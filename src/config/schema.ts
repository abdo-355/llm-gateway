import { z } from 'zod';

export const ProviderAuthSchema = z.object({
  type: z.enum(['none', 'bearer', 'header']),
  env: z.string(),
  headerName: z.string().optional(),
});

export const ProviderModelsSchema = z.object({
  mode: z.enum(['allowlist', 'denylist']),
  list: z.array(z.string()),
});

export const ProviderCapabilitiesSchema = z.object({
  streaming: z.boolean(),
  tools: z.boolean(),
  structuredOutputs: z.enum(['none', 'json_object', 'json_schema_strict', 'model_dependent', 'unknown']),
});

export const ProviderLimitsSchema = z.object({
  rpm: z.number().int().positive().optional(),
  tpm: z.number().int().positive().optional(),
  dailyRequests: z.number().int().positive().optional(),
  maxConcurrency: z.number().int().positive().optional(),
});

export const ProviderConfigSchema = z.object({
  id: z.string().min(1),
  baseUrl: z.string().url(),
  auth: ProviderAuthSchema,
  models: ProviderModelsSchema,
  capabilities: ProviderCapabilitiesSchema,
  limits: ProviderLimitsSchema,
  providerType: z.enum(['openai', 'vertex']).optional().default('openai'),
});

export const CertificationSchema = z.object({
  provider: z.string(),
  model: z.string(),
  strictSchema: z.boolean(),
});

export const AppConfigSchema = z.object({
  providers: z.array(ProviderConfigSchema).min(1, 'At least one provider is required'),
  certifications: z.array(CertificationSchema).default([]),
});

// OpenAI request schemas
export const OpenAIMessageSchema = z.object({
  role: z.enum(['system', 'user', 'assistant', 'tool']),
  content: z.union([z.string(), z.array(z.any())]).optional(),
  name: z.string().optional(),
  tool_calls: z.array(z.any()).optional(),
  tool_call_id: z.string().optional(),
});

export const OpenAIToolSchema = z.object({
  type: z.literal('function'),
  function: z.object({
    name: z.string(),
    description: z.string().optional(),
    parameters: z.record(z.any()),
    strict: z.boolean().optional(),
  }),
});

export const ResponseFormatSchema = z.object({
  type: z.enum(['text', 'json_object', 'json_schema']),
  json_schema: z.object({
    name: z.string(),
    description: z.string().optional(),
    schema: z.record(z.any()),
    strict: z.boolean().optional(),
  }).optional(),
});

export const RouterHintsSchema = z.object({
  profile: z.enum(['cheap_fast', 'reliable_structured', 'balanced']).optional(),
  requirements: z.object({
    output: z.enum(['text', 'json_schema_strict']).optional(),
    streaming: z.enum(['required', 'preferred', 'forbidden']).optional(),
    tools: z.enum(['required', 'allowed', 'forbidden']).optional(),
  }).optional(),
  budget: z.object({ mode: z.enum(['free_only', 'allow_paid']) }).optional(),
  slo: z.object({
    max_latency_ms: z.number().positive().optional(),
    hard_timeout_ms: z.number().positive().optional(),
  }).optional(),
  providers: z.object({
    allow: z.array(z.string()).optional(),
    deny: z.array(z.string()).optional(),
    prefer: z.array(z.string()).optional(),
  }).optional(),
  fallback: z.object({
    max_attempts: z.number().int().min(1).max(5).optional().default(3),
    on_429: z.boolean().optional().default(true),
    on_timeout: z.boolean().optional().default(true),
    on_5xx: z.boolean().optional().default(true),
  }).optional(),
  trace: z.object({
    request_id: z.string().uuid().optional(),
    tags: z.array(z.string()).optional(),
  }).optional(),
});

export const ChatCompletionRequestSchema = z.object({
  messages: z.array(OpenAIMessageSchema).min(1, 'At least one message is required'),
  model: z.string().min(1, 'Model is required'),
  frequency_penalty: z.number().min(-2).max(2).optional(),
  logit_bias: z.record(z.number()).optional(),
  logprobs: z.boolean().optional(),
  top_logprobs: z.number().int().min(0).max(20).optional(),
  max_tokens: z.number().int().positive().optional(),
  max_completion_tokens: z.number().int().positive().optional(),
  n: z.number().int().positive().max(128).optional().default(1),
  presence_penalty: z.number().min(-2).max(2).optional(),
  response_format: ResponseFormatSchema.optional(),
  seed: z.number().int().optional(),
  stop: z.union([z.string(), z.array(z.string()).max(4)]).optional(),
  stream: z.boolean().optional().default(false),
  stream_options: z.object({ include_usage: z.boolean().optional() }).optional(),
  temperature: z.number().min(0).max(2).optional(),
  top_p: z.number().min(0).max(1).optional(),
  tools: z.array(OpenAIToolSchema).optional(),
  tool_choice: z.union([
    z.enum(['none', 'auto', 'required']),
    z.object({ type: z.literal('function'), function: z.object({ name: z.string() }) }),
  ]).optional(),
  parallel_tool_calls: z.boolean().optional(),
  user: z.string().optional(),
  metadata: z.record(z.any()).optional(),
  router: RouterHintsSchema.optional(),
});
