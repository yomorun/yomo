package core

import (
	"math/rand"
	"sync"
	"time"
)

var (
	once sync.Once
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

// CloseReason represents the reason of the closed connection.
type CloseReason = string

// CloseReason represents the reason of the closed connection.
const (
	CloseReasonUnknownError     CloseReason = "Unknown Error"
	CloseReasonIllegalProtocol  CloseReason = "Illegal Protocol"
	CloseReasonKeepAliveTimeout CloseReason = "KeepAlive Timeout"
	CloseReasonLocalClosed      CloseReason = "Local Closed"
	CloseReasonPeerClosed       CloseReason = "Peer Closed"
	CloseReasonReceivedRejected CloseReason = "Received Rejected"
	CloseReasonReceivedGoaway   CloseReason = "Received Goaway"
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
