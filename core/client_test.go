package core

import (
	"context"
	"errors"
	"testing"

	"github.com/lucas-clemente/quic-go"
	"github.com/stretchr/testify/assert"
)

const testaddr = "127.0.0.1:19999"

func Test_Client_Dial_Nothing(t *testing.T) {
	ctx := context.Background()

	client := NewClient("source", ClientTypeSource)

	assert.Equal(t, ConnStateReady, client.state, "client state should be ConnStateReady")

	err := client.Connect(ctx, testaddr)

	assert.Equal(t, ConnStateDisconnected, client.state, "client state should be ConnStateDisconnected")

	qerr := &quic.IdleTimeoutError{}

	assert.True(t, errors.As(err, &qerr), "dial timeout")
}
