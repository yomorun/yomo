package register

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
)

func TestRegister(t *testing.T) {
	r := NewDefault()

	SetRegister(r)
	assert.Equal(t, r, GetRegister())

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

	err := RegisterFunction(functionDefinition, 1, nil)
	assert.NoError(t, err)

	gotErr := RegisterFunction(functionDefinition, 2, nil)
	assert.EqualError(t, gotErr, "function `function1` already registered")

	toolCalls, err := ListToolCalls(nil)
	assert.NoError(t, err)
	assert.Equal(t, functionDefinition.Name, toolCalls[0].Function.Name)
	assert.Equal(t, functionDefinition.Description, toolCalls[0].Function.Description)

	UnregisterFunction(1, nil)
	toolCalls, err = ListToolCalls(nil)
	assert.NoError(t, err)
	assert.Zero(t, len(toolCalls))
}
