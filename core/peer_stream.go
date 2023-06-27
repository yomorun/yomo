package core

import (
	"context"
	"io"
	"sync"

	"github.com/yomorun/yomo/pkg/id"
	"golang.org/x/exp/slog"
)

// Peer represents a peer in a network that can open a writer and observe tagged streams,
// and handle them in an observer.
type Peer struct {
	once sync.Once
	// the tag of the writer in the ObserveHandler
	observeHandlerWriterTag string

	conn           UniStreamPeerConnection
	logger         *slog.Logger
	fillWriterFunc func(string, string, io.Writer) error
}

// NewPeer returns a new peer.
func NewPeer(conn UniStreamPeerConnection, logger *slog.Logger, fillWriterFunc func(string, string, io.Writer) error) *Peer {
	peer := &Peer{
		conn:           conn,
		logger:         logger,
		fillWriterFunc: fillWriterFunc,
	}

	return peer
}

// SetObserveHandlerWriterTag sets a tag for other peer can observe the writer stream in Observers handler.
// If this function is not called, writing to the writer in the ObserverHandler will not do anything.
// Note That multiple calling this function will have no effect.
func (p *Peer) SetObserveHandlerWriterTag(tag string) {
	p.once.Do(func() {
		p.observeHandlerWriterTag = tag
	})
}

// Open opens a writer with the given tag, which other peers can observe.
// The returned writer can be used to write to the stream associated with the given tag.
func (p *Peer) Open(tag string) (io.WriteCloser, error) {
	w, err := p.conn.OpenUniStream()
	if err != nil {
		return nil, err
	}

	p.logger.Debug("peer opens a writer", "tag", tag)

	id := id.New()

	return w, p.fillWriterFunc(id, tag, w)
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
		// accept and pure the reader.
		r, err := p.conn.AcceptUniStream(context.Background())
		if err != nil {
			return err
		}

		// open and pure the writer if observeHandlerWriterTag is not empty.
		var w io.Writer
		if p.observeHandlerWriterTag != "" {
			w, err = p.Open(p.observeHandlerWriterTag)
			if err != nil {
				return err
			}
		} else {
			// discard writer if there is no tag.
			w = io.Discard
		}

		// dispatch the reader and writer to the observer.
		observer.Handle(r, w)
	}
}

// Broker accepts streams from Peer and docks them to another Peer.
type Broker struct {
	ctx             context.Context
	ctxCancel       context.CancelFunc
	readerChan      chan taggedReader
	readEOFChan     chan string // if read EOF, send to this chan
	observerChan    chan taggedConnection
	logger          *slog.Logger
	drainReaderFunc func(io.Reader) (string, string, error)
}

// NewBroker creates a new broker.
// The broker accepts streams from Peer and docks them to another Peer.
func NewBroker(ctx context.Context, drainReaderFunc func(io.Reader) (string, string, error), logger *slog.Logger) *Broker {
	ctx, ctxCancel := context.WithCancel(ctx)

	broker := &Broker{
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		readerChan:      make(chan taggedReader),
		readEOFChan:     make(chan string),
		observerChan:    make(chan taggedConnection),
		logger:          logger,
		drainReaderFunc: drainReaderFunc,
	}

	go broker.run()

	return broker
}

// AccepStream continusly accepts uniStreams from conn and retrives the tag from the reader accepted.
func (b *Broker) AccepStream(conn UniStreamConnection) {
	go func() {
		for {
			select {
			case <-b.ctx.Done():
				return
			default:
			}
			r, err := conn.AcceptUniStream(b.ctx)
			if err != nil {
				b.logger.Debug("failed to accept a uniStream", "error", err)
				break
			}
			id, tag, err := b.drainReaderFunc(r)
			if err != nil {
				b.logger.Debug("ack peer stream failed", "error", err)
				continue
			}
			b.readerChan <- taggedReader{id: id, r: r, tag: tag}
		}
	}()
}

// Observe makes the conn observe the given tag.
// If an conn observes a tag, it will be notified to open a new stream to dock with
// the tagged stream when it arrives.
func (b *Broker) Observe(tag string, conn UniStreamConnection) {
	item := taggedConnection{
		tag:  tag,
		conn: conn,
	}
	b.logger.Debug("accept an observer", "tag", tag, "conn_id", conn.ID())
	b.observerChan <- item
}

