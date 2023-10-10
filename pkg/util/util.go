package util

import (
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
