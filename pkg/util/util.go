package util

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
)

func BindLocalPort(bindPort uint16) (*net.UDPConn, error) {
	// Turn the address string into a UDPAddr for the connection
	bindAddrString := fmt.Sprintf(":%d", bindPort)
	bindLocalAddr, err := net.ResolveUDPAddr("udp4", bindAddrString)
	if err != nil {
		return nil, err
	}

	// Bind on the local UDP port:  this sets the source port
	// and creates a conn
	conn, err := net.ListenUDP("udp4", bindLocalAddr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func RemotePort(addrPort netip.AddrPort) (*net.UDPAddr, error) {
	addrString := fmt.Sprintf("%s:%d", addrPort.Addr().String(), addrPort.Port())
	remoteAddr, err := net.ResolveUDPAddr("udp4", addrString)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Sending to %s:%d\n",
		remoteAddr.IP.String(), remoteAddr.Port)
	return remoteAddr, nil
}

func Uint32ToIp(u uint32) netip.Addr {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, u)
	ipBytes := []byte(ip)
	return netip.AddrFrom4([4]byte(ipBytes))
}

func IpToUint32(ip netip.Addr) uint32 {
	return binary.BigEndian.Uint32(ip.AsSlice())
}

func Uint32ToBytes(u uint32) []byte {
	b := make([]byte, 0)
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
	b := make([]byte, 0)
	binary.BigEndian.PutUint16(b, u)
	return b
}

func BytesToUint16(u []byte) uint16 {
	if len(u) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(u)
}
