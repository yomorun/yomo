// Package serverless provides the server serverless function context.
package serverless

import (
	"context"
	"io"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
)

// Context sfn handler context
type Context struct {
	client    *core.Client
	dataFrame *frame.DataFrame
	streamed  bool
	stream    io.ReadCloser
}

// NewContext creates a new serverless Context
func NewContext(client *core.Client, dataFrame *frame.DataFrame) *Context {
	c := &Context{
		client:    client,
		dataFrame: dataFrame,
	}
	// streamed
	m, err := metadata.Decode(c.dataFrame.Metadata)
	if err != nil {
		c.streamed = false
	} else {
		c.streamed = core.GetStreamedFromMetadata(m)
	}
	// stream
	if c.streamed {
		stream, err := c.requestStream(context.Background())
		if err == nil {
			c.client.Logger().Info("[context] request stream success")
			c.stream = stream
		}
	}
	return c
}

// Tag returns the tag of the data frame
func (c *Context) Tag() uint32 {
	return c.dataFrame.Tag
}

// Data returns the data of the data frame
func (c *Context) Data() []byte {
	return c.dataFrame.Payload
}

// Write writes the data
func (c *Context) Write(tag uint32, data []byte) error {
	if data == nil {
		return nil
	}

	dataFrame := &frame.DataFrame{
		Tag:      tag,
		Metadata: c.dataFrame.Metadata,
		Payload:  data,
	}

	return c.client.WriteFrame(dataFrame)
}

// Streamed returns whether the data is streamed.
func (c *Context) Streamed() bool {
	return c.streamed
}

// Stream returns the stream.
func (c *Context) Stream() io.Reader {
	// TODO: need to improvement
	defer c.stream.Close()
	return c.stream
	/*
		// TEST: test data stream
		dataStream := c.stream
		defer dataStream.Close()
		bufSize := 32 * 1024
		buf := make([]byte, bufSize)
		received := bytes.NewBuffer(nil)
		done := false
		for {
			if done {
				break
			}
			n, err := dataStream.Read(buf)
			// c.client.Logger().Debug("!!!context received bytes!!!", "n", n, "err", err)
			if err != nil {
				if err == io.EOF {
					c.client.Logger().Debug("context received data stream done")
					done = true
				} else {
					c.client.Logger().Error("failed to read data stream", "err", err)
					break
				}
			}
			received.Write(buf[:n])
		}
		l := received.Len()
		c.client.Logger().Debug("!!!context receive completed!!!", "len", l, "buf", string(received.Bytes()[l-1000:]))

		return nil
	*/
}

func (c *Context) requestStream(ctx context.Context) (quic.Stream, error) {
	client := c.client
	dataFrame := c.dataFrame
	client.Logger().Debug("sfn receive data stream -- start")
	// process data stream
STREAM:
	// fmt.Printf("client: %+v\n", client)
	qconn := client.Connection()
	// fmt.Printf("quic connection: %+v\n", qconn)
	dataStream, err := qconn.AcceptStream(ctx)
	if err != nil {
		client.Logger().Error("sfn request stream error", "err", err)
		return nil, err
	}
	client.Logger().Debug("sfn accept stream success", "stream_id", dataStream.StreamID())
	// read stream frame
	fs := core.NewFrameStream(dataStream, y3codec.Codec(), y3codec.PacketReadWriter())
	f, err := fs.ReadFrame()
	if err != nil {
		client.Logger().Warn("failed to read data stream", "err", err)
		return nil, err
	}
	// raw data stream id
	dataStreamID := string(dataFrame.Payload)
	switch f.Type() {
	case frame.TypeStreamFrame:
		streamFrame := f.(*frame.StreamFrame)
		// if stream id is same, pipe stream
		if streamFrame.ID != dataStreamID {
			client.Logger().Debug(
				"stream id is not same, continue",
				"stream_id", dataStream.StreamID(),
				"datastream_id", dataStreamID,
				"received_id", streamFrame.ID,
				"client_id", streamFrame.ClientID,
				"tag", streamFrame.Tag,
			)
			goto STREAM
		}
		client.Logger().Info(
			"!!!sfn stream is ready!!!",
			"remote_addr", qconn.RemoteAddr().String(),
			"datastream_id", streamFrame.ID,
			"stream_id", dataStream.StreamID(),
			"client_id", streamFrame.ClientID,
			"id", streamFrame.ID,
			"tag", streamFrame.Tag,
		)
	default:
		client.Logger().Error("!!!unexpected frame!!!", "unexpected_frame_type", f.Type().String())
	}
	client.Logger().Debug("sfn receive data stream -- end")

	return dataStream, nil
}
