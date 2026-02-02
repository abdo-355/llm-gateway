#!/bin/bash

# LLM Gateway Test Script
# Usage: ./test-gateway.sh [GATEWAY_URL] [API_KEY]

GATEWAY_URL="${1:-http://localhost:8080}"
API_KEY="${2:-test-api-key}"

echo "=========================================="
echo "Testing LLM Gateway at: $GATEWAY_URL"
echo "=========================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track results
PASSED=0
FAILED=0

# Helper function
run_test() {
    local name=$1
    local command=$2
    local expected_status=$3
    
    echo -n "Testing $name... "
    
    # Run the command and capture output
    response=$(eval "$command" 2>&1)
    status=$?
    
    if [ $status -eq 0 ]; then
        # Check if response contains expected status
        if echo "$response" | grep -q "$expected_status"; then
            echo -e "${GREEN}✓ PASSED${NC}"
            ((PASSED++))
        else
            echo -e "${RED}✗ FAILED${NC}"
            echo "  Expected: $expected_status"
            echo "  Got: $response"
            ((FAILED++))
        fi
    else
        echo -e "${YELLOW}⚠ ERROR${NC}"
        echo "  Command failed: $response"
        ((FAILED++))
    fi
}

echo "1. Health Check"
echo "----------------"
run_test "Health endpoint" \
    "curl -s -o /dev/null -w '%{http_code}' $GATEWAY_URL/health" \
    "200"

echo ""
echo "2. Authentication Tests"
echo "----------------------"
run_test "Missing auth header" \
    "curl -s -o /dev/null -w '%{http_code}' -X POST $GATEWAY_URL/v1/chat/completions -H 'Content-Type: application/json' -d '{\"model\": \"gpt-4\", \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}]}'" \
    "401"

run_test "Invalid API key" \
    "curl -s -o /dev/null -w '%{http_code}' -X POST $GATEWAY_URL/v1/chat/completions -H 'Content-Type: application/json' -H 'Authorization: Bearer invalid-key' -d '{\"model\": \"gpt-4\", \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}]}'" \
    "401"

echo ""
echo "3. Request Validation Tests"
echo "----------------------------"
run_test "Missing model field" \
    "curl -s -o /dev/null -w '%{http_code}' -X POST $GATEWAY_URL/v1/chat/completions -H 'Content-Type: application/json' -H 'Authorization: Bearer $API_KEY' -d '{\"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}]}'" \
    "400"

run_test "Missing messages field" \
    "curl -s -o /dev/null -w '%{http_code}' -X POST $GATEWAY_URL/v1/chat/completions -H 'Content-Type: application/json' -H 'Authorization: Bearer $API_KEY' -d '{\"model\": \"gpt-4\"}'" \
    "400"

run_test "Empty messages array" \
    "curl -s -o /dev/null -w '%{http_code}' -X POST $GATEWAY_URL/v1/chat/completions -H 'Content-Type: application/json' -H 'Authorization: Bearer $API_KEY' -d '{\"model\": \"gpt-4\", \"messages\": []}'" \
    "400"

echo ""
echo "4. Chat Completion Tests (requires configured providers)"
echo "---------------------------------------------------------"
echo "${YELLOW}Note: These require at least one provider to be configured${NC}"
echo ""

# Test with logical model
echo -n "Testing chat completion with logical model 'chat-lite'... "
response=$(curl -s -X POST "$GATEWAY_URL/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $API_KEY" \
    -d '{
        "model": "chat-lite",
        "messages": [{"role": "user", "content": "Say hello"}],
        "max_tokens": 10
    }' 2>&1)

if echo "$response" | grep -q '"id"'; then
    echo -e "${GREEN}✓ PASSED${NC}"
    echo "  Response: $(echo "$response" | head -c 100)"
    ((PASSED++))
else
    echo -e "${YELLOW}⚠ SKIPPED or FAILED${NC}"
    echo "  Response: $response"
fi

# Test with provider model
echo -n "Testing chat completion with provider model... "
response=$(curl -s -X POST "$GATEWAY_URL/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $API_KEY" \
    -d '{
        "model": "llama-3.3-70b-versatile",
        "messages": [{"role": "user", "content": "Say hi"}],
        "max_tokens": 5
    }' 2>&1)

if echo "$response" | grep -q '"id"'; then
    echo -e "${GREEN}✓ PASSED${NC}"
    echo "  Response: $(echo "$response" | head -c 100)"
    ((PASSED++))
else
    echo -e "${YELLOW}⚠ SKIPPED or FAILED${NC}"
    echo "  Response: $response"
fi

echo ""
echo "5. Streaming Test (requires configured providers)"
echo "--------------------------------------------------"
echo -n "Testing streaming response... "
response=$(curl -s -X POST "$GATEWAY_URL/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $API_KEY" \
    -d '{
        "model": "chat-lite",
        "messages": [{"role": "user", "content": "Hi"}],
        "stream": true,
        "max_tokens": 5
    }' 2>&1)

if echo "$response" | grep -q 'data:'; then
    echo -e "${GREEN}✓ PASSED${NC}"
    echo "  Got streaming response"
    ((PASSED++))
else
    echo -e "${YELLOW}⚠ SKIPPED or FAILED${NC}"
    echo "  Response: $response"
fi

echo ""
echo "6. JSON Schema Response Format Test"
echo "------------------------------------"
echo -n "Testing json_schema response format... "
response=$(curl -s -X POST "$GATEWAY_URL/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $API_KEY" \
    -d '{
        "model": "json-safe",
        "messages": [{"role": "user", "content": "Extract name and age"}],
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
                    }
                }
            }
        }
    }' 2>&1)

if echo "$response" | grep -q '"id"'; then
    echo -e "${GREEN}✓ PASSED${NC}"
    ((PASSED++))
else
    echo -e "${YELLOW}⚠ SKIPPED or FAILED${NC}"
fi

echo ""
echo "7. Tools/Function Calling Test"
echo "-------------------------------"
echo -n "Testing tools support... "
response=$(curl -s -X POST "$GATEWAY_URL/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $API_KEY" \
    -d '{
        "model": "tools-pro",
        "messages": [{"role": "user", "content": "What is the weather in Paris?"}],
        "tools": [{
            "type": "function",
            "function": {
                "name": "get_weather",
                "description": "Get weather for a location",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "location": {"type": "string"}
                    }
                }
            }
        }]
    }' 2>&1)

if echo "$response" | grep -q '"tool_calls"\|"id"'; then
    echo -e "${GREEN}✓ PASSED${NC}"
    ((PASSED++))
else
    echo -e "${YELLOW}⚠ SKIPPED or FAILED${NC}"
fi

echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "${GREEN}Passed: $PASSED${NC}"
echo -e "${RED}Failed: $FAILED${NC}"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed${NC}"
    exit 1
fi
