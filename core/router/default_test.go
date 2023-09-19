package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

func TestRouter(t *testing.T) {
	router := Default()

	m := metadata.M{}

	route := router.Route(m)

	err := route.Add("conn-1", "sfn-1", []frame.Tag{frame.Tag(1)})
	assert.NoError(t, err)

	ids := route.GetForwardRoutes(frame.Tag(1))
	assert.Equal(t, []string{"conn-1"}, ids)

	err = route.Remove("conn-1")
	assert.NoError(t, err)

	ids = route.GetForwardRoutes(frame.Tag(1))
	assert.Equal(t, []string(nil), ids)

	router.Clean()

	ids = route.GetForwardRoutes(frame.Tag(1))
	assert.Equal(t, []string(nil), ids)
}
