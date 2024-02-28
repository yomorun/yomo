// Package metadata defines Metadata of the DataFrame.
package metadata

import (
	"github.com/vmihailenco/msgpack/v5"
)

// M stores additional information about the application.
//
//	There are two types of metadata in yomo:
//	 1. Metadata from `Authentication.Authenticate()`, This is connection-level metadata.
//	 2. Metadata from the DataFrame, This is frame-level metadata.
//
// the main responsibility of Metadata is to route messages to connection handler.
type M map[string]string

// New creates an M from a given key-values map.
func New(mds ...map[string]string) M {
	m := M{}
	for _, md := range mds {
		for k, v := range md {
			m.Set(k, v)
		}
	}
	return m
}

// Decode decodes a byte array to M.
func Decode(data []byte) (M, error) {
	m := M{}
	if len(data) == 0 {
		return m, nil
	}
	return m, msgpack.Unmarshal(data, &m)
}

// Get returns the value of the given key.
func (m M) Get(k string) (string, bool) {
	v, ok := m[k]
	return v, ok
}

// Set sets the value of the given key. if the key is empty, it will do nothing.
func (m M) Set(k, v string) {
	if len(k) == 0 {
		return
	}
	m[k] = v
}

// Range iterates over all keys and values.
func (m M) Range(f func(k, v string) bool) {
	for k, v := range m {
		if !f(k, v) {
			break
		}
	}
}

// Clone clones the metadata.
func (m M) Clone() M {
	if m == nil {
		return nil
	}
	if len(m) == 0 {
		return M{}
	}
	m2 := M{}
	for k, v := range m {
		m2.Set(k, v)
	}
	return m2
}

// Encode encodes the metadata to byte array.
func (m M) Encode() ([]byte, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return msgpack.Marshal(m)
}

// yomo reserved metadata keys.
const (
	// the keys for yomo working.
	SourceIDKey = "yomo-source-id"
	TIDKey      = "yomo-tid"

	// the keys for tracing.
	TraceIDKey = "yomo-trace-id"
	SpanIDKey  = "yomo-span-id"

	// the keys for target system working.
	TargetKey       = "yomo-target"
	WantedTargetKey = "yomo-wanted-target"
)
