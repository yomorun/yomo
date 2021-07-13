package streamfunction

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/yomorun/yomo/core/rx"
	mockrx "github.com/yomorun/yomo/core/rx/mock"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/streamfunction/mock"
	"go.uber.org/goleak"
)

func TestPipeHandler(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFnRx := mock.NewMockStreamfnRx(ctrl)
	var mockWriter bytes.Buffer

	cli := &clientImpl{
		Impl: &client.Impl{
			Writer: &mockWriter,
		},
		fnRx: mockFnRx,
	}

	// mock
	data := []byte{10, 01}
	mockFnRx.
		EXPECT().
		GetAppendedStream(gomock.Any(), gomock.Any()).
		Return(mockrx.Stream(data)).
		AnyTimes()

	mockHandler := func(rxstream rx.Stream) rx.Stream {
		// always return a fixed data in mock handler.
		return mockrx.Stream([]byte{01})
	}

	// pipe handler
	cli.Pipe(mockHandler)
	got := mockWriter.Bytes()

	// wrap data with framing.
	f := framing.NewPayloadFrame(data)
	expected := f.Bytes()

	// assert
	if !bytes.Equal(got, expected) {
		t.Errorf("cli.Pipe, got %v, want %v", got, expected)
	}
}
