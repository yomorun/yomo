package ai

import (
	"encoding/json"

	"github.com/sashabaranov/go-openai"
)

// DecodeChatCompletionRequest decodes openai.ChatCompletionRequest from JSON data.
// If a response_format is present, we cannot directly unmarshal it as a ChatCompletionRequest
// because the schema field is a json.Unmarshaler.
// Therefore, we need to use a temporary tmpRequest to unmarshal it.
func DecodeChatCompletionRequest(data []byte) (req openai.ChatCompletionRequest, err error) {
	type tmpRequest struct {
		openai.ChatCompletionRequest
		ResponseFormat *struct {
			*openai.ChatCompletionResponseFormat
			JSONSchema *struct {
				*openai.ChatCompletionResponseFormatJSONSchema
				Schema json.RawMessage `json:"schema"` // json.RawMessage implements json.Unmarshaler
			} `json:"json_schema"`
		} `json:"response_format"`
	}

	var tmp tmpRequest
	if err := json.Unmarshal(data, &tmp); err != nil {
		return openai.ChatCompletionRequest{}, err
	}

	req = tmp.ChatCompletionRequest

	if format := tmp.ResponseFormat; format != nil {
		req.ResponseFormat = format.ChatCompletionResponseFormat
		if jsonSchema := format.JSONSchema; jsonSchema != nil {
			req.ResponseFormat.JSONSchema = jsonSchema.ChatCompletionResponseFormatJSONSchema
			if schema := format.JSONSchema.Schema; schema != nil {
				format.JSONSchema.ChatCompletionResponseFormatJSONSchema.Schema = schema
			}
		}
	}

	return req, nil
}
