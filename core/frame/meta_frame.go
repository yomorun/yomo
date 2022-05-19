package frame

import (
	"github.com/google/uuid"
	"github.com/yomorun/y3"
)

// MetaFrame is a Y3 encoded bytes, SeqID is a fixed value of TYPE_ID_TRANSACTION.
// used for describes metadata for a DataFrame.
type MetaFrame struct {
	tid     string
	extInfo []byte
}

// NewMetaFrame creates a new MetaFrame instance.
func NewMetaFrame() *MetaFrame {
	return &MetaFrame{
		tid: uuid.NewString(),
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

// SetExtInfo set the extended information.
func (m *MetaFrame) SetExtInfo(extInfo []byte) {
	m.extInfo = extInfo
}

// TransactionID returns the extended information
func (m *MetaFrame) ExtInfo() []byte {
	return m.extInfo
}

// Encode implements Frame.Encode method.
func (m *MetaFrame) Encode() []byte {
	meta := y3.NewNodePacketEncoder(byte(TagOfMetaFrame))

	transactionID := y3.NewPrimitivePacketEncoder(byte(TagOfTransactionID))
	transactionID.SetStringValue(m.tid)
	meta.AddPrimitivePacket(transactionID)

	extInfo := y3.NewPrimitivePacketEncoder(byte(TagOfExtInfo))
	extInfo.SetBytesValue(m.extInfo)
	meta.AddPrimitivePacket(extInfo)

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
			break
		case byte(TagOfExtInfo):
			meta.extInfo = v.ToBytes()
			break
		}
	}

	return meta, nil
}
