package frame

import (
	"strconv"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/yomorun/y3"
)

// MetaFrame is a Y3 encoded bytes, SeqID is a fixed value of TYPE_ID_TRANSACTION.
// used for describes metadata for a DataFrame.
type MetaFrame struct {
	tid  string
	data []byte
}

// NewMetaFrame creates a new MetaFrame instance.
func NewMetaFrame() *MetaFrame {
	tid, err := gonanoid.New()
	if err != nil {
		tid = strconv.FormatInt(time.Now().UnixMicro(), 10)
	}
	return &MetaFrame{tid: tid}
}

// SetTransactinID set the transaction ID.
func (m *MetaFrame) SetTransactionID(transactionID string) {
	m.tid = transactionID
}

// TransactionID returns transactionID
func (m *MetaFrame) TransactionID() string {
	return m.tid
}

// SetMetaData set the extra info of the application
func (m *MetaFrame) SetMetaData(data []byte) {
	m.data = data
}

// MetaData returns the extra info of the application
func (m *MetaFrame) MetaData() []byte {
	return m.data
}

// Encode implements Frame.Encode method.
func (m *MetaFrame) Encode() []byte {
	meta := y3.NewNodePacketEncoder(byte(TagOfMetaFrame))

	transactionID := y3.NewPrimitivePacketEncoder(byte(TagOfTransactionID))
	transactionID.SetStringValue(m.tid)
	meta.AddPrimitivePacket(transactionID)

	if m.data != nil {
		data := y3.NewPrimitivePacketEncoder(byte(TagOfMetaData))
		data.SetBytesValue(m.data)
		meta.AddPrimitivePacket(data)
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
		case byte(TagOfMetaData):
			meta.data = v.ToBytes()
			break
		}
	}

	return meta, nil
}
