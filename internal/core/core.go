package core

const (
	ConnStateDisconnected   ConnState = "Disconnected"
	ConnStateConnecting     ConnState = "Connecting"
	ConnStateConnected      ConnState = "Connected"
	ConnStateAuthenticating ConnState = "Authenticating"
	ConnStateAccepted       ConnState = "Accepted"
	ConnStateRejected       ConnState = "Rejected"
	ConnStatePing           ConnState = "Ping"
	ConnStatePong           ConnState = "Pong"
	ConnStateTransportData  ConnState = "TransportData"
)
