// Package serverless provides the server serverless function context.
package serverless

import (
	"context"
	"errors"
	"fmt"
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
		stream, err := c.readStream(context.Background())
		if err == nil {
			c.stream = stream
		} else {
			c.client.Logger().Error("context read stream error", "err", err)
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
	defer c.stream.Close()
	return c.stream
}

func (c *Context) readStream(ctx context.Context) (quic.Stream, error) {
	client := c.client
	dataFrame := c.dataFrame
	dataStreamID := string(dataFrame.Payload)
	client.Logger().Debug(fmt.Sprintf("context receive stream[%s] -- start", dataStreamID))
	// process data stream
STREAM:
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		qconn := client.Connection()
		if qconn == nil {
			err := errors.New("quic connection is nil")
			client.Logger().Error(err.Error())
			return nil, err
		}
		dataStream, err := qconn.AcceptStream(ctx)
		if err != nil {
			client.Logger().Error("context request stream error", "err", err, "datastream_id", dataStreamID)
			return nil, err
		}
		client.DataStreams().Store(dataStreamID, dataStream)
		client.Logger().Debug("context accept stream success", "datastream_id", dataStreamID, "stream_id", dataStream.StreamID())
		// read stream frame
		fs := core.NewFrameStream(dataStream, y3codec.Codec(), y3codec.PacketReadWriter())
		f, err := fs.ReadFrame()
		if err != nil {
			client.Logger().Warn("failed to read data stream", "err", err, "datastream_id", dataStreamID)
			return nil, err
		}
		switch f.Type() {
		case frame.TypeStreamFrame:
			streamFrame := f.(*frame.StreamFrame)
			// lookup data stream
			// if streamFrame.ID != dataStreamID {
			reader, ok := client.DataStreams().Load(dataStreamID)
			if !ok {
				client.Logger().Debug(
					"data strem is not found, continue",
					"stream_id", dataStream.StreamID(),
					"datastream_id", dataStreamID,
					"received_id", streamFrame.ID,
					"client_id", streamFrame.ClientID,
					"tag", streamFrame.Tag,
				)
				goto STREAM
			}
			defer client.DataStreams().Delete(dataStreamID)
			client.Logger().Debug(
				"data stream is ready",
				"remote_addr", qconn.RemoteAddr().String(),
				"datastream_id", streamFrame.ID,
				"stream_id", dataStream.StreamID(),
				"stream_chunk_szie", streamFrame.ChunkSize,
				"client_id", streamFrame.ClientID,
				"tag", streamFrame.Tag,
			)
			return reader.(quic.Stream), nil
		default:
			client.Logger().Error("!!!unexpected frame!!!", "unexpected_frame_type", f.Type().String())
		}
		client.Logger().Debug(fmt.Sprintf("context receive stream[%s] -- end", dataStreamID))

		return dataStream, nil
	}
}
