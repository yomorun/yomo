package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataFrameEncode(t *testing.T) {
	var userDataTag Tag = 0x15
	d := NewDataFrame()
	d.SetCarriage(userDataTag, []byte("yomo"))
	d.SetBroadcast(true)

	assert.EqualValues(t, "", d.SourceID())

	tidbuf := []byte(d.TransactionID())
	result := []byte{
		0x80 | byte(TagOfDataFrame), byte(len(tidbuf) + 4 + 8 + 5 + 3),
		0x80 | byte(TagOfMetaFrame), byte(len(tidbuf) + 2 + 2 + 3),
		byte(TagOfTransactionID), byte(len(tidbuf))}
	result = append(result, tidbuf...)
	result = append(result, byte(TagOfSourceID), 0x0)
	result = append(result, byte(TagOfBroadcast), 0x1, 0x1)
	result = append(result, 0x80|byte(TagOfPayloadFrame), 0x09,
		0x01, 0x1, 0x15, 0x02, 0x04, 0x79, 0x6F, 0x6D, 0x6F)
	assert.Equal(t, result, d.Encode())
}

func TestDataFrameDecode(t *testing.T) {
	var userDataTag Tag = 0x15
	buf := []byte{
		0x80 | byte(TagOfDataFrame), 0x10 + 3,
		0x80 | byte(TagOfMetaFrame), 0x06 + 3,
		byte(TagOfTransactionID), 0x04, 0x31, 0x32, 0x33, 0x34,
		byte(TagOfBroadcast), 0x01, 0x01,
		0x80 | byte(TagOfPayloadFrame), 0x09,
		0x01, 0x1, 0x15, 0x02, 0x04, 0x79, 0x6F, 0x6D, 0x6F}
	data, err := DecodeToDataFrame(buf)
	assert.NoError(t, err)

	assert.EqualValues(t, 0x15, data.Tag())
	assert.EqualValues(t, "1234", data.TransactionID())
	assert.EqualValues(t, userDataTag, data.GetDataTag())
	assert.EqualValues(t, []byte("yomo"), data.GetCarriage())
	assert.EqualValues(t, true, data.IsBroadcast())
}

func TestDataFramePool(t *testing.T) {
	prev := NewDataFrame()
	prev.SetCarriage(Tag(0x15), []byte("yomo"))
	prev.SetBroadcast(true)

	prev.Clean()
	assert.Equal(t, "", prev.GetMetaFrame().TransactionID())

	curr := NewDataFrame()
	assert.Equal(t, prev, curr)

	// prev and curr has same point.
	assert.Equal(t, curr.GetMetaFrame().TransactionID(), prev.GetMetaFrame().TransactionID())
}
