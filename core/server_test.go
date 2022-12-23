package core

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/ylog"
	yauth "github.com/yomorun/yomo/pkg/auth"
	"github.com/yomorun/yomo/pkg/config"
)

// tokenAuth be set up for unittest.
var tokenAuth = yauth.NewTokenAuth()

func init() {
	tokenAuth.Init("token-for-test")
}

type mockConnectorArgs struct {
	name        string
	clientID    string
	clientType  byte
	obversedTag frame.Tag
	connID      string
	stream      *streamAssert
}

// buildMockConnector build a mock connector according to `args`
// for preparing dataFrame testing.
func buildMockConnector(router router.Router, metadataBuilder metadata.Builder, args []mockConnectorArgs) Connector {
	logger := ylog.Default()

	connector := newConnector(logger)

	for _, arg := range args {

		handshakeFrame := frame.NewHandshakeFrame(
			arg.name,
			arg.clientID,
			arg.clientType,
			[]frame.Tag{arg.obversedTag},
			"token",
			"mock-token",
		)

		metadata, _ := metadataBuilder.Build(handshakeFrame)

		conn := newConnection(
			handshakeFrame.Name,
			handshakeFrame.ClientID,
			ClientType(handshakeFrame.ClientType),
			metadata,
			arg.stream,
			handshakeFrame.ObserveDataTags,
			logger,
		)

		route := router.Route(conn.Metadata())

		route.Add(arg.connID, arg.name, []frame.Tag{arg.obversedTag})

		connector.Add(arg.connID, conn)
	}

	return connector
}

func TestHandleDataFrame(t *testing.T) {
	metadataBuilder := metadata.DefaultBuilder()

	var (
		sfnStream1   = newStreamAssert([]byte{})
		sfnStream2   = newStreamAssert([]byte{})
		sourceStream = newStreamAssert([]byte{})
		zipperStream = newStreamAssert([]byte{})
	)

	sourceConnID := "source-conn-id-2"
	zipperConnID := "zipper-conn-id-2"

	routers := router.Default([]config.App{{Name: "sfn-1"}, {Name: "sfn-2"}})

	connector := buildMockConnector(routers, metadataBuilder, []mockConnectorArgs{
		{
			name:        "sfn-1",
			clientID:    "sfn-id-1",
			clientType:  byte(ClientTypeStreamFunction),
			obversedTag: 1,
			connID:      "sfn-conn-id-1",
			stream:      sfnStream1,
		},
		{
			name:        "sfn-2",
			clientID:    "sfn-id-2",
			clientType:  byte(ClientTypeStreamFunction),
			obversedTag: 2,
			connID:      "sfn-conn-id-2",
			stream:      sfnStream2,
		},
		{
			name:        "source-1",
			clientID:    sourceConnID,
			clientType:  byte(ClientTypeSource),
			obversedTag: 1,
			connID:      sourceConnID,
			stream:      sourceStream,
		},
		{
			name:        "zipper-1",
			clientID:    "zipper-id-2",
			clientType:  byte(ClientTypeUpstreamZipper),
			obversedTag: 1,
			connID:      zipperConnID,
			stream:      zipperStream,
		},
	})
	defer connector.Clean()

	server := &Server{connector: connector, logger: ylog.Default()}

	server.ConfigRouter(routers)
	server.ConfigMetadataBuilder(metadataBuilder)

	t.Run("test write data from source", func(t *testing.T) {
		var (
			payload = []byte("hello yomo")
			tag     = frame.Tag(1)
		)

		dataFrame := frame.NewDataFrame()
		dataFrame.SetCarriage(tag, payload)
		dataFrame.SetSourceID(sourceConnID)

		c := &Context{
			connID: sourceConnID,
			Stream: sourceStream,
			Frame:  dataFrame,
			Logger: server.logger,
		}

		err := server.handleDataFrame(c)
		assert.NoError(t, err, "server.handleDataFrame() should not return error")

		assert.Equal(t, server.StatsCounter(), int64(1))

		// sfn-1 obverse tag 1
		sfnStream1.writeEqual(t, dataFrame.Encode())

		// sfn-2 do not obverse tag 1
		sfnStream2.writeEqual(t, []byte{})

		t.Run("test response with BackflowFrame", func(t *testing.T) {
			err = server.handleBackflowFrame(c)
			assert.NoError(t, err)

			sourceStream.writeEqual(t, frame.NewBackflowFrame(tag, payload).Encode())
		})
	})

	t.Run("test write data from zipper", func(t *testing.T) {
		dataFrame := frame.NewDataFrame()
		dataFrame.SetCarriage(2, []byte("hello yomo"))
		dataFrame.SetSourceID(zipperConnID)

		c := &Context{
			connID: zipperConnID,
			Stream: zipperStream,
			Frame:  dataFrame,
			Logger: server.logger,
		}

		err := server.handleDataFrame(c)
		assert.NoError(t, err, "server.handleDataFrame() should not return error")

		assert.Equal(t, server.StatsCounter(), int64(2))

		sfnStream2.writeEqual(t, dataFrame.Encode())
	})

}

