package auth

import (
	"bytes"

	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
)

var _ = auth.Authentication(&AppKeyAuth{})

type AppKeyAuth struct {
	authType auth.AuthType
	payload  []byte
}

func NewAppKeyAuth(appID string, appSecret string) *AppKeyAuth {
	var buf bytes.Buffer
	buf.WriteString(appID)
	buf.WriteString(appSecret)
	payload := buf.Bytes()

	return &AppKeyAuth{
		authType: auth.AuthTypeAppKey,
		payload:  payload,
	}
}

func (a *AppKeyAuth) Type() auth.AuthType {
	return auth.AuthType(a.authType)
}

// func (a *AppKeyAuth) Authenticate(store store.Store, f *frame.HandshakeFrame) bool {
func (a *AppKeyAuth) Authenticate(f *frame.HandshakeFrame) bool {
	authType := auth.AuthType(f.AuthType())
	if a.authType != authType {
		return false
	}
	if bytes.Equal(a.payload, f.AuthPayload()) {
		return true
	}
	return false
}
