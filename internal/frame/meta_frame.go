package frame

import (
	"github.com/yomorun/y3"
)

// MetaFrame defines the data structure of meta data in a `DataFrame`
type MetaFrame struct {
	// transactionID is the unique identifier of the transaction
	transactionID string
	// issuer issue this transaction
	issuer string
}

// NewMetaFrame creates a new MetaFrame with a given transactionID
func NewMetaFrame(tid string, issuer string) *MetaFrame {
	return &MetaFrame{
		transactionID: tid,
		issuer:        issuer,
	}
}

// TransactionID returns the transactionID of the MetaFrame
func (m *MetaFrame) TransactionID() string {
	return m.transactionID
}

// Issuer returns the issuer of the MetaFrame
func (m *MetaFrame) Issuer() string {
	return m.issuer
}

// Encode returns Y3 encoded bytes of the MetaFrame
func (m *MetaFrame) Encode() []byte {
	metaNode := y3.NewNodePacketEncoder(byte(TagOfMetaFrame))
	// TransactionID string
	tidPacket := y3.NewPrimitivePacketEncoder(byte(TagOfTransactionID))
	tidPacket.SetStringValue(m.transactionID)
	// add TransactionID to MetaFrame
	metaNode.AddPrimitivePacket(tidPacket)

	return metaNode.Encode()
}

// DecodeToMetaFrame decodes Y3 encoded bytes to a MetaFrame
func DecodeToMetaFrame(buf []byte) (*MetaFrame, error) {
	packet := &y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, packet)

	if err != nil {
		return nil, err
	}

	var tid string
	if s, ok := packet.PrimitivePackets[byte(TagOfTransactionID)]; ok {
		tid, err = s.ToUTF8String()
		if err != nil {
			return nil, err
		}
	}

	var issuer string
	if s, ok := packet.PrimitivePackets[byte(TagOfIssuer)]; ok {
		issuer, err = s.ToUTF8String()
		if err != nil {
			return nil, err
		}
	}
	meta := &MetaFrame{
		transactionID: tid,
		issuer:        issuer,
	}
	return meta, nil
}
