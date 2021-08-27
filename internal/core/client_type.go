package core

const (
	// ConnTypeNone is connection type "None".
	ConnTypeNone ConnectionType = 0xFF
	// ConnTypeSource is connection type "Source".
	ConnTypeSource ConnectionType = 0x5F
	// ConnTypeUpstreamZipper is connection type "Upstream Zipper".
	ConnTypeUpstreamZipper ConnectionType = 0x5E
	// ConnTypeStreamFunction is connection type "Stream Function".
	ConnTypeStreamFunction ConnectionType = 0x5D
)

// ConnectionType represents the connection type.
type ConnectionType byte

func (c ConnectionType) String() string {
	switch c {
	case ConnTypeSource:
		return "Source"
	case ConnTypeUpstreamZipper:
		return "Upstream Zipper"
	case ConnTypeStreamFunction:
		return "Stream Function"
	default:
		return "None"
	}
}
