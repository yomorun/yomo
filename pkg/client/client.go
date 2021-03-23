package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"

	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

var FLOW = []byte{0, 0}
var SINK = []byte{0, 1}

type client struct {
	ip       string
	port     int
	name     string
	readers  chan io.Reader
	writers  []io.Writer
	session  quic.Client
	signal   quic.Stream
	stream   quic.Stream
	accepted chan bool
}

func Connect(ip string, port int) *client {
	c := &client{
		ip:       ip,
		port:     port,
		readers:  make(chan io.Reader),
		writers:  make([]io.Writer, 0),
		accepted: make(chan bool),
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

	_, err = c.signal.Write([]byte(c.name))

	if err != nil {
		panic(err)
	}

	// flow ,sink create stream or heartbeat
	go func() {
		for {
			buf := make([]byte, 2)
			n, err := c.signal.Read(buf)
			if err != nil {
				panic(err)
			}
			switch n {
			case 1:
				if buf[0] == byte(0) {
					c.signal.Write(buf[:n])
				} else {
					c.accepted <- true
				}
			case 2:
				stream, err := c.session.CreateStream(context.Background())

				if err != nil {
					panic(err)
				}

				c.readers <- stream
				c.writers = append(c.writers, stream)
				stream.Write([]byte("王耀光"))
			}

		}
	}()

	return c
}

// source
func (c *client) Writer() (*client, error) {
	for {
		select {
		case _, ok := <-c.accepted:
			if !ok {
				return nil, errors.New("not accepted")
			}
			fmt.Println("client-www1:")
			stream, err := c.session.CreateStream(context.Background())

			if err != nil {
				return nil, err
			}
			c.stream = stream
			return c, nil
		}
	}

}

// flow
func (c *client) ReadWriter() (*client, error) {
	// first send flow-signal to zipper

	for {
		select {
		case _, ok := <-c.accepted:
			if !ok {
				return nil, errors.New("not accepted")
			}
			fmt.Println("client-www2:")
			_, err := c.signal.Write(FLOW)
			if err != nil {
				return nil, err
			}
			return c, nil
		}
	}
}

func (c *client) Reader() (*client, error) {
	// first send sink-signal to zipper
	for {
		select {
		case _, ok := <-c.accepted:
			if !ok {
				return nil, errors.New("not accepted")
			}
			fmt.Println("client-www3:")
			_, err := c.signal.Write(SINK)

			if err != nil {
				return nil, err
			}

			return c, nil
		}
	}

}

// source
func (c *client) Write(b []byte) (int, error) {
	return c.stream.Write(b)
}

// flow || sink
func (c *client) Pipe(f func(rxstream rx.RxStream) rx.RxStream) {
	rxstream := rx.FromReaderWithY3(c.readers)
	stream := f(rxstream)

	rxstream.Connect(context.Background())

	for customer := range stream.Observe() {
		if customer.Error() {
			panic(customer.E)
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
}

func (c *client) Close() {
	c.session.Close()
}
