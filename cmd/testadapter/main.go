package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/sirupsen/logrus"
)

func main() {
	var canInterface string
	flag.StringVar(&canInterface, "iface", "", "CAN interface name (required)")
	flag.Parse()

	if canInterface == "" {
		fmt.Fprintf(os.Stderr, "Error: -iface flag is required\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -iface <interface_name>\n", os.Args[0])
		os.Exit(1)
	}

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel) // Enable debug logging

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

	// Create SocketCANEndpoint
	endpoint := socketcanendpoint.NewSocketCANEndpoint(log, canInterface)
	bus := n2k.NewN2kService(endpoint, log)
	bus.Start(ctx)

	/* 	// Create CANAdapter
	   	adapter := canadapter.NewCANAdapter(log)
	   	subs := subscribe.New()
	   	pub := pgn.NewPublisher(adapter)
	   	ps := pkt.NewPacketStruct()
	   	ps.SetOutput(subs)
	   	adapter.SetOutput(ps)

	   	// Connect the adapter to the endpoint for writing frames
	   	adapter.SetWriter(endpoint)

	   	// Start the endpoint in a goroutine
	   	go func() {
	   		if err := endpoint.Run(ctx); err != nil {
	   			log.Errorf("endpoint exited with error: %v", err)
	   		}
	   	}( */
	// Wait for endpoint to start
	time.Sleep(200 * time.Millisecond)

	log.Info("Testing CANAdapter WritePgn...")

	// Test 1: Single frame PGN (should work)
	log.Info("Testing single frame PGN...")
	info1 := n2k.MessageInfo{
		PGN:      n2k.EngineParametersRapidUpdatePgn, // Engine Parameters Rapid Update
		SourceId: 0xFE,
		TargetId: 0x0,
		Priority: 0x3,
	}
	rpm := float32(1600)
	p := n2k.EngineParametersRapidUpdate{
		Info:     info1,
		Instance: n2k.SingleEngineOrDualEnginePort,
		Speed:    &rpm,
	}

	log.Debugf("Writing single frame PGN: PGN=0x%X, SourceId=0x%X", info1.PGN, info1.SourceId)
	err := bus.Write(&p)
	if err != nil {
		log.Errorf("Failed to write single frame PGN: %v", err)
	} else {
		log.Info("Successfully wrote single frame PGN")
	}

	time.Sleep(100 * time.Millisecond)

}
