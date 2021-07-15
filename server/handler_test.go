package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/client"
)

func TestServerHandlerListen(t *testing.T) {
	serverHandler := NewServerHandler(testConfig, testMeshURL)
	assert.NotNil(t, serverHandler)
	err := serverHandler.Listen()
	assert.Nil(t, err)

}

// TestServerHandlerRead
// TODO
func testServerHandlerRead(t *testing.T) {
	// new a server handler
	serverHandler := NewServerHandler(testConfig, testMeshURL)
	assert.NotNil(t, serverHandler)
	var err error
	// err := serverHandler.Listen()
	// assert.Nil(t, err)

	// test data
	data := []byte("test")

	// new quic server
	addr := fmt.Sprintf("%s:%d", testConfig.Host, testConfig.Port)
	t.Logf("server listen: %s\n", addr)
	server := New(testConfig, WithMeshConfURL(testMeshURL))
	go func() {
		assert.NotNil(t, server)
		err = server.ServeWithHandler(addr, serverHandler)
		assert.NotNil(t, err)
	}()

	// source
	source := client.New("source", quic.ConnTypeSource)
	source, _ = source.BaseConnect(testConfig.Host, testConfig.Port)
	defer source.Close()
	n, err := source.Write(data)
	assert.Nil(t, err)
	t.Logf("soure write %d bytes: %s", n, data)
	conn := serverHandler.GetConn("source")
	assert.NotNil(t, conn)
	// handler.data
	assert.Equal(t, true, conn.Ready)
	for {
		buf := serverHandler.GetData()
		// frame length: 3
		if len(buf) > 3 {
			actual := buf[3:]
			t.Logf("handler.data: %s", actual)
			assert.Equal(t, data, actual)
			break
		}
	}
	time.Sleep(time.Second)
	server.Close()
}
