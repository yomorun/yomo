package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlogwriter"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
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
	// ai function
	aiFunctionInputModel  any
	aiFunctionDescription string
	aiFunctionDefinition  string

	disableOtelTrace bool
}

// DefaultClientQuicConfig be used when the `quicConfig` of client is nil.
var DefaultClientQuicConfig = &quic.Config{
	Versions:                       []quic.Version{quic.Version1, quic.Version2},
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

// WithAIFunctionDefinition sets AI function definition for the client.
func WithAIFunctionDefinition(description string, inputModel any) ClientOption {
	return func(o *clientOptions) {
		o.aiFunctionDescription = description
		o.aiFunctionInputModel = inputModel
	}
}

// WithAIFunctionJsonDefinition sets AI function definition for the client in the form of jsonschema string.
func WithAIFunctionJsonDefinition(jsonschema string) ClientOption {
	return func(o *clientOptions) {
		o.aiFunctionDefinition = jsonschema
	}
}

// DisableOtelTrace determines whether to disable otel trace.
func DisableOtelTrace() ClientOption {
	return func(o *clientOptions) {
		o.disableOtelTrace = false
	}
}

// qlog helps developers to debug quic protocol.
// See more: https://quic-go.net/docs/quic/qlog
func qlogTraceEnabled() bool {
	return strings.ToLower(os.Getenv("YOMO_QLOG_TRACE")) == "true"
}

func qlogTracer(_ context.Context, isClient bool, connID quic.ConnectionID) qlogwriter.Trace {
	role := "server"
	if isClient {
		role = "client"
	}
	filename := fmt.Sprintf("./log_%s_%s.qlog", connID, role)
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("qlog trace error: %s\n", err)
	}
	return qlogwriter.NewFileSeq(f)
}
