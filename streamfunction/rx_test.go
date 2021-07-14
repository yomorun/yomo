package streamfunction

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/core/rx/mock"
)

var impl = newStreamFnRx()

func TestAppendNewData(t *testing.T) {
	t.Run("append new data when one raw data", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rawStream := mock.Stream([]byte{10})
		fnStream := mock.StreamWithInterval(time.Millisecond, []byte{01})
		result := impl.AppendNewDataToRawStream(rawStream, fnStream)

		rxgo.Assert(ctx, t, result, rxgo.HasItem([]byte{10, 01}), rxgo.HasNoError())
	})

	t.Run("append new data when multi raw data", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rawStream := mock.Stream([]byte{10}, []byte{11})
		fnStream := mock.StreamWithInterval(time.Millisecond, []byte{01})
		result := impl.AppendNewDataToRawStream(rawStream, fnStream)

		rxgo.Assert(ctx, t, result, rxgo.HasItem([]byte{10, 11, 01}), rxgo.HasNoError())
	})
}

func TestSkipNewData(t *testing.T) {
	t.Run("Skip when the type is not []byte", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rawStream := mock.Stream([]byte{10})
		fnStream := mock.StreamWithInterval(time.Millisecond, "not a []byte")
		result := impl.AppendNewDataToRawStream(rawStream, fnStream)

		rxgo.Assert(ctx, t, result, rxgo.HasItem([]byte{10}), rxgo.HasNoError())
	})
}

func TestGetAppendedStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRxFactory := mock.NewMockFactory(ctrl)
	mockRxStream := mock.NewMockStream(ctrl)

	impl := &rxImpl{
		rxFactory: mockRxFactory,
	}

	// mock
	mockReader := make(chan io.Reader)

	mockHandler := func(rxstream rx.Stream) rx.Stream {
		// always return a fixed data in mock handler.
		return mock.Stream([]byte{01})
	}

	mockRxFactory.
		EXPECT().
		FromReaderWithDecoder(gomock.Eq(mockReader)).
		Return(mockRxStream).
		AnyTimes()

	mockRxStream.
		EXPECT().
		RawBytes().
		Return(mock.Stream([]byte{10})).
		AnyTimes()

	mockRxStream.
		EXPECT().
		Connect(gomock.Any()).
		AnyTimes()

	// result
	result := impl.GetAppendedStream(mockReader, mockHandler)
	rxgo.Assert(ctx, t, result, rxgo.HasItem([]byte{10, 01}), rxgo.HasNoError())
}
