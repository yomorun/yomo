package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviders(t *testing.T) {
	p1, _ := NewMock("name-1")
	p2, _ := NewMock("name-2")
	p3, _ := NewMock("name-3")

	RegisterProvider(p1)
	RegisterProvider(p2)
	RegisterProvider(p3)

	t.Run("ListProviders", func(t *testing.T) {
		val := ListProviders()
		expected := []string{"name-1", "name-2", "name-3"}
		assert.ElementsMatch(t, expected, val)
	})

	t.Run("GetProvider", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			p, err := GetProvider("name-1")
			assert.NoError(t, err)
			assert.Equal(t, p1, p)
		})
		t.Run("name is empty", func(t *testing.T) {
			p, err := GetProvider("")
			assert.NoError(t, err)
			assert.Equal(t, p1, p)
		})
		t.Run("not found", func(t *testing.T) {
			p, err := GetProvider("name-x")
			assert.ErrorIs(t, err, ErrNotExistsProvider)
			assert.Nil(t, p)
		})
	})

}
