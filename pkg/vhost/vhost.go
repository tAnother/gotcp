package vhost

import (
	"fmt"
	"iptcp-nora-yu/pkg/ipstack"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/proto"
	"log"
	"os"

	"iptcp-nora-yu/pkg/tcpstack"
)

var logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

type VHost struct {
	TCP *tcpstack.TCPGlobalInfo
}

func New(config *lnxconfig.IPConfig) (*VHost, error) {
	ipst, err := ipstack.Init(config)
	if err != nil {
		return nil, err
	}
	tcpst, err := tcpstack.Init(ipst)
	if err != nil {
		return nil, err
	}
	host := &VHost{tcpst}
	ipst.RegisterRecvHandler(proto.ProtoNumTest, testRecvHandler)
	return host, nil
}

func testRecvHandler(packet *proto.IPPacket) {
	fmt.Printf("Received test packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n",
		packet.Header.Src, packet.Header.Dst, packet.Header.TTL, packet.Payload)
}
