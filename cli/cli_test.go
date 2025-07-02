package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDoc(t *testing.T) {
	doc, err := Doc("serve")
	assert.NoError(t, err, "Doc should not return an error")
	assert.NotEmpty(t, doc, "Doc should not be empty")
	t.Log(doc)
}
