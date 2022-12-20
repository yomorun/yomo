package frame

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReadUntil(t *testing.T) {
	type fields struct {
		frames    []Frame
		intervals time.Duration
	}
	type args struct {
		t       Type
		timeout time.Duration
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		wantErr   error
		wantFrame Frame
	}{
		{
			name: "read until timeout",
			fields: fields{
				frames:    []Frame{NewDataFrame(), NewHandshakeAckFrame()},
				intervals: time.Second,
			},
			args: args{
				t:       TagOfHandshakeAckFrame,
				timeout: time.Millisecond,
			},
			wantErr:   ErrReadUntilTimeout{t: TagOfHandshakeAckFrame},
			wantFrame: nil,
		},
		{
			name: "read until success",
			fields: fields{
				frames:    []Frame{NewDataFrame(), NewHandshakeAckFrame()},
				intervals: time.Microsecond,
			},
			args: args{
				t:       TagOfHandshakeAckFrame,
				timeout: time.Millisecond,
			},
			wantErr:   nil,
			wantFrame: NewHandshakeAckFrame(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				frameReader = newMockFrameReader(tt.fields.intervals, tt.fields.frames...)
			)

			frm, err := ReadUntil(frameReader, tt.args.t, tt.args.timeout)

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantFrame, frm)
		})
	}
}

// mockFrameReader mock a FrameReader
type mockFrameReader struct {
	mu        sync.Mutex
	seq       int
	intervals time.Duration
	frames    []Frame
}

func newMockFrameReader(intervals time.Duration, frames ...Frame) *mockFrameReader {
	return &mockFrameReader{
		intervals: intervals,
		frames:    frames,
	}
}

func (r *mockFrameReader) ReadFrame() (Frame, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.seq >= len(r.frames) {
		return nil, errors.New("all data has been read")
	}

	time.Sleep(r.intervals)

	frm := r.frames[r.seq]
	r.seq++

	return frm, nil
}
