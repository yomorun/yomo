package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/logging"
	"github.com/quic-go/quic-go/qlog"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

// ClientOption YoMo client options
type ClientOption func(*clientOptions)

// clientOptions are the options for YoMo client.
type clientOptions struct {
	observeDataTags []frame.Tag
	quicConfig      *quic.Config
	tlsConfig       *tls.Config
	credential      *auth.Credential
	reconnect       bool
	nonBlockWrite   bool
	logger          *slog.Logger
	tracerProvider  trace.TracerProvider
	// ai function
	aiFunctionInputModel  any
	aiFunctionDescription string
}

// DefaultClientQuicConfig be used when the `quicConfig` of client is nil.
var DefaultClientQuicConfig = &quic.Config{
	Versions:                       []quic.VersionNumber{quic.Version1, quic.Version2},
	MaxIdleTimeout:                 time.Second * 40,
	KeepAlivePeriod:                time.Second * 20,
	MaxIncomingStreams:             1000,
	MaxIncomingUniStreams:          1000,
	HandshakeIdleTimeout:           time.Second * 3,
	InitialStreamReceiveWindow:     1024 * 1024 * 2,
	InitialConnectionReceiveWindow: 1024 * 1024 * 2,
	TokenStore:                     quic.NewLRUTokenStore(10, 5),
}

func defaultClientOption() *clientOptions {
	opts := &clientOptions{
		observeDataTags: make([]frame.Tag, 0),
		quicConfig:      DefaultClientQuicConfig,
		tlsConfig:       pkgtls.MustCreateClientTLSConfig(),
		credential:      auth.NewCredential(""),
		logger:          ylog.Default(),
	}

	return opts
}

// WithCredential sets the client credential method (used by client).
func WithCredential(payload string) ClientOption {
	return func(o *clientOptions) {
		o.credential = auth.NewCredential(payload)
	}
}

// WithClientTLSConfig sets tls config for the client.
func WithClientTLSConfig(tc *tls.Config) ClientOption {
	return func(o *clientOptions) {
		if tc != nil {
			o.tlsConfig = tc
		}
	}
}

// WithClientQuicConfig sets quic config for the client.
func WithClientQuicConfig(qc *quic.Config) ClientOption {
	return func(o *clientOptions) {
		o.quicConfig = qc
	}
}

// WithReConnect makes client Connect until success, unless authentication fails.
func WithReConnect() ClientOption {
	return func(o *clientOptions) {
		o.reconnect = true
	}
}

// WithNonBlockWrite makes client WriteFrame non-blocking.
func WithNonBlockWrite() ClientOption {
	return func(o *clientOptions) {
		o.nonBlockWrite = true
	}
}

// WithLogger sets logger for the client.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(o *clientOptions) {
		o.logger = logger
	}
}

// WithTracerProvider sets tracer provider for the client.
func WithTracerProvider(tp trace.TracerProvider) ClientOption {
	return func(o *clientOptions) {
		o.tracerProvider = tp
	}
}

// WithAIFunctionDefinition sets AI function definition for the client.
func WithAIFunctionDefinition(description string, inputModel any) ClientOption {
	return func(o *clientOptions) {
		o.aiFunctionDescription = description
		o.aiFunctionInputModel = inputModel
	}
}

// qlog helps developers to debug quic protocol.
// See more: https://github.com/quic-go/quic-go?tab=readme-ov-file#quic-event-logging-using-qlog
func qlogTraceEnabled() bool {
	return strings.ToLower(os.Getenv("YOMO_QLOG_TRACE")) == "true"
}

func qlogTracer(ctx context.Context, p logging.Perspective, connID quic.ConnectionID) *logging.ConnectionTracer {
	role := "server"
	if p == logging.PerspectiveClient {
		role = "client"
	}
	filename := fmt.Sprintf("./log_%s_%s.qlog", connID, role)
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("qlog trace error: %s\n", err)
	}
	return qlog.NewConnectionTracer(f, p, connID)
}
