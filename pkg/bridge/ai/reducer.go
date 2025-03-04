package ai

import (
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/serverless"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/pkg/listener/mem"
)

var _ yomo.Source = &memSource{}

type memSource struct {
	id   string
	cred *auth.Credential
	conn *mem.FrameConn
}

func NewSource(conn *mem.FrameConn, cred *auth.Credential) yomo.Source {
	return &memSource{
		id:   id.New(),
		conn: conn,
		cred: cred,
	}
}

func (m *memSource) Connect() error {
	hf := &frame.HandshakeFrame{
		Name:        "fc-source",
		ID:          m.id,
		ClientType:  byte(core.ClientTypeSource),
		AuthName:    m.cred.Name(),
		AuthPayload: m.cred.Payload(),
		Version:     core.Version,
	}

	return m.conn.Handshake(hf)
}

func (m *memSource) Write(_ uint32, _ []byte) error  { panic("unimplemented") }
func (m *memSource) Close() error                    { panic("unimplemented") }
func (m *memSource) SetErrorHandler(_ func(_ error)) { panic("unimplemented") }

func (m *memSource) WriteWithTarget(tag uint32, data []byte, target string) error {
	md := core.NewMetadata(m.id, id.New())
	if target != "" {
		core.SetMetadataTarget(md, target)
	}
	mdBytes, _ := md.Encode()

	f := &frame.DataFrame{
		Tag:      tag,
		Metadata: mdBytes,
		Payload:  data,
	}

	return m.conn.WriteFrame(f)
}

type memStreamFunction struct {
	id           string
	observedTags []uint32
	handler      core.AsyncHandler
	cred         *auth.Credential
	conn         *mem.FrameConn
}

// NewReducer creates a new instance of memory StreamFunction.
func NewReducer(conn *mem.FrameConn, cred *auth.Credential) yomo.StreamFunction {
	return &memStreamFunction{
		id:   id.New(),
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
		ID:              m.id,
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
	serverlessCtx := serverless.NewContext(m.conn, dataFrame.Tag, core.NewMetadata(m.id, id.New()), dataFrame.Payload)
	m.handler(serverlessCtx)
}

func (m *memStreamFunction) SetHandler(fn core.AsyncHandler) error {
	m.handler = fn
	return nil
}

func (m *memStreamFunction) SetObserveDataTags(tags ...uint32) { m.observedTags = tags }
func (m *memStreamFunction) Init(_ func() error) error         { panic("unimplemented") }
func (m *memStreamFunction) SetCronHandler(_ string, _ core.CronHandler) error {
	panic("unimplemented")
}
func (m *memStreamFunction) SetErrorHandler(_ func(err error))        { panic("unimplemented") }
func (m *memStreamFunction) SetPipeHandler(fn core.PipeHandler) error { panic("unimplemented") }
func (m *memStreamFunction) SetWantedTarget(string)                   { panic("unimplemented") }
func (m *memStreamFunction) Wait()                                    {}

var _ yomo.StreamFunction = &memStreamFunction{}
