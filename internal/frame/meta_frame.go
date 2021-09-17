package frame

import (
	"fmt"

	"github.com/yomorun/y3"
)

const (
	// MetadataIssuer describes issuer
	MetadataIssuer = "_issuer_"
	// MetadataTID describes transaction ID
	MetadataTID = "_transaction-id_"
	// MetadataGID describes global ID
	MetadataGID = "_global-id_"
)

// MetaFrame holds metadatas with DataFrames
type MetaFrame interface {
	// Encode will encode to raw bytes
	Encode() []byte

	// Get will return the value of specified name
	Get(name string) string

	// Set add metadata
	Set(name string, val string)

	// GetMetadatas return all the metadatas
	GetMetadatas() []*Metadata

	// GetIssuer return issuer
	GetIssuer() string
}

// Metadata describles the structure.
type Metadata struct {
	// Name represents name.
	Name string
	// Value represents value.
	Value string
}

// NewMetadata create Metadata instance with given name and value.
func NewMetadata(name string, value string) *Metadata {
	return &Metadata{
		Name:  name,
		Value: value,
	}
}

// String returns string representation of the MetaFrame.
func (m Metadata) String() string {
	return fmt.Sprintf(`"%s":"%s"`, m.Name, m.Value)
}

// NewMetaFrame creates a new MetaFrame instance.
func NewMetaFrame(datas ...*Metadata) MetaFrame {
	// cleanup duplicated metadata
	cleanData := make([]*Metadata, 0)
	keys := make(map[string]byte)
	for _, d := range datas {
		l := len(keys)
		keys[d.Name] = 0
		// append metadata
		if len(keys) != l {
			cleanData = append(cleanData, d)
		} else { // update latest value
			for _, cd := range cleanData {
				if cd.Name == d.Name {
					cd.Value = d.Value
				}
			}
		}
	}

	return &metaFrame{
		data: cleanData,
	}
}

// MetaFrame defines the data structure of meta data in a `DataFrame`
type metaFrame struct {
	// globalID is the unique identifier of the global transaction
	// globalID string
	// // transactionID is the unique identifier of the transaction
	// transactionID string
	// // issuer issue this transaction
	// issuer string
	data []*Metadata
}

// NewMetaFrame creates a new MetaFrame with a given transactionID
// func NewMetaFrame(tid string, issuer string) *MetaFrame {
// 	return &MetaFrame{
// 		transactionID: tid,
// 		issuer:        issuer,
// 	}
// }

// TransactionID returns the transactionID of the MetaFrame
// func (m *MetaFrame) TransactionID() string {
// 	return m.transactionID
// }

// // Issuer returns the issuer of the MetaFrame
// func (m *MetaFrame) Issuer() string {
// 	return m.issuer
// }

// Encode returns Y3 encoded bytes of the MetaFrame
func (m *metaFrame) Encode() []byte {
	metaNode := y3.NewNodePacketEncoder(byte(TagOfMetaFrame))
	// TransactionID string
	// tidPacket := y3.NewPrimitivePacketEncoder(byte(TagOfTransactionID))
	// tidPacket.SetStringValue(m.transactionID)
	// // add TransactionID to MetaFrame
	// metaNode.AddPrimitivePacket(tidPacket)

	// // Issuer
	// issuerPacket := y3.NewPrimitivePacketEncoder(byte(TagOfIssuer))
	// issuerPacket.SetStringValue(m.issuer)
	// metaNode.AddPrimitivePacket(issuerPacket)
	for i, d := range m.data {
		// node
		node := y3.NewNodePacketEncoder(byte(i))
		// name
		name := y3.NewPrimitivePacketEncoder(0x01)
		name.SetStringValue(d.Name)
		node.AddPrimitivePacket(name)
		// value
		value := y3.NewPrimitivePacketEncoder(0x02)
		value.SetStringValue(d.Value)
		node.AddPrimitivePacket(value)
		//
		metaNode.AddNodePacket(node)
	}

	return metaNode.Encode()
}

func (m *metaFrame) Get(name string) string {
	for _, data := range m.data {
		if data.Name == name {
			return data.Value
		}
	}
	return ""
}

func (m *metaFrame) Set(name string, value string) {
	if len(m.data) == 0 {
		m.data = append(m.data, NewMetadata(name, value))
		return
	}
	keys := make(map[string]byte, 0)
	// update latest value
	for _, d := range m.data {
		keys[d.Name] = 0
		if d.Name == name {
			d.Value = value
		}
	}
	// append new metadata
	if _, ok := keys[name]; !ok {
		m.data = append(m.data, NewMetadata(name, value))
	}
}

func (m *metaFrame) GetMetadatas() []*Metadata {
	return m.data
}

func (m *metaFrame) GetIssuer() string {
	return m.Get(MetadataIssuer)
}

// DecodeToMetaFrame decodes Y3 encoded bytes to a MetaFrame
func DecodeToMetaFrame(buf []byte) (MetaFrame, error) {
	packet := &y3.NodePacket{}
	_, err := y3.DecodeToNodePacket(buf, packet)

	if err != nil {
		return nil, err
	}

	// var tid string
	// if s, ok := packet.PrimitivePackets[byte(TagOfTransactionID)]; ok {
	// 	tid, err = s.ToUTF8String()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	// var issuer string
	// if s, ok := packet.PrimitivePackets[byte(TagOfIssuer)]; ok {
	// 	issuer, err = s.ToUTF8String()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }
	// meta := &metaFrame{
	// 	transactionID: tid,
	// 	issuer:        issuer,
	// }
	// return meta, nil
	data := make([]*Metadata, 0)
	for _, p := range packet.NodePackets {
		md := Metadata{}
		if v, ok := p.PrimitivePackets[0x01]; ok {
			if name, err := v.ToUTF8String(); err == nil {
				md.Name = name
			}
		}
		if v, ok := p.PrimitivePackets[0x02]; ok {
			if value, err := v.ToUTF8String(); err == nil {
				md.Value = value
			}
		}
		data = append(data, &md)
	}

	return &metaFrame{
		data: data,
	}, nil
}
