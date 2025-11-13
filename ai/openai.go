package ai

import (
	"encoding/json"

	"github.com/sashabaranov/go-openai"
)

// DecodeChatCompletionRequest decodes openai.ChatCompletionRequest from JSON data.
// If a response_format is present, we cannot directly unmarshal it as a ChatCompletionRequest
// because the schema field is a json.Unmarshaler.
// Therefore, we need to use a temporary tmpRequest to unmarshal it.
func DecodeChatCompletionRequest(data []byte) (req openai.ChatCompletionRequest, extraBody map[string]string, err error) {

	var tmp tmpRequest
	if err := json.Unmarshal(data, &tmp); err != nil {
		return openai.ChatCompletionRequest{}, nil, err
	}

	req = tmp.ChatCompletionRequest

	if format := tmp.ResponseFormat; format != nil {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: format.Type,
		}
		if jsonSchema := format.JSONSchema; jsonSchema != nil {
			req.ResponseFormat.JSONSchema = &openai.ChatCompletionResponseFormatJSONSchema{
				Name:        jsonSchema.Name,
				Description: jsonSchema.Description,
				Strict:      jsonSchema.Strict,
			}
			if schema := jsonSchema.Schema; schema != nil && string(schema) != "null" {
				req.ResponseFormat.JSONSchema.Schema = schema
			}
		}
	}

	return req, tmp.ExtraBody.YomoToolCustom, nil
}

type tmpJSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      bool            `json:"strict"`
}

type tmpResponseFormat struct {
	Type       openai.ChatCompletionResponseFormatType `json:"type"`
	JSONSchema *tmpJSONSchema                          `json:"json_schema,omitempty"`
}

type tmpRequest struct {
	openai.ChatCompletionRequest
	ResponseFormat *tmpResponseFormat `json:"response_format"`
	ExtraBody      *extraBody         `json:"extra_body,omitempty"`
}

type mapAny map[string]any

type extraBody struct {
	mapAny
	YomoToolCustom map[string]string `json:"yomo_tool_custom,omitempty"`
}
