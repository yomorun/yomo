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

var FLOWORSINK = []byte{0, 0}

type client struct {
	ip       string
	port     int
	name     string
	readers  chan io.Reader
	writers  []io.Writer
	session  quic.Client
	signal   quic.Stream
	stream   quic.Stream
	accepted chan int
}

func Connect(ip string, port int) *client {
	c := &client{
		ip:       ip,
		port:     port,
		readers:  make(chan io.Reader, 1),
		writers:  make([]io.Writer, 0),
		accepted: make(chan int),
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
				} else if buf[0] == byte(1) {
					c.accepted <- 1
				} else {
					c.accepted <- 2
				}
			case 2:
				stream, err := c.session.CreateStream(context.Background())

				if err != nil {
					panic(err)
				}

				c.readers <- stream
				c.writers = append(c.writers, stream)
				stream.Write([]byte{0}) //create stream
			}

		}
	}()

	return c
}

func (c *client) Stream() (*client, error) {
	for {
		select {
		case i, ok := <-c.accepted:
			if !ok {
				return nil, errors.New("not accepted")
			}
			stream, err := c.session.CreateStream(context.Background())
			if err != nil {
				return nil, err
			}
			if i == 2 {
				_, err = stream.Write([]byte{0}) //create flow and sink stream
				if err != nil {
					return nil, err
				}
			}
			c.stream = stream
			c.readers <- stream
			c.writers = append(c.writers, stream)
			return c, nil
		}
	}
}

// source
func (c *client) Write(b []byte) (int, error) {
	fmt.Println("=======================")
	return c.stream.Write(b)
}

// flow || sink
func (c *client) Pipe(f func(rxstream rx.RxStream) rx.RxStream) {
	rxstream := rx.FromReaderWithY3(c.readers)
	stream := f(rxstream)

	rxstream.Connect(context.Background())

	for customer := range stream.Observe() {
		fmt.Println(customer.V)
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
