package frame

type ConnectionCloseFrame struct {
	Message  string
	ClientID string
}

// Type returns the type of ConnectionFrame.
func (f *ConnectionCloseFrame) Type() Type     { return TagOfDataFrame }
func (f *ConnectionCloseFrame) Encode() []byte { return []byte{} }

func DecodeToConnectionCloseFrame(b []byte) (*ConnectionCloseFrame, error) {
	return &ConnectionCloseFrame{}, nil
}
