package gemini

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yomorun/yomo/ai"
)

func TestGeminiProvider_Name(t *testing.T) {
	provider := &GeminiProvider{}

	name := provider.Name()

	if name != "gemini" {
		t.Errorf("Name() = %v, want %v", name, "gemini")
	}
}

func TestGeminiProvider_getApiUrl(t *testing.T) {
	provider := &GeminiProvider{
		APIKey: "test-api-key",
	}

	expected := "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=test-api-key"

	result := provider.getApiUrl()

	if result != expected {
		t.Errorf("getApiUrl() = %v, want %v", result, expected)
	}
}

func TestNewProvider(t *testing.T) {
	apiKey := "test-api-key"
	provider := NewProvider(apiKey)

	if provider.APIKey != apiKey {
		t.Errorf("NewProvider() = %v, want %v", provider.APIKey, apiKey)
	}
}

func TestNewProviderWithEnvVar(t *testing.T) {
	// Set up
	expectedAPIKey := "test-api-key"
	os.Setenv("GEMINI_API_KEY", expectedAPIKey)

	// Call the function under test
	provider := NewProvider("")

	// Check the result
	if provider.APIKey != expectedAPIKey {
		t.Errorf("NewProvider() = %v, want %v", provider.APIKey, expectedAPIKey)
	}
}

func TestNewProviderWithoutEnvVar(t *testing.T) {
	// Set up
	os.Unsetenv("GEMINI_API_KEY")

	// Call the function under test
	provider := NewProvider("")

	// Check the result
	if provider.APIKey != "" {
		t.Errorf("NewProvider() = %v, want %v", provider.APIKey, "")
	}
}

func TestGeminiProvider_GetOverview_Empty(t *testing.T) {
	provider := &GeminiProvider{}

	result, err := provider.GetOverview()
	if err != nil {
		t.Errorf("GetOverview() error = %v, wantErr %v", err, nil)
		return
	}

	if len(result.Functions) != 0 {
		t.Errorf("GetOverview() = %v, want %v", len(result.Functions), 0)
	}
}

func TestGeminiProvider_GetOverview_NotEmpty(t *testing.T) {
	provider := &GeminiProvider{}

	// Add a function to the fns map
	fns.Store("test", &connectedFn{
		tag: 1,
		fd: &FunctionDeclaration{
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
		},
	})

	result, err := provider.GetOverview()
	if err != nil {
		t.Errorf("GetOverview() error = %v, wantErr %v", err, nil)
		return
	}

	if len(result.Functions) != 1 {
		t.Errorf("GetOverview() = %v, want %v", len(result.Functions), 1)
	}
}

func TestGeminiProvider_ListToolCalls_Empty(t *testing.T) {
	fns = sync.Map{}
	provider := &GeminiProvider{}

	result, err := provider.ListToolCalls()
	if err != nil {
		t.Errorf("ListToolCalls() error = %v, wantErr %v", err, nil)
		return
	}

	if len(result) != 0 {
		t.Errorf("ListToolCalls() = %v, want %v", len(result), 0)
	}
}

func TestGeminiProvider_ListToolCalls_NotEmpty(t *testing.T) {
	provider := &GeminiProvider{}

	// Add a function to the fns map
	fns.Store("test", &connectedFn{
		tag: 1,
		fd: &FunctionDeclaration{
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
		},
	})

	result, err := provider.ListToolCalls()
	if err != nil {
		t.Errorf("ListToolCalls() error = %v, wantErr %v", err, nil)
		return
	}

	if len(result) != 1 {
		t.Errorf("ListToolCalls() = %v, want %v", len(result), 1)
	}

	// TearDown
	fns = sync.Map{}
}

