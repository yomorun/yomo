package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestZipper New a YoMo Zipper.
func TestZipperNew(t *testing.T) {
	server := New(testConfig, WithMeshConfURL(testMeshURL))
	assert.NotNil(t, server)
}

// TestZipperServe serves a YoMo Zipper.
func TestZipperServe(t *testing.T) {
	// new
	server := New(testConfig, WithMeshConfURL(testMeshURL))
	assert.NotNil(t, server)
	c := make(chan bool, 0)
	defer close(c)
	// serve
	go func(t *testing.T) {
		err := server.Serve(fmt.Sprintf("%s:%d", testConfig.Host, testConfig.Port+1))
		t.Logf("server.Serve err: %v", err)
		assert.NotNil(t, err)
		<-c
	}(t)
	time.Sleep(1 * time.Second)
	// close
	t.Log("server close")
	err := server.Close()
	assert.Nil(t, err)
	c <- true
}