func TestHandShake(t *testing.T) {
	type args struct {
		clientID                 string
		token                    string
		clientType               byte
		stream                   *streamAssert
		clientName               string
		clientNameConfigInServer string
	}
	tests := []struct {
		name           string
		args           args
		handshakeTimes int
		wantResp       []byte
		wantAddConn    bool
		wantConnName   string
	}{
		{
			name: "test source: auth failed and return RejectFrame",
			args: args{
				clientID:                 "mock-client-id",
				token:                    "token-mock",
				clientType:               byte(ClientTypeSource),
				stream:                   newStreamAssert([]byte{}),
				clientName:               "source-mock",
				clientNameConfigInServer: "source-mock",
			},
			handshakeTimes: 1,
			wantResp:       frame.NewRejectedFrame("handshake authentication fails, client credential name is token").Encode(),
			wantAddConn:    false,
			wantConnName:   "",
		},
		{
			name: "test sfn: handshake success",
			args: args{
				clientID:                 "mock-client-id",
				token:                    "token-for-test", // equal `tokenAuth` token for passing auth
				clientType:               byte(ClientTypeStreamFunction),
				stream:                   newStreamAssert([]byte{}),
				clientName:               "sfn-1",
				clientNameConfigInServer: "sfn-1",
			},
			handshakeTimes: 1,
			wantResp:       frame.NewHandshakeAckFrame().Encode(),
			wantAddConn:    true,
			wantConnName:   "sfn-1",
		},
		{
			name: "test sfn: duplicate name and return GowayFrame",
			args: args{
				clientID:                 "mock-client-id",
				token:                    "token-for-test", // equal `tokenAuth` token for passing auth
				clientType:               byte(ClientTypeStreamFunction),
				stream:                   newStreamAssert([]byte{}),
				clientName:               "sfn-1",
				clientNameConfigInServer: "sfn-1",
			},
			handshakeTimes: 2,
			wantResp: composeFrametoBytes(
				frame.NewHandshakeAckFrame(), // first handshake.
				frame.NewGoawayFrame("SFN[sfn-1] is already linked to another connection"), // second handshake, write to old connection.
				frame.NewHandshakeAckFrame(), // second handshake, write to new connection.
			),
			wantAddConn:  true,
			wantConnName: "sfn-1",
		},
		{
			name: "test upstream zipper: handshake success",
			args: args{
				clientID:                 "mock-client-id",
				token:                    "token-for-test", // equal `tokenAuth` token for passing auth
				clientType:               byte(ClientTypeUpstreamZipper),
				stream:                   newStreamAssert([]byte{}),
				clientName:               "zipper-1",
				clientNameConfigInServer: "zipper-1",
			},
			handshakeTimes: 1,
			wantResp:       frame.NewHandshakeAckFrame().Encode(),
			wantAddConn:    true,
			wantConnName:   "zipper-1",
		},
		{
			name: "test unknown client: handshake failed",
			args: args{
				clientID:                 "mock-client-id",
				token:                    "token-for-test", // equal `tokenAuth` token for passing auth
				clientType:               0x7b,             // the unknown clientType
				stream:                   newStreamAssert([]byte{}),
				clientName:               "zipper-1",
				clientNameConfigInServer: "zipper-1",
			},
			handshakeTimes: 1,
			wantResp:       []byte("closed"),
			wantAddConn:    false,
			wantConnName:   "",
		},
	}

	logger := ylog.Default()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{connector: newConnector(logger), logger: logger, opts: defaultServerOptions()}

			server.ConfigRouter(router.Default([]config.App{{Name: tt.args.clientNameConfigInServer}}))

			server.opts.auths = map[string]auth.Authentication{
				tokenAuth.Name(): tokenAuth,
			}

			server.ConfigMetadataBuilder(metadata.DefaultBuilder())

			var (
				clientID   = tt.args.clientID
				token      = tt.args.token
				clientType = byte(tt.args.clientType)
				stream     = tt.args.stream
				clientName = tt.args.clientName
			)

			c := &Context{
				connID: clientID,
				Stream: stream,
				Frame:  frame.NewHandshakeFrame(clientName, clientID, clientType, []frame.Tag{frame.Tag(1)}, "token", token),
				Logger: server.logger,
			}

			for n := 0; n < tt.handshakeTimes; n++ {
				// TODO: this function should not return an error,
				// there maybe has a bug when unknown client type, because close connection
				// close first, then write goawayFrame to stream.
				server.handleHandshakeFrame(c)
			}

			// validate connector.
			conn := server.Connector().Get(clientID)

			addConn := conn != nil

			assert.Equal(t, tt.wantAddConn, addConn, "conn should be added to connector")

			if addConn {
				assert.Equal(t, tt.wantConnName, conn.Name())
			}

			// validate response to stream.
			stream.writeEqual(t, tt.wantResp)
		})
	}

}

// streamAssert implements `io.ReadWriteCloser`,
// It init from a byte array from test Read, `writeEqual` assert Write result.
type streamAssert struct {
	mu sync.Mutex
	r  *bytes.Buffer
	w  *bytes.Buffer
}

func newStreamAssert(initdata []byte) *streamAssert {
	w := bytes.NewBuffer(initdata)
	r := bytes.NewBuffer([]byte{})

	return &streamAssert{w: w, r: r}
}

func (s *streamAssert) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.r.Read(p)
}
func (s *streamAssert) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.w.Write(p)
}

func (s *streamAssert) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.w.Write([]byte("closed"))
	return err
}

func (s *streamAssert) writeEqual(t *testing.T, expected []byte, msgAndArgs ...interface{}) {
	assert.Equal(t, expected, s.w.Bytes(), msgAndArgs...)
}

// composeFrametoBytes compose frame.Encode to one byte list.
// It is a helper function for testing.
func composeFrametoBytes(frms ...frame.Frame) []byte {
	result := []byte{}
	for _, f := range frms {
		result = append(result, f.Encode()...)
	}
	return result
}
