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
	yauth "github.com/yomorun/yomo/pkg/auth"
	"github.com/yomorun/yomo/pkg/config"
)

// tokenAuth be set up for unittest.
var tokenAuth = yauth.NewTokenAuth()

func init() {
	tokenAuth.Init("token-for-test")
}

func Test_HandShake(t *testing.T) {
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
			wantResp:       []byte{},
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
			wantResp:       frame.NewGoawayFrame("SFN[sfn-1] is already linked to another connection").Encode(),
			wantAddConn:    true,
			wantConnName:   "sfn-1",
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
			wantResp:       []byte{},
			wantAddConn:    true,
			wantConnName:   "zipper-1",
		},
		{
			name: "test unknown client: handshake failed",
			args: args{
				clientID:                 "mock-client-id",
				token:                    "token-for-test", // equal `tokenAuth` token for passing auth
				clientType:               0x7b,
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{connector: newConnector()}

			server.ConfigRouter(router.Default([]config.App{{Name: tt.args.clientNameConfigInServer}}))

			server.opts.Auths = map[string]auth.Authentication{
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
