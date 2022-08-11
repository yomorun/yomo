package frame

import (
	"github.com/yomorun/y3"
)

// ConnectToFrame creates a new ConnectToFrame
type ConnectToFrame struct {
	addr string
}

// NewConnectToFrame creates a new ConnectToFrame
func NewConnectToFrame(addr string) *ConnectToFrame {
	return &ConnectToFrame{addr: addr}
}

// Type gets the type of Frame.
func (f *ConnectToFrame) Type() Type {
	return TagOfConnectToFrame
}

// Encode to Y3 encoded bytes
func (f *ConnectToFrame) Encode() []byte {
	goaway := y3.NewNodePacketEncoder(byte(f.Type()))
	// message
	msgBlock := y3.NewPrimitivePacketEncoder(byte(TagOfConnectToAddr))
	msgBlock.SetStringValue(f.addr)

	goaway.AddPrimitivePacket(msgBlock)

	return goaway.Encode()
}

// Addr connect to address
func (f *ConnectToFrame) Addr() string {
	return f.addr
}

// DecodeToConnectToFrame decodes Y3 encoded bytes to ConnectToFrame
func DecodeToConnectToFrame(buf []byte) (*ConnectToFrame, error) {
	node := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &node)
	if err != nil {
		return nil, err
	}

	connectTo := &ConnectToFrame{}
	// addr
	if addrBlock, ok := node.PrimitivePackets[byte(TagOfConnectToAddr)]; ok {
		addr, err := addrBlock.ToUTF8String()
		if err != nil {
			return nil, err
		}
		connectTo.addr = addr
	}
	return connectTo, nil
}
