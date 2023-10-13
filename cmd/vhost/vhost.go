package main

import (
	"flag"
	"fmt"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/repl"
)

func main() {
	// 0. read .lnx file from command line
	arg := flag.String("config", "", "specify the config file")
	flag.Parse()
	if *arg == "" {
		fmt.Println("usage: vhost --config <lnx file>")
		return
	}

	// parse .lnx file
	ipconfig, err := lnxconfig.ParseConfig(*arg)
	if err != nil {
		fmt.Println("usage: vhost --config <lnx file>")
		return
	}

	// 1. init a host ipnode, set up routing table, handlers etc.
	host, err := ipnode.NewHost(ipconfig)
	if err != nil {
		fmt.Println(err)
		return
	}

	// init a repl
	repl := repl.NewRepl()

	// 2. iterate all interfaces, start listening (go routines)
	for _, i := range host.Interfaces {
		go func(i *ipnode.Interface) {
			host.ListenOn(i) /// need to exit if conn initialization fails?
		}(i)
	}

	// 3. run the repl
	repl.Run(host)
}
