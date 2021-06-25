package decoder

import (
	"bufio"
	"errors"
	"io"

	"github.com/yomorun/yomo/pkg/framing"
)

const (
	minBuffSize = 3 * 1024
	maxBuffSize = 16*1024*1024 + framing.FrameLengthFieldSize
)

// FrameDecoder defines a decoder for decoding frames which have a header of length.
type FrameDecoder bufio.Scanner

// Read reads next frame in bytes.
// if rawFrame == true, it returns the raw frame without frame length field.
func (p *FrameDecoder) Read(rawFrame bool) ([]byte, error) {
	scanner := (*bufio.Scanner)(p)
	if !scanner.Scan() {
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		return nil, err
	}

	// read raw frame
	if rawFrame {
		raw := scanner.Bytes()[framing.FrameLengthFieldSize:]
		return raw, nil
	}

	// read full frame includes frame length field.
	return scanner.Bytes(), nil
}

func doSplit(data []byte, eof bool) (advance int, token []byte, err error) {
	if eof {
		return
	}
	if len(data) < framing.FrameLengthFieldSize {
		return
	}
	frameLength, data := framing.ReadFrameLength(data)
	if frameLength < 1 {
		err = errors.New("invalid frame length")
		return
	}
	frameSize := frameLength + framing.FrameLengthFieldSize
	if frameSize <= len(data) {
		return frameSize, data[:frameSize], nil
	}
	return
}

// NewFrameDecoder creates a new frame decoder.
func NewFrameDecoder(r io.Reader) *FrameDecoder {
	scanner := bufio.NewScanner(r)
	scanner.Split(doSplit)
	buf := make([]byte, 0, minBuffSize)
	scanner.Buffer(buf, maxBuffSize)
	return (*FrameDecoder)(scanner)
}
