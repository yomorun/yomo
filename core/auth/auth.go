package auth

// Authentication for server
type Authentication interface {
	Authenticate(payload []byte) bool
	Name() string
}

// Credential for client
type Credential interface {
	Payload() []byte
	Name() string
}

// None auth

var _ Authentication = (*NoneAuth)(nil)

// NoneAuth defaults authentication
type NoneAuth struct{}

func NewNoneAuth() *NoneAuth {
	return &NoneAuth{}
}

func (a *NoneAuth) Authenticate(payload []byte) bool {
	return true
}

func (a *NoneAuth) Name() string {
	return "none"
}

var _ = Credential(&NoneCredential{})

type NoneCredential struct{}

func NewNoneCredendial() *NoneCredential {
	return &NoneCredential{}
}

func (c *NoneCredential) Payload() []byte {
	return nil
}

func (c *NoneCredential) Name() string {
	return "none"
}
