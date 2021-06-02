package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

const (
	// ClientTypeSource represents the client type of Source.
	ClientTypeSource = "source"

	// ClientTypeServerless represents the client type of Serverless.
	ClientTypeServerless = "serverless"

	// ClientTypeZipperSender represents the client type of ZipperSender.
	ClientTypeZipperSender = "zipper-sender"
)

var (
	// SignalHeartbeat represents the signal of Heartbeat.
	SignalHeartbeat = []byte{0}

	// SignalAccepted represents the signal of Accpeted.
	SignalAccepted = []byte{1}

	// SignalFlowSink represents the signal for flow/sink.
	SignalFlowSink = []byte{0, 0}
)

// NegotiationPayload represents the payload for negotiation.
type NegotiationPayload struct {
	AppName    string `json:"app_name"`
	ClientType string `json:"client_type"`
}

type client interface {
	io.Writer
	Close() error
	Retry()
}

// SourceClient is the client for YoMo-Source.
// https://yomo.run/source
type SourceClient interface {
	client

	// Connect to YoMo-Zipper
	Connect(ip string, port int) (SourceClient, error)
}

// ServerlessClient is the client for YoMo-Serverless.
type ServerlessClient interface {
	client

	// Connect to YoMo-Zipper
	Connect(ip string, port int) (ServerlessClient, error)

	// Pipe the Handler function.
	Pipe(f func(rxstream rx.RxStream) rx.RxStream)
}

// ZipperSenderClient is the client for Zipper-Sender to connect the downsteam Zipper-Receiver  in edge-mesh.
type ZipperSenderClient interface {
	client

	// Connect to downsteam Zipper-Receiver
	Connect(ip string, port int) (ZipperSenderClient, error)
}

type clientImpl struct {
	zipperIP   string
	zipperPort int
	name       string
	clientType string
	readers    chan io.Reader
	writer     io.Writer
	session    quic.Client
	signal     quic.Stream
	stream     quic.Stream
	heartbeat  chan byte
	mutex      sync.Mutex
}

type sourceClientImpl struct {
	*clientImpl
}

type serverlessClientImpl struct {
	*clientImpl
}

type zipperSenderClientImpl struct {
	*clientImpl
}

// newClient creates a new client.
func newClient(appName string, clientType string) *clientImpl {
	c := &clientImpl{
		name:       appName,
		clientType: clientType,
		readers:    make(chan io.Reader, 1),
		heartbeat:  make(chan byte),
	}
	return c
}

// connect to yomo-zipper.
// TODO: login auth
func (c *clientImpl) connect(ip string, port int) (*clientImpl, error) {
	c.zipperIP = ip
	c.zipperPort = port
	addr := fmt.Sprintf("%s:%d", ip, port)
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

	// send negotiation payload to zipper
	payload := NegotiationPayload{
		AppName:    c.name,
		ClientType: c.clientType,
	}
	buf, _ := json.Marshal(payload)
	_, err = c.signal.Write(buf)

	if err != nil {
		fmt.Println("client [Write] Error:", err)
		return c, err
	}

	// flow, sink create stream or heartbeat
	accepted := make(chan bool)

	go func() {
		defer close(accepted)
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
				if c.clientType == ClientTypeSource {
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
				c.writer = stream
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
				// reconnect if didn't receive the heartbeat after 1s.
				c.mutex.Lock()
				c.connect(c.zipperIP, c.zipperPort)
				c.mutex.Unlock()
			}
		}

	}()

	// waiting when the connection is accepted.
	<-accepted
	log.Print("✅ Connected to zipper ", addr)
	return c, nil
}

// Write the data to downstream.
func (c *clientImpl) Write(b []byte) (int, error) {
	if c.stream != nil {
		return c.stream.Write(b)
	} else {
		return 0, errors.New("not found stream")
	}
}

// Retry the connection between client and server.
func (c *clientImpl) Retry() {
	for {
		_, err := c.connect(c.zipperIP, c.zipperPort)
		if err == nil {
			break
		} else {
			time.Sleep(time.Second)
		}
	}
}

// Close the client.
func (c *clientImpl) Close() error {
	err := c.session.Close()
	c.heartbeat = make(chan byte)
	c.signal = nil
	return err
}

// NewSource setups the client of YoMo-Source.
func NewSource(appName string) SourceClient {
	c := &sourceClientImpl{
		clientImpl: newClient(appName, ClientTypeSource),
	}
	return c
}

// Connect to yomo-zipper.
func (c *sourceClientImpl) Connect(ip string, port int) (SourceClient, error) {
	cli, err := c.connect(ip, port)
	if err != nil {
		return nil, err
	}
	return &sourceClientImpl{
		cli,
	}, nil
}

// NewServerless setups the client of YoMo-Serverless.
// The "appName" should match the name of flows (or sinks) in workflow.yaml in zipper.
func NewServerless(appName string) ServerlessClient {
	c := &serverlessClientImpl{
		clientImpl: newClient(appName, ClientTypeServerless),
	}
	return c
}

// Connect to yomo-zipper.
func (c *serverlessClientImpl) Connect(ip string, port int) (ServerlessClient, error) {
	cli, err := c.connect(ip, port)
	if err != nil {
		return nil, err
	}
	return &serverlessClientImpl{
		cli,
	}, nil
}

// Pipe the handler function in flow/sink serverless.
func (c *serverlessClientImpl) Pipe(f func(rxstream rx.RxStream) rx.RxStream) {
	rxstream := rx.FromReaderWithDecoder(c.readers)
	stream := f(rxstream)

	rxstream.Connect(context.Background())

	for customer := range stream.Observe() {
		if customer.Error() {
			panic(customer.E)
		} else if customer.V != nil {
			if c.writer == nil {
				continue
			}

			buf, ok := (customer.V).([]byte)
			if !ok {
				log.Print("❌ Please add the encode/marshal operator in the end of your Serverless handler.")
				continue
			}
			_, err := c.writer.Write(buf)
			if err != nil {
				log.Print("❌ Send data to zipper failed. ", err)
			}
		}

	}
}

// NewZipperSender setups the client of Zipper-Sender.
func NewZipperSender(appName string) ZipperSenderClient {
	c := &zipperSenderClientImpl{
		clientImpl: newClient(appName, ClientTypeZipperSender),
	}
	return c
}

// Connect to downstream zipper-receiver in edge-mesh.
func (c *zipperSenderClientImpl) Connect(ip string, port int) (ZipperSenderClient, error) {
	cli, err := c.connect(ip, port)
	if err != nil {
		return nil, err
	}
	return &zipperSenderClientImpl{
		cli,
	}, nil
}
