package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
)

func TestConnection(t *testing.T) {
	var (
		name     = "test-data-connection"
		id       = "123456"
		styp     = ClientTypeStreamFunction
		observed = []uint32{1, 2, 3}
		md       metadata.M
	)

	connection := newConnection(name, id, styp, md, observed, nil, ylog.Default())

	t.Run("ConnectionInfo", func(t *testing.T) {
		assert.Equal(t, id, connection.ID())
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
