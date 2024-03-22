package gemini

import (
	"reflect"
	"testing"

	"github.com/yomorun/yomo/ai"
)

func TestConvertPropertyToStandard(t *testing.T) {
	properties := map[string]*Property{
		"prop1": {Type: "type1", Description: "desc1"},
		"prop2": {Type: "type2", Description: "desc2"},
	}

	expected := map[string]*ai.ParameterProperty{
		"prop1": {Type: "type1", Description: "desc1"},
		"prop2": {Type: "type2", Description: "desc2"},
	}

	result := convertPropertyToStandard(properties)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("convertPropertyToStandard() = %v, want %v", result, expected)
	}
}

func TestConvertPropertyToStandard_NilInput(t *testing.T) {
	result := convertPropertyToStandard(nil)

	if result != nil {
		t.Errorf("convertPropertyToStandard() = %v, want %v", result, nil)
	}
}

func TestConvertFunctionParametersToStandard(t *testing.T) {
	parameters := &FunctionParameters{
		Type: "type1",
		Properties: map[string]*Property{
			"prop1": {Type: "type1", Description: "desc1"},
			"prop2": {Type: "type2", Description: "desc2"},
		},
		Required: []string{"prop1"},
	}

	expected := &ai.FunctionParameters{
		Type: "type1",
		Properties: map[string]*ai.ParameterProperty{
			"prop1": {Type: "type1", Description: "desc1"},
			"prop2": {Type: "type2", Description: "desc2"},
		},
		Required: []string{"prop1"},
	}

	result := convertFunctionParametersToStandard(parameters)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("convertFunctionParametersToStandard() = %v, want %v", result, expected)
	}
}

func TestConvertFunctionParametersToStandard_NilInput(t *testing.T) {
	result := convertFunctionParametersToStandard(nil)

	if result != nil {
		t.Errorf("convertFunctionParametersToStandard() = %v, want %v", result, nil)
	}
}

func TestConvertFunctionDeclarationToStandard(t *testing.T) {
	functionDeclaration := &FunctionDeclaration{
		Name:        "function1",
		Description: "desc1",
		Parameters: &FunctionParameters{
			Type: "type1",
			Properties: map[string]*Property{
				"prop1": {Type: "type1", Description: "desc1"},
				"prop2": {Type: "type2", Description: "desc2"},
			},
			Required: []string{"prop1"},
		},
	}

	expected := &ai.FunctionDefinition{
		Name:        "function1",
		Description: "desc1",
		Parameters: &ai.FunctionParameters{
			Type: "type1",
			Properties: map[string]*ai.ParameterProperty{
				"prop1": {Type: "type1", Description: "desc1"},
				"prop2": {Type: "type2", Description: "desc2"},
			},
			Required: []string{"prop1"},
		},
	}

	result := convertFunctionDeclarationToStandard(functionDeclaration)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("convertFunctionDeclarationToStandard() = %v, want %v", result, expected)
	}
}

func TestConvertFunctionDeclarationToStandard_NilInput(t *testing.T) {
	result := convertFunctionDeclarationToStandard(nil)

	if result != nil {
		t.Errorf("convertFunctionDeclarationToStandard() = %v, want %v", result, nil)
	}
}

func TestConvertStandardToProperty(t *testing.T) {
	properties := map[string]*ai.ParameterProperty{
		"prop1": {Type: "type1", Description: "desc1"},
		"prop2": {Type: "type2", Description: "desc2"},
	}

	expected := map[string]*Property{
		"prop1": {Type: "type1", Description: "desc1"},
		"prop2": {Type: "type2", Description: "desc2"},
	}

	result := convertStandardToProperty(properties)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("convertStandardToProperty() = %v, want %v", result, expected)
	}
}

func TestConvertStandardToProperty_NilInput(t *testing.T) {
	result := convertStandardToProperty(nil)

	if result != nil {
		t.Errorf("convertStandardToProperty() = %v, want %v", result, nil)
	}
}

