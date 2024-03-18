package mock

import (
	"sync"

	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/guest"
)

var _ serverless.Context = (*MockContext)(nil)

// WriteRecord composes the data, tag and target.
type WriteRecord struct {
	Data   []byte
	Tag    uint32
	Target string
}

// MockContext mock context.
type MockContext struct {
	data []byte
	tag  uint32

	mu      sync.Mutex
	wrSlice []WriteRecord
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

func (c *MockContext) Metadata(_ string) (string, bool) {
	panic("not implemented")
}

func (m *MockContext) HTTP() serverless.HTTP {
	return &guest.GuestHTTP{}
}

func (c *MockContext) Write(tag uint32, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.wrSlice = append(c.wrSlice, WriteRecord{
		Data: data,
		Tag:  tag,
	})

	return nil
}

func (c *MockContext) WriteWithTarget(tag uint32, data []byte, target string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.wrSlice = append(c.wrSlice, WriteRecord{
		Data:   data,
		Tag:    tag,
		Target: target,
	})

	return nil
}

// RecordsWritten returns the data records be written with `ctx.Write`.
func (c *MockContext) RecordsWritten() []WriteRecord {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.wrSlice
}
