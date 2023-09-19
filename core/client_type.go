package core

// ClientType is the type of client.
type ClientType byte

const (
	// ClientTypeSource is client type "Source".
	// "Source" type client sends data to "Stream Function" stream generally.
	ClientTypeSource ClientType = 0x5F

	// ClientTypeUpstreamZipper is client type "Upstream Zipper".
	// "Upstream Zipper" type client sends data from "Source" to other zipper node.
	// With "Upstream Zipper", the yomo can run in mesh mode.
	ClientTypeUpstreamZipper ClientType = 0x5E

	// ClientTypeStreamFunction is client type "Stream Function".
	// "Stream Function" handles data from source.
	ClientTypeStreamFunction ClientType = 0x5D
)

var clientTypeStringMap = map[ClientType]string{
	ClientTypeSource:         "Source",
	ClientTypeUpstreamZipper: "UpstreamZipper",
	ClientTypeStreamFunction: "StreamFunction",
}

// String returns string for ClientType.
func (c ClientType) String() string {
	str, ok := clientTypeStringMap[c]
	if !ok {
		return "Unknown"
	}
	return str
}