func TestConvertStandardToFunctionParameters(t *testing.T) {
	parameters := &ai.FunctionParameters{
		Type: "type1",
		Properties: map[string]*ai.ParameterProperty{
			"prop1": {Type: "type1", Description: "desc1"},
			"prop2": {Type: "type2", Description: "desc2"},
		},
		Required: []string{"prop1"},
	}

	expected := &FunctionParameters{
		Type: "type1",
		Properties: map[string]*Property{
			"prop1": {Type: "type1", Description: "desc1"},
			"prop2": {Type: "type2", Description: "desc2"},
		},
		Required: []string{"prop1"},
	}

	result := convertStandardToFunctionParameters(parameters)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("convertStandardToFunctionParameters() = %v, want %v", result, expected)
	}
}

func TestConvertStandardToFunctionParameters_NilInput(t *testing.T) {
	result := convertStandardToFunctionParameters(nil)

	if result != nil {
		t.Errorf("convertStandardToFunctionParameters() = %v, want %v", result, nil)
	}
}

func TestConvertStandardToFunctionDeclaration(t *testing.T) {
	functionDefinition := &ai.FunctionDefinition{
		Name:        "function1",
		Description: "desc1",
		Parameters: &ai.FunctionParameters{
			Type: "type1",
			Properties: map[string]*ai.ParameterProperty{
				"prop1": {Type: "type1", Description: "desc1"},
				"prop2": {Type: "type2", Description: "desc2"},
			},
			Required: []string{"prop1"},
		},
	}

	expected := &FunctionDeclaration{
		Name:        "function1",
		Description: "desc1",
		Parameters: &FunctionParameters{
			Type: "type1",
			Properties: map[string]*Property{
				"prop1": {Type: "type1", Description: "desc1"},
				"prop2": {Type: "type2", Description: "desc2"},
			},
			Required: []string{"prop1"},
		},
	}

	result := convertStandardToFunctionDeclaration(functionDefinition)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("convertStandardToFunctionDeclaration() = %v, want %v", result, expected)
	}
}

func TestConvertStandardToFunctionDeclaration_NilInput(t *testing.T) {
	result := convertStandardToFunctionDeclaration(nil)

	if result != nil {
		t.Errorf("convertStandardToFunctionDeclaration() = %v, want %v", result, nil)
	}
}

func TestGenerateJSONSchemaArguments(t *testing.T) {
	args := map[string]interface{}{
		"arg1": "value1",
		"arg2": "value2",
	}

	expected := `{"arg1":"value1","arg2":"value2"}`

	result := generateJSONSchemaArguments(args)

	if result != expected {
		t.Errorf("generateJSONSchemaArguments() = %v, want %v", result, expected)
	}
}

func TestGenerateJSONSchemaArguments_EmptyArgs(t *testing.T) {
	args := map[string]interface{}{}

	expected := `{}`

	result := generateJSONSchemaArguments(args)

	if result != expected {
		t.Errorf("generateJSONSchemaArguments() = %v, want %v", result, expected)
	}
}