func TestGeminiProvider_RegisterFunction(t *testing.T) {
	provider := &GeminiProvider{}
	tag := uint32(1)
	connID := uint64(1)
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

	err := provider.RegisterFunction(tag, functionDefinition, connID)
	if err != nil {
		t.Errorf("RegisterFunction() error = %v, wantErr %v", err, nil)
		return
	}

	value, ok := fns.Load(connID)
	if !ok {
		t.Errorf("RegisterFunction() did not store the function correctly")
		return
	}

	cf := value.(*connectedFn)
	if cf.connID != connID || cf.tag != tag || !reflect.DeepEqual(cf.fd, convertStandardToFunctionDeclaration(functionDefinition)) {
		t.Errorf("RegisterFunction() = %v, want %v", cf, &connectedFn{
			connID: connID,
			tag:    tag,
			fd:     convertStandardToFunctionDeclaration(functionDefinition),
		})
	}
}

func TestGeminiProvider_UnregisterFunction(t *testing.T) {
	provider := &GeminiProvider{}
	connID := uint64(1)

	// Add a function to the fns map
	fns.Store(connID, &connectedFn{
		tag: 1,
		fd: &FunctionDeclaration{
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
		},
	})

	err := provider.UnregisterFunction("function1", connID)
	if err != nil {
		t.Errorf("UnregisterFunction() error = %v, wantErr %v", err, nil)
		return
	}

	_, ok := fns.Load(connID)
	if ok {
		t.Errorf("UnregisterFunction() did not remove the function correctly")
	}

	// TearDown
	fns = sync.Map{}
}

func TestGeminiProvider_GetChatCompletions_NoFunctions(t *testing.T) {
	fns = sync.Map{}

	provider := &GeminiProvider{}

	result, err := provider.GetChatCompletions("test")

	if !errors.Is(err, ai.ErrNoFunctionCall) {
		t.Errorf("GetChatCompletions() error = %v, wantErr %v", err, ai.ErrNoFunctionCall)
		return
	}

	if result.Content != "no toolCalls" {
		t.Errorf("GetChatCompletions() = %v, want %v", result.Content, "no toolCalls")
	}
}

func TestGeminiProvider_prepareRequestBody_NilInstruction(t *testing.T) {
	provider := &GeminiProvider{}

	userInstruction := ""
	expected := &RequestBody{
		Contents: Contents{
			Role: "user",
			Parts: Parts{
				Text: userInstruction,
			},
		},
		Tools: []Tool{},
	}

	result := provider.prepareRequestBody(userInstruction)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("prepareRequestBody() = %v, want %v", result, expected)
	}
}

func TestGeminiProvider_prepareRequestBody_Cleanup(t *testing.T) {
	t.Log("-------------tear down------------")
	// TearDown
	fns = sync.Map{}
}

func TestGeminiProvider_prepareRequestBody_NoFunctions(t *testing.T) {
	provider := &GeminiProvider{}

	userInstruction := "test instruction"
	expected := &RequestBody{
		Contents: Contents{
			Role: "user",
			Parts: Parts{
				Text: userInstruction,
			},
		},
		Tools: []Tool{},
	}

	result := provider.prepareRequestBody(userInstruction)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("prepareRequestBody() = %v, want %v", result, expected)
	}
}

func TestGeminiProvider_prepareRequestBody_EmptyInstruction(t *testing.T) {
	provider := &GeminiProvider{}

	userInstruction := ""
	expected := &RequestBody{
		Contents: Contents{
			Role: "user",
			Parts: Parts{
				Text: userInstruction,
			},
		},
		Tools: []Tool{},
	}

	result := provider.prepareRequestBody(userInstruction)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("prepareRequestBody() = %v, want %v", result, expected)
	}
}

func TestGeminiProvider_prepareRequestBody(t *testing.T) {
	provider := &GeminiProvider{}

	// Add a function to the fns map
	fns.Store(uint64(1), &connectedFn{
		tag: 1,
		fd: &FunctionDeclaration{
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
		},
	})

	userInstruction := "test instruction"
	expected := &RequestBody{
		Contents: Contents{
			Role: "user",
			Parts: Parts{
				Text: userInstruction,
			},
		},
		Tools: []Tool{
			{
				FunctionDeclarations: []*FunctionDeclaration{
					{
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
					},
				},
			},
		},
	}

	result := provider.prepareRequestBody(userInstruction)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("prepareRequestBody() = %v, want %v", result, expected)
	}
}

