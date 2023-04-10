package yerr

import (
	"errors"
	"testing"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	var (
		err  = errors.New("closed")
		code = ErrorCodeClosed
	)

	se := New(code, err)

	assert.Equal(t, "NetClosed error: message=closed", se.Error())
	assert.Equal(t, code, se.ErrorCode())
}

func TestErrorCode(t *testing.T) {
	code := ErrorCodeData

	assert.Equal(t, "DataFrame", code.String())

	unknown := ErrorCode(123321)
	assert.Equal(t, "XXX", unknown.String())

	qcode := quic.ApplicationErrorCode(206)

	yes := Is(qcode, code)

	assert.True(t, yes)

	parsed := Parse(qcode)
	assert.Equal(t, code, parsed)

	to := code.To()
	assert.Equal(t, to, qcode)
}

func TestDuplicateName(t *testing.T) {
	var (
		err    = errors.New("errmsg")
		connID = "mock-id"
	)

	se := NewDuplicateNameError(connID, err)

	assert.Equal(t, err.Error(), se.Error())
	assert.Equal(t, ErrorCodeDuplicateName, se.ErrorCode())
	assert.Equal(t, connID, se.StreamID())
}
