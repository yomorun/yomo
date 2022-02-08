package frame

import (
	"time"

	"github.com/yomorun/y3"
)

const (
	// LoadBalanceRandomPick means Zipper will choose SFN instance by a load balance strategy.
	LoadBalanceRandomPick byte = 0x00
	// LoadBalanceBindInstance means data will be sent to the specific SFN according to instanceID.
	LoadBalanceBindInstance byte = 0x01
	// LoadBalanceBroadcast means data will be sent to all SFN instances.
	LoadBalanceBroadcast byte = 0x02
)

// MetaFrame is a Y3 encoded bytes, used for describing metadata of a DataFrame.
type MetaFrame struct {
	timestamp int64
	// LBType is load balance type.
	LBType byte
	// ToInstanceID is the downstream SFN instance id, only used when LBType equals LoadBalanceBindInstance.
	ToInstanceID string
}

// NewMetaFrame creates a new MetaFrame instance.
func NewMetaFrame() *MetaFrame {
	return &MetaFrame{
		timestamp: time.Now().UnixNano() / 1000, // go 1.17: UnixMicro()
		LBType:    LoadBalanceRandomPick,
	}
}

// Timestamp returns Unix time in microsecends.
func (m *MetaFrame) Timestamp() int64 {
	return m.timestamp
}

// Encode implements Frame.Encode method.
func (m *MetaFrame) Encode() []byte {
	meta := y3.NewNodePacketEncoder(byte(TagOfMetaFrame))

	enc := y3.NewPrimitivePacketEncoder(byte(TagOfTimestamp))
	enc.SetInt64Value(m.timestamp)
	meta.AddPrimitivePacket(enc)

	if m.LBType != LoadBalanceRandomPick {
		enc = y3.NewPrimitivePacketEncoder(byte(TagOfLBType))
		enc.SetBytesValue([]byte{m.LBType})
		meta.AddPrimitivePacket(enc)

		if m.LBType == LoadBalanceBindInstance {
			instanceID := y3.NewPrimitivePacketEncoder(byte(TagOfToInstanceID))
			instanceID.SetStringValue(m.ToInstanceID)
			meta.AddPrimitivePacket(instanceID)
		}
	}

	return meta.Encode()
}

// DecodeToMetaFrame decode a MetaFrame instance from given buffer.
func DecodeToMetaFrame(buf []byte) (*MetaFrame, error) {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &nodeBlock)
	if err != nil {
		return nil, err
	}

	meta := &MetaFrame{LBType: LoadBalanceRandomPick}
	for k, v := range nodeBlock.PrimitivePackets {
		switch k {
		case byte(TagOfTimestamp):
			val, _ := v.ToInt64()
			meta.timestamp = val
		case byte(TagOfLBType):
			meta.LBType = v.ToBytes()[0]
		case byte(TagOfToInstanceID):
			val, _ := v.ToUTF8String()
			meta.ToInstanceID = val
		}
	}

	return meta, nil
}
