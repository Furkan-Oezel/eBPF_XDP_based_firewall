package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

func main() {
	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal("Removing memlock:", err)
	}

	// Load the compiled eBPF ELF and load it into the kernel.
	var objs firewallObjects
	if err := loadFirewallObjects(&objs, nil); err != nil {
		log.Fatal("Loading eBPF objects:", err)
	}

	mapPath := "/sys/fs/bpf/my_map" // Specify your desired path
	if err := objs.Map.Pin(mapPath); err != nil {
		log.Fatalf("Error pinning map: %s", err)
	}

	defer func() {
		if err := os.Remove(mapPath); err != nil {
			log.Printf("Error unpinning map: %s", err)
		}
	}()

	defer objs.Close()

	ifname := "eth0" // Change this to an interface on your machine.
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		log.Fatalf("Getting interface %s: %s", ifname, err)
	}

	// Attach count_packets to the network interface.
	link, err := link.AttachXDP(link.XDPOptions{
		Program:   objs.XdpFilterIpRange,
		Interface: iface.Index,
	})
	if err != nil {
		log.Fatal("Attaching XDP:", err)
	}
	defer link.Close()

	log.Printf("Counting incoming packets on %s..", ifname)

	// Periodically fetch the packet counter from Map(bpf map),
	// exit the program when interrupted.
	tick := time.Tick(time.Second)
	stop := make(chan os.Signal, 5)
	signal.Notify(stop, os.Interrupt)
	for {
		select {
		case <-tick:
			var count uint64
			err := objs.Map.Lookup(uint32(0), &count)
			if err != nil {
				log.Fatal("Map lookup:", err)
			}
			log.Printf("Received %b packets", count)
		case <-stop:
			log.Print("Received signal, exiting..")
			return
		}
	}
}
