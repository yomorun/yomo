package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

<<<<<<< HEAD
func TestContextKeys(t *testing.T) {
=======
func Test_Context_Keys(t *testing.T) {
>>>>>>> 7baa084 (test: context keys)
	c := &Context{}

	t.Run("any keys", func(t *testing.T) {
		var (
			key   = "AnyKey"
			value = "any"
		)
		c.Set(key, value)

		got, ok := c.Get(key)

		assert.Equal(t, value, got)
		assert.Equal(t, true, ok)
	})

	t.Run("string keys", func(t *testing.T) {
		var (
			key   = "StringKey"
			value = "string"
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetString(key))
	})

	t.Run("bool keys", func(t *testing.T) {
		var (
			key   = "BoolKey"
			value = true
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetBool(key))
	})

	t.Run("int keys", func(t *testing.T) {
		var (
			key   = "IntKey"
			value = int(1)
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetInt(key))
	})

	t.Run("int64 keys", func(t *testing.T) {
		var (
			key   = "Int64Key"
			value = int64(2)
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetInt64(key))
	})

	t.Run("uint keys", func(t *testing.T) {
		var (
			key   = "UintKey"
			value = uint(3)
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetUint(key))
	})

	t.Run("uint64 keys", func(t *testing.T) {
		var (
			key   = "Uint64Key"
			value = uint64(4)
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetUint64(key))
	})

	t.Run("float64 keys", func(t *testing.T) {
		var (
			key   = "Float64Key"
			value = float64(5)
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetFloat64(key))
	})

	t.Run("time keys", func(t *testing.T) {
		var (
			key   = "TimeKey"
			value = timeForTest()
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetTime(key))
	})

	t.Run("duration keys", func(t *testing.T) {
		var (
			key   = "DurationKey"
			value = time.Second
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetDuration(key))
	})

	t.Run("[]string keys", func(t *testing.T) {
		var (
			key   = "StringSliceKey"
			value = []string{"a", "b", "c"}
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetStringSlice(key))
	})

	t.Run("map[string]interface{} keys", func(t *testing.T) {
		var (
			key   = "StringMapKey"
			value = map[string]interface{}{"aaa": 12}
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetStringMap(key))
	})

	t.Run("map[string]interface{} keys", func(t *testing.T) {
		var (
			key   = "StringMapStringKey"
			value = map[string]string{"aaa": "eee"}
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetStringMapString(key))
	})

	t.Run("map[string][]string keys", func(t *testing.T) {
		var (
			key   = "StringMapStringSliceKey"
			value = map[string][]string{"aaa": {"c", "d", "e"}}
		)
		c.Set(key, value)
		assert.Equal(t, value, c.GetStringMapStringSlice(key))
	})
}

func timeForTest() time.Time {
	result, _ := time.Parse("2006-01-02 15:04:05", "2021-10-09 15:21:16")
	return result
}
