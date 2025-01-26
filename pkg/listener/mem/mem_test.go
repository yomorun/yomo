package mem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
)

const (
	handshakeName = "hello yomo"
	streamContent = "hello stream"
	CloseMessage  = "bye!"
)

func TestMemAddr(t *testing.T) {
	fconn := NewFrameConn(context.TODO())

	assert.Equal(t, "mem", fconn.LocalAddr().Network())
	assert.Equal(t, "mem://local", fconn.LocalAddr().String())

	assert.Equal(t, "mem", fconn.RemoteAddr().Network())
	assert.Equal(t, "mem://remote", fconn.RemoteAddr().String())
}

func TestListener(t *testing.T) {
	listener := Listen()

	go func() {
		if err := runListener(t, listener); err != nil {
			panic(err)
		}
	}()

	fconn, err := listener.Dial()
	assert.NoError(t, err)

	time.AfterFunc(time.Second, func() {
		err := fconn.CloseWithError(CloseMessage)
		assert.NoError(t, err)
	})

	err = fconn.Handshake(&frame.HandshakeFrame{Name: handshakeName})
	assert.NoError(t, err)

	for {
		f, err := fconn.ReadFrame()
		if err != nil {
			se := new(frame.ErrConnClosed)
			assert.True(t, errors.As(err, &se))
			assert.Equal(t, frame.NewErrConnClosed(false, CloseMessage), err)
			return
		}
		df, ok := f.(*frame.DataFrame)
		if !ok {
			t.Fatalf("unexpected frame: %v", f)
		}
		assert.Equal(t, streamContent, string(df.Payload))
	}
}

func runListener(t *testing.T, l *Listener) error {
	fconn, err := l.Accept(context.TODO())
	if err != nil {
		return err
	}

	f, err := fconn.ReadFrame()
	assert.NoError(t, err)
	assert.Equal(t, f.Type(), frame.TypeHandshakeFrame)

	if err := fconn.WriteFrame(&frame.HandshakeAckFrame{}); err != nil {
		return err
	}

	time.AfterFunc(time.Second, func() {
		fconn.CloseWithError(CloseMessage)
		l.Close()
	})

	for range 10 {
		fconn.WriteFrame(&frame.DataFrame{
			Tag:     0x34,
			Payload: []byte(streamContent),
		})
	}

	return nil
}
