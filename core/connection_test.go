package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
)

func TestConnection(t *testing.T) {
	var (
		id       = uint64(1023)
		name     = "test-data-connection"
		clientID = "123456"
		styp     = ClientTypeStreamFunction
		observed = []uint32{1, 2, 3}
		md       metadata.M
	)

	connection := newConnection(id, name, clientID, styp, md, observed, nil, ylog.Default())

	t.Run("ConnectionInfo", func(t *testing.T) {
		assert.Equal(t, id, connection.ID())
		assert.Equal(t, clientID, connection.ClientID())
		assert.Equal(t, name, connection.Name())
		assert.Equal(t, styp, connection.ClientType())
		assert.Equal(t, md, connection.Metadata())
		assert.Equal(t, observed, connection.ObserveDataTags())
	})
}

func TestClientTypeString(t *testing.T) {
	assert.Equal(t, ClientTypeSource.String(), "Source")
	assert.Equal(t, ClientTypeStreamFunction.String(), "StreamFunction")
	assert.Equal(t, ClientTypeUpstreamZipper.String(), "UpstreamZipper")
	assert.Equal(t, ClientType(0).String(), "Unknown")
}

func TestNextIncrID(t *testing.T) {
	first := uint64(0)
	for i := 0; i < 1000; i++ {
		got := incrID()
		assert.True(t, got >= first)
	}
}
