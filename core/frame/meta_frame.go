package frame

import (
	"fmt"
	"strconv"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/yomorun/y3"
)

// MetaFrame is a Y3 encoded bytes, SeqID is a fixed value of TYPE_ID_TRANSACTION.
// used for describes metadata for a DataFrame.
type MetaFrame struct {
	tid      string
	metadata []byte
	sourceID string
	dispatch Dispatch
}

// NewMetaFrame creates a new MetaFrame instance.
func NewMetaFrame() *MetaFrame {
	tid, err := gonanoid.New()
	if err != nil {
		tid = strconv.FormatInt(time.Now().Unix(), 10) // todo: UnixMicro since go 1.17
	}
	return &MetaFrame{tid: tid, dispatch: DispatchDirected}
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

// SetDispatch set dispatch mode
func (m *MetaFrame) SetDispatch(mode Dispatch) {
	m.dispatch = mode
}

// Dispatch get dispatch mode
func (m *MetaFrame) Dispatch() Dispatch {
	return m.dispatch
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
	if m.metadata != nil {
		metadata := y3.NewPrimitivePacketEncoder(byte(TagOfMetadata))
		metadata.SetBytesValue(m.metadata)
		meta.AddPrimitivePacket(metadata)
	}

	// dispatch mode
	dispatch := y3.NewPrimitivePacketEncoder(byte(TagOfDispatch))
	dispatch.SetBytesValue([]byte{m.dispatch})
	meta.AddPrimitivePacket(dispatch)

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
		case byte(TagOfMetadata):
			meta.metadata = v.ToBytes()
		case byte(TagOfSourceID):
			sourceID, err := v.ToUTF8String()
			if err != nil {
				return nil, err
			}
			meta.sourceID = sourceID
		case byte(TagOfDispatch):
			dispatch := v.ToBytes()
			fmt.Printf("dispatch: %v", dispatch)
			if len(dispatch) < 1 {
				meta.dispatch = DispatchDirected
			}
			meta.dispatch = dispatch[0]
		}
	}

	return meta, nil
}
