package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetaFrameEncode(t *testing.T) {
	m1 := NewMetaFrame()
	m1.LBType = LoadBalanceBindInstance
	m1.ToInstanceID = "abc"
	buf := m1.Encode()
	m2, err := DecodeToMetaFrame(buf)
	assert.NoError(t, err)
	assert.EqualValues(t, m1.Timestamp(), m2.Timestamp())
	assert.EqualValues(t, m1.LBType, m2.LBType)
	assert.EqualValues(t, m1.ToInstanceID, m2.ToInstanceID)
}
