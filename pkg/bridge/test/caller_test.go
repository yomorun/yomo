package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/metadata"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
)

func TestCaller(t *testing.T) {
	cc := &testComponentCreator{flow: newMockDataFlow(newHandler(time.Millisecond).handle)}

	md, err := cc.ExchangeMetadata("")
	assert.NoError(t, err)

	caller, err := pkgai.NewCaller(cc.CreateSource(""), cc.CreateReducer(""), md, time.Minute)
	assert.NoError(t, err)

	defer caller.Close()

	assert.Equal(t, md, caller.Metadata())

	var (
		prompt = "hello system prompt"
		op     = pkgai.SystemPromptOpPrefix
	)
	caller.SetSystemPrompt(prompt, op)
	gotPrompt, gotOp := caller.GetSystemPrompt()
	assert.Equal(t, prompt, gotPrompt)
	assert.Equal(t, op, gotOp)
}

type testComponentCreator struct {
	flow *mockDataFlow
}

func (c *testComponentCreator) CreateSource(_ string) yomo.Source {
	return c.flow
}

func (c *testComponentCreator) CreateReducer(_ string) yomo.StreamFunction {
	return c.flow
}

func (c *testComponentCreator) ExchangeMetadata(_ string) (metadata.M, error) {
	return metadata.M{"hello": "llm bridge"}, nil
}
