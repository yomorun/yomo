package framing

// HeartbeatFrame represents a HEARTBEAT frame.
type HeartbeatFrame struct {
	*frame
}

// NewHeartbeatFrame inits a new HEARTBEAT frame.
func NewHeartbeatFrame(opts ...Option) *HeartbeatFrame {
	return &HeartbeatFrame{
		frame: newFrame(FrameTypeHeartbeat, opts...),
	}
}
