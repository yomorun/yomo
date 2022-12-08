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

// 必须包含时间戳，日志的基础属性
// host (对用户应该不可见，但是是一个很重要的 information)
// 基础的 log msg，日志为什么打印
// 打印日志的 yomo 组件 (是 server 还是 client 打印的日志，区分打印的信息的对象，因为相同的信息可能在 server 和 client 都打印)
// client 信息, clientName, clientType, clientID, local addr ( 和 host 内容重复，也是和 host 相同问题)
// server 信息，serverName, remote addr(对用户是否可见)
// error error 信息
// 传输的Frame Type 和 Frame 的 debug 内容，dataTag 和 tid，（MetaFrame）拥有 tid 的日志都应该是 info level
// 其他的日志信息，如认证方式
