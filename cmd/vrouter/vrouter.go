package main

import (
	"flag"
	"fmt"
	"iptcp-nora-yu/pkg/ipnode"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/repl"
	"time"
)

func main() {
	arg := flag.String("config", "", "specify the config file")
	flag.Parse()
	if *arg == "" {
		fmt.Println("usage: vrouter --config <lnx file>")
	}

	ipconfig, err := lnxconfig.ParseConfig(*arg)
	if err != nil {
		fmt.Println("usage: vrouter --config <lnx file>")
		return
	}

	router, err := ipnode.NewRouter(ipconfig)
	if err != nil {
		fmt.Println(err)
		return
	}

	repl := repl.NewRepl()

	// start listening on each interface
	for _, i := range router.Interfaces {
		go func(i *ipnode.Interface) {
			router.ListenOn(i)
		}(i)
	}

	// set up a ticker (5s) to send RIP to neighbors
	go func(router *ipnode.Node) {
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			router.SendRIPUpdate()
		}
	}(router)

	repl.Run(router)
}
