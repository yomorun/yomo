package mock

import (
	"sync"

	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/guest"
)

type DataAndTag struct {
	Data []byte
	Tag  uint32
}

// MockContext mock context.
type MockContext struct {
	data []byte
	tag  uint32

	mu      sync.Mutex
	wrSlice []DataAndTag
}

// NewMockContext returns the mock context.
// the data is that returned by ctx.Data(), the tag is that returned by ctx.Tag().
func NewMockContext(data []byte, tag uint32) *MockContext {
	return &MockContext{
		data: data,
		tag:  tag,
	}
}

func (c *MockContext) Data() []byte {
	return c.data
}
func (c *MockContext) Tag() uint32 {
	return c.tag
}
func (m *MockContext) HTTP() serverless.HTTP {
	return &guest.GuestHTTP{}
}

func (c *MockContext) Write(tag uint32, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.wrSlice = append(c.wrSlice, DataAndTag{
		Data: data,
		Tag:  tag,
	})

	return nil
}

func (c *MockContext) RecordWritten() []DataAndTag {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.wrSlice
}
