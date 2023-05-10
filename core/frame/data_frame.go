package frame

import (
	"fmt"

	"github.com/yomorun/y3"
)

// DataFrame defines the data structure carried with user's data
// transferring within YoMo
type DataFrame struct {
	metaFrame    *MetaFrame
	payloadFrame *PayloadFrame
}

// String method implements fmt %v
func (d DataFrame) String() string {
	data := d.GetCarriage()
	length := len(data)
	if length > debugFrameSize {
		data = data[:debugFrameSize]
	}
	return fmt.Sprintf("tid=%s | tag=%#x | source=%s | data[%d]=%#x", d.metaFrame.tid, d.Tag(), d.SourceID(), length, data)
}

// NewDataFrame create `DataFrame` with a transactionID string,
// consider change transactionID to UUID type later
func NewDataFrame() (data *DataFrame) {
	data = newDataFrame()
	data.metaFrame.tid = randString()
	return
}

func newDataFrame() (data *DataFrame) {
	data = new(DataFrame)
	data.metaFrame = new(MetaFrame)
	data.payloadFrame = new(PayloadFrame)

	return
}

// Type gets the type of Frame.
func (d *DataFrame) Type() Type {
	return TagOfDataFrame
}

// Tag return the tag of carriage data.
func (d *DataFrame) Tag() Tag {
	return d.payloadFrame.Tag
}

// SetCarriage set user's raw data in `DataFrame`
func (d *DataFrame) SetCarriage(tag Tag, carriage []byte) {
	d.payloadFrame = &PayloadFrame{
		Tag:      tag,
		Carriage: carriage,
	}
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
func (d *DataFrame) GetDataTag() Tag {
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

// SetBroadcast set broadcast mode
func (d *DataFrame) SetBroadcast(enabled bool) {
	d.metaFrame.SetBroadcast(enabled)
}

// IsBroadcast returns the broadcast mode is enabled
func (d *DataFrame) IsBroadcast() bool {
	return d.metaFrame.IsBroadcast()
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

	data := newDataFrame()

	if metaBlock, ok := packet.NodePackets[byte(TagOfMetaFrame)]; ok {
		err := DecodeToMetaFrame(metaBlock.GetRawBytes(), data.metaFrame)
		if err != nil {
			return nil, err
		}
	}

	if payloadBlock, ok := packet.NodePackets[byte(TagOfPayloadFrame)]; ok {
		err := DecodeToPayloadFrame(payloadBlock.GetRawBytes(), data.payloadFrame)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}
