package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

var (
	// SignalHeartbeat represents the signal of Heartbeat.
	SignalHeartbeat = []byte{0}

	// SignalAccepted represents the signal of Accpeted.
	SignalAccepted = []byte{1}

	// SignalFlowSink represents the signal for flow/sink.
	SignalFlowSink = []byte{0, 0}
)

type client struct {
	zipperIP   string
	zipperPort int
	name       string
	isSource   bool // isSource indicates whether it is a yomo-source client.
	readers    chan io.Reader
	writers    []io.Writer
	session    quic.Client
	signal     quic.Stream
	stream     quic.Stream
	heartbeat  chan byte
}

// SourceClient is the client for YoMo-Source.
// https://yomo.run/source
type SourceClient struct {
	*client
}

// ServerlessClient is the client for YoMo-Serverless.
type ServerlessClient struct {
	*client
}

// connect to yomo-zipper.
// TODO: login auth
func (c *client) connect() (*client, error) {
	addr := fmt.Sprintf("%s:%d", c.zipperIP, c.zipperPort)
	log.Println("Connecting to zipper", addr, "...")
	// connect to yomo-zipper
	quic_cli, err := quic.NewClient(addr)
	if err != nil {
		fmt.Println("client [NewClient] Error:", err)
		return c, err
	}
	// create stream
	quic_stream, err := quic_cli.CreateStream(context.Background())
	if err != nil {
		fmt.Println("client [CreateStream] Error:", err)
		return c, err
	}

	c.session = quic_cli
	c.signal = quic_stream

	// send name to zipper
	_, err = c.signal.Write([]byte(c.name))

	if err != nil {
		fmt.Println("client [Write] Error:", err)
		return c, err
	}

	// flow, sink create stream or heartbeat
	accepted := make(chan bool)
	defer close(accepted)

	go func() {
		for {
			buf := make([]byte, 2)
			n, err := c.signal.Read(buf)
			if err != nil {
				break
			}
			value := buf[:n]

			if bytes.Equal(value, SignalHeartbeat) {
				// heartbeart
				c.heartbeat <- buf[0]
			} else if bytes.Equal(value, SignalAccepted) {
				// accepted
				if c.isSource {
					// create the stream from source.
					stream, err := c.session.CreateStream(context.Background())
					if err != nil {
						fmt.Println("client [session.CreateStream] Error:", err)
						break
					}
					c.stream = stream
				}
				accepted <- true
			} else if bytes.Equal(value, SignalFlowSink) {
				// create stream
				stream, err := c.session.CreateStream(context.Background())

				if err != nil {
					log.Println(err)
					break
				}

				c.readers <- stream
				c.writers = append(c.writers, stream)
				stream.Write(SignalHeartbeat)
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
				// disconnect if didn't receive the heartbeat after 1s.
				return
			}
		}

	}()

	// waiting when the connection is accepted.
	<-accepted
	log.Print("✅ Connected to zipper ", addr)
	return c, nil
}

// Retry the connection between client and server.
func (c *client) Retry() {
	for {
		_, err := c.connect()
		if err == nil {
			break
		} else {
			time.Sleep(time.Second)
		}
	}
}

// Close the client.
func (c *client) Close() {
	c.session.Close()
	c.writers = make([]io.Writer, 0)
	c.heartbeat = make(chan byte)
	c.signal = nil
}

// NewSource setups the client of YoMo-Source.
func NewSource(appName string) *SourceClient {
	c := &SourceClient{
		client: &client{
			name:      appName,
			isSource:  true,
			readers:   make(chan io.Reader, 1),
			writers:   make([]io.Writer, 0),
			heartbeat: make(chan byte),
		},
	}
	return c
}

// Connect to yomo-zipper.
func (c *SourceClient) Connect(ip string, port int) (*SourceClient, error) {
	c.zipperIP = ip
	c.zipperPort = port

	cli, err := c.connect()
	if err != nil {
		return nil, err
	}
	return &SourceClient{
		cli,
	}, nil
}

// NewServerless setups the client of YoMo-Serverless.
// The "appName" should match the name of flows (or sinks) in workflow.yaml in zipper.
func NewServerless(appName string) *ServerlessClient {
	c := &ServerlessClient{
		client: &client{
			name:      appName,
			isSource:  false,
			readers:   make(chan io.Reader, 1),
			writers:   make([]io.Writer, 0),
			heartbeat: make(chan byte),
		},
	}
	return c
}

// Write the data to YoMo-Zipper.
func (c *SourceClient) Write(b []byte) (int, error) {
	if c.client.stream != nil {
		return c.stream.Write(b)
	} else {
		return 0, errors.New("not found stream")
	}
}

// Connect to yomo-zipper.
func (c *ServerlessClient) Connect(ip string, port int) (*ServerlessClient, error) {
	c.zipperIP = ip
	c.zipperPort = port

	cli, err := c.connect()
	if err != nil {
		return nil, err
	}
	return &ServerlessClient{
		cli,
	}, nil
}

// Pipe the handler function in flow/sink serverless.
func (c *ServerlessClient) Pipe(f func(rxstream rx.RxStream) rx.RxStream) {
	rxstream := rx.FromReaderWithDecoder(c.readers)
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
					buf, ok := (customer.V).([]byte)
					if !ok {
						log.Printf("❌ Please add the encode/marshal operator in the end of your Serverless handler.")
						break loop
					}
					_, err := w.Write(buf)
					if err != nil {
						index = rand.Intn(len(c.writers))
						break loop
					}
				} else {
					w.Write(SignalHeartbeat)
				}
			}

		}
	}
}
