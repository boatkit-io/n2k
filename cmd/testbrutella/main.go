package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brutella/can"
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
	log.SetLevel(logrus.InfoLevel)

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

	// Create bus directly using brutella/can
	bus, err := can.NewBusForInterfaceWithName(canInterface)
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}

	// Set up message handler
	bus.Subscribe(can.NewHandler(func(frame can.Frame) {
		log.Infof("Received frame: ID=0x%X, Length=%d, Data=%02X", frame.ID, frame.Length, frame.Data[:frame.Length])
	}))

	// Start the bus in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- bus.ConnectAndPublish()
	}()

	// Wait for bus to start
	time.Sleep(500 * time.Millisecond)

	log.Info("Bus started, beginning frame transmission...")

	// Test frames
	testFrames := []can.Frame{
		{ID: 0x123, Length: 8, Data: [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
		{ID: 0x456, Length: 4, Data: [8]byte{0x11, 0x22, 0x33, 0x44}},
		{ID: 0x18FF1234 | 0x80000000, Length: 8, Data: [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
	}

	for i, frame := range testFrames {
		log.Infof("Publishing frame %d: ID=0x%X", i, frame.ID)
		err := bus.Publish(frame)
		if err != nil {
			log.Errorf("Failed to publish frame %d: %v", i, err)
		} else {
			log.Infof("Successfully published frame %d", i)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for context cancellation
	select {
	case <-ctx.Done():
		log.Info("Context cancelled, stopping...")
	case err := <-errChan:
		log.Errorf("Bus error: %v", err)
	}

	// Disconnect
	if err := bus.Disconnect(); err != nil {
		log.Errorf("Failed to disconnect bus: %v", err)
	}
}
