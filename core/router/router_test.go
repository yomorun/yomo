package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/metadata"
)

func TestRouter(t *testing.T) {
	router := Default()

	err := router.Add("conn-1", []uint32{1}, metadata.M{})
	assert.NoError(t, err)

	err = router.Add("conn-2", []uint32{1}, metadata.M{})
	assert.NoError(t, err)

	err = router.Add("conn-3", []uint32{1}, metadata.M{})
	assert.NoError(t, err)

	ids := router.Route(1, nil)
	assert.ElementsMatch(t, []string{"conn-1", "conn-2", "conn-3"}, ids)

	router.Remove("conn-1")

	ids = router.Route(1, nil)
	assert.ElementsMatch(t, []string{"conn-2", "conn-3"}, ids)

	router.Release()

	ids = router.Route(1, nil)
	assert.Equal(t, []string(nil), ids)
}

func TestTargetRouter(t *testing.T) {
	router := Default()

	err := router.Add("conn-1", []uint32{1}, metadata.M{metadata.WantedTargetKey: "target-1"})
	assert.NoError(t, err)

	err = router.Add("conn-2", []uint32{1}, metadata.M{metadata.WantedTargetKey: "target-1"})
	assert.NoError(t, err)

	err = router.Add("conn-3", []uint32{1}, metadata.M{metadata.WantedTargetKey: "target-2"})
	assert.NoError(t, err)

	ids := router.Route(1, metadata.M{metadata.TargetKey: "target-1"})
	assert.ElementsMatch(t, []string{"conn-1", "conn-2"}, ids)

	ids = router.Route(1, metadata.M{})
	assert.ElementsMatch(t, []string{"conn-1", "conn-2", "conn-3"}, ids)

	router.Release()

	ids = router.Route(1, nil)
	assert.Equal(t, []string(nil), ids)
}
