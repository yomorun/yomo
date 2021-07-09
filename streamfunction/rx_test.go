package streamfunction

import (
	"context"
	"testing"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/rx"
	"go.uber.org/goleak"
)

var impl = newStreamFnRx()

func Test_Append_New_Data_To_Raw_Stream(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawStream := rx.MockStream([]byte{10})
	fnStream := rx.MockStreamWithInterval(time.Millisecond, []byte{01})
	result := impl.appendNewDataToRawStream(rawStream, fnStream)

	rxgo.Assert(ctx, t, result, rxgo.HasItem([]byte{10, 01}), rxgo.HasNoError())
}

func Test_Append_New_Data_To_Raw_Stream_When_Multi_Sources(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawStream := rx.MockStream([]byte{10}, []byte{11})
	fnStream := rx.MockStreamWithInterval(time.Millisecond, []byte{01})
	result := impl.appendNewDataToRawStream(rawStream, fnStream)

	rxgo.Assert(ctx, t, result, rxgo.HasItem([]byte{10, 11, 01}), rxgo.HasNoError())
}

func Test_Skip_New_Data_When_No_Bytes(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawStream := rx.MockStream([]byte{10})
	fnStream := rx.MockStreamWithInterval(time.Millisecond, "not a []byte")
	result := impl.appendNewDataToRawStream(rawStream, fnStream)

	rxgo.Assert(ctx, t, result, rxgo.HasItem([]byte{10}), rxgo.HasNoError())
}
