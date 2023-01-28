package core

import (
	"math/rand"
	"time"
)

// ConnState represents the state of the connection.
type ConnState = string

// ConnState represents the state of a connection.
const (
	ConnStateReady        ConnState = "Ready"
	ConnStateDisconnected ConnState = "Disconnected"
	ConnStateConnecting   ConnState = "Connecting"
	ConnStateConnected    ConnState = "Connected"
	ConnStateClosed       ConnState = "Closed"
)

func init() {
	rand.Seed(time.Now().Unix())
}
