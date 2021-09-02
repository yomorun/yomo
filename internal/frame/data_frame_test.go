package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataFrameEncode(t *testing.T) {
	var userDataTag byte = 0x15
	d := NewDataFrame("1234", "issuer")
	d.SetCarriage(userDataTag, []byte("yomo"))
	assert.Equal(t, []byte{
		0x80 | byte(TagOfDataFrame), 0x10,
		0x80 | byte(TagOfMetaFrame), 0x06,
		byte(TagOfTransactionID), 0x04, 0x31, 0x32, 0x33, 0x34,
		0x80 | byte(TagOfPayloadFrame), 0x06,
		userDataTag, 0x04, 0x79, 0x6F, 0x6D, 0x6F}, d.Encode())
}

func TestDataFrameDecode(t *testing.T) {
	var userDataTag byte = 0x15
	buf := []byte{
		0x80 | byte(TagOfDataFrame), 0x10,
		0x80 | byte(TagOfMetaFrame), 0x06,
		byte(TagOfTransactionID), 0x04, 0x31, 0x32, 0x33, 0x34,
		0x80 | byte(TagOfPayloadFrame), 0x06,
		userDataTag, 0x04, 0x79, 0x6F, 0x6D, 0x6F}
	data, err := DecodeToDataFrame(buf)
	assert.NoError(t, err)
	assert.EqualValues(t, "1234", data.TransactionID())
	assert.EqualValues(t, userDataTag, data.GetDataTagID())
	assert.EqualValues(t, []byte("yomo"), data.GetCarriage())
}
