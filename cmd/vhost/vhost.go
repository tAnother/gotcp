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
	for _, i := range host.Interfaces {
		go func(i *ipnode.Interface) {
			host.ListenOn(i) /// need to exit if conn initialization fails?
		}(i)
	}

	repl := repl.NewRepl()
	repl.Run(host)
}
