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

// Prefix is the prefix for logger.
const (
	ClientLogPrefix     = "\033[36m[core:client]\033[0m "
	ServerLogPrefix     = "\033[32m[core:server]\033[0m "
	ParseFrameLogPrefix = "\033[36m[core:stream_parser]\033[0m "
)

func init() {
	rand.Seed(time.Now().Unix())
}
