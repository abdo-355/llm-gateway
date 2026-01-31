import {
  ProviderAuthSchema,
  ProviderModelsSchema,
  ProviderCapabilitiesSchema,
  ProviderLimitsSchema,
  ProviderCostsSchema,
  ProviderConfigSchema,
  CertificationSchema,
  AppConfigSchema,
  OpenAIMessageSchema,
  OpenAIToolSchema,
  ResponseFormatSchema,
  RouterHintsSchema,
  ChatCompletionRequestSchema,
} from './schema';

describe('Schema Validation', () => {
  describe('ProviderAuthSchema', () => {
    it('should validate valid auth config', () => {
      const result = ProviderAuthSchema.safeParse({
        type: 'bearer',
        env: 'API_KEY',
      });
      expect(result.success).toBe(true);
    });

    it('should validate all auth types', () => {
      const types = ['none', 'bearer', 'header'];
      types.forEach(type => {
        const result = ProviderAuthSchema.safeParse({ type, env: 'KEY' });
        expect(result.success).toBe(true);
      });
    });

    it('should reject invalid auth type', () => {
      const result = ProviderAuthSchema.safeParse({
        type: 'invalid',
        env: 'API_KEY',
      });
      expect(result.success).toBe(false);
    });

    it('should require both fields', () => {
      const result = ProviderAuthSchema.safeParse({ type: 'bearer' });
      expect(result.success).toBe(false);
    });
  });

  describe('ProviderModelsSchema', () => {
    it('should validate allowlist mode', () => {
      const result = ProviderModelsSchema.safeParse({
        mode: 'allowlist',
        list: ['gpt-4', 'gpt-3.5'],
      });
      expect(result.success).toBe(true);
    });

    it('should validate denylist mode', () => {
      const result = ProviderModelsSchema.safeParse({
        mode: 'denylist',
        list: ['deprecated-model'],
      });
      expect(result.success).toBe(true);
    });

    it('should allow empty list', () => {
      const result = ProviderModelsSchema.safeParse({
        mode: 'allowlist',
        list: [],
      });
      expect(result.success).toBe(true);
    });

    it('should reject invalid mode', () => {
      const result = ProviderModelsSchema.safeParse({
        mode: 'invalid',
        list: ['model'],
      });
      expect(result.success).toBe(false);
    });
  });

  describe('ProviderCapabilitiesSchema', () => {
    it('should validate full capabilities', () => {
      const result = ProviderCapabilitiesSchema.safeParse({
        streaming: true,
        tools: true,
        structuredOutputs: 'json_schema_strict',
      });
      expect(result.success).toBe(true);
    });

    it('should validate all structured output types', () => {
      const types = ['none', 'json_object', 'json_schema_strict', 'model_dependent', 'unknown'];
      types.forEach(type => {
        const result = ProviderCapabilitiesSchema.safeParse({
          streaming: false,
          tools: false,
          structuredOutputs: type,
        });
        expect(result.success).toBe(true);
      });
    });

    it('should reject invalid structured output type', () => {
      const result = ProviderCapabilitiesSchema.safeParse({
        streaming: true,
        tools: true,
        structuredOutputs: 'invalid',
      });
      expect(result.success).toBe(false);
    });
  });

  describe('ProviderLimitsSchema', () => {
    it('should validate all optional limits', () => {
      const result = ProviderLimitsSchema.safeParse({
        rpm: 100,
        tpm: 10000,
        dailyRequests: 1000,
        maxConcurrency: 10,
      });
      expect(result.success).toBe(true);
    });

    it('should validate empty limits', () => {
      const result = ProviderLimitsSchema.safeParse({});
      expect(result.success).toBe(true);
    });

    it('should reject negative values', () => {
      const result = ProviderLimitsSchema.safeParse({ rpm: -100 });
      expect(result.success).toBe(false);
    });

    it('should reject zero values', () => {
      const result = ProviderLimitsSchema.safeParse({ tpm: 0 });
      expect(result.success).toBe(false);
    });

    it('should reject decimal values', () => {
      const result = ProviderLimitsSchema.safeParse({ rpm: 100.5 });
      expect(result.success).toBe(false);
    });
  });

  describe('ProviderCostsSchema', () => {
    it('should validate cost config', () => {
      const result = ProviderCostsSchema.safeParse({
        per1kInputTokens: 0.01,
        per1kOutputTokens: 0.02,
      });
      expect(result.success).toBe(true);
    });

    it('should validate empty costs', () => {
      const result = ProviderCostsSchema.safeParse({});
      expect(result.success).toBe(true);
    });

    it('should validate partial costs', () => {
      const result = ProviderCostsSchema.safeParse({
        per1kInputTokens: 0.01,
      });
      expect(result.success).toBe(true);
    });

    it('should reject negative costs', () => {
      const result = ProviderCostsSchema.safeParse({
        per1kInputTokens: -0.01,
      });
      expect(result.success).toBe(false);
    });

    it('should reject zero costs', () => {
      const result = ProviderCostsSchema.safeParse({
        per1kOutputTokens: 0,
      });
      expect(result.success).toBe(false);
    });
  });

  describe('ProviderConfigSchema', () => {
    const validProvider = {
      id: 'openai',
      baseUrl: 'https://api.openai.com/v1',
      auth: { type: 'bearer', env: 'OPENAI_API_KEY' },
      models: { mode: 'allowlist', list: ['gpt-4'] },
      capabilities: { streaming: true, tools: true, structuredOutputs: 'json_object' },
      limits: { rpm: 100 },
      costs: { per1kInputTokens: 0.01 },
    };

    it('should validate full provider config', () => {
      const result = ProviderConfigSchema.safeParse(validProvider);
      expect(result.success).toBe(true);
    });

    it('should validate without optional costs', () => {
      const { costs, ...withoutCosts } = validProvider;
      const result = ProviderConfigSchema.safeParse(withoutCosts);
      expect(result.success).toBe(true);
    });

    it('should reject empty id', () => {
      const result = ProviderConfigSchema.safeParse({
        ...validProvider,
        id: '',
      });
      expect(result.success).toBe(false);
    });

    it('should reject invalid URL', () => {
      const result = ProviderConfigSchema.safeParse({
        ...validProvider,
        baseUrl: 'not-a-url',
      });
      expect(result.success).toBe(false);
    });

    it('should reject missing required fields', () => {
      const result = ProviderConfigSchema.safeParse({
        id: 'test',
        baseUrl: 'https://api.test.com',
      });
      expect(result.success).toBe(false);
    });
  });

  describe('CertificationSchema', () => {
    it('should validate certification', () => {
      const result = CertificationSchema.safeParse({
        provider: 'openai',
        model: 'gpt-4',
        strictSchema: true,
      });
      expect(result.success).toBe(true);
    });

    it('should validate with strictSchema false', () => {
      const result = CertificationSchema.safeParse({
        provider: 'anthropic',
        model: 'claude-3',
        strictSchema: false,
      });
      expect(result.success).toBe(true);
    });

    it('should reject missing fields', () => {
      const result = CertificationSchema.safeParse({
        provider: 'openai',
      });
      expect(result.success).toBe(false);
    });
  });

  describe('AppConfigSchema', () => {
    const validAppConfig = {
      providers: [
        {
          id: 'openai',
          baseUrl: 'https://api.openai.com/v1',
          auth: { type: 'bearer', env: 'OPENAI_API_KEY' },
          models: { mode: 'allowlist', list: ['gpt-4'] },
          capabilities: { streaming: true, tools: true, structuredOutputs: 'json_object' },
          limits: {},
        },
      ],
    };

    it('should validate app config with providers', () => {
      const result = AppConfigSchema.safeParse(validAppConfig);
      expect(result.success).toBe(true);
    });

    it('should validate with certifications', () => {
      const result = AppConfigSchema.safeParse({
        ...validAppConfig,
        certifications: [{ provider: 'openai', model: 'gpt-4', strictSchema: true }],
      });
      expect(result.success).toBe(true);
    });

    it('should use default empty certifications', () => {
      const result = AppConfigSchema.safeParse(validAppConfig);
      expect(result.success).toBe(true);
      if (result.success) {
        expect(result.data.certifications).toEqual([]);
      }
    });

    it('should reject empty providers array', () => {
      const result = AppConfigSchema.safeParse({ providers: [] });
      expect(result.success).toBe(false);
    });

    it('should reject missing providers', () => {
      const result = AppConfigSchema.safeParse({});
      expect(result.success).toBe(false);
    });
  });

  describe('OpenAIMessageSchema', () => {
    it('should validate user message', () => {
      const result = OpenAIMessageSchema.safeParse({
        role: 'user',
        content: 'Hello',
      });
      expect(result.success).toBe(true);
    });

    it('should validate all roles', () => {
      const roles = ['system', 'user', 'assistant', 'tool'];
      roles.forEach(role => {
        const result = OpenAIMessageSchema.safeParse({ role, content: 'test' });
        expect(result.success).toBe(true);
      });
    });

    it('should validate with array content', () => {
      const result = OpenAIMessageSchema.safeParse({
        role: 'user',
        content: [{ type: 'text', text: 'Hello' }],
      });
      expect(result.success).toBe(true);
    });

    it('should validate with tool call fields', () => {
      const result = OpenAIMessageSchema.safeParse({
        role: 'assistant',
        tool_calls: [{ id: 'call_1', function: { name: 'test' } }],
      });
      expect(result.success).toBe(true);
    });

    it('should validate tool message with tool_call_id', () => {
      const result = OpenAIMessageSchema.safeParse({
        role: 'tool',
        content: 'Result',
        tool_call_id: 'call_1',
      });
      expect(result.success).toBe(true);
    });

    it('should reject invalid role', () => {
      const result = OpenAIMessageSchema.safeParse({
        role: 'invalid',
        content: 'test',
      });
      expect(result.success).toBe(false);
    });
  });

  describe('OpenAIToolSchema', () => {
    it('should validate function tool', () => {
      const result = OpenAIToolSchema.safeParse({
        type: 'function',
        function: {
          name: 'get_weather',
          description: 'Get weather for a location',
          parameters: { type: 'object', properties: {} },
        },
      });
      expect(result.success).toBe(true);
    });

    it('should validate with strict option', () => {
      const result = OpenAIToolSchema.safeParse({
        type: 'function',
        function: {
          name: 'calc',
          parameters: {},
          strict: true,
        },
      });
      expect(result.success).toBe(true);
    });

    it('should reject non-function type', () => {
      const result = OpenAIToolSchema.safeParse({
        type: 'retrieval',
        function: { name: 'test', parameters: {} },
      });
      expect(result.success).toBe(false);
    });

    it('should require function field', () => {
      const result = OpenAIToolSchema.safeParse({
        type: 'function',
      });
      expect(result.success).toBe(false);
    });
  });

  describe('ResponseFormatSchema', () => {
    it('should validate text format', () => {
      const result = ResponseFormatSchema.safeParse({ type: 'text' });
      expect(result.success).toBe(true);
    });

    it('should validate json_object format', () => {
      const result = ResponseFormatSchema.safeParse({ type: 'json_object' });
      expect(result.success).toBe(true);
    });

    it('should validate json_schema format', () => {
      const result = ResponseFormatSchema.safeParse({
        type: 'json_schema',
        json_schema: {
          name: 'schema',
          schema: { type: 'object' },
        },
      });
      expect(result.success).toBe(true);
    });

    it('should validate json_schema with description', () => {
      const result = ResponseFormatSchema.safeParse({
        type: 'json_schema',
        json_schema: {
          name: 'schema',
          description: 'A test schema',
          schema: {},
          strict: true,
        },
      });
      expect(result.success).toBe(true);
    });

    it('should reject invalid type', () => {
      const result = ResponseFormatSchema.safeParse({ type: 'xml' });
      expect(result.success).toBe(false);
    });
  });

  describe('RouterHintsSchema', () => {
    it('should validate minimal hints', () => {
      const result = RouterHintsSchema.safeParse({});
      expect(result.success).toBe(true);
    });

    it('should validate full hints', () => {
      const result = RouterHintsSchema.safeParse({
        profile: 'balanced',
        requirements: {
          output: 'json_schema_strict',
          streaming: 'required',
          tools: 'allowed',
        },
        budget: { mode: 'allow_paid' },
        slo: { max_latency_ms: 5000, hard_timeout_ms: 10000 },
        providers: {
          allow: ['openai'],
          deny: ['unreliable'],
          prefer: ['anthropic'],
        },
        fallback: {
          max_attempts: 2,
          on_429: true,
          on_timeout: false,
          on_5xx: true,
        },
        trace: {
          request_id: '550e8400-e29b-41d4-a716-446655440000',
          tags: ['production', 'v1'],
        },
      });
      expect(result.success).toBe(true);
    });

    it('should validate all profiles', () => {
      const profiles = ['cheap_fast', 'reliable_structured', 'balanced'];
      profiles.forEach(profile => {
        const result = RouterHintsSchema.safeParse({ profile });
        expect(result.success).toBe(true);
      });
    });

    it('should validate all requirement options', () => {
      const streaming = ['required', 'preferred', 'forbidden'];
      const tools = ['required', 'allowed', 'forbidden'];

      streaming.forEach(s => {
        tools.forEach(t => {
          const result = RouterHintsSchema.safeParse({
            requirements: { streaming: s, tools: t },
          });
          expect(result.success).toBe(true);
        });
      });
    });

    it('should use fallback defaults', () => {
      const result = RouterHintsSchema.safeParse({
        fallback: {},
      });
      expect(result.success).toBe(true);
      if (result.success) {
        expect(result.data.fallback?.max_attempts).toBe(3);
        expect(result.data.fallback?.on_429).toBe(true);
        expect(result.data.fallback?.on_timeout).toBe(true);
        expect(result.data.fallback?.on_5xx).toBe(true);
      }
    });

    it('should reject invalid max_attempts', () => {
      const result = RouterHintsSchema.safeParse({
        fallback: { max_attempts: 10 },
      });
      expect(result.success).toBe(false);
    });

    it('should reject zero max_attempts', () => {
      const result = RouterHintsSchema.safeParse({
        fallback: { max_attempts: 0 },
      });
      expect(result.success).toBe(false);
    });

    it('should reject non-UUID request_id', () => {
      const result = RouterHintsSchema.safeParse({
        trace: { request_id: 'not-a-uuid' },
      });
      expect(result.success).toBe(false);
    });
  });

  describe('ChatCompletionRequestSchema', () => {
    const validRequest = {
      messages: [{ role: 'user', content: 'Hello' }],
      model: 'gpt-4',
    };

    it('should validate minimal request', () => {
      const result = ChatCompletionRequestSchema.safeParse(validRequest);
      expect(result.success).toBe(true);
    });

    it('should validate full request', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        frequency_penalty: 0.5,
        logit_bias: { '100': 1 },
        logprobs: true,
        top_logprobs: 5,
        max_tokens: 100,
        max_completion_tokens: 100,
        n: 1,
        presence_penalty: 0.5,
        response_format: { type: 'json_object' },
        seed: 123,
        stop: 'END',
        stream: true,
        stream_options: { include_usage: true },
        temperature: 0.7,
        top_p: 0.9,
        tools: [{ type: 'function', function: { name: 'test', parameters: {} } }],
        tool_choice: 'auto',
        parallel_tool_calls: false,
        user: 'user-123',
        metadata: { session: 'abc' },
        router: { profile: 'balanced' },
      });
      expect(result.success).toBe(true);
    });

    it('should use default n=1', () => {
      const result = ChatCompletionRequestSchema.safeParse(validRequest);
      expect(result.success).toBe(true);
      if (result.success) {
        expect(result.data.n).toBe(1);
      }
    });

    it('should use default stream=false', () => {
      const result = ChatCompletionRequestSchema.safeParse(validRequest);
      expect(result.success).toBe(true);
      if (result.success) {
        expect(result.data.stream).toBe(false);
      }
    });

    it('should reject empty messages array', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        messages: [],
      });
      expect(result.success).toBe(false);
    });

    it('should reject empty model', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        model: '',
      });
      expect(result.success).toBe(false);
    });

    it('should reject frequency_penalty > 2', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        frequency_penalty: 3,
      });
      expect(result.success).toBe(false);
    });

    it('should reject frequency_penalty < -2', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        frequency_penalty: -3,
      });
      expect(result.success).toBe(false);
    });

    it('should reject n > 128', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        n: 200,
      });
      expect(result.success).toBe(false);
    });

    it('should reject negative n', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        n: -1,
      });
      expect(result.success).toBe(false);
    });

    it('should reject top_logprobs > 20', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        logprobs: true,
        top_logprobs: 25,
      });
      expect(result.success).toBe(false);
    });

    it('should reject negative top_logprobs', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        logprobs: true,
        top_logprobs: -1,
      });
      expect(result.success).toBe(false);
    });

    it('should validate stop as string', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        stop: 'END',
      });
      expect(result.success).toBe(true);
    });

    it('should validate stop as array', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        stop: ['stop1', 'stop2'],
      });
      expect(result.success).toBe(true);
    });

    it('should reject stop array > 4 items', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        stop: ['a', 'b', 'c', 'd', 'e'],
      });
      expect(result.success).toBe(false);
    });

    it('should reject temperature > 2', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        temperature: 3,
      });
      expect(result.success).toBe(false);
    });

    it('should reject negative temperature', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        temperature: -0.1,
      });
      expect(result.success).toBe(false);
    });

    it('should reject top_p > 1', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        top_p: 1.1,
      });
      expect(result.success).toBe(false);
    });

    it('should reject negative top_p', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        top_p: -0.1,
      });
      expect(result.success).toBe(false);
    });

    it('should validate tool_choice as string', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        tool_choice: 'required',
      });
      expect(result.success).toBe(true);
    });

    it('should validate tool_choice as object', () => {
      const result = ChatCompletionRequestSchema.safeParse({
        ...validRequest,
        tool_choice: {
          type: 'function',
          function: { name: 'get_weather' },
        },
      });
      expect(result.success).toBe(true);
    });
  });
});
