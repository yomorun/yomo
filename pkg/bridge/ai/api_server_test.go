package ai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
)

func TestHandleOverview(t *testing.T) {
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
	register.SetRegister(register.NewDefault())
	register.RegisterFunction(100, functionDefinition, 200, nil)

	// Create a new request
	req, err := http.NewRequest("GET", "/overview", nil)
	assert.NoError(t, err)

	req = req.WithContext(WithCallerContext(req.Context(), &Caller{md: metadata.New()}))

	// Record the response
	rr := httptest.NewRecorder()

	// Create a handler function
	handler := http.HandlerFunc(HandleOverview)

	// Serve the request
	handler.ServeHTTP(rr, req)

	// Check the response status code
	assert.Equal(t, http.StatusOK, rr.Code)

	// Check the response body
	// This is a basic check for an empty body, replace with your own logic
	assert.Equal(t, "{\"Functions\":{\"100\":{\"name\":\"function1\",\"description\":\"desc1\",\"parameters\":{\"type\":\"type1\",\"properties\":{\"prop1\":{\"type\":\"type1\",\"description\":\"desc1\"},\"prop2\":{\"type\":\"type2\",\"description\":\"desc2\"}},\"required\":[\"prop1\"]}}}}\n", rr.Body.String())
}
