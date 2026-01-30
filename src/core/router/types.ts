import { z } from 'zod';

export const RouterProfileSchema = z.enum(['cheap_fast', 'reliable_structured', 'balanced']);
export type RouterProfile = z.infer<typeof RouterProfileSchema>;

export const RouterOutputRequirementSchema = z.enum(['text', 'json_schema_strict']);
export type RouterOutputRequirement = z.infer<typeof RouterOutputRequirementSchema>;

export const RouterHintsSchema = z.object({
  profile: RouterProfileSchema.optional(),
  requirements: z.object({
    output: RouterOutputRequirementSchema.optional(),
    streaming: z.enum(['required', 'preferred', 'forbidden']).optional(),
    tools: z.enum(['required', 'allowed', 'forbidden']).optional(),
  }).optional(),
  budget: z.object({
    mode: z.enum(['free_only', 'allow_paid']),
  }).optional(),
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
    max_attempts: z.number().int().positive().max(5).optional().default(3),
    on_429: z.boolean().optional().default(true),
    on_timeout: z.boolean().optional().default(true),
    on_5xx: z.boolean().optional().default(true),
  }).optional(),
  trace: z.object({
    request_id: z.string().uuid().optional(),
    tags: z.array(z.string()).optional(),
  }).optional(),
});

export type RouterHints = z.infer<typeof RouterHintsSchema>;

export interface DerivedRequirements {
  output: RouterOutputRequirement;
  streaming: 'required' | 'preferred' | 'forbidden';
  tools: 'required' | 'allowed' | 'forbidden';
}