func TestParseAPIResponseBody(t *testing.T) {
	respBody := []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"converter","args":{"timeString":"1900-01-01 07:00:00","targetTimezone":"Asia/Singapore","sourceTimezone":"America/Los_Angeles"}}}],"role":"model"},"finishReason":"STOP","index":0}],"promptFeedback":{"safetyRatings":[{"category":"HARM_CATEGORY_SEXUALLY_EXPLICIT","probability":"NEGLIGIBLE"},{"category":"HARM_CATEGORY_HATE_SPEECH","probability":"NEGLIGIBLE"},{"category":"HARM_CATEGORY_HARASSMENT","probability":"NEGLIGIBLE"},{"category":"HARM_CATEGORY_DANGEROUS_CONTENT","probability":"NEGLIGIBLE"}]}}`)
	expected := &Response{
		Candidates: []Candidate{
			{
				Content: &CandidateContent{
					Parts: []*Part{
						{
							FunctionCall: &FunctionCall{
								Name: "converter",
								Args: map[string]interface{}{
									"timeString":     "1900-01-01 07:00:00",
									"targetTimezone": "Asia/Singapore",
									"sourceTimezone": "America/Los_Angeles",
								},
							},
						},
					},
					Role: "model",
				},
				FinishReason: "STOP",
				Index:        0,
			},
		},
	}

	result, err := parseAPIResponseBody(respBody)
	if err != nil {
		t.Fatalf("parseAPIResponseBody() error = %v, wantErr %v", err, false)
	}

	if !reflect.DeepEqual(result.Candidates, expected.Candidates) {
		t.Errorf("parseAPIResponseBody() = %v, want %v", result, expected)
	}
}

func TestParseAPIResponseBody_InvalidJSON(t *testing.T) {
	respBody := []byte(`invalid json`)

	_, err := parseAPIResponseBody(respBody)
	if err == nil {
		t.Errorf("parseAPIResponseBody() error = %v, wantErr %v", err, true)
	}
}

func TestParseAPIResponseBody_JSON(t *testing.T) {
	str := "{\n  \"candidates\": [\n    {\n      \"content\": {\n        \"parts\": [\n          {\n            \"functionCall\": {\n              \"name\": \"converter\",\n              \"args\": {\n                \"timeString\": \"1900-01-01 07:00:00\",\n                \"targetTimezone\": \"Asia/Singapore\",\n                \"sourceTimezone\": \"America/Los_Angeles\"\n              }\n            }\n          }\n        ],\n        \"role\": \"model\"\n      },\n      \"finishReason\": \"STOP\",\n      \"index\": 0\n    }\n  ],\n  \"promptFeedback\": {\n    \"safetyRatings\": [\n      {\n        \"category\": \"HARM_CATEGORY_SEXUALLY_EXPLICIT\",\n        \"probability\": \"NEGLIGIBLE\"\n      },\n      {\n        \"category\": \"HARM_CATEGORY_HATE_SPEECH\",\n        \"probability\": \"NEGLIGIBLE\"\n      },\n      {\n        \"category\": \"HARM_CATEGORY_HARASSMENT\",\n        \"probability\": \"NEGLIGIBLE\"\n      },\n      {\n        \"category\": \"HARM_CATEGORY_DANGEROUS_CONTENT\",\n        \"probability\": \"NEGLIGIBLE\"\n      }\n    ]\n  }\n}\n"

	respBody := []byte(str)

	expected := &Response{
		Candidates: []Candidate{
			{
				Content: &CandidateContent{
					Parts: []*Part{
						{
							FunctionCall: &FunctionCall{
								Name: "converter",
								Args: map[string]interface{}{
									"timeString":     "1900-01-01 07:00:00",
									"targetTimezone": "Asia/Singapore",
									"sourceTimezone": "America/Los_Angeles",
								},
							},
						},
					},
					Role: "model",
				},
				FinishReason: "STOP",
				Index:        0,
			},
		},
	}

	result, err := parseAPIResponseBody(respBody)
	if err != nil {
		t.Fatalf("parseAPIResponseBody() error = %v, wantErr %v", err, false)
	}

	if !reflect.DeepEqual(result.Candidates, expected.Candidates) {
		t.Errorf("parseAPIResponseBody() = %v, want %v", result, expected)
	}
}

func TestParseToolCallFromResponse(t *testing.T) {
	resp := &Response{
		Candidates: []Candidate{
			{
				Content: &CandidateContent{
					Parts: []*Part{
						{
							FunctionCall: &FunctionCall{
								Name: "find_theaters",
								Args: map[string]interface{}{
									"location": "Mountain View, CA",
									"movie":    "Barbie",
								},
							},
						},
					},
				},
			},
		},
	}

	expected := []ai.ToolCall{
		{
			Function: &ai.FunctionDefinition{
				Name:      "find_theaters",
				Arguments: "{\"location\":\"Mountain View, CA\",\"movie\":\"Barbie\"}",
			},
			ID:   "cc-gemini-id",
			Type: "cc-function",
		},
	}

	result := parseToolCallFromResponse(resp)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("parseToolCallFromResponse() = %v, want %v", result, expected)
	}
}
