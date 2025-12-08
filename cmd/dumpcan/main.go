package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
	log.SetLevel(logrus.DebugLevel) // Show info messages for better debugging

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
	endpoint := socketcanendpoint.NewSocketCANEndpoint(log, canInterface)

	// Wire it all up
	bus := n2k.NewN2kService(endpoint, log)

	// Start the pipeline
	if err := bus.Start(ctx); err != nil {
		log.Errorf("n2k service exited with error: %v", err)
		exitCode = 1
		return
	}

	// Subscribe to all PGNs and dump them to stdout
	_, err := bus.SubscribeToAllStructs(func(p any) {
		fmt.Printf("%s\n", n2k.DebugDumpPGN(p))
	})
	if err != nil {
		log.Errorf("failed to subscribe to all structs: %v", err)
		exitCode = 1
		return
	}

	// Wait for context cancellation (from signal handler)
	<-ctx.Done()
	log.Info("Shutting down...")
	if err := bus.Stop(); err != nil {
		log.Errorf("Error stopping bus: %v", err)
	}
	log.Info("Shutdown complete")
}
