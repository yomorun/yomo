package auth

import (
	"bytes"

	"github.com/yomorun/yomo/core/auth"
)

var _ = auth.Credential(&AppKeyCredential{})

type AppKeyCredential struct {
	authType auth.AuthType
	payload  []byte
}

func NewAppKeyCredential(appID string, appSecret string) *AppKeyCredential {
	var buf bytes.Buffer
	buf.WriteString(appID)
	buf.WriteString(appSecret)
	payload := buf.Bytes()

	return &AppKeyCredential{
		authType: auth.AuthTypeAppKey,
		payload:  payload,
	}
}

func (a *AppKeyCredential) Type() auth.AuthType {
	return auth.AuthType(a.authType)
}

func (a *AppKeyCredential) Payload() []byte {
	return a.payload
}
