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
	tid       string
	metadata  []byte
	sourceID  string
	broadcast bool
}

// randString genetates a random string.
func randString() string {
	tid, err := gonanoid.New()
	if err != nil {
		tid = strconv.FormatInt(time.Now().UnixMicro(), 10)
	}
	return tid
}

// SetTransactionID set the transaction ID.
func (m *MetaFrame) SetTransactionID(transactionID string) {
	m.tid = transactionID
}

// TransactionID returns transactionID
func (m *MetaFrame) TransactionID() string {
	return m.tid
}

// SetMetadata set the extra info of the application
func (m *MetaFrame) SetMetadata(metadata []byte) {
	m.metadata = metadata
}

// Metadata returns the extra info of the application
func (m *MetaFrame) Metadata() []byte {
	return m.metadata
}

// SetSourceID set the source ID.
func (m *MetaFrame) SetSourceID(sourceID string) {
	m.sourceID = sourceID
}

// SourceID returns source ID
func (m *MetaFrame) SourceID() string {
	return m.sourceID
}

// SetBroadcast set broadcast mode
func (m *MetaFrame) SetBroadcast(enabled bool) {
	m.broadcast = enabled
}

// IsBroadcast returns the broadcast mode is enabled
func (m *MetaFrame) IsBroadcast() bool {
	return m.broadcast
}

// Encode implements Frame.Encode method.
func (m *MetaFrame) Encode() []byte {
	meta := y3.NewNodePacketEncoder(byte(TagOfMetaFrame))
	// transaction ID
	transactionID := y3.NewPrimitivePacketEncoder(byte(TagOfTransactionID))
	transactionID.SetStringValue(m.tid)
	meta.AddPrimitivePacket(transactionID)

	// source ID
	sourceID := y3.NewPrimitivePacketEncoder(byte(TagOfSourceID))
	sourceID.SetStringValue(m.sourceID)
	meta.AddPrimitivePacket(sourceID)

	// metadata
	if m.metadata != nil || len(m.metadata) == 0 {
		metadata := y3.NewPrimitivePacketEncoder(byte(TagOfMetadata))
		metadata.SetBytesValue(m.metadata)
		meta.AddPrimitivePacket(metadata)
	}

	// broadcast mode
	broadcast := y3.NewPrimitivePacketEncoder(byte(TagOfBroadcast))
	broadcast.SetBoolValue(m.broadcast)
	meta.AddPrimitivePacket(broadcast)

	return meta.Encode()
}

// DecodeToMetaFrame decode a MetaFrame instance from given buffer.
func DecodeToMetaFrame(buf []byte, meta *MetaFrame) error {
	nodeBlock := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &nodeBlock)
	if err != nil {
		return err
	}

	for k, v := range nodeBlock.PrimitivePackets {
		switch k {
		case byte(TagOfTransactionID):
			val, err := v.ToUTF8String()
			if err != nil {
				return err
			}
			meta.tid = val
		case byte(TagOfMetadata):
			meta.metadata = v.ToBytes()
		case byte(TagOfSourceID):
			sourceID, err := v.ToUTF8String()
			if err != nil {
				return err
			}
			meta.sourceID = sourceID
		case byte(TagOfBroadcast):
			broadcast, err := v.ToBool()
			if err != nil {
				return err
			}
			meta.broadcast = broadcast
		}
	}

	return nil
}
