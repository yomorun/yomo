package core

import "github.com/yomorun/yomo/core/metadata"

// NewDefaultMetadata returns a default metadata.
func NewDefaultMetadata(sourceID string, broadcast bool, tid string, sid string) metadata.M {
	broadcastString := "false"
	if broadcast {
		broadcastString = "true"
	}
	return metadata.M{
		metadata.SourceIDKey:  sourceID,
		metadata.BroadcastKey: broadcastString,
		metadata.TIDKey:       tid,
		metadata.SIDKey:       sid,
	}
}

// GetSourceIDFromMetadata gets sourceID from metadata.
func GetSourceIDFromMetadata(m metadata.M) string {
	sourceID, _ := m.Get(metadata.SourceIDKey)
	return sourceID
}

// GetBroadcastFromMetadata gets broadcast from metadata.
func GetBroadcastFromMetadata(m metadata.M) bool {
	broadcast, _ := m.Get(metadata.BroadcastKey)
	return broadcast == "true"
}

// GetTIDFromMetadata gets TID from metadata.
func GetTIDFromMetadata(m metadata.M) string {
	tid, _ := m.Get(metadata.TIDKey)
	return tid
}

// GetSIDFromMetadata gets SID from metadata.
func GetSIDFromMetadata(m metadata.M) string {
	sid, _ := m.Get(metadata.SIDKey)
	return sid
}
