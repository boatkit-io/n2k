package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boatkit-io/tugboat/pkg/canbus"
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

	// Create SocketCAN channel directly (like working tests)
	options := canbus.SocketCANChannelOptions{
		InterfaceName:        canInterface,
		BitRate:              250000,
		ForceBounceInterface: false,
		MessageHandler: func(frame can.Frame) {
			log.Infof("Received frame: ID=0x%X, Length=%d, Data=%02X", frame.ID, frame.Length, frame.Data[:frame.Length])
		},
	}

	channel := canbus.NewSocketCANChannel(log, options)

	// Start the channel in a goroutine (like working tests)
	errChan := make(chan error, 1)
	go func() {
		errChan <- channel.Run(ctx)
	}()

	// Wait for channel to start
	time.Sleep(200 * time.Millisecond)

	log.Info("Channel started, beginning frame transmission...")

	// Test 1: Standard frames (like working tests)
	log.Info("Testing standard frames...")
	standardFrames := []can.Frame{
		{ID: 0x123, Length: 8, Data: [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
		{ID: 0x456, Length: 4, Data: [8]byte{0x11, 0x22, 0x33, 0x44}},
		{ID: 0x789, Length: 2, Data: [8]byte{0xAA, 0xBB}},
	}

	for i, frame := range standardFrames {
		err := channel.WriteFrame(frame)
		if err != nil {
			log.Errorf("Failed to write standard frame %d: %v", i, err)
		} else {
			log.Infof("Wrote standard frame %d: ID=0x%X", i, frame.ID)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Test 2: Extended frames (like working tests)
	log.Info("Testing extended frames...")
	extendedFrames := []can.Frame{
		{ID: 0x18FF1234 | 0x80000000, Length: 8, Data: [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
		{ID: 0x1AFF5678 | 0x80000000, Length: 6, Data: [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}},
		{ID: 0x1BFF9ABC | 0x80000000, Length: 4, Data: [8]byte{0xAA, 0xBB, 0xCC, 0xDD}},
	}

	for i, frame := range extendedFrames {
		err := channel.WriteFrame(frame)
		if err != nil {
			log.Errorf("Failed to write extended frame %d: %v", i, err)
		} else {
			log.Infof("Wrote extended frame %d: ID=0x%X", i, frame.ID)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Test 3: NMEA 2000 style frames (like n2k pipeline)
	log.Info("Testing NMEA 2000 style frames...")
	n2kFrames := []can.Frame{
		{ID: 0xFE0001FE, Length: 8, Data: [8]byte{0x01, 0x55, 0x24, 0x24, 0x06, 0x7F, 0xFF, 0xFF}},
		{ID: 0xFE0002FE, Length: 8, Data: [8]byte{0x01, 0x25, 0x02, 0x2D, 0x02, 0x00, 0xF0, 0xFF}},
		{ID: 0xFE0003FE, Length: 8, Data: [8]byte{0xE6, 0x24, 0x84, 0x16, 0x17, 0x4C, 0x08, 0xB7}},
	}

	for i, frame := range n2kFrames {
		err := channel.WriteFrame(frame)
		if err != nil {
			log.Errorf("Failed to write N2K frame %d: %v", i, err)
		} else {
			log.Infof("Wrote N2K frame %d: ID=0x%X", i, frame.ID)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Test 4: Continuous transmission (like spewpgns)
	log.Info("Starting continuous transmission...")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	frameCounter := 0
	for {
		select {
		case <-ctx.Done():
			log.Info("Context cancelled, stopping transmission")
			return
		case <-ticker.C:
			frameCounter++

			// Send a mix of frame types
			frames := []can.Frame{
				{ID: 0x123, Length: 8, Data: [8]byte{byte(frameCounter), 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
				{ID: 0x18FF1234 | 0x80000000, Length: 8, Data: [8]byte{byte(frameCounter), 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}},
				{ID: 0xFE0001FE, Length: 8, Data: [8]byte{byte(frameCounter), 0x55, 0x24, 0x24, 0x06, 0x7F, 0xFF, 0xFF}},
			}

			for i, frame := range frames {
				err := channel.WriteFrame(frame)
				if err != nil {
					log.Errorf("Failed to write continuous frame %d: %v", i, err)
				} else {
					log.Infof("Wrote continuous frame %d: ID=0x%X", i, frame.ID)
				}
			}

			log.Infof("Sent batch %d", frameCounter)
		}
	}
}
