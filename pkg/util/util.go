package util

import (
	"encoding/binary"
	"math"
	"net"
	"net/netip"
)

func Uint32ToIp(u uint32) netip.Addr {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, u)
	ipBytes := []byte(ip)
	return netip.AddrFrom4([4]byte(ipBytes))
}

func IpToUint32(ip netip.Addr) uint32 {
	return binary.BigEndian.Uint32(ip.AsSlice())
}

func PrefixLenToIPMask(b int) uint32 {
	return ^((1 << (32 - b)) - 1)
}

func IPMaskToPrefixLen(m uint32) int {
	return 32 - int(math.Log2(float64(^m+1)))
}

func Uint32ToBytes(u uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, u)
	return b
}

func BytesToUint32(u []byte) uint32 {
	if len(u) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(u)
}

func Uint16ToBytes(u uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, u)
	return b
}

func BytesToUint16(u []byte) uint16 {
	if len(u) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(u)
}
