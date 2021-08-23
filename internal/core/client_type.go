package core

const (
	ConnTypeNone           ConnectionType = 0xFF
	ConnTypeSource         ConnectionType = 0x5F
	ConnTypeZipperSender   ConnectionType = 0x5E
	ConnTypeStreamFunction ConnectionType = 0x5D
)

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
