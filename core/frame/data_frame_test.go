package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataFrameEncode(t *testing.T) {
	d1 := NewDataFrame()
	d1.SetCarriage(0x15, []byte("yomo"))
	d1.GetMetaFrame().LBType = LoadBalanceBindInstance
	d1.GetMetaFrame().ToInstanceID = "abc"
	buf := d1.Encode()
	d2, err := DecodeToDataFrame(buf)
	assert.NoError(t, err)
	assert.EqualValues(t, d1.GetDataTag(), d2.GetDataTag())
	assert.EqualValues(t, d1.GetCarriage(), d2.GetCarriage())
	assert.EqualValues(t, d1.GetMetaFrame().Timestamp(), d2.GetMetaFrame().Timestamp())
	assert.EqualValues(t, d1.GetMetaFrame().LBType, d2.GetMetaFrame().LBType)
	assert.EqualValues(t, d1.GetMetaFrame().ToInstanceID, d2.GetMetaFrame().ToInstanceID)
}
