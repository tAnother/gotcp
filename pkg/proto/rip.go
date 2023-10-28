package proto

import (
	"encoding/binary"
	"fmt"
)

const (
	INFINITY         uint32 = 16
	ripEntriesOffset int    = 4 // sizeof(command) + sizeof(numEntries)
)

type RipEntry struct {
	Cost    uint32 // <= 16. we define INFINITY to be 16
	Address uint32 // IP addr
	Mask    uint32 // subnet mask
}

type RoutingCmdType uint16

const (
	RoutingCmdTypeRequest  RoutingCmdType = 1 // for request of routing info
	RoutingCmdTypeResponse RoutingCmdType = 2
)

type RipMsg struct {
	Command    RoutingCmdType
	NumEntries uint16 // <= 64 (and must be 0 for a request command)
	Entries    []*RipEntry
}

func (m *RipMsg) Marshal() ([]byte, error) {
	b := make([]byte, 0)
	command := uint16ToBytes(uint16(m.Command))
	numEntries := uint16ToBytes(m.NumEntries)
	b = append(b, command...)
	b = append(b, numEntries...)
	for _, entry := range m.Entries {
		b = append(b, entry.marshal()...)
	}
	return b, nil
}

func (m *RipMsg) Unmarshal(b []byte) error {
	command := bytesToUint16(b[0:2])
	m.Command = RoutingCmdType(command)
	m.NumEntries = bytesToUint16(b[2:4])
	m.Entries = make([]*RipEntry, m.NumEntries)
	for i := 0; i < int(m.NumEntries); i++ {
		entryBytes := b[ripEntriesOffset+i*12 : ripEntriesOffset+(i+1)*12]
		m.Entries[i] = new(RipEntry)
		err := m.Entries[i].unmarshal(entryBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RipEntry) marshal() []byte {
	b := make([]byte, 0)
	cost := uint32ToBytes(r.Cost)
	addr := uint32ToBytes(r.Address)
	mask := uint32ToBytes(r.Mask)
	b = append(b, cost...)
	b = append(b, addr...)
	b = append(b, mask...)
	return b
}

func (r *RipEntry) unmarshal(b []byte) error {
	if len(b) < 12 {
		return fmt.Errorf("invalid input")
	}
	r.Cost = bytesToUint32(b[0:4])
	r.Address = bytesToUint32(b[4:8])
	r.Mask = bytesToUint32(b[8:12])
	return nil
}

func uint32ToBytes(u uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, u)
	return b
}

func bytesToUint32(u []byte) uint32 {
	if len(u) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(u)
}

func uint16ToBytes(u uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, u)
	return b
}

func bytesToUint16(u []byte) uint16 {
	if len(u) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(u)
}
