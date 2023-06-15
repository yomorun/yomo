package core

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/exp/slog"
)

// Peer represents a peer in a network that can open a writer and observe tagged streams, and handle them in an observer.
type Peer struct {
	once             sync.Once
	tag              string
	conn             UniStreamOpenAccepter
	codec            frame.Codec
	packetReadWriter frame.PacketReadWriter
}

// NewPeer returns a new peer.
func NewPeer(conn UniStreamOpenAccepter, codec frame.Codec, packetReadWriter frame.PacketReadWriter) *Peer {
	peer := &Peer{
		conn:             conn,
		codec:            codec,
		packetReadWriter: packetReadWriter,
	}

	return peer
}

// SetObverseTag sets a tag for other peer can obverse the stream in Observers handler.
// If this function is not called, writing to the writer in the ObserverHandler will not do anything.
// Note That multiple calling this function will have no effect.
func (p *Peer) SetObverseTag(tag string) {
	p.once.Do(func() {
		p.tag = tag
	})
}

// Open opens a writer with the given tag, which other peers can observe.
// The returned writer can be used to write to the stream associated with the given tag.
func (p *Peer) Open(ctx context.Context, tag string) (io.Writer, error) {
	w, err := p.conn.OpenUniStream()
	if err != nil {
		return nil, err
	}

	f := &frame.ObserveFrame{
		Tag: tag,
	}

	b, err := p.codec.Encode(f)
	if err != nil {
		return nil, err
	}

	_, err = w.Write(b)

	return w, err
}

// Observe observes tagged streams and handles them in an observer.
// The observer is responsible for handling the tagged streams and writing to a new peer stream.
func (p *Peer) Observe(tag string, observer Observer) error {
	// peer request to observe stream in the specified tag.
	err := p.conn.RequestObserve(tag)
	if err != nil {
		return err
	}
	// then waiting and handling the stream reponsed by server.
	return p.observing(observer)
}

func (p *Peer) observing(observer Observer) error {
	for {
		r, err := p.conn.AcceptUniStream(context.Background())
		if err != nil {
			return err
		}
		var w io.Writer
		// tag
		if p.tag != "" {
			w, err = p.conn.OpenUniStream()
			if err != nil {
				return err
			}
			err = fillWriter(w, p.tag, p.codec, p.packetReadWriter)
			if err != nil {
				return err
			}
		} else {
			w = io.Discard
		}
		observer.Handle(r, w)
	}
}

// UniStreamOpenAccepter opens and accepts uniStream.
type UniStreamOpenAccepter interface {
	UniStreamOpener
	UniStreamAccepter
	// RequestObserve requests server to observe stream be taged in the specified tag.
	RequestObserve(tag string) error
}

// Observer is responsible for handling tagged streams.
type Observer interface {
	// Handle is the function responsible for handling tagged streams and writing to a new peer stream.
	// The `r` parameter is used to read data from the tagged stream, and the `w` parameter is used to write data to a new peer stream.
	Handle(r io.Reader, w io.Writer)
}

// UniStreamOpener opens uniStream.
type UniStreamOpener interface {
	// ID returns the ID of the opener.
	ID() string
	// OpenUniStream is the open function.
	OpenUniStream() (io.Writer, error)
}

// UniStreamAccepter accepts uniStream.
type UniStreamAccepter interface {
	// ID returns the ID of the accepter.
	ID() string
	// AcceptUniStream is the accept function.
	AcceptUniStream(context.Context) (io.Reader, error)
}

// Broker is responsible for accepting streams and docking them to taged connection.
type Broker struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	readerChan   chan tagedReader
	obverserChan chan tagedStreamOpenner
	logger       *slog.Logger
}

// NewStreamBroker creates a new broker.
// The broker is responsible for accepting streams and docking them to taged peer.
func NewStreamBroker(ctx context.Context) *Broker {
	ctx, ctxCancel := context.WithCancel(ctx)

	broker := &Broker{
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		readerChan:   make(chan tagedReader),
		obverserChan: make(chan tagedStreamOpenner),
		logger:       slog.New(slog.NewJSONHandler(os.Stdout)),
	}

	go broker.run()

	return broker
}

// AcceptStream accepts a uniStream from accepter and retrives the tag from the reader accepted.
func (b *Broker) AcceptStream(accepter UniStreamAccepter, codec frame.Codec, packetReadWriter frame.PacketReadWriter) {
	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}
		r, err := accepter.AcceptUniStream(b.ctx)
		if err != nil {
			b.logger.Debug("failed to accept a uniStream", "error", err)
			continue
		}
		tag, err := drainReader(r, codec, packetReadWriter)
		if err != nil {
			b.logger.Debug("ack peer stream failed", "error", err)
			continue
		}
		b.readerChan <- tagedReader{r: r, tag: tag}
	}
}

// Observe makes the opener observe the given tag.
// If an opener observes a tag, it will be notified to open a new stream to dock with
// the tagged stream when it arrives.
func (b *Broker) Observe(tag string, opener UniStreamOpener) {
	item := tagedStreamOpenner{
		tag:    tag,
		opener: opener,
	}
	b.logger.Debug("accept an obverser", "tag", tag, "opener_id", opener.ID())
	b.obverserChan <- item
}

func (b *Broker) run() {
	// observers is a collection of connections.
	// The keys in observers are tags that are used to identify the observers.
	// The values in observers are maps where the keys are observer IDs and the values are the observers themselves.
	// The value maps ensure that each ID has only one corresponding observer.
	observers := make(map[string]map[string]UniStreamOpener)
	for {
		select {
		case <-b.ctx.Done():
			b.logger.Debug("broker is closed")
			return
		case o := <-b.obverserChan:
			m, ok := observers[o.tag]
			if !ok {
				observers[o.tag] = map[string]UniStreamOpener{
					o.opener.ID(): o.opener,
				}
			} else {
				m[o.opener.ID()] = o.opener
			}
		case r := <-b.readerChan:
			vv, ok := observers[r.tag]
			if !ok {
				continue
			}

			ws := make([]io.Writer, 0)

			for _, opener := range vv {
				w, err := opener.OpenUniStream()
				if err != nil {
					b.logger.Debug("failed to accept a uniStream", "error", err)
					continue
				}
				ws = append(ws, w)
			}
			go func() {
				_, err := io.Copy(io.MultiWriter(ws...), r.r)
				if err != nil {
					if err == io.EOF {
						b.logger.Debug("writing to all observers has been completed.")
					} else {
						b.logger.Debug("failed to write a uniStream", "error", err)
					}
				}
			}()
		}
	}
}

func fillWriter(r io.Writer, tag string, codec frame.Codec, packetReadWriter frame.PacketReadWriter) error {
	f := &frame.ObserveFrame{
		Tag: tag,
	}
	b, err := codec.Encode(f)
	if err != nil {
		return err
	}
	return packetReadWriter.WritePacket(r, f.Type(), b)
}

// drainReader drains tag from the reader and returns the tag.
func drainReader(r io.Reader, codec frame.Codec, packetReadWriter frame.PacketReadWriter) (tag string, err error) {
	ft, b, err := packetReadWriter.ReadPacket(r)
	if err != nil {
		return "", err
	}
	if ft != frame.TypeObserveFrame {
		return "", errors.New("read unexpected frame")
	}

	f := new(frame.ObserveFrame)

	if err := codec.Decode(b, f); err != nil {
		return "", err
	}

	return f.Tag, nil
}

type tagedReader struct {
	tag string
	r   io.Reader
}

type tagedStreamOpenner struct {
	tag    string
	opener UniStreamOpener
}
