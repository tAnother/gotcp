package vrouter

import (
	"encoding/binary"
	"fmt"
	"iptcp-nora-yu/pkg/ipstack"
	"iptcp-nora-yu/pkg/proto"
	"math"
	"net"
	"net/netip"
)

// Create and marshal rip response to dest
func createRipResponse(entries []*ipstack.RoutingEntry, dest netip.Addr) ([]byte, error) {
	response := &proto.RipMsg{
		Command:    proto.RoutingCmdTypeResponse,
		NumEntries: uint16(len(entries)),
		Entries:    routingEntriesToRipEntries(entries, dest),
	}
	responseBytes, err := response.Marshal()
	if err != nil {
		return nil, fmt.Errorf("error marshaling rip msg: %v", err)
	}
	return responseBytes, nil
}

func routingEntriesToRipEntries(entries []*ipstack.RoutingEntry, dest netip.Addr) []*proto.RipEntry {
	ripEntries := make([]*proto.RipEntry, len(entries))
	for i, entry := range entries {
		ripEntries[i] = &proto.RipEntry{
			Address: ipToUint32(entry.Prefix.Addr()),
			Mask:    prefixLenToIPMask(entry.Prefix.Bits()),
		}
		if entry.NextHop == dest { // sending back to the original proposer - employ split horizon with poisoned reverse
			ripEntries[i].Cost = proto.INFINITY
		} else {
			ripEntries[i].Cost = uint32(entry.Cost)
		}
	}
	return ripEntries
}

func ripEntriesToRoutingEntries(entries []*proto.RipEntry, proposer netip.Addr) []*ipstack.RoutingEntry {
	routingEntries := make([]*ipstack.RoutingEntry, len(entries))
	for i, entry := range entries {
		routingEntries[i] = &ipstack.RoutingEntry{
			RouteType: ipstack.RouteTypeRIP,
			Prefix:    netip.PrefixFrom(uint32ToIp(entry.Address), ipMaskToPrefixLen(entry.Mask)),
			NextHop:   proposer,
			Cost:      min(entry.Cost+1, proto.INFINITY),
		}
	}
	return routingEntries
}

func uint32ToIp(u uint32) netip.Addr {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, u)
	ipBytes := []byte(ip)
	return netip.AddrFrom4([4]byte(ipBytes))
}

func ipToUint32(ip netip.Addr) uint32 {
	return binary.BigEndian.Uint32(ip.AsSlice())
}

func prefixLenToIPMask(b int) uint32 {
	return ^((1 << (32 - b)) - 1)
}

func ipMaskToPrefixLen(m uint32) int {
	return 32 - int(math.Log2(float64(^m+1)))
}
