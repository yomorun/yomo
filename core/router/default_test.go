package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouter(t *testing.T) {
	router := Default()

	err := router.Add(&RouteParams{ID: "conn-1", ObserveDataTags: []uint32{1}})
	assert.NoError(t, err)

	err = router.Add(&RouteParams{ID: "conn-2", ObserveDataTags: []uint32{1}})
	assert.NoError(t, err)

	err = router.Add(&RouteParams{ID: "conn-3", ObserveDataTags: []uint32{1}})
	assert.NoError(t, err)

	ids := router.Get(&RouteParams{ObserveDataTags: []uint32{1}})
	assert.ElementsMatch(t, []string{"conn-1", "conn-2", "conn-3"}, ids)

	err = router.Remove(&RouteParams{ID: "conn-1"})
	assert.NoError(t, err)

	ids = router.Get(&RouteParams{ObserveDataTags: []uint32{1}})
	assert.ElementsMatch(t, []string{"conn-2", "conn-3"}, ids)

	router.Release()

	ids = router.Get(&RouteParams{ObserveDataTags: []uint32{1}})
	assert.Equal(t, []string(nil), ids)
}
