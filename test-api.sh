#!/bin/bash

# Test script for LLM Gateway
# Make sure server is running and GATEWAY_API_KEY is set

echo "=== Test 1: Health Check (No Auth Required) ==="
curl -s http://localhost:8080/health | jq .
echo ""

echo "=== Test 2: Basic Chat Request with chat-lite ==="
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat-lite",
    "messages": [{"role": "user", "content": "Say hello"}]
  }' | jq .
echo ""

echo "=== Test 3: Chat Request with Router Hints (Force Groq) ==="
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat-lite",
    "messages": [{"role": "user", "content": "Hi"}],
    "router": {
      "providers": {"allow": ["groq"], "prefer": ["groq"]},
      "slo": {"max_latency_ms": 5000}
    }
  }' | jq .
echo ""

echo "=== Test 4: Streaming Request ==="
echo "Streaming response:"
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat-lite",
    "messages": [{"role": "user", "content": "Count to 3"}],
    "stream": true
  }'
echo ""
echo ""

echo "=== Test 5: Invalid Model (Should Return Error) ==="
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nonexistent-model",
    "messages": [{"role": "user", "content": "Hello"}]
  }' | jq .
echo ""

echo "=== Test 6: Missing Messages (Validation Error) ==="
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat-lite"
  }' | jq .
echo ""

echo "=== Test 7: Metrics Endpoint ==="
curl -s http://localhost:8080/metrics -H "Authorization: Bearer $GATEWAY_API_KEY"
echo ""

echo "=== Test 8: chat-pro Model ==="
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat-pro",
    "messages": [{"role": "user", "content": "What is 2+2?"}]
  }' | jq .
echo ""

echo "=== Test 9: Code Generation ==="
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "code-fast",
    "messages": [{"role": "user", "content": "Write a Python function to reverse a string"}]
  }' | jq .
echo ""

echo "=== All tests completed! ==="
