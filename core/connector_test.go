package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
)

func TestConnector(t *testing.T) {
	connector := NewConnector(context.Background())

	connID := "id-1"

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

	t.Run("Store Reove and Get", func(t *testing.T) {
		conn1 := mockConn(connID, "name-1")
		err := connector.Store(connID, conn1)
		assert.NoError(t, err)

		err = connector.Reove(connID)
		assert.NoError(t, err)

		gotStream, ok, err := connector.Get(connID)
		assert.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, gotStream, nil)
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
		conn1 := mockConn("id-1", "name-1")
		err := connector.Store(conn1.ID(), conn1)
		assert.NoError(t, err)

		// store twice.
		conn2 := mockConn("id-2", "name-2")
		err = connector.Store(conn2.ID(), conn2)
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
			conn1 := mockConn("id-1", "name-1")
			err := connector.Store(conn1.ID(), conn1)
			assert.ErrorIs(t, err, ErrConnectorClosed)
		})

		t.Run("Get", func(t *testing.T) {
			conn1 := mockConn("id-1", "name-1")
			gotStream, ok, err := connector.Get(conn1.ID())
			assert.False(t, ok)
			assert.Equal(t, gotStream, nil)
			assert.ErrorIs(t, err, ErrConnectorClosed)
		})

		t.Run("Delete", func(t *testing.T) {
			err = connector.Reove(connID)
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
func mockConn(id, name string) Connection {
	return newConnection(name, id, ClientType(0), nil, []frame.Tag{0}, nil, nil)
}