func TestGeminiProvider_prepareRequestBody_JSON(t *testing.T) {
	provider := &GeminiProvider{}

	// Add a function to the fns map
	fns.Store(uint64(1), &connectedFn{
		tag: 1,
		fd: &FunctionDeclaration{
			Name:        "find_movies",
			Description: "find movie titles currently playing in theaters based on any description, genre, title words, etc.",
			Parameters: &FunctionParameters{
				Type: "object",
				Properties: map[string]*Property{
					"location":    {Type: "string", Description: "The city and state, e.g. San Francisco, CA or a zip code e.g. 95616"},
					"description": {Type: "string", Description: "Any kind of description including category or genre, title words, attributes, etc."},
				},
				Required: []string{"description"},
			},
		},
	})

	fns.Store(uint64(2), &connectedFn{
		tag: 2,
		fd: &FunctionDeclaration{
			Name:        "find_theaters",
			Description: "find theaters based on location and optionally movie title which are is currently playing in theaters",
			Parameters: &FunctionParameters{
				Type: "object",
				Properties: map[string]*Property{
					"location": {Type: "string", Description: "The city and state, e.g. San Francisco, CA or a zip code e.g. 95616"},
					"movie":    {Type: "string", Description: "Any movie title"},
				},
				Required: []string{"location"},
			},
		},
	})

	fns.Store(uint64(3), &connectedFn{
		tag: 3,
		fd: &FunctionDeclaration{
			Name:        "get_showtimes",
			Description: "Find the start times for movies playing in a specific theater",
			Parameters: &FunctionParameters{
				Type: "object",
				Properties: map[string]*Property{
					"location": {Type: "string", Description: "The city and state, e.g. San Francisco, CA or a zip code e.g. 95616"},
					"movie":    {Type: "string", Description: "Any movie title"},
					"theater":  {Type: "string", Description: "Name of the theater"},
					"date":     {Type: "string", Description: "Date for requested showtime"},
				},
				Required: []string{"location", "movie", "theater", "date"},
			},
		},
	})

	userInstruction := "Which theaters in Mountain View show Barbie movie?"

	expected := `{
    "contents": {
      "role": "user",
      "parts": {
        "text": "Which theaters in Mountain View show Barbie movie?"
    }
  },
  "tools": [
    {
      "function_declarations": [
        {
          "name": "find_movies",
          "description": "find movie titles currently playing in theaters based on any description, genre, title words, etc.",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "The city and state, e.g. San Francisco, CA or a zip code e.g. 95616"
              },
              "description": {
                "type": "string",
                "description": "Any kind of description including category or genre, title words, attributes, etc."
              }
            },
            "required": [
              "description"
            ]
          }
        },
        {
          "name": "find_theaters",
          "description": "find theaters based on location and optionally movie title which are is currently playing in theaters",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "The city and state, e.g. San Francisco, CA or a zip code e.g. 95616"
              },
              "movie": {
                "type": "string",
                "description": "Any movie title"
              }
            },
            "required": [
              "location"
            ]
          }
        },
        {
          "name": "get_showtimes",
          "description": "Find the start times for movies playing in a specific theater",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "The city and state, e.g. San Francisco, CA or a zip code e.g. 95616"
              },
              "movie": {
                "type": "string",
                "description": "Any movie title"
              },
              "theater": {
                "type": "string",
                "description": "Name of the theater"
              },
              "date": {
                "type": "string",
                "description": "Date for requested showtime"
              }
            },
            "required": [
              "location",
              "movie",
              "theater",
              "date"
            ]
          }
        }
      ]
    }
  ]
}`

	result := provider.prepareRequestBody(userInstruction)

	jsonBody, err := json.Marshal(result)
	if err != nil {
		t.Errorf("Error preparing request body: %v", err)
	}

	require.JSONEqf(t, expected, string(jsonBody), "prepareRequestBody() = %v, want %v", string(jsonBody), expected)

	// if string(jsonBody) != expected {
	// 	t.Errorf("prepareRequestBody() = %v, want %v", string(jsonBody), expected)
	// }
}
