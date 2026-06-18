package services

import (
	"encoding/json"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/require"
)

func TestValidateStructuredOutputResponse_JSONSchemaRejectsInvalidProviderOutput(t *testing.T) {
	strict := true
	req := types.ChatCompletionRequest{
		ResponseFormat: &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:   "person",
				Schema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`),
				Strict: &strict,
			},
		},
	}
	content := `{"name":123}`
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
	}

	err := validateStructuredOutputResponse(req, resp, "provider-a", "model-1")
	require.Error(t, err)
	require.IsType(t, &errors.ParseError{}, err)
}
