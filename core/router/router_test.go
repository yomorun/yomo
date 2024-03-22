package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/metadata"
)

func TestRouter(t *testing.T) {
	router := Default()

	err := router.Add(1, []uint32{1}, metadata.M{})
	assert.NoError(t, err)

	err = router.Add(2, []uint32{1}, metadata.M{})
	assert.NoError(t, err)

	err = router.Add(3, []uint32{1}, metadata.M{})
	assert.NoError(t, err)

	ids := router.Route(1, nil)
	assert.ElementsMatch(t, []uint64{1, 2, 3}, ids)

	router.Remove(1)

	ids = router.Route(1, nil)
	assert.ElementsMatch(t, []uint64{2, 3}, ids)

	router.Release()

	ids = router.Route(1, nil)
	assert.Equal(t, []uint64(nil), ids)
}

func TestTargetRouter(t *testing.T) {
	router := Default()

	err := router.Add(1, []uint32{1}, metadata.M{metadata.WantedTargetKey: "target-1"})
	assert.NoError(t, err)

	err = router.Add(2, []uint32{1}, metadata.M{metadata.WantedTargetKey: "target-1"})
	assert.NoError(t, err)

	err = router.Add(3, []uint32{1}, metadata.M{metadata.WantedTargetKey: "target-2"})
	assert.NoError(t, err)

	ids := router.Route(1, metadata.M{metadata.TargetKey: "target-1"})
	assert.ElementsMatch(t, []uint64{1, 2}, ids)

	ids = router.Route(1, metadata.M{})
	assert.ElementsMatch(t, []uint64{1, 2, 3}, ids)

	router.Release()

	ids = router.Route(1, nil)
	assert.Equal(t, []uint64(nil), ids)
}
