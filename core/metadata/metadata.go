package metadata

import (
	"github.com/vmihailenco/msgpack/v5"
)

type M map[string]string

func New(data []byte) (M, error) {
	m := M{}
	return m, msgpack.Unmarshal(data, &m)
}

func (m M) Get(k string) (string, bool) {
	v, ok := m[k]
	return v, ok
}

func (m M) Set(k, v string) {
	if len(k) == 0 {
		return
	}
	m[k] = v
}

func (m M) Range(f func(k, v string) bool) {
	for k, v := range m {
		if !f(k, v) {
			break
		}
	}
}

func (m M) Delete(k string) {
	delete(m, k)
}

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

func (m M) Encode() ([]byte, error) {
	return msgpack.Marshal(m)
}
