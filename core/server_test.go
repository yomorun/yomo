package core

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
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

		connectionFrame := &frame.ConnectionFrame{
			Name:            arg.name,
			ClientID:        arg.clientID,
			ClientType:      arg.clientType,
			ObserveDataTags: []frame.Tag{arg.obversedTag},
			Metadata:        []byte{},
		}

		metadata, _ := metadataBuilder.Build(connectionFrame)

		conn := newConnection(
			connectionFrame.Name,
			connectionFrame.ClientID,
			ClientType(connectionFrame.ClientType),
			metadata,
			arg.stream,
			connectionFrame.ObserveDataTags,
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
			conn:   &connection{clientID: sourceConnID},
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
			conn:   &connection{clientID: zipperConnID},
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

func (s *streamAssert) Context() context.Context { return context.TODO() }

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
