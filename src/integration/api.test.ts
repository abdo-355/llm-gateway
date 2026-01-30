import request from 'supertest';
import express, { Application } from 'express';
import { createChatCompletionsRouter } from '../http/routes/chatCompletions';
import { loadConfig } from '../config/loader';
import { QuotaManager } from '../core/quota/manager';
import { HealthMonitor } from '../core/health/monitor';
import { CircuitBreakerRegistry } from '../core/health/breaker';
import { OpenAICompatibleClient } from '../providers/openaiCompatible/client';
import { Logger } from '../logging';
import { v4 as uuidv4 } from 'uuid';

// Mock the dependencies
jest.mock('../config/loader');
jest.mock('../providers/openaiCompatible/client');
jest.mock('../logging');

const mockedLoadConfig = loadConfig as jest.MockedFunction<typeof loadConfig>;
const mockedProviderClient = OpenAICompatibleClient as jest.MockedClass<typeof OpenAICompatibleClient>;
const mockedLogger = Logger as jest.MockedClass<typeof Logger>;

describe('Chat Completions API', () => {
  let app: Application;
  let mockConfig: any;
  let mockClient: any;
  let quotaManager: QuotaManager;
  let healthMonitor: HealthMonitor;
  let circuitBreakers: CircuitBreakerRegistry;

  beforeEach(() => {
    mockConfig = {
      providers: [
        {
          id: 'test-provider',
          kind: 'openai_compatible',
          base_url: 'https://test.com',
          auth: { type: 'none' },
          models: { mode: 'allowlist', allow: ['test-model'] },
          capabilities: {
            chat_completions: true,
            streaming: true,
            tools: true,
            structured_outputs: {
              json_schema_strict: 'json_schema_strict',
              json_object: true,
            },
          },
          limits: {
            daily_requests: 1000,
            rpm: 60,
          },
        },
      ],
      certifications: [
        { provider: 'test-provider', model: 'test-model', json_schema_strict: true, tested_at: '2026-01-29' },
      ],
    };

    mockedLoadConfig.mockReturnValue(mockConfig);

    mockClient = {
      callChatCompletions: jest.fn(),
      streamChatCompletions: jest.fn(),
    };
    mockedProviderClient.mockImplementation(() => mockClient);

    mockedLogger.mockImplementation(() => ({
      child: jest.fn().mockReturnValue({
        info: jest.fn(),
        error: jest.fn(),
        warn: jest.fn(),
        debug: jest.fn(),
      }),
    }) as any);

    quotaManager = new QuotaManager();
    healthMonitor = new HealthMonitor();
    circuitBreakers = new CircuitBreakerRegistry();

    app = express();
    app.use(express.json());
    app.use((req, res, next) => {
      (req as any).requestId = uuidv4();
      next();
    });
    app.use('/v1', createChatCompletionsRouter(
      mockConfig,
      mockClient,
      quotaManager,
      healthMonitor,
      circuitBreakers,
      new Logger()
    ));
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('should return 400 for invalid request body', async () => {
    const response = await request(app)
      .post('/v1/chat/completions')
      .send({ invalid: 'body' });

    expect(response.status).toBe(400);
    expect(response.body.error.code).toBe('validation_failed');
  });

  it('should return 422 when no eligible providers', async () => {
    // Exhaust quota
    for (let i = 0; i < 1000; i++) {
      quotaManager.recordRequest('test-provider');
    }

    const response = await request(app)
      .post('/v1/chat/completions')
      .send({
        model: 'test',
        messages: [{ role: 'user', content: 'Hello' }],
      });

    expect(response.status).toBe(422);
    expect(response.body.error.code).toBe('NO_ELIGIBLE_PROVIDER');
  });

  it('should successfully call provider and return response', async () => {
    const mockResponse = {
      id: 'test-id',
      object: 'chat.completion',
      created: Date.now(),
      model: 'test-model',
      choices: [{
        index: 0,
        message: { role: 'assistant', content: 'Hello!' },
        finish_reason: 'stop',
      }],
      usage: {
        prompt_tokens: 10,
        completion_tokens: 5,
        total_tokens: 15,
      },
    };

    mockClient.callChatCompletions.mockResolvedValue(mockResponse);

    const response = await request(app)
      .post('/v1/chat/completions')
      .send({
        model: 'test',
        messages: [{ role: 'user', content: 'Hello' }],
      });

    expect(response.status).toBe(200);
    expect(response.body.choices[0].message.content).toBe('Hello!');
    expect(mockClient.callChatCompletions).toHaveBeenCalledTimes(1);
  });

  it('should handle strict schema requirements', async () => {
    const mockResponse = {
      id: 'test-id',
      object: 'chat.completion',
      created: Date.now(),
      model: 'test-model',
      choices: [{
        index: 0,
        message: { role: 'assistant', content: '{"name":"John"}' },
        finish_reason: 'stop',
      }],
    };

    mockClient.callChatCompletions.mockResolvedValue(mockResponse);

    const response = await request(app)
      .post('/v1/chat/completions')
      .send({
        model: 'test',
        messages: [{ role: 'user', content: 'Extract name' }],
        response_format: {
          type: 'json_schema',
          json_schema: {
            name: 'person',
            strict: true,
            schema: {
              type: 'object',
              properties: { name: { type: 'string' } },
              required: ['name'],
            },
          },
        },
      });

    expect(response.status).toBe(200);
  });
});
