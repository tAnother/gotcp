package main

import (
	"flag"
	"fmt"
	"iptcp-nora-yu/pkg/ipstack"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/repl"
	"iptcp-nora-yu/pkg/tcpstack"
	"iptcp-nora-yu/pkg/vhost"
)

func main() {
	arg := flag.String("config", "", "specify the config file")
	flag.Parse()
	if *arg == "" {
		fmt.Println("usage: vhost --config <lnx file>")
		return
	}

	ipconfig, err := lnxconfig.ParseConfig(*arg)
	if err != nil {
		fmt.Println("usage: vhost --config <lnx file>")
		return
	}

	host, err := vhost.New(ipconfig)
	if err != nil {
		fmt.Println(err)
		return
	}

	host.TCP.IP.Start()

	repls := []*repl.REPL{ipstack.IpRepl(host.TCP.IP), tcpstack.TcpRepl(host.TCP)}
	combined, err := repl.CombineRepls(repls)
	if err != nil {
		fmt.Println(err)
		return
	}
	combined.Run()
}
