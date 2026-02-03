// Jest setup file - runs before all tests
// Sets required environment variables for testing

process.env.GATEWAY_API_KEY = 'test-gateway-api-key';
process.env.GROQ_API_KEY = 'test-groq-api-key';
process.env.CEREBRAS_API_KEY = 'test-cerebras-api-key';
process.env.MISTRAL_API_KEY = 'test-mistral-api-key';
process.env.GOOGLE_VERTEX_API_KEY = 'test-vertex-api-key';

// Optional env vars with defaults
process.env.PORT = '8080';
process.env.NODE_ENV = 'test';
process.env.REDIS_URL = 'redis://localhost:6379';
process.env.REDIS_KEY_PREFIX = 'test_gateway';
process.env.LOG_LEVEL = 'error'; // Quiet logging during tests
process.env.RATE_LIMIT_PER_IP = '100';
process.env.RATE_LIMIT_WINDOW_MS = '60000';
process.env.CORS_ORIGINS = '';
