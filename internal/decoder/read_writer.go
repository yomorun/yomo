package decoder

import (
	"io"

	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
)

// ReadWriter is the Read and Writer which wrapping the data by frame.
type ReadWriter interface {
	Reader
	Writer
}

// Reader is the interface that wraps the basic Read method with framing.
type Reader interface {
	// Read next frame.
	Read() chan framing.Frame
}

// Writer is the interface that wraps the basic Write method with framing.
type Writer interface {
	// Write a frame.
	Write(f framing.Frame) error
}

// NewReadWriter creates a new decoder.ReadWriter by io.ReadWriter.
func NewReadWriter(readWriter io.ReadWriter) ReadWriter {
	return &readWriterImpl{
		reader: NewReader(readWriter),
		writer: NewWriter(readWriter),
	}
}

type readWriterImpl struct {
	reader Reader
	writer Writer
}

// Read next frame.
func (rw *readWriterImpl) Read() chan framing.Frame {
	return rw.reader.Read()
}

// Write a frame.
func (rw *readWriterImpl) Write(f framing.Frame) error {
	return rw.writer.Write(f)
}

type readerImpl struct {
	reader io.Reader
}

// NewReader inits a new Reader.
func NewReader(reader io.Reader) Reader {
	return &readerImpl{
		reader,
	}
}

// Read next frame.
func (r *readerImpl) Read() chan framing.Frame {
	next := make(chan framing.Frame)
	fd := NewFrameDecoder(r.reader)

	go func() {
		defer close(next)

	LOOP:
		for {
			// read next raw frame.
			buf, err := fd.Read(true)
			if err == io.EOF {
				break LOOP
			}
			if err != nil {
				if err.Error() != "Application error 0x0" {
					logger.Debug("[Decoder ReadeWriter] read the bytes failed.", "err", err, "bytes", logger.BytesString(buf))
				}
				break LOOP
			}

			if len(buf) == 0 {
				continue
			}

			f, err := framing.FromRawBytes(buf)
			if err != nil {
				logger.Debug("[Decoder ReadeWriter] read the frame from bytes failed.", "err", err, "bytes", logger.BytesString(buf))
				break LOOP
			}

			next <- f
		}
	}()

	return next
}

type writerImpl struct {
	writer io.Writer
}

// NewWriter inits a new Writer.
func NewWriter(writer io.Writer) Writer {
	return &writerImpl{
		writer,
	}
}

// Write a frame.
func (r *writerImpl) Write(f framing.Frame) error {
	_, err := r.writer.Write(f.Bytes())
	return err
}
