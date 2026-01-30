import { z } from 'zod';
import { RouterHintsSchema } from '../router/types';

export const OpenAIMessageContentSchema = z.union([
  z.string(),
  z.array(z.object({
    type: z.enum(['text', 'image_url']),
    text: z.string().optional(),
    image_url: z.object({
      url: z.string(),
      detail: z.enum(['auto', 'low', 'high']).optional(),
    }).optional(),
  })),
]);

export const OpenAIMessageSchema = z.object({
  role: z.enum(['system', 'user', 'assistant', 'tool']),
  content: OpenAIMessageContentSchema.optional(),
  name: z.string().optional(),
  tool_calls: z.array(z.object({
    id: z.string(),
    type: z.literal('function'),
    function: z.object({
      name: z.string(),
      arguments: z.string(),
    }),
  })).optional(),
  tool_call_id: z.string().optional(),
});

export const OpenAIToolFunctionSchema = z.object({
  name: z.string(),
  description: z.string().optional(),
  parameters: z.record(z.any()),
  strict: z.boolean().optional(),
});

export const OpenAIToolSchema = z.object({
  type: z.literal('function'),
  function: OpenAIToolFunctionSchema,
});

export const ResponseFormatSchema = z.union([
  z.object({ type: z.literal('text') }),
  z.object({ type: z.literal('json_object') }),
  z.object({
    type: z.literal('json_schema'),
    json_schema: z.object({
      description: z.string().optional(),
      name: z.string(),
      schema: z.record(z.any()),
      strict: z.boolean().optional(),
    }),
  }),
]);

export const ChatCompletionRequestSchema = z.object({
  messages: z.array(OpenAIMessageSchema).min(1),
  model: z.string(),
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
  stream_options: z.object({
    include_usage: z.boolean().optional(),
  }).optional(),
  temperature: z.number().min(0).max(2).optional(),
  top_p: z.number().min(0).max(1).optional(),
  tools: z.array(OpenAIToolSchema).optional(),
  tool_choice: z.union([
    z.enum(['none', 'auto', 'required']),
    z.object({
      type: z.literal('function'),
      function: z.object({ name: z.string() }),
    }),
  ]).optional(),
  parallel_tool_calls: z.boolean().optional(),
  user: z.string().optional(),
  metadata: z.record(z.any()).optional(),
  router: RouterHintsSchema.optional(),
});

export type ChatCompletionRequest = z.infer<typeof ChatCompletionRequestSchema>;

export const ChatCompletionResponseSchema = z.object({
  id: z.string(),
  object: z.literal('chat.completion'),
  created: z.number(),
  model: z.string(),
  choices: z.array(z.object({
    index: z.number(),
    message: z.object({
      role: z.enum(['assistant']),
      content: z.string().nullable(),
      tool_calls: z.array(z.object({
        id: z.string(),
        type: z.literal('function'),
        function: z.object({
          name: z.string(),
          arguments: z.string(),
        }),
      })).optional(),
      refusal: z.string().optional(),
    }),
    logprobs: z.any().nullable().optional(),
    finish_reason: z.enum(['stop', 'length', 'tool_calls', 'content_filter', 'function_call']).nullable(),
  })),
  usage: z.object({
    prompt_tokens: z.number(),
    completion_tokens: z.number(),
    total_tokens: z.number(),
    prompt_tokens_details: z.object({
      cached_tokens: z.number().optional(),
    }).optional(),
    completion_tokens_details: z.object({
      reasoning_tokens: z.number().optional(),
    }).optional(),
  }).optional(),
  system_fingerprint: z.string().optional(),
  error: z.object({
    type: z.string(),
    code: z.string(),
    message: z.string(),
    request_id: z.string().optional(),
    details: z.record(z.any()).optional(),
  }).optional(),
});

export type ChatCompletionResponse = z.infer<typeof ChatCompletionResponseSchema>;

export const SSEChunkSchema = z.object({
  id: z.string(),
  object: z.literal('chat.completion.chunk'),
  created: z.number(),
  model: z.string(),
  system_fingerprint: z.string().optional(),
  choices: z.array(z.object({
    index: z.number(),
    delta: z.object({
      role: z.enum(['assistant']).optional(),
      content: z.string().nullable().optional(),
      tool_calls: z.array(z.object({
        index: z.number(),
        id: z.string().optional(),
        type: z.literal('function').optional(),
        function: z.object({
          name: z.string().optional(),
          arguments: z.string().optional(),
        }).optional(),
      })).optional(),
      refusal: z.string().optional(),
    }),
    logprobs: z.any().nullable().optional(),
    finish_reason: z.enum(['stop', 'length', 'tool_calls', 'content_filter', 'function_call']).nullable().optional(),
  })),
  usage: z.object({
    prompt_tokens: z.number(),
    completion_tokens: z.number(),
    total_tokens: z.number(),
  }).optional(),
});

export type SSEChunk = z.infer<typeof SSEChunkSchema>;

export interface GatewayError {
  type: 'gateway_error';
  code: string;
  message: string;
  request_id: string;
  details?: Record<string, any>;
}