// Close closes the broker.
func (b *Broker) Close() error {
	b.ctxCancel()
	return nil
}

func (b *Broker) run() {
	var (
		// observers is a collection of connections.
		// The keys in observers are tags that are used to identify the observers.
		// The values in observers are maps where the keys are observer IDs and the values are the observers themselves.
		// The value maps ensure that each ID has only one corresponding observer.
		observers = make(map[string]map[string]UniStreamConnection)

		// readers stores readers.
		// The key is reader tag,
		// The value is a map where the keys are the id and the value is the reader.
		// Using a map means that each tag only has one corresponding reader and
		// new stream cannot cover the old stream in same tag.
		readers = make(map[string]map[string]io.ReadCloser)
	)
	for {
		select {
		case <-b.ctx.Done():
			b.logger.Debug("broker is closed")
			return
		case o := <-b.observerChan:
			// if the writer opener is already registered, observe the writer directly.
			rm, ok := readers[o.tag]
			if ok {
				for rid, r := range rm {
					w, err := o.conn.OpenUniStream()
					if err != nil {
						b.logger.Debug("failed to accept a uniStream", "error", err)
						continue
					}
					go b.copyWithLog(o.tag, w, r, b.logger)
					// delete the reader that has been observed.
					delete(rm, rid)
				}
			}
			// if the writer opener is empty,
			// store the observer and waiting the writer be registered.
			m, ok := observers[o.tag]
			if !ok {
				observers[o.tag] = map[string]UniStreamConnection{
					o.conn.ID(): o.conn,
				}
			} else {
				m[o.conn.ID()] = o.conn
			}
		case r := <-b.readerChan:
			// if there donot have any observers,
			// store the reader for waiting comming observer to observe it.
			vv, ok := observers[r.tag]
			if !ok {
				rm, ok := readers[r.tag]
				if ok {
					rm[r.id] = r.r
				} else {
					// if there donot has an old writer, store it.
					readers[r.tag] = map[string]io.ReadCloser{
						r.id: r.r,
					}
				}
				continue
			}

			// if there has observers, copy the writer to them one-to-one.
			for _, opener := range vv {
				w, err := opener.OpenUniStream()
				if err != nil {
					b.logger.Debug("failed to accept a uniStream", "error", err)
					delete(vv, opener.ID())
					break
				}
				// one observer can only observe once.
				delete(vv, opener.ID())

				go b.copyWithLog(r.tag, w, r.r, b.logger)
			}
		case tag := <-b.readEOFChan:
			delete(readers, tag)
		}
	}
}

func (b *Broker) copyWithLog(tag string, dst io.Writer, src io.Reader, logger *slog.Logger) {
	_, err := io.Copy(dst, src)
	if err != nil {
		if err == io.EOF {
			b.readEOFChan <- tag
			logger.Debug("writing to all observers has been completed.")
		} else {
			logger.Debug("failed to write a uniStream", "error", err)
		}
	}
}

// Observer is responsible for handling tagged streams.
type Observer interface {
	// Handle is the function responsible for handling tagged streams and writing to a new peer stream.
	// The `r` parameter is used to read data from the tagged stream, and the `w` parameter is used to write data to a new peer stream.
	Handle(r io.Reader, w io.Writer)
}

// ObserveHandleFunc handles tagged streams.
type ObserveHandleFunc func(r io.Reader, w io.Writer)

// Handle calls ObserveHandleFunc itself.
func (f ObserveHandleFunc) Handle(r io.Reader, w io.Writer) { f(r, w) }

// UniStreamConnection opens and accepts uniStream.
type UniStreamConnection interface {
	// ID returns the ID of the connection.
	ID() string
	// OpenUniStream opens uniStream.
	OpenUniStream() (io.WriteCloser, error)
	// AcceptUniStream accepts uniStream.
	AcceptUniStream(context.Context) (io.ReadCloser, error)
}

// UniStreamPeerConnection opens and accepts uniStreams,
// Adding a new method for requesting observe a tag. just work for peer side.
type UniStreamPeerConnection interface {
	// basic connection.
	UniStreamConnection
	// RequestObserve requests server to observe stream be tagged in the specified tag.
	RequestObserve(tag string) error
}

type taggedReader struct {
	id  string
	tag string
	r   io.ReadCloser
}

type taggedConnection struct {
	tag  string
	conn UniStreamConnection
}
