package framing

// HeartbeatFrame represents a heartbeat frame.
type HeartbeatFrame struct {
	*frame
}

// NewHeartbeatFrame inits a new HeartbeatFrame.
func NewHeartbeatFrame() *HeartbeatFrame {
	return &HeartbeatFrame{
		frame: newFrame(FrameTypeHeartbeat),
	}
}
