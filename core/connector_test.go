package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
)

func TestConnector(t *testing.T) {
	connector := NewConnector(context.Background())

	connID := uint64(32)

	t.Run("Store and Get", func(t *testing.T) {
		conn1 := mockConn(connID, "name-1")
		err := connector.Store(connID, conn1)
		assert.NoError(t, err)

		// store twice.
		conn2 := mockConn(connID, "name-2")
		err = connector.Store(connID, conn2)
		assert.NoError(t, err)

		gotStream, ok, err := connector.Get(connID)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, gotStream, conn2)
	})

	t.Run("Store Remove and Get", func(t *testing.T) {
		conn1 := mockConn(connID, "name-1")
		err := connector.Store(connID, conn1)
		assert.NoError(t, err)

		err = connector.Remove(connID)
		assert.NoError(t, err)

		_, ok, err := connector.Get(connID)
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("Find", func(t *testing.T) {
		conn1 := mockConn(connID, "name-1")
		err := connector.Store(connID, conn1)
		assert.NoError(t, err)

		t.Run("ok", func(t *testing.T) {
			ds, err := connector.Find(func(si ConnectionInfo) bool { return si.ID() == connID })
			assert.NoError(t, err)
			assert.Contains(t, ds, conn1)
		})

		t.Run("not ok", func(t *testing.T) {
			ds, err := connector.Find(func(si ConnectionInfo) bool { return si.ID() != connID })
			assert.NoError(t, err)
			assert.NotContains(t, ds, conn1)
		})
	})

	t.Run("Snapshot", func(t *testing.T) {
		conn1 := mockConn(uint64(1), "name-1")
		err := connector.Store(conn1.ID(), conn1)
		assert.NoError(t, err)

		// store twice.
		conn2 := mockConn(uint64(2), "name-2")
		err = connector.Store(conn2.ID(), conn2)
		assert.NoError(t, err)

		got := connector.Snapshot()
		assert.Equal(t, map[string]string{"1": "name-1", "2": "name-2", "32": "name-1"}, got)
	})

	t.Run("Close", func(t *testing.T) {
		connector := NewConnector(context.Background())

		err := connector.Close()
		assert.NoError(t, err)

		err = connector.Close()
		assert.ErrorIs(t, err, ErrConnectorClosed)

		t.Run("Store", func(t *testing.T) {
			conn1 := mockConn(uint64(1), "name-1")
			err := connector.Store(conn1.ID(), conn1)
			assert.ErrorIs(t, err, ErrConnectorClosed)
		})

		t.Run("Get", func(t *testing.T) {
			conn1 := mockConn(uint64(1), "name-1")
			_, ok, err := connector.Get(conn1.ID())
			assert.False(t, ok)
			assert.ErrorIs(t, err, ErrConnectorClosed)
		})

		t.Run("Delete", func(t *testing.T) {
			err = connector.Remove(connID)
			assert.ErrorIs(t, err, ErrConnectorClosed)
		})

		t.Run("Find", func(t *testing.T) {
			ds, err := connector.Find(func(si ConnectionInfo) bool { return si.ID() == connID })
			assert.ErrorIs(t, err, ErrConnectorClosed)
			assert.Empty(t, ds)
		})

		t.Run("Snapshot", func(t *testing.T) {
			assert.Empty(t, connector.Snapshot())
		})
	})
}

// mockConn returns a connection that only includes an ID and a name.
// This function is used for unit testing purposes.
func mockConn(id uint64, name string) *Connection {
	return newConnection(id, name, "mock-id", ClientType(0), nil, []frame.Tag{0}, nil, ylog.Default())
}
