package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test(t *testing.T) {
	var md = M{"aaa": "bbb"}

	t.Run("Get", func(t *testing.T) {
		got, ok := md.Get("aaa")
		assert.True(t, ok)
		assert.Equal(t, "bbb", got)
	})

	t.Run("Set", func(t *testing.T) {
		md.Set("ccc", "ddd")
		got, ok := md.Get("ccc")
		assert.True(t, ok)
		assert.Equal(t, "ddd", got)
	})

	t.Run("Set Empty key", func(t *testing.T) {
		md.Set("", "eee")
		got, ok := md.Get("")
		assert.False(t, ok)
		assert.Equal(t, "", got)

	})

	t.Run("Range", func(t *testing.T) {
		md2 := M{}

		md.Range(func(k string, v string) bool {
			md2.Set(k, v)
			return true
		})

		assert.Equal(t, md, md2)
	})

	t.Run("Range One key", func(t *testing.T) {
		md2 := M{}

		md.Range(func(k string, v string) bool {
			if k == "aaa" {
				md2.Set(k, v)
				return false
			}
			return true
		})

		got, ok := md2.Get("aaa")
		assert.True(t, ok)
		assert.Equal(t, "bbb", got)
	})

	t.Run("Clone", func(t *testing.T) {
		md2 := md.Clone()
		assert.Equal(t, md, md2)
	})

	t.Run("Clone Empty", func(t *testing.T) {
		md2 := M{}

		md3 := md2.Clone()
		assert.Equal(t, md2, md3)

		md4 := M(nil)
		md5 := md4.Clone()
		assert.Equal(t, md4, md5)
	})

	t.Run("Delete", func(t *testing.T) {
		md.Delete("aaa")
		got, ok := md.Get("aaa")
		assert.False(t, ok)
		assert.Equal(t, "", got)
	})

	t.Run("Encode", func(t *testing.T) {
		b, err := md.Encode()
		assert.NoError(t, err)

		md2, err := New(b)
		assert.NoError(t, err)

		assert.Equal(t, md, md2)
		t.Run("nil", func(t *testing.T) {
			md2, err := New(nil)
			assert.NoError(t, err)
			assert.Equal(t, M{}, md2)

			b, err := md2.Encode()
			assert.NoError(t, err)
			assert.Equal(t, []byte(nil), b)
		})
	})
}
