package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/config"
)

func TestRouter(t *testing.T) {
	router := Default([]config.App{{Name: "sfn-1"}})

	m := &metadata.Default{}

	route := router.Route(m)

	err := route.Add("conn-1", "sfn-1", []frame.Tag{frame.Tag(1)})
	assert.NoError(t, err)

	ids := route.GetForwardRoutes(frame.Tag(1))
	assert.Equal(t, []string{"conn-1"}, ids)

	err = route.Add("conn-2", "sfn-2", []frame.Tag{frame.Tag(2)})
	assert.EqualError(t, err, "SFN[sfn-2] does not exist in config functions")

	err = route.Add("conn-3", "sfn-1", []frame.Tag{frame.Tag(1)})
	assert.EqualError(t, err, "SFN[sfn-1] is already linked to another stream")

	err = route.Remove("conn-1")
	assert.NoError(t, err)

	ids = route.GetForwardRoutes(frame.Tag(1))
	assert.Equal(t, []string{"conn-3"}, ids)

	router.Clean()

	ids = route.GetForwardRoutes(frame.Tag(1))
	assert.Equal(t, []string(nil), ids)
}
