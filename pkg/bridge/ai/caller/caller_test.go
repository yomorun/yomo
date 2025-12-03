package caller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/bridge/mock"
)

func TestCaller(t *testing.T) {
	cc := &testComponentCreator{flow: mock.NewDataFlow(mock.NewHandler(time.Millisecond).Handle)}

	md, err := cc.ExchangeMetadata("")
	assert.NoError(t, err)

	caller, err := NewCaller(cc.CreateSource(""), cc.CreateReducer(""), md, time.Minute)
	assert.NoError(t, err)

	defer caller.Close()

	assert.Equal(t, md, caller.Metadata())

	var (
		prompt = "hello system prompt"
		op     = SystemPromptOpPrefix
	)
	caller.SetSystemPrompt(prompt, op)
	gotPrompt, gotOp := caller.GetSystemPrompt()
	assert.Equal(t, prompt, gotPrompt)
	assert.Equal(t, op, gotOp)
}

type testComponentCreator struct {
	flow *mock.DataFlow
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
