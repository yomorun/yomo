package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestServer New a YoMo Server.
func TestServerNew(t *testing.T) {
}

// TestServerServe serves a YoMo server.
func TestServerServe(t *testing.T) {
	// new
	server := New(testConfig, WithMeshConfURL(testMeshURL))
	assert.NotNil(t, server)
	c := make(chan bool, 0)
	defer close(c)
	// serve
	// go func(c chan bool) {
	go func(t *testing.T) {
		err := server.Serve(fmt.Sprintf("%s:%d", testConfig.Host, testConfig.Port))
		t.Logf("server.Serve err: %v", err)
		assert.NotNil(t, err)
		b := <-c
		t.Log("<-c")
		t.Logf("server.Serve signal: %v", b)
	}(t)
	time.Sleep(1 * time.Second)
	// close
	t.Log("server close")
	err := server.Close()
	assert.Nil(t, err)
	c <- true
	t.Log("c<-")
}

// TestServerServeWithHandler serves a YoMo Server with handler.
// TODO
func TestServerServeWithHandler(t *testing.T) {
}
