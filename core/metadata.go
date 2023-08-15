package core

import "github.com/yomorun/yomo/core/metadata"

const (
	MetadataSourceIDKey  = "yomo-source-id"
	MetadataBroadcastKey = "yomo-broadcast"
	MetadataTIDKey       = "yomo-tid"
	MetadataSIDKey       = "yomo-sid"
)

// NewDefaultMetadata returns a default metadata.
func NewDefaultMetadata(sourceID string, broadcast bool, tid string, sid string) metadata.M {
	broadcastString := "false"
	if broadcast {
		broadcastString = "true"
	}
	return metadata.M{
		MetadataSourceIDKey:  sourceID,
		MetadataBroadcastKey: broadcastString,
		MetadataTIDKey:       tid,
		MetadataSIDKey:       sid,
	}
}

// GetSourceIDFromMetadata gets sourceID from metadata.
func GetSourceIDFromMetadata(m metadata.M) string {
	sourceID, _ := m.Get(MetadataSourceIDKey)
	return sourceID
}

// GetBroadcastFromMetadata gets broadcast from metadata.
func GetBroadcastFromMetadata(m metadata.M) bool {
	broadcast, _ := m.Get(MetadataBroadcastKey)
	return broadcast == "true"
}

// GetTIDFromMetadata gets TID from metadata.
func GetTIDFromMetadata(m metadata.M) string {
	tid, _ := m.Get(MetadataTIDKey)
	return tid
}

// GetSIDFromMetadata gets SID from metadata.
func GetSIDFromMetadata(m metadata.M) string {
	sid, _ := m.Get(MetadataSIDKey)
	return sid
}

// SetTIDToMetadata sets tid to metadata.
func SetTIDToMetadata(m metadata.M, tid string) {
	m.Set(MetadataTIDKey, tid)
}

// SetSIDToMetadata sets sid to metadata.
func SetSIDToMetadata(m metadata.M, sid string) {
	m.Set(MetadataSIDKey, sid)
}
