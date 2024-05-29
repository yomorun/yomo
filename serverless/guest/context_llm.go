package guest

import "github.com/yomorun/yomo/ai"

func (c *GuestContext) ReadLLMArguments(args any) error {
	panic("not implemented")
}

func (c *GuestContext) WriteLLMResult(result string) error {
	panic("not implemented")
}

func (c *GuestContext) LLMFunctionCall() (*ai.FunctionCall, error) {
	panic("not implemented")
}
