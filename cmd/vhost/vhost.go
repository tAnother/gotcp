package main

import (
	"flag"
	"fmt"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/lnxconfig"
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

	host.Node.Start()

	repl := ipnode.IpRepl(host.Node)
	// repl, err := repl.CombineRepls([]*REPL{ipnode.IpRepl(host.Node), vhost.TcpRepl()})
	repl.Run()
}
