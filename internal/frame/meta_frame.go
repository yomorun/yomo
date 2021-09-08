package frame

import (
	"fmt"
	"sync"

	"github.com/yomorun/y3"
)

const (
	MetadataIssuer = "_issuer_"
	MetadataTID    = "_transaction-id_"
	MetadataGID    = "_global-id_"
)

type MetaFrame interface {
	Encode() []byte
	Get(name string) string
	Set(name string, val string)
	GetMetadatas() []*Metadata
	GetIssuer() string
}

type Metadata struct {
	Name  string
	Value string
}

func NewMetadata(name string, value string) *Metadata {
	return &Metadata{
		Name:  name,
		Value: value,
	}
}

func (m Metadata) String() string {
	return fmt.Sprintf(`"%s":"%s"`, m.Name, m.Value)
}

func NewMetaFrame(datas ...*Metadata) MetaFrame {
	// cleanup duplicated metadata
	cleanData := make([]*Metadata, 0)
	keys := make(map[string]byte, 0)
	for _, d := range datas {
		l := len(keys)
		keys[d.Name] = 0
		if len(keys) != l {
			cleanData = append(cleanData, d)
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
	mu   sync.Mutex
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

	for _, data := range m.data {
		if data.Name == name {
			data.Value = value
		} else {
			m.data = append(m.data, NewMetadata(name, value))
		}
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
