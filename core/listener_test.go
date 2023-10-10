package core

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

const testHost = "localhost:9008"

const (
	handshakeName = "hello yomo"
	streamContent = "hello stream"
	CloseMessage  = "bye!"
)

func TestFrameConnection(t *testing.T) {
	go func() {
		if err := runListener(t); err != nil {
			panic(err)
		}
	}()

	fconn, err := DialAddr(context.TODO(), testHost, y3codec.Codec(), y3codec.PacketReadWriter(), pkgtls.MustCreateClientTLSConfig(), defaultClientOption().quicConfig)
	assert.NoError(t, err)

	err = fconn.WriteFrame(&frame.HandshakeAckFrame{})
	assert.NoError(t, err)

	for {
		select {
		case f := <-fconn.ReadFrame():
			assert.Equal(t, frame.TypeHandshakeFrame, f.Type())
			hf := f.(*frame.HandshakeFrame)
			assert.Equal(t, handshakeName, hf.Name)
		case stream := <-fconn.AcceptStream():
			rd, err := io.ReadAll(stream)
			assert.NoError(t, err)
			assert.Equal(t, streamContent, string(rd))
		case <-fconn.Context().Done():
			assert.Equal(t, &ErrConnectionClosed{CloseMessage}, context.Cause(fconn.Context()))
			return
		}
	}
}

func runListener(t *testing.T) error {
	listener, err := ListenAddr(testHost, pkgtls.MustCreateServerTLSConfig(testHost), DefalutQuicConfig, y3codec.Codec(), y3codec.PacketReadWriter())
	if err != nil {
		return err
	}

	time.AfterFunc(3*time.Second, func() {
		listener.Close()
	})

	fconn, err := listener.Accept(context.TODO())
	if err != nil {
		return err
	}

	f := <-fconn.ReadFrame()
	assert.Equal(t, f.Type(), frame.TypeHandshakeAckFrame)

	if err := fconn.WriteFrame(&frame.HandshakeFrame{Name: handshakeName}); err != nil {
		return err
	}

	stream, err := fconn.OpenStream()
	if err != nil {
		return err
	}
	_, _ = stream.Write([]byte(streamContent))
	_ = stream.Close()

	time.AfterFunc(time.Second, func() {
		err := fconn.CloseWithError(CloseMessage)
		assert.NoError(t, err)

		// close twice has no effect.
		err = fconn.CloseWithError(CloseMessage)
		assert.NoError(t, err)

		_, err = fconn.OpenStream()
		assert.Equal(t, &ErrConnectionClosed{CloseMessage}, err)

		err = fconn.WriteFrame(&frame.DataFrame{Payload: []byte("aaaa")})
		assert.Equal(t, &ErrConnectionClosed{CloseMessage}, err)
	})

	return nil
}
