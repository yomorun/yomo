package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

var FLOWORSINK = []byte{0, 0}

type client struct {
	ip        string
	port      int
	name      string
	readers   chan io.Reader
	writers   []io.Writer
	session   quic.Client
	signal    quic.Stream
	stream    quic.Stream
	heartbeat chan byte
	accepted  chan int
	err       error
}

func Connect(ip string, port int) *client {
	c := &client{
		ip:        ip,
		port:      port,
		readers:   make(chan io.Reader, 1),
		writers:   make([]io.Writer, 0),
		accepted:  make(chan int),
		heartbeat: make(chan byte),
		err:       nil,
	}
	return c
}

//TODO login auth
func (c *client) Name(name string) *client {
	c.name = name
	quic_cli, err := quic.NewClient(fmt.Sprintf("%s:%d", c.ip, c.port))
	if err != nil {
		fmt.Println("client [NewClient] Error:", err)
		c.err = err
		return c
	}

	quic_stream, err := quic_cli.CreateStream(context.Background())
	if err != nil {
		fmt.Println("client [CreateStream] Error:", err)
		c.err = err
		return c
	}

	c.session = quic_cli
	c.signal = quic_stream

	_, err = c.signal.Write([]byte(c.name))

	if err != nil {
		fmt.Println("client [Write] Error:", err)
		c.err = err
		return c
	}

	// flow ,sink create stream or heartbeat
	go func() {
		for {
			buf := make([]byte, 2)
			n, err := c.signal.Read(buf)
			if err != nil {
				break
			}
			switch n {
			case 1:
				if buf[0] == byte(0) {
					c.heartbeat <- buf[0]
				} else if buf[0] == byte(1) {
					c.accepted <- 1
				} else {
					c.accepted <- 2
				}
			case 2:
				stream, err := c.session.CreateStream(context.Background())

				if err != nil {
					break
				}

				c.readers <- stream
				c.writers = append(c.writers, stream)
				stream.Write([]byte{0}) //create stream
			}

		}
	}()

	go func() {
		defer c.Close()
		for {
			select {
			case item, ok := <-c.heartbeat:
				if !ok {
					return
				}
				_, err := c.signal.Write([]byte{item})

				if err != nil {
					return
				}
			case <-time.After(time.Second):
				return
			}
		}

	}()

	return c
}

func (c *client) Stream() (*client, error) {
	if c.err != nil {
		return nil, c.err
	}

	for {
		select {
		case item, ok := <-c.accepted:
			if !ok {
				return nil, errors.New("not accepted")
			}

			if item == 1 {
				stream, err := c.session.CreateStream(context.Background())
				if err != nil {
					return nil, err
				}
				c.stream = stream
			}

			return c, nil
		}
	}
}

func (c *client) reTry() {
	for {
		_, err := c.Name(c.name).Stream()
		if err == nil {
			break
		} else {
			time.Sleep(time.Second)
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
	close(c.accepted)
	close(c.heartbeat)
	c.writers = make([]io.Writer, 0)
	c.accepted = make(chan int)
	c.heartbeat = make(chan byte)
	c.signal = nil
	c.stream = nil
	c.reTry()
}
