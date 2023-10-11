package proto

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

func (m *RIPMsg) Marshal() ([]byte, error) {
	return nil, nil
}

func (m *RIPMsg) UnMarshal(b []byte) error {
	return nil
}
