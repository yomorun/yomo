package core

import (
	"github.com/yomorun/yomo/core/metadata"
)

// NewMetadata returns metadata for yomo working.
func NewMetadata(sourceID, tid string) metadata.M {
	md := metadata.M{
		metadata.SourceIDKey: sourceID,
		metadata.TIDKey:      tid,
	}
	return md
}

// GetTIDFromMetadata gets TID from metadata.
func GetTIDFromMetadata(m metadata.M) string {
	tid, _ := m.Get(metadata.TIDKey)
	return tid
}

// SetMetadataTarget sets target in metadata.
func SetMetadataTarget(m metadata.M, target string) {
	m.Set(metadata.TargetKey, target)
}
