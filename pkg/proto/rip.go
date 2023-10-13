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

type ripEntry struct {
	Cost    uint32 // <= 16. we define INFINITY to be 16
	Address uint32 // IP addr
	Mask    uint32 // subnet mask
}

type RIPMsg struct {
	Command    RoutingCmdType
	NumEntries uint16 // <= 64 (and must be 0 for a request command)
	Entries    []*ripEntry
}

func (m *RIPMsg) Marshal() []byte {
	b := make([]byte, 0)
	command := util.Uint16ToBytes(uint16(m.Command))
	numEntries := util.Uint16ToBytes(m.NumEntries)
	b = append(b, command...)
	b = append(b, numEntries...)
	for _, entry := range m.Entries {
		b = append(b, entry.Marshal()...)
	}
	return b
}

func RIPMsgUnMarshal(b []byte) (ripMsg *RIPMsg, err error) {
	command := util.BytesToUint16(b[0:2])
	switch command {
	case 1:
		ripMsg.Command = RoutingCmdTypeRequest
	case 2:
		ripMsg.Command = RoutingCmdTypeResponse
	default:
		return nil, fmt.Errorf("invalid command")
	}
	ripMsg.NumEntries = util.BytesToUint16(b[2:4])
	ripMsg.Entries = make([]*ripEntry, ripMsg.NumEntries)
	for i := 0; i < int(ripMsg.NumEntries); i++ {
		buf := b[4+i*12 : 4+(i+1)*12]
		ripMsg.Entries[i], err = ripEntryUnMarshal(buf[0:12])
		if err != nil {
			return ripMsg, err
		}
	}
	return ripMsg, nil
}

func (r *ripEntry) Marshal() []byte {
	b := make([]byte, 0)
	cost := util.Uint32ToBytes(r.Cost)
	addr := util.Uint32ToBytes(r.Address)
	mask := util.Uint32ToBytes(r.Mask)
	b = append(b, cost...)
	b = append(b, addr...)
	b = append(b, mask...)
	return b
}

func ripEntryUnMarshal(input []byte) (*ripEntry, error) {
	if len(input) < 12 {
		return nil, fmt.Errorf("invalid input")
	}
	return &ripEntry{
		Cost:    util.BytesToUint32(input[0:4]),
		Address: util.BytesToUint32(input[4:8]),
		Mask:    util.BytesToUint32(input[8:12]),
	}, nil
}
