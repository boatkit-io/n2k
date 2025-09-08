package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/boatkit-io/n2k/internal/adapter/canadapter"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/boatkit-io/n2k/internal/subscribe"
	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/sirupsen/logrus"
)

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing
	var canInterface string
	flag.StringVar(&canInterface, "iface", "", "CAN interface name (required)")
	flag.Parse()

	if canInterface == "" {
		fmt.Fprintf(os.Stderr, "Error: -iface flag is required\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -iface <interface_name>\n", os.Args[0])
		exitCode = 1
		return
	}

	log := logrus.StandardLogger()
	log.SetLevel(logrus.ErrorLevel) // Only show errors, not info messages

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info("Received shutdown signal, stopping...")
		cancel()
	}()

	// Build the pipeline
	subs := subscribe.New()
	adapter := canadapter.NewCANAdapter(log)
	packetStruct := pkt.NewPacketStruct()
	endpoint := socketcanendpoint.NewSocketCANEndpoint(log, canInterface)

	// Wire it all up
	endpoint.SetOutput(adapter)
	adapter.SetOutput(packetStruct)
	packetStruct.SetOutput(subs)
	adapter.SetWriter(endpoint)

	// Subscribe to all PGNs and dump them to stdout
	_, err := subs.SubscribeToAllStructs(func(p any) {
		fmt.Printf("%s\n", n2k.DebugDumpPGN(p))
	})
	if err != nil {
		log.Errorf("failed to subscribe to all structs: %v", err)
		exitCode = 1
		return
	}

	// Start the pipeline
	go func() {
		if err := endpoint.Run(ctx); err != nil {
			log.Errorf("endpoint exited with error: %v", err)
		}
	}()

	// Wait for context cancellation (from signal handler)
	<-ctx.Done()
	log.Info("Shutdown complete")
}
