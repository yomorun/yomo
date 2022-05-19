package frame

import (
	"github.com/google/uuid"
	"github.com/yomorun/y3"
)

// MetaFrame is a Y3 encoded bytes, SeqID is a fixed value of TYPE_ID_TRANSACTION.
// used for describes metadata for a DataFrame.
type MetaFrame struct {
	tid     string
	appInfo []byte
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

// SetAppInfo set the extra application information.
func (m *MetaFrame) SetAppInfo(appInfo []byte) {
	m.appInfo = appInfo
}

// AppInfo returns the extra application information.
func (m *MetaFrame) AppInfo() []byte {
	return m.appInfo
}

// Encode implements Frame.Encode method.
func (m *MetaFrame) Encode() []byte {
	meta := y3.NewNodePacketEncoder(byte(TagOfMetaFrame))

	transactionID := y3.NewPrimitivePacketEncoder(byte(TagOfTransactionID))
	transactionID.SetStringValue(m.tid)
	meta.AddPrimitivePacket(transactionID)

	if m.appInfo != nil {
		appInfo := y3.NewPrimitivePacketEncoder(byte(TagOfAppInfo))
		appInfo.SetBytesValue(m.appInfo)
		meta.AddPrimitivePacket(appInfo)
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
			break
		case byte(TagOfAppInfo):
			meta.appInfo = v.ToBytes()
			break
		}
	}

	return meta, nil
}
