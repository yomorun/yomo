package openai

import (
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/metadata"
)

type ILLMClient interface {
	ChatCompletion(apiEndpoint string, authHeaderKey string, authHeaderValue string, baseRequestbody ReqBody, baseSystemMessage string, userInstruction string, chainMessage ai.ChainMessage, md metadata.M, ifWithTool bool) (*ai.InvokeResponse, error)
}
