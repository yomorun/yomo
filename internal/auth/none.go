package auth

// type AuthNone struct{}

// func (a *AuthNone) Authenticate(f *frame.HandshakeFrame) bool {
// 	return true
// }

var _ = Credential(&CredentialNone{})

type CredentialNone struct{}

func NewCredendialNone() *CredentialNone {
	return &CredentialNone{}
}

func (c *CredentialNone) Type() AuthType {
	return AuthTypeNone
}

func (c *CredentialNone) Payload() []byte {
	return nil
}
