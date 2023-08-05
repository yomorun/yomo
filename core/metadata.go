package core

import "github.com/yomorun/yomo/core/metadata"

var (
	metadataSourceIDKey  = "yomo-source-id"
	metadataBroadcastKey = "yomo-broadcast"
	metadataTIDKey       = "yomo-tid"
)

// NewDefaultMetadata returns a default metadata.
func NewDefaultMetadata(sourceID string, broadcast bool, tid string) metadata.M {
	broadcastString := "false"
	if broadcast {
		broadcastString = "true"
	}
	return metadata.M{
		metadataSourceIDKey:  sourceID,
		metadataBroadcastKey: broadcastString,
		metadataTIDKey:       tid,
	}
}

// GetSourceIDFromMetadata gets sourceID from metadata.
func GetSourceIDFromMetadata(m metadata.M) string {
	sourceID, _ := m.Get(metadataSourceIDKey)
	return sourceID
}

// GetBroadcastFromMetadata gets broadcast from metadata.
func GetBroadcastFromMetadata(m metadata.M) bool {
	broadcast, _ := m.Get(metadataBroadcastKey)
	return broadcast == "true"
}

// GetTIDFromMetadata gets TID from metadata.
func GetTIDFromMetadata(m metadata.M) string {
	tid, _ := m.Get(metadataTIDKey)
	return tid
}
