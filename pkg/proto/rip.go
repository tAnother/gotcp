package proto

import (
	"fmt"
	"iptcp-nora-yu/pkg/util"
)

type RoutingCmdType uint16

const (
	RoutingCmdTypeRequest  RoutingCmdType = 1 // for request of routing info
	RoutingCmdTypeResponse RoutingCmdType = 2

	INFINITY uint32 = 16
)

type RipEntry struct {
	Cost    uint32 // <= 16. we define INFINITY to be 16
	Address uint32 // IP addr
	Mask    uint32 // subnet mask
}

type RipMsg struct {
	Command    RoutingCmdType
	NumEntries uint16 // <= 64 (and must be 0 for a request command)
	Entries    []*RipEntry
}

func (m *RipMsg) Marshal() ([]byte, error) {
	b := make([]byte, 0)
	var command []byte
	switch m.Command {
	case RoutingCmdTypeRequest:
		command = util.Uint16ToBytes(1)
	case RoutingCmdTypeResponse:
		command = util.Uint16ToBytes(2)
	default:
		return nil, fmt.Errorf("invalid command type")
	}
	numEntries := util.Uint16ToBytes(m.NumEntries)
	b = append(b, command...)
	b = append(b, numEntries...)
	for _, entry := range m.Entries {
		b = append(b, entry.marshal()...)
	}
	return b, nil
}

func (m *RipMsg) Unmarshal(b []byte) error {
	command := util.BytesToUint16(b[0:2])
	switch command {
	case 1:
		m.Command = RoutingCmdTypeRequest
	case 2:
		m.Command = RoutingCmdTypeResponse
	default:
		return fmt.Errorf("invalid routing command")
	}
	m.NumEntries = util.BytesToUint16(b[2:4])
	m.Entries = make([]*RipEntry, m.NumEntries)
	for i := 0; i < int(m.NumEntries); i++ {
		buf := b[4+i*12 : 4+(i+1)*12]
		err := m.Entries[i].Unmarshal(buf[0:12])
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RipEntry) marshal() []byte {
	b := make([]byte, 0)
	cost := util.Uint32ToBytes(r.Cost)
	addr := util.Uint32ToBytes(r.Address)
	mask := util.Uint32ToBytes(r.Mask)
	b = append(b, cost...)
	b = append(b, addr...)
	b = append(b, mask...)
	return b
}

func (r *RipEntry) Unmarshal(b []byte) error {
	if len(b) < 12 {
		return fmt.Errorf("invalid input")
	}
	r.Cost = util.BytesToUint32(b[0:4])
	r.Address = util.BytesToUint32(b[4:8])
	r.Mask = util.BytesToUint32(b[8:12])
	return nil
}
