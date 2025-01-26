package ai

import (
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/serverless"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/pkg/listener/mem"
)

var _ yomo.Source = &memSource{}

type memSource struct {
	cred *auth.Credential
	conn *mem.FrameConn
}

func NewSource(conn *mem.FrameConn, cred *auth.Credential) yomo.Source {
	return &memSource{
		conn: conn,
		cred: cred,
	}
}

func (m *memSource) Connect() error {
	hf := &frame.HandshakeFrame{
		Name:        "fc-source",
		ID:          id.New(),
		ClientType:  byte(core.ClientTypeSource),
		AuthName:    m.cred.Name(),
		AuthPayload: m.cred.Payload(),
		Version:     core.Version,
	}

	return m.conn.Handshake(hf)
}

func (m *memSource) Write(tag uint32, data []byte) error {
	df := &frame.DataFrame{
		Tag:     tag,
		Payload: data,
	}
	return m.conn.WriteFrame(df)
}

func (m *memSource) Close() error                                       { return nil }
func (m *memSource) SetErrorHandler(_ func(_ error))                    {}
func (m *memSource) WriteWithTarget(_ uint32, _ []byte, _ string) error { return nil }

type memStreamFunction struct {
	observedTags []uint32
	handler      core.AsyncHandler
	cred         *auth.Credential
	conn         *mem.FrameConn
}

// NewReducer creates a new instance of memory StreamFunction.
func NewReducer(conn *mem.FrameConn, cred *auth.Credential) yomo.StreamFunction {
	return &memStreamFunction{
		conn: conn,
		cred: cred,
	}
}

func (m *memStreamFunction) Close() error {
	return nil
}

func (m *memStreamFunction) Connect() error {
	hf := &frame.HandshakeFrame{
		Name:            "fc-reducer",
		ID:              id.New(),
		ClientType:      byte(core.ClientTypeStreamFunction),
		AuthName:        m.cred.Name(),
		AuthPayload:     m.cred.Payload(),
		ObserveDataTags: m.observedTags,
		Version:         core.Version,
	}

	if err := m.conn.Handshake(hf); err != nil {
		return nil
	}

	go func() {
		for {
			f, err := m.conn.ReadFrame()
			if err != nil {
				return
			}

			switch ff := f.(type) {
			case *frame.DataFrame:
				go m.onDataFrame(ff)
			default:
				return
			}
		}
	}()

	return nil
}

func (m *memStreamFunction) onDataFrame(dataFrame *frame.DataFrame) {
	md, err := metadata.Decode(dataFrame.Metadata)
	if err != nil {
		return
	}

	serverlessCtx := serverless.NewContext(m.conn, dataFrame.Tag, md, dataFrame.Payload)
	m.handler(serverlessCtx)
}

func (m *memStreamFunction) SetHandler(fn core.AsyncHandler) error {
	m.handler = fn
	return nil
}

func (m *memStreamFunction) Init(_ func() error) error                         { return nil }
func (m *memStreamFunction) SetCronHandler(_ string, _ core.CronHandler) error { return nil }
func (m *memStreamFunction) SetErrorHandler(_ func(err error))                 {}
func (m *memStreamFunction) SetObserveDataTags(tags ...uint32)                 { m.observedTags = tags }
func (m *memStreamFunction) SetPipeHandler(fn core.PipeHandler) error          { return nil }
func (m *memStreamFunction) SetWantedTarget(string)                            {}
func (m *memStreamFunction) Wait()                                             {}

var _ yomo.StreamFunction = &memStreamFunction{}
