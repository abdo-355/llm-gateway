package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func validateStructuredOutputResponse(req types.ChatCompletionRequest, resp *types.ChatCompletionResponse, providerID, model string) error {
	format := req.ResponseFormat
	if format == nil || format.Type == "" || format.Type == "text" {
		return nil
	}
	if resp == nil {
		return errors.NewEmptyResponseError(providerID, model, httpStatusOK)
	}
	if len(resp.Choices) == 0 {
		return errors.NewEmptyResponseError(providerID, model, httpStatusOK)
	}

	switch format.Type {
	case "json_object":
		for i, choice := range resp.Choices {
			content, err := structuredChoiceContent(choice, i, providerID, model)
			if err != nil {
				return err
			}
			if err := validateJSONObjectContent(content); err != nil {
				return errors.NewParseError(
					fmt.Sprintf("Provider %s/%s returned content that does not satisfy response_format json_object", providerID, model),
					"json_object",
					providerID,
					model,
					truncateString(content, 500),
					err,
				)
			}
		}
	case "json_schema":
		schema, err := compileResponseJSONSchema(format)
		if err != nil {
			return err
		}
		for i, choice := range resp.Choices {
			content, err := structuredChoiceContent(choice, i, providerID, model)
			if err != nil {
				return err
			}
			value, err := jsonschema.UnmarshalJSON(strings.NewReader(content))
			if err != nil {
				return errors.NewParseError(
					fmt.Sprintf("Provider %s/%s returned invalid JSON for response_format json_schema", providerID, model),
					"json_schema",
					providerID,
					model,
					truncateString(content, 500),
					err,
				)
			}
			if err := schema.Validate(value); err != nil {
				return errors.NewParseError(
					fmt.Sprintf("Provider %s/%s returned JSON that does not satisfy response_format json_schema", providerID, model),
					"json_schema",
					providerID,
					model,
					truncateString(content, 500),
					err,
				)
			}
		}
	}

	return nil
}

const httpStatusOK = 200

func structuredChoiceContent(choice types.Choice, index int, providerID, model string) (string, error) {
	if choice.Message.Content == nil {
		return "", errors.NewParseError(
			fmt.Sprintf("Provider %s/%s returned no content for structured output choice %d", providerID, model, index),
			"structured_output",
			providerID,
			model,
			"",
			fmt.Errorf("missing assistant message content"),
		)
	}

	content := strings.TrimSpace(*choice.Message.Content)
	if content == "" {
		return "", errors.NewParseError(
			fmt.Sprintf("Provider %s/%s returned empty content for structured output choice %d", providerID, model, index),
			"structured_output",
			providerID,
			model,
			"",
			fmt.Errorf("empty assistant message content"),
		)
	}

	return content, nil
}

func validateJSONObjectContent(content string) error {
	var value map[string]any
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return err
	}
	if value == nil {
		return fmt.Errorf("content is valid JSON but not an object")
	}
	return nil
}

func compileResponseJSONSchema(format *types.ResponseFormat) (*jsonschema.Schema, error) {
	if format.JSONSchema == nil || len(format.JSONSchema.Schema) == 0 {
		return nil, errors.NewValidationError("response_format json_schema must include a schema", nil)
	}

	var schemaDoc any
	if err := json.Unmarshal(format.JSONSchema.Schema, &schemaDoc); err != nil {
		return nil, errors.NewValidationError(fmt.Sprintf("invalid response_format json_schema: %v", err), nil)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return nil, errors.NewValidationError(fmt.Sprintf("invalid response_format json_schema: %v", err), nil)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, errors.NewValidationError(fmt.Sprintf("invalid response_format json_schema: %v", err), nil)
	}

	return schema, nil
}
