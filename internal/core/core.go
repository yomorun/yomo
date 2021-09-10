package core

import "sync"

var (
	once sync.Once
)

const (
	// ConnState
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
	// Logger prefix
	ClientLogPrefix     = "\033[36m[core:client]\033[0m "
	ServerLogPrefix     = "\033[32m[core:server]\033[0m "
	ParseFrameLogPrefix = "\033[36m[core:stream_parser]\033[0m "
)
