package ai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/metadata"
)

func TestCaller(t *testing.T) {
	cc := &testComponentCreator{flow: newMockDataFlow(newHandler(time.Millisecond).handle)}

	md, err := cc.ExchangeMetadata("")
	assert.NoError(t, err)

	caller, err := NewCaller(cc.CreateSource(""), cc.CreateReducer(""), md, time.Minute)
	assert.NoError(t, err)

	defer caller.Close()

	assert.Equal(t, md, caller.Metadata())

	sysPrompt := "hello system prompt"
	caller.SetSystemPrompt(sysPrompt)
	assert.Equal(t, sysPrompt, caller.GetSystemPrompt())
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
