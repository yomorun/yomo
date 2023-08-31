package core

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/metadata"
	"golang.org/x/exp/slog"
)

func TestMetadata(t *testing.T) {
	md := NewDefaultMetadata("source", true, "xxxxxxx", "sssssss", true)

	assert.Equal(t, "source", GetSourceIDFromMetadata(md))
	assert.Equal(t, true, GetBroadcastFromMetadata(md))
	assert.Equal(t, "xxxxxxx", GetTIDFromMetadata(md))
	assert.Equal(t, "sssssss", GetSIDFromMetadata(md))
	assert.Equal(t, true, GetTracedFromMetadata(md))

	SetTIDToMetadata(md, "ccccccc")
	assert.Equal(t, "ccccccc", GetTIDFromMetadata(md))

	SetSIDToMetadata(md, "aaaaaaa")
	assert.Equal(t, "aaaaaaa", GetSIDFromMetadata(md))

	SetTracedToMetadata(md, false)
	assert.Equal(t, false, GetTracedFromMetadata(md))
}

func TestMetadataSlogAttr(t *testing.T) {
	md := metadata.New(map[string]string{
		"aaaa": "bbbb",
	})

	buf := bytes.NewBuffer(nil)

	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelDebug,
		// display time attr.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "time" {
				return slog.Attr{}
			}
			return a
		},
	}))

	logger.Debug("test metadata", MetadataSlogAttr(md))

	assert.Equal(t, "level=DEBUG msg=\"test metadata\" metadata.aaaa=bbbb\n", buf.String())
}
