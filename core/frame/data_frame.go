package frame

import (
	"github.com/yomorun/y3"
)

// DataFrame defines the data structure carried with user's data
// transferring within YoMo
type DataFrame struct {
	metaFrame    *MetaFrame
	payloadFrame *PayloadFrame
}

// NewDataFrame create `DataFrame` with a transactionID string,
// consider change transactionID to UUID type later
func NewDataFrame() *DataFrame {
	data := &DataFrame{
		metaFrame: NewMetaFrame(),
	}
	return data
}

// Type gets the type of Frame.
func (d *DataFrame) Type() Type {
	return TagOfDataFrame
}

// Tag return the tag of carriage data.
func (d *DataFrame) Tag() byte {
	return d.payloadFrame.Tag
}

// SetCarriage set user's raw data in `DataFrame`
func (d *DataFrame) SetCarriage(tag byte, carriage []byte) {
	d.payloadFrame = NewPayloadFrame(tag).SetCarriage(carriage)
}

// GetCarriage return user's raw data in `DataFrame`
func (d *DataFrame) GetCarriage() []byte {
	return d.payloadFrame.Carriage
}

// TransactionID return transactionID string
func (d *DataFrame) TransactionID() string {
	return d.metaFrame.TransactionID()
}

// SetTransactionID set transactionID string
func (d *DataFrame) SetTransactionID(transactionID string) {
	d.metaFrame.SetTransactionID(transactionID)
}

// GetMetaFrame return MetaFrame.
func (d *DataFrame) GetMetaFrame() *MetaFrame {
	return d.metaFrame
}

// GetDataTag return the Tag of user's data
func (d *DataFrame) GetDataTag() byte {
	return d.payloadFrame.Tag
}

// SetSourceID set the source ID.
func (d *DataFrame) SetSourceID(sourceID string) {
	d.metaFrame.SetSourceID(sourceID)
}

// SourceID returns source ID
func (d *DataFrame) SourceID() string {
	return d.metaFrame.SourceID()
}

// Encode return Y3 encoded bytes of `DataFrame`
func (d *DataFrame) Encode() []byte {
	data := y3.NewNodePacketEncoder(byte(d.Type()))
	// MetaFrame
	data.AddBytes(d.metaFrame.Encode())
	// PayloadFrame
	data.AddBytes(d.payloadFrame.Encode())

	return data.Encode()
}

// DecodeToDataFrame decode Y3 encoded bytes to `DataFrame`
func DecodeToDataFrame(buf []byte) (*DataFrame, error) {
	packet := y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, &packet)
	if err != nil {
		return nil, err
	}

	data := &DataFrame{}

	if metaBlock, ok := packet.NodePackets[byte(TagOfMetaFrame)]; ok {
		meta, err := DecodeToMetaFrame(metaBlock.GetRawBytes())
		if err != nil {
			return nil, err
		}
		data.metaFrame = meta
	}

	if payloadBlock, ok := packet.NodePackets[byte(TagOfPayloadFrame)]; ok {
		payload, err := DecodeToPayloadFrame(payloadBlock.GetRawBytes())
		if err != nil {
			return nil, err
		}
		data.payloadFrame = payload
	}

	return data, nil
}
