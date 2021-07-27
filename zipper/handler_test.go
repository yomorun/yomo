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
func TestServerHandlerRead(t *testing.T) {
	// new a server handler
	serverHandler := newQuicHandler(testConfig, testMeshURL)
	assert.NotNil(t, serverHandler)
	var err error

	// close signal
	c := make(chan bool, 0)
	defer close(c)

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
		<-c
	}()

	// define a function to check if the data is received.
	serverHandler.onReceivedData = func(buf []byte) {
		fmt.Println(buf)
		t.Logf("handler.data: %s", buf)
		assert.Equal(t, data, buf)
	}

	// source
	source := client.New("source", quic.ConnTypeSource)
	source, _ = source.BaseConnect(testConfig.Host, testConfig.Port)
	defer source.Close()
	n, err := source.Write(data)
	assert.Nil(t, err)
	t.Logf("source write %d bytes: %s", n, data)
	conn := serverHandler.getConn("source")
	assert.NotNil(t, conn)

	time.Sleep(time.Second)
	server.Close()
	c <- true
}
