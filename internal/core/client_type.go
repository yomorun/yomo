package core

const (
	// ConnTypeNone is connection type "None".
	ConnTypeNone ConnectionType = 0xFF
	// ConnTypeNone is connection type "Source".
	ConnTypeSource ConnectionType = 0x5F
	// ConnTypeNone is connection type "Upstream Zipper".
	ConnTypeZipperSender ConnectionType = 0x5E
	// ConnTypeNone is connection type "Stream Function".
	ConnTypeStreamFunction ConnectionType = 0x5D
)

// ConnectionType represents the connection type.
type ConnectionType byte

func (c ConnectionType) String() string {
	switch c {
	case ConnTypeSource:
		return "Source"
	case ConnTypeZipperSender:
		return "Zipper Sender"
	case ConnTypeStreamFunction:
		return "Stream Function"
	default:
		return "None"
	}
}
