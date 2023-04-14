package core

import (
	"crypto/tls"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
	"golang.org/x/exp/slog"
)

// ClientOption YoMo client options
type ClientOption func(*clientOptions)

// clientOptions are the options for YoMo client.
type clientOptions struct {
	observeDataTags     []frame.Tag
	quicConfig          *quic.Config
	tlsConfig           *tls.Config
	credential          *auth.Credential
	connectUntilSucceed bool
	nonBlockWrite       bool
	logger              *slog.Logger
}

func defaultClientOption() *clientOptions {
	logger := ylog.Default()

	defalutQuicConfig := &quic.Config{
		Versions:                       []quic.VersionNumber{quic.VersionDraft29, quic.Version1, quic.Version2},
		MaxIdleTimeout:                 time.Second * 40,
		KeepAlivePeriod:                time.Second * 20,
		MaxIncomingStreams:             1000,
		MaxIncomingUniStreams:          1000,
		HandshakeIdleTimeout:           time.Second * 3,
		InitialStreamReceiveWindow:     1024 * 1024 * 2,
		InitialConnectionReceiveWindow: 1024 * 1024 * 2,
		TokenStore:                     quic.NewLRUTokenStore(10, 5),
	}

	opts := &clientOptions{
		observeDataTags: make([]frame.Tag, 0),
		quicConfig:      defalutQuicConfig,
		tlsConfig:       pkgtls.MustCreateClientTLSConfig(),
		credential:      auth.NewCredential(""),
		logger:          logger,
	}

	return opts
}

// WithObserveDataTags sets data tag list for the client.
func WithObserveDataTags(tags ...frame.Tag) ClientOption {
	return func(o *clientOptions) {
		o.observeDataTags = tags
	}
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

// WithConnectUntilSucceed makes client Connect until seccssed.
func WithConnectUntilSucceed() ClientOption {
	return func(o *clientOptions) {
		o.connectUntilSucceed = true
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

// ClientType is equal to StreamType.
type ClientType = StreamType

const (
	// ClientTypeSource is equal to StreamTypeSource.
	ClientTypeSource ClientType = StreamTypeSource

	// ClientTypeUpstreamZipper is equal to StreamTypeUpstreamZipper.
	ClientTypeUpstreamZipper ClientType = StreamTypeUpstreamZipper

	// ClientTypeStreamFunction is equal to StreamTypeStreamFunction.
	ClientTypeStreamFunction ClientType = StreamTypeStreamFunction
)
