// JSON value types for type safety
export type JsonPrimitive = string | number | boolean | null;
export type JsonArray = JsonValue[];
export type JsonObject = { [key: string]: JsonValue };
export type JsonValue = JsonPrimitive | JsonArray | JsonObject;

// OpenAI-compatible types for request/response
export interface OpenAIMessage {
  role: 'system' | 'user' | 'assistant' | 'tool';
  content?: string | Array<{ type: string; text?: string; image_url?: { url: string; detail?: string } }>;
  name?: string;
  tool_calls?: Array<{
    id: string;
    type: 'function';
    function: { name: string; arguments: string };
  }>;
  tool_call_id?: string;
}

export interface OpenAITool {
  type: 'function';
  function: {
    name: string;
    description?: string;
    parameters: JsonObject;
    strict?: boolean;
  };
}

export interface ResponseFormat {
  type: 'text' | 'json_object' | 'json_schema';
  json_schema?: {
    name: string;
    description?: string;
    schema: JsonObject;
    strict?: boolean;
  };
}

export interface ChatCompletionRequest {
  messages: OpenAIMessage[];
  model: string;
  frequency_penalty?: number;
  logit_bias?: Record<string, number>;
  logprobs?: boolean;
  top_logprobs?: number;
  max_tokens?: number;
  max_completion_tokens?: number;
  n?: number;
  presence_penalty?: number;
  response_format?: ResponseFormat;
  seed?: number;
  stop?: string | string[];
  stream?: boolean;
  stream_options?: { include_usage?: boolean };
  temperature?: number;
  top_p?: number;
  tools?: OpenAITool[];
  tool_choice?: 'none' | 'auto' | 'required' | { type: 'function'; function: { name: string } };
  parallel_tool_calls?: boolean;
  user?: string;
  metadata?: JsonObject;
}

export interface ChatCompletionResponse {
  id: string;
  object: 'chat.completion';
  created: number;
  model: string;
  choices: Array<{
    index: number;
    message: {
      role: 'assistant';
      content: string | null;
      tool_calls?: Array<{
        id: string;
        type: 'function';
        function: { name: string; arguments: string };
      }>;
      refusal?: string;
    };
    logprobs?: Logprobs;
    finish_reason: 'stop' | 'length' | 'tool_calls' | 'content_filter' | 'function_call' | null;
  }>;
  usage?: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
    prompt_tokens_details?: { cached_tokens?: number };
    completion_tokens_details?: { reasoning_tokens?: number };
  };
  system_fingerprint?: string;
}

// Log probabilities type
export interface Logprobs {
  content?: Array<{
    token: string;
    logprob: number;
    bytes: number[] | null;
    top_logprobs: Array<{
      token: string;
      logprob: number;
      bytes: number[] | null;
    }>;
  }>;
  refusal?: Array<{
    token: string;
    logprob: number;
    bytes: number[] | null;
    top_logprobs: Array<{
      token: string;
      logprob: number;
      bytes: number[] | null;
    }>;
  }>;
}

export interface SSEChunk {
  id: string;
  object: 'chat.completion.chunk';
  created: number;
  model: string;
  system_fingerprint?: string;
  choices: Array<{
    index: number;
    delta: {
      role?: 'assistant';
      content?: string | null;
      tool_calls?: Array<{
        index: number;
        id?: string;
        type?: 'function';
        function?: { name?: string; arguments?: string };
      }>;
      refusal?: string;
    };
    logprobs?: Logprobs;
    finish_reason?: 'stop' | 'length' | 'tool_calls' | 'content_filter' | 'function_call' | null;
  }>;
  usage?: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
}

// Router hints from request
export interface RouterHints {
  profile?: 'cheap_fast' | 'reliable_structured' | 'balanced';
  requirements?: {
    output?: 'text' | 'json_schema_strict';
    streaming?: 'required' | 'preferred' | 'forbidden';
    tools?: 'required' | 'allowed' | 'forbidden';
  };
  budget?: {
    mode: 'free_only' | 'allow_paid';
  };
  slo?: {
    max_latency_ms?: number;
    hard_timeout_ms?: number;
  };
  providers?: {
    allow?: string[];
    deny?: string[];
    prefer?: string[];
  };
  fallback?: {
    max_attempts?: number;
    on_429?: boolean;
    on_timeout?: boolean;
    on_5xx?: boolean;
  };
  trace?: {
    request_id?: string;
    tags?: string[];
  };
}

// Provider types
export type ProviderAuthType = 'none' | 'bearer' | 'header';

export interface ProviderAuth {
  type: ProviderAuthType;
  env: string;
  headerName?: string; // For 'header' auth type (e.g., 'x-goog-api-key')
}

export interface ProviderModels {
  limits?: Record<string, ModelLimits>;
  mode: 'allowlist' | 'denylist';
  list: string[];
}

export interface ProviderCapabilities {
  streaming: boolean;
  tools: boolean;
  structuredOutputs: 'none' | 'json_object' | 'json_schema_strict' | 'model_dependent' | 'unknown';
}

export interface ProviderLimits {
  rpm?: number;
  tpm?: number;
  dailyRequests?: number;
  maxConcurrency?: number;
  // Extended limits for per-model tracking
  rph?: number;        // Requests per hour
  rpd?: number;        // Requests per day
  tph?: number;        // Tokens per hour
  tpd?: number;        // Tokens per day
  tpmu?: number;       // Tokens per month
}

// Per-model limit configuration
export interface ModelLimits {
  rpm?: number;
  rph?: number;        // Requests per hour
  rpd?: number;        // Requests per day
  tpm?: number;
  tph?: number;        // Tokens per hour
  tpd?: number;        // Tokens per day
  tpmu?: number;       // Tokens per month
}

export interface ProviderCosts {
  per1kInputTokens?: number;
  per1kOutputTokens?: number;
}

export type ProviderType = 'openai' | 'vertex';

export interface ProviderConfig {
  id: string;
  baseUrl: string;
  auth: ProviderAuth;
  models: ProviderModels;
  capabilities: ProviderCapabilities;
  limits: ProviderLimits;
  costs?: ProviderCosts;
  providerType?: ProviderType; // Defaults to 'openai' for backward compatibility
}

// Vertex AI specific types
export interface VertexAIContent {
  role: string;
  parts: Array<{ text: string }>;
}

export interface VertexAIRequest {
  contents: VertexAIContent[];
  generationConfig?: {
    temperature?: number;
    maxOutputTokens?: number;
    topP?: number;
    topK?: number;
  };
}

export interface VertexAIResponse {
  candidates?: Array<{
    content?: {
      parts?: Array<{ text?: string }>;
      role?: string;
    };
    finishReason?: string;
    safetyRatings?: unknown[];
  }>;
  usageMetadata?: {
    promptTokenCount?: number;
    candidatesTokenCount?: number;
    totalTokenCount?: number;
  };
}

// Certification types
export interface Certification {
  provider: string;
  model: string;
  strictSchema: boolean;
}

// App config
export interface AppConfig {
  providers: ProviderConfig[];
  certifications: Certification[];
}

// Derived requirements
export interface DerivedRequirements {
  output: 'text' | 'json_schema_strict';
  streaming: 'required' | 'preferred' | 'forbidden';
  tools: 'required' | 'allowed' | 'forbidden';
}

// Gateway error
export interface GatewayError {
  type: 'gateway_error' | 'authentication_error' | 'validation_error' | 'rate_limit_error';
  code: string;
  message: string;
  request_id?: string;
  details?: JsonObject;
}
