package core

const (
	// ClientTypeNone is connection type "None".
	ClientTypeNone ClientType = 0xFF
	// ClientTypeSource is connection type "Source".
	ClientTypeSource ClientType = 0x5F
	// ClientTypeUpstreamZipper is connection type "Upstream Zipper".
	ClientTypeUpstreamZipper ClientType = 0x5E
	// ClientTypeStreamFunction is connection type "Stream Function".
	ClientTypeStreamFunction ClientType = 0x5D
)

// ClientType represents the connection type.
type ClientType byte

func (c ClientType) String() string {
	switch c {
	case ClientTypeSource:
		return "Source"
	case ClientTypeUpstreamZipper:
		return "Upstream Zipper"
	case ClientTypeStreamFunction:
		return "Stream Function"
	default:
		return "None"
	}
}
