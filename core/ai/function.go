package ai

import (
	"encoding/json"
	"errors"

	"github.com/invopop/jsonschema"
	"golang.org/x/exp/slog"
)

type FunctionCaller interface {
	Name() string
	Description() string
	InputSchema() any
}

func RegisterFunctionCaller(
	appID string,
	tag uint32,
	name string,
	description string,
	inputSchema any,
) error {
	if inputSchema == nil {
		// TODO: need to unregister function
		return nil
	}
	fd := &FunctionDefinition{
		Name:        name,
		Description: description,
	}
	functionParameters, err := parseFunctionParameters(inputSchema)
	if err != nil {
		slog.Error("parse function parameters",
			"app_id", appID,
			"tag", tag,
			"err", err,
		)
		return err
	}
	fd.Parameters = functionParameters
	functionDefinition, err := json.Marshal(fd)
	slog.Info("function definition", "schema", string(functionDefinition))
	return RegisterFunction(appID, tag, string(functionDefinition))
}

func parseFunctionParameters(inputSchema any) (*FunctionParameters, error) {
	r := new(jsonschema.Reflector)
	schema := r.Reflect(inputSchema)
	for _, m := range schema.Definitions {
		functionParameters := &FunctionParameters{
			Type:       m.Type,
			Required:   m.Required,
			Properties: make(map[string]*ParameterProperty),
		}
		for pair := m.Properties.Oldest(); pair != nil; pair = pair.Next() {
			// slog.Info("function parameter",
			// 	"name", pair.Key,
			// 	"type", pair.Value.Type,
			// 	"title", pair.Value.Title,
			// 	"description", pair.Value.Description,
			// )
			functionParameters.Properties[pair.Key] = &ParameterProperty{
				Type:        pair.Value.Type,
				Description: pair.Value.Description,
			}
		}
		return functionParameters, nil
	}
	return nil, errors.New("invalid schema definitions")
}
