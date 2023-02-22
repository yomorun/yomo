package frame

type ConnectionFrame struct {
	// Name is the name of connection.
	Name string
	// ClientID represents client ID.
	ClientID string
	// ClientType represents client type (source, sfn or upStream).
	ClientType byte
	// ObserveDataTags are the client data tag list.
	ObserveDataTags []Tag
	// Metadata holds Connection metadata.
	Metadata []byte
}

// Type returns the type of ConnectionFrame.
func (f *ConnectionFrame) Type() Type     { return TagOfDataFrame }
func (f *ConnectionFrame) Encode() []byte { return []byte{} }

func DecodeToConnectionFrame(b []byte) (*ConnectionFrame, error) { return &ConnectionFrame{}, nil }
