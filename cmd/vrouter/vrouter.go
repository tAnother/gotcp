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
		fmt.Println("usage: vrouter --config <lnx file>")
	}

	// parse .lnx file
	ipconfig, err := lnxconfig.ParseConfig(*arg)
	if err != nil {
		fmt.Println("usage: vrouter --config <lnx file>")
		return
	}

	// 1. init a router ipnode, set up routing table, handlers etc.
	router, err := ipnode.NewRouter(ipconfig)
	if err != nil {
		fmt.Println(err)
		return
	}

	// init a repl
	repl := repl.NewRepl()

	// 2. iterate all interfaces, start listening (go routines)
	router.Node.InterfacesMu.RLock()
	for _, i := range router.Node.Interfaces {
		go func() {
			router.Node.ListenOn(i)
		}()
	}
	router.Node.InterfacesMu.RUnlock()

	// 3. set up a ticker (12s) to update routing table entries? (go routines)

	// 4. set up a ticker (5s) to send RIP to neighbors (go routines)

	// 5. run the repl
	repl.Run(&router.Node) /// seems sussy...
}
