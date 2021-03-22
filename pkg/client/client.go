package client

import (
	"context"
	"fmt"
	"io"
	"math/rand"

	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

const FLOW = byte(0)
const SINK = byte(1)

type client struct {
	ip      string
	port    int
	name    string
	readers chan io.Reader
	writers []io.Writer
	session quic.Client
	signal  quic.Stream
	stream  quic.Stream
}

func Connect(ip string, port int) *client {
	c := &client{
		ip:      ip,
		port:    port,
		readers: make(chan io.Reader),
		writers: make([]io.Writer, 0),
	}
	return c
}

//TODO login auth
func (c *client) Name(name string) *client {
	c.name = name
	quic_cli, err := quic.NewClient(fmt.Sprintf("%s:%d", c.ip, c.port))
	if err != nil {
		panic(err)
	}

	quic_stream, err := quic_cli.CreateStream(context.Background())
	if err != nil {
		panic(err)
	}

	c.session = quic_cli
	c.signal = quic_stream

	_, err = quic_stream.Write([]byte(c.name))

	if err != nil {
		panic(err)
	}

	// flow ,sink create stream
	go func() {
		for {
			buf := make([]byte, 1)
			_, err := c.signal.Read(buf)

			if err != nil {
				panic(err)
			}

			stream, err := c.session.CreateStream(context.Background())

			if err != nil {
				panic(err)
			}

			c.readers <- stream
			c.writers = append(c.writers, stream)
		}
	}()

	return c
}

// source
func (c *client) Writer() (*client, error) {
	stream, err := c.session.CreateStream(context.Background())

	if err != nil {
		return nil, err
	}
	c.stream = stream
	return c, nil
}

// flow
func (c *client) ReadWriter() (*client, error) {
	// first send flow-signal to zipper
	_, err := c.signal.Write([]byte{FLOW})

	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *client) Reader() (*client, error) {
	// first send sink-signal to zipper
	_, err := c.signal.Write([]byte{SINK})

	if err != nil {
		return nil, err
	}

	return c, nil
}

// source
func (c *client) Write(b []byte) error {
	_, err := c.stream.Write(b)
	return err
}

// flow || sink
func (c *client) Pipe(f func(rxstream rx.RxStream) rx.RxStream) error {
	rxstream := rx.FromReaderWithY3(c.readers)
	stream := f(rxstream)

	rxstream.Connect(context.Background())

	go func() {
		for customer := range stream.Observe() {
			if customer.Error() {
				fmt.Println(customer.E.Error())
			} else if customer.V != nil {
				index := rand.Intn(len(c.writers))

			loop:
				for i, w := range c.writers {
					if index == i {
						_, err := w.Write((customer.V).([]byte))
						if err != nil {
							index = rand.Intn(len(c.writers))
							break loop
						}
					}
				}

			}
		}
	}()
	return nil
}

func (c *client) Close() {
	c.session.Close()
}
