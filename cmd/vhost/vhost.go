package main

import (
	"flag"
	"fmt"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/repl"
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

	host, err := ipnode.NewHost(ipconfig)
	if err != nil {
		fmt.Println(err)
		return
	}

	// start listening on each interface
	host.BindUDP()
	for _, i := range host.Interfaces {
		go host.ListenOn(i)
	}

	repl := repl.NewRepl()
	repl.Run(host)
}
