package zipper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewSender setups the client of Upstream YoMo-Zipper (formerly Zipper-Sender).
func TestNewSender(t *testing.T) {
	sender := NewSender("sender")
	assert.NotNil(t, sender)
}

// TestSenderConnect to downstream YoMo-Zipper-Receiver in edge-mesh.
func TestSenderConnect(t *testing.T) {
	// new server/serve
	server := New(testConfig, WithMeshConfURL(testMeshURL))
	assert.NotNil(t, server)
	go func() {
		server.Serve(fmt.Sprintf("%s:%d", testConfig.Host, testConfig.Port))
	}()
	// new sender
	sender := NewSender("sender")
	assert.NotNil(t, sender)
	// sender connect to server
	sender, err := sender.Connect(testConfig.Host, testConfig.Port)
	assert.Nil(t, err)
	assert.NotNil(t, sender)
	// close server
	server.Close()

}
