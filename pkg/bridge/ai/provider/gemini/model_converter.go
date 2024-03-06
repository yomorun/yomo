package gemini

import (
	"encoding/json"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
)

func convertStandardToFunctionDeclaration(functionDefinition *ai.FunctionDefinition) *FunctionDeclaration {
	if functionDefinition == nil {
		return nil
	}

	return &FunctionDeclaration{
		Name:        functionDefinition.Name,
		Description: functionDefinition.Description,
		Parameters:  convertStandardToFunctionParameters(functionDefinition.Parameters),
	}
}

func convertFunctionDeclarationToStandard(functionDefinition *FunctionDeclaration) *ai.FunctionDefinition {
	if functionDefinition == nil {
		return nil
	}

	return &ai.FunctionDefinition{
		Name:        functionDefinition.Name,
		Description: functionDefinition.Description,
		Parameters:  convertFunctionParametersToStandard(functionDefinition.Parameters),
	}
}

func convertStandardToFunctionParameters(parameters *ai.FunctionParameters) *FunctionParameters {
	if parameters == nil {
		return nil
	}

	return &FunctionParameters{
		Type:       parameters.Type,
		Properties: convertStandardToProperty(parameters.Properties),
		Required:   parameters.Required,
	}
}

func convertFunctionParametersToStandard(parameters *FunctionParameters) *ai.FunctionParameters {
	if parameters == nil {
		return nil
	}

	return &ai.FunctionParameters{
		Type:       parameters.Type,
		Properties: convertPropertyToStandard(parameters.Properties),
		Required:   parameters.Required,
	}
}

func convertStandardToProperty(properties map[string]*ai.ParameterProperty) map[string]*Property {
	if properties == nil {
		return nil
	}

	result := make(map[string]*Property)
	for k, v := range properties {
		result[k] = &Property{
			Type:        v.Type,
			Description: v.Description,
		}
	}
	return result
}

func convertPropertyToStandard(properties map[string]*Property) map[string]*ai.ParameterProperty {
	if properties == nil {
		return nil
	}

	result := make(map[string]*ai.ParameterProperty)
	for k, v := range properties {
		result[k] = &ai.ParameterProperty{
			Type:        v.Type,
			Description: v.Description,
		}
	}
	return result
}

// generateJSONSchemaArguments generates the JSON schema arguments from OpenAPI compatible arguments
// https://ai.google.dev/docs/function_calling#how_it_works
func generateJSONSchemaArguments(args map[string]interface{}) string {
	schema := make(map[string]interface{})

	for k, v := range args {
		schema[k] = v
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return ""
	}

	return string(schemaJSON)
}

func parseAPIResponseBody(respBody []byte) (*Response, error) {
	var response *Response
	err := json.Unmarshal(respBody, &response)
	if err != nil {
		ylog.Error("parseAPIResponseBody", "err", err, "respBody", string(respBody))
		return nil, err
	}
	return response, nil
}

func parseToolCallFromResponse(response *Response) []ai.ToolCall {
	calls := make([]ai.ToolCall, 0)
	for _, candidate := range response.Candidates {
		fn := candidate.Content.Parts[0].FunctionCall
		fd := &ai.FunctionDefinition{
			Name:      fn.Name,
			Arguments: generateJSONSchemaArguments(fn.Args),
		}
		call := ai.ToolCall{
			ID:       "cc-gemini-id",
			Type:     "cc-function",
			Function: fd,
		}
		calls = append(calls, call)
	}
	return calls
}
