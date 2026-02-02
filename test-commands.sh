# Quick Test Commands for LLM Gateway

## Prerequisites
- Gateway running (npm run dev or docker-compose up)
- Set your API key: export GATEWAY_API_KEY="your-api-key"

## 1. Health Check
curl http://localhost:8080/health

## 2. Test Authentication (should fail without key)
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "chat-lite", "messages": [{"role": "user", "content": "Hello"}]}'

## 3. Simple Chat Completion
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{
    "model": "chat-lite",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 50
  }'

## 4. Streaming Request
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{
    "model": "chat-lite",
    "messages": [{"role": "user", "content": "Say hello"}],
    "stream": true,
    "max_tokens": 50
  }'

## 5. JSON Schema Response Format
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{
    "model": "json-safe",
    "messages": [{"role": "user", "content": "Extract name and age from: John is 25 years old"}],
    "response_format": {
      "type": "json_schema",
      "json_schema": {
        "name": "person",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "name": {"type": "string"},
            "age": {"type": "number"}
          },
          "required": ["name", "age"]
        }
      }
    }
  }'

## 6. Tool/Function Calling
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{
    "model": "tools-pro",
    "messages": [{"role": "user", "content": "What is the weather in Paris?"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get current weather for a location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {"type": "string"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
          },
          "required": ["location"]
        }
      }
    }],
    "tool_choice": "auto"
  }'

## 7. With All Parameters
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{
    "model": "chat-pro",
    "messages": [{"role": "user", "content": "Hello"}],
    "temperature": 0.7,
    "max_tokens": 100,
    "top_p": 0.9,
    "frequency_penalty": 0.5,
    "presence_penalty": 0.5,
    "n": 1,
    "seed": 123,
    "stop": ["END"],
    "user": "user-123",
    "metadata": {"session": "abc"}
  }'

## 8. With Router Hints (Custom Gateway Feature)
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{
    "model": "chat-pro",
    "messages": [{"role": "user", "content": "Hello"}],
    "router": {
      "profile": "cheap_fast",
      "slo": {"max_latency_ms": 5000},
      "fallback": {"max_attempts": 3}
    }
  }'

## 9. Check Response Headers
curl -i -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -d '{
    "model": "chat-lite",
    "messages": [{"role": "user", "content": "Hi"}],
    "max_tokens": 5
  }' 2>&1 | head -30

## 10. Metrics Endpoint
curl http://localhost:8080/metrics
