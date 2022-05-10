package frame

import (
	"github.com/google/uuid"
	"github.com/yomorun/y3"
)

// MetaFrame is a Y3 encoded bytes, SeqID is a fixed value of TYPE_ID_TRANSACTION.
// used for describes metadata for a DataFrame.
type MetaFrame struct {
	tid      string
	extFrame ExtFrame
}

// NewMetaFrame creates a new MetaFrame instance.
func NewMetaFrame() *MetaFrame {
	var extFrame ExtFrame
	if extFrameBuilder != nil {
		extFrame = extFrameBuilder.NewExtFrame()
	}
	return &MetaFrame{
		tid:      uuid.NewString(),
		extFrame: extFrame,
	}
}

// SetTransactinID set the transaction ID.
func (m *MetaFrame) SetTransactionID(transactionID string) {
	m.tid = transactionID
}

// TransactionID returns transactionID
func (m *MetaFrame) TransactionID() string {
	return m.tid
}

// SetExtFrame set the extFrame.
func (m *MetaFrame) SetExtFrame(extFrame ExtFrame) {
	m.extFrame = extFrame
}

// GetExtFrame returns extFrame
func (m *MetaFrame) GetExtFrame() ExtFrame {
	return m.extFrame
}

// Encode implements Frame.Encode method.
func (m *MetaFrame) Encode() []byte {
	meta := y3.NewNodePacketEncoder(byte(TagOfMetaFrame))

	transactionID := y3.NewPrimitivePacketEncoder(byte(TagOfTransactionID))
	transactionID.SetStringValue(m.tid)
	meta.AddPrimitivePacket(transactionID)

	if m.extFrame != nil {
		ext := y3.NewPrimitivePacketEncoder(byte(TagOfExtFrame))
		ext.SetBytesValue(m.extFrame.Encode())
		meta.AddPrimitivePacket(ext)
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

	meta := &MetaFrame{}
	for k, v := range nodeBlock.PrimitivePackets {
		switch k {
		case byte(TagOfTransactionID):
			val, err := v.ToUTF8String()
			if err != nil {
				return nil, err
			}
			meta.tid = val
		case byte(TagOfExtFrame):
			if extFrameBuilder != nil {
				extFrame, err := extFrameBuilder.DecodeToExtFrame(v.ToBytes())
				if err != nil {
					return nil, err
				}
				meta.extFrame = extFrame
			}
		}
	}

	return meta, nil
}
