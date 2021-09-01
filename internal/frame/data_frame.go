package frame

import (
	"github.com/yomorun/y3"
)

// DataFrame defines the data structure carried with user's data
// when transfering within YoMo
type DataFrame struct {
	metaFrame    *MetaFrame
	payloadFrame *PayloadFrame
}

// NewDataFrame create `DataFrame` with a transactionID string,
// consider change transactionID to UUID type later
func NewDataFrame(transactionID string, issuer string) *DataFrame {
	data := &DataFrame{
		metaFrame: NewMetaFrame(transactionID, issuer),
	}
	return data
}

// Type gets the type of Frame.
func (d *DataFrame) Type() FrameType {
	return TagOfDataFrame
}

// SetCarriage set user's raw data in `DataFrame`
func (d *DataFrame) SetCarriage(sid byte, carriage []byte) {
	d.payloadFrame = NewPayloadFrame(sid).SetCarriage(carriage)
}

// GetCarriage return user's raw data in `DataFrame`
func (d *DataFrame) GetCarriage() []byte {
	return d.payloadFrame.Carriage
}

// TransactionID return transactionID string
func (d *DataFrame) TransactionID() string {
	return d.metaFrame.TransactionID()
}

// Issuer return issuer
func (d *DataFrame) Issuer() string {
	return d.metaFrame.Issuer()
}

// GetDataTagID return the Tag of user's data
func (d *DataFrame) GetDataTagID() byte {
	return d.payloadFrame.Sid
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
