package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
)

func convertPart(req openai.ChatCompletionRequest, model *genai.GenerativeModel) []genai.Part {
	parts := []genai.Part{}

	if len(req.Tools) > 0 {
		model.Tools = convertTools(req.Tools)
	}

	for _, message := range req.Messages {
		switch message.Role {
		case openai.ChatMessageRoleUser:
			parts = append(parts, genai.Text(message.Content))
		case openai.ChatMessageRoleSystem:
			if message.Content != "" {
				model.SystemInstruction = &genai.Content{Parts: []genai.Part{genai.Text(message.Content)}}
			}
			for _, tc := range message.ToolCalls {
				args := map[string]any{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				parts = append(parts,
					genai.FunctionCall{
						Name: tc.Function.Name,
						Args: args,
					},
				)
			}
		case openai.ChatMessageRoleAssistant:
			for _, tc := range message.ToolCalls {
				args := map[string]any{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				parts = append(parts, genai.FunctionCall{
					Name: tc.Function.Name,
					Args: args,
				})
			}
		case openai.ChatMessageRoleTool:
			resp := map[string]any{}
			if err := json.Unmarshal([]byte(message.Content), &resp); err != nil {
				resp["result"] = message.Content
			}

			toolID := message.ToolCallID
			parts = append(parts, genai.FunctionResponse{
				Name:     toolID[:len(toolID)-4],
				Response: resp,
			})
		}
	}

	return parts
}

func convertTools(tools []openai.Tool) []*genai.Tool {
	var result []*genai.Tool

	for _, tool := range tools {
		params := &ai.FunctionParameters{}

		raw, _ := json.Marshal(tool.Function.Parameters)
		_ = json.Unmarshal(raw, params)

		item := &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  convertFunctionParameters(params),
			}},
		}
		result = append(result, item)
	}

	return result
}

func convertFunctionParameters(params *ai.FunctionParameters) *genai.Schema {
	genaiSchema := &genai.Schema{
		Type:       genai.TypeObject,
		Required:   params.Required,
		Properties: make(map[string]*genai.Schema, len(params.Properties)),
	}

	for k, v := range params.Properties {
		genaiSchema.Properties[k] = convertProperty(v)
	}

	return genaiSchema
}

// convertType converts jsonschema type to gemini type
// https://datatracker.ietf.org/doc/html/draft-bhutton-json-schema-validation-00#section-6.1.1
func convertType(t string) genai.Type {
	tt, ok := typeMap[t]
	if !ok {
		return genai.TypeUnspecified
	}
	return tt
}

var typeMap = map[string]genai.Type{
	"string":  genai.TypeString,
	"integer": genai.TypeInteger,
	"number":  genai.TypeNumber,
	"boolean": genai.TypeBoolean,
	"array":   genai.TypeArray,
	"object":  genai.TypeObject,
	"null":    genai.TypeUnspecified,
}

func convertProperty(prop *ai.ParameterProperty) *genai.Schema {
	enums := []string{}
	for _, v := range prop.Enum {
		switch v := v.(type) {
		case string:
			enums = append(enums, v)
		default:
			enums = append(enums, fmt.Sprintf("%v", v))
		}
	}
	return &genai.Schema{
		Type:        convertType(prop.Type),
		Description: prop.Description,
		Enum:        enums,
	}
}
