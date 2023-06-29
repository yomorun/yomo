package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
)

func TestConnector(t *testing.T) {
	connector := NewConnector(context.Background())

	streamID := "id-1"

	t.Run("Store and Get", func(t *testing.T) {
		stream1 := mockDataStream(streamID, "name-1")
		err := connector.Store(streamID, stream1)
		assert.NoError(t, err)

		// store twice.
		stream2 := mockDataStream(streamID, "name-2")
		err = connector.Store(streamID, stream2)
		assert.NoError(t, err)

		gotStream, ok, err := connector.Get(streamID)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, gotStream, stream2)
	})

	t.Run("Store Delete and Get", func(t *testing.T) {
		stream1 := mockDataStream(streamID, "name-1")
		err := connector.Store(streamID, stream1)
		assert.NoError(t, err)

		err = connector.Delete(streamID)
		assert.NoError(t, err)

		gotStream, ok, err := connector.Get(streamID)
		assert.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, gotStream, nil)
	})

	t.Run("Find", func(t *testing.T) {
		stream1 := mockDataStream(streamID, "name-1")
		err := connector.Store(streamID, stream1)
		assert.NoError(t, err)

		t.Run("ok", func(t *testing.T) {
			ds, err := connector.Find(func(si StreamInfo) bool { return si.ID() == streamID })
			assert.NoError(t, err)
			assert.Contains(t, ds, stream1)
		})

		t.Run("not ok", func(t *testing.T) {
			ds, err := connector.Find(func(si StreamInfo) bool { return si.ID() != streamID })
			assert.NoError(t, err)
			assert.NotContains(t, ds, stream1)
		})
	})

	t.Run("Snapshot", func(t *testing.T) {
		stream1 := mockDataStream("id-1", "name-1")
		err := connector.Store(stream1.ID(), stream1)
		assert.NoError(t, err)

		// store twice.
		stream2 := mockDataStream("id-2", "name-2")
		err = connector.Store(stream2.ID(), stream2)
		assert.NoError(t, err)

		got := connector.Snapshot()
		assert.Equal(t, map[string]string{"id-1": "name-1", "id-2": "name-2"}, got)
	})

	t.Run("Close", func(t *testing.T) {
		connector := NewConnector(context.Background())

		err := connector.Close()
		assert.NoError(t, err)

		err = connector.Close()
		assert.ErrorIs(t, err, ErrConnectorClosed)

		t.Run("Store", func(t *testing.T) {
			stream1 := mockDataStream("id-1", "name-1")
			err := connector.Store(stream1.ID(), stream1)
			assert.ErrorIs(t, err, ErrConnectorClosed)
		})

		t.Run("Get", func(t *testing.T) {
			stream1 := mockDataStream("id-1", "name-1")
			gotStream, ok, err := connector.Get(stream1.ID())
			assert.False(t, ok)
			assert.Equal(t, gotStream, nil)
			assert.ErrorIs(t, err, ErrConnectorClosed)
		})

		t.Run("Delete", func(t *testing.T) {
			err = connector.Delete(streamID)
			assert.ErrorIs(t, err, ErrConnectorClosed)
		})

		t.Run("Find", func(t *testing.T) {
			ds, err := connector.Find(func(si StreamInfo) bool { return si.ID() == streamID })
			assert.ErrorIs(t, err, ErrConnectorClosed)
			assert.Empty(t, ds)
		})

		t.Run("Snapshot", func(t *testing.T) {
			assert.Empty(t, connector.Snapshot())
		})
	})
}

// mockDataStream returns a data stream that only includes an ID and a name.
// This function is used for unit testing purposes.
func mockDataStream(id, name string) DataStream {
	return newDataStream(name, id, StreamType(0), nil, []frame.Tag{0}, nil)
}
