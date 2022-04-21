package core

import (
	"math/rand"
	"sync"
	"time"
)

var (
	once sync.Once
)

// ConnState represents the state of a connection.
const (
	ConnStateReady          ConnState = "Ready"
	ConnStateDisconnected   ConnState = "Disconnected"
	ConnStateConnecting     ConnState = "Connecting"
	ConnStateConnected      ConnState = "Connected"
	ConnStateAuthenticating ConnState = "Authenticating"
	ConnStateAccepted       ConnState = "Accepted"
	ConnStateRejected       ConnState = "Rejected"
	ConnStatePing           ConnState = "Ping"
	ConnStatePong           ConnState = "Pong"
	ConnStateTransportData  ConnState = "TransportData"
	ConnStateAborted        ConnState = "Aborted"
	ConnStateClosed         ConnState = "Closed" // close connection by server
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
