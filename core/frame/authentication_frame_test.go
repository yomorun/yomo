package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthenticationFrame(t *testing.T) {
	m := NewAuthenticationFrame("token", "a")
	assert.Equal(t, []byte{
		0x80 | byte(TagOfAuthenticationFrame), 0xa,
		byte(TagOfAuthenticationName), 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e,
		byte(TagOfAuthenticationPayload), 0x01, 0x61,
	},
		m.Encode(),
	)

	authenticate, err := DecodeToAuthenticationFrame(m.Encode())
	assert.NoError(t, err)
	assert.EqualValues(t, "token", authenticate.AuthName())
	assert.EqualValues(t, "a", authenticate.AuthPayload())
}
