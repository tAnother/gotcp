package main

import (
	"flag"
	"fmt"
	"iptcp-nora-yu/pkg/ipstack"
	"iptcp-nora-yu/pkg/lnxconfig"
	"iptcp-nora-yu/pkg/vrouter"
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

	router, err := vrouter.New(ipconfig)
	if err != nil {
		fmt.Println(err)
		return
	}

	router.IP.Start()

	// send out request to fill routing table
	router.SendRipRequest()

	// set up a ticker (5s) to send RIP to neighbors
	go func(router *vrouter.VRouter) {
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			router.SendPeriodicRipUpdate()
		}
	}(router)

	// routing table garbage collection
	go func(router *vrouter.VRouter) {
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			router.RemoveExpiredEntries()
		}
	}(router)

	repl := ipstack.IpRepl(router.IP)
	repl.Run()
}
