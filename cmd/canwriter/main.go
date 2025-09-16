package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
)

func main() {
	var (
		iface      = flag.String("iface", "can0", "CAN interface name")
		count      = flag.Int("count", 10, "Number of frames to send")
		interval   = flag.Duration("interval", 100*time.Millisecond, "Interval between frames")
		verbose    = flag.Bool("verbose", false, "Enable verbose logging")
		baseID     = flag.Uint("baseid", 0x18F00400, "Base CAN ID (hex format supported)")
		pattern    = flag.String("pattern", "increment", "Data pattern: increment, random, fixed, nmea")
		sourceAddr = flag.Uint("source", 100, "NMEA 2000 source address (0-255)")
		pgn        = flag.Uint("pgn", 61184, "NMEA 2000 PGN for nmea pattern")
	)
	flag.Parse()

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Validate parameters
	if *sourceAddr > 255 {
		log.Fatalf("Source address must be 0-255, got %d", *sourceAddr)
	}

	// Open the CAN interface
	bus, err := can.NewBusForInterfaceWithName(*iface)
	if err != nil {
		log.Fatalf("Failed to open CAN interface %s: %v", *iface, err)
	}
	defer bus.Disconnect()

	logrus.Infof("Opened CAN interface %s", *iface)
	logrus.Infof("Sending %d frames with %v interval", *count, *interval)
	logrus.Infof("Base ID: 0x%X, Pattern: %s", *baseID, *pattern)

	if *pattern == "nmea" {
		logrus.Infof("NMEA 2000 mode: PGN=%d, Source=%d", *pgn, *sourceAddr)
	}

	// Generate and send frames
	successCount := 0
	for i := 0; i < *count; i++ {
		frame := generateFrame(i, *pattern, uint32(*baseID), uint8(*sourceAddr), uint32(*pgn))

		logrus.Debugf("Sending frame %d: ID=0x%X, Length=%d, Data=%02X",
			i+1, frame.ID, frame.Length, frame.Data[:frame.Length])

		err := bus.Publish(frame)
		if err != nil {
			log.Printf("Failed to send frame %d: %v", i+1, err)
			continue
		}

		successCount++
		fmt.Printf("Sent frame %d: ID=0x%X, Data=%02X\n",
			i+1, frame.ID, frame.Data[:frame.Length])

		if i < *count-1 {
			time.Sleep(*interval)
		}
	}

	logrus.Infof("Successfully sent %d/%d frames", successCount, *count)
}

// generateFrame creates a CAN frame based on the specified pattern
func generateFrame(index int, pattern string, baseID uint32, sourceAddr uint8, pgn uint32) can.Frame {
	var frame can.Frame

	switch pattern {
	case "nmea":
		frame = generateNMEAFrame(index, pgn, sourceAddr)
	case "random":
		frame = generateRandomFrame(index, baseID)
	case "fixed":
		frame = generateFixedFrame(index, baseID)
	default: // "increment"
		frame = generateIncrementFrame(index, baseID)
	}

	return frame
}

// generateNMEAFrame creates an NMEA 2000 compatible frame
func generateNMEAFrame(index int, pgn uint32, sourceAddr uint8) can.Frame {
	// NMEA 2000 frame ID format: Priority(3) + Reserved(1) + PGN(18) + Source(8)
	priority := uint32(6) // Normal priority
	frameID := (priority << 26) | (pgn << 8) | uint32(sourceAddr)

	// Set the Extended Frame Format (EFF) bit (bit 31) for 29-bit CAN ID
	// This is required for NMEA 2000 which uses extended CAN IDs
	frameID |= 0x80000000 // MaskEff from brutella/can constants

	frame := can.Frame{
		ID:     frameID,
		Length: 8,
	}

	// Sample NMEA 2000 data (could be engine RPM, speed, etc.)
	frame.Data[0] = uint8(index)                // Sequence counter
	frame.Data[1] = uint8((index * 100) & 0xFF) // RPM low byte
	frame.Data[2] = uint8((index * 100) >> 8)   // RPM high byte
	frame.Data[3] = 0xFF                        // Reserved
	frame.Data[4] = uint8(index * 2)            // Temperature
	frame.Data[5] = 0xFF                        // Reserved
	frame.Data[6] = 0xFF                        // Reserved
	frame.Data[7] = 0xFF                        // Reserved

	return frame
}

// generateRandomFrame creates a frame with random-ish data
func generateRandomFrame(index int, baseID uint32) can.Frame {
	frame := can.Frame{
		ID:     baseID + uint32(index),
		Length: 8,
	}

	// Pseudo-random data based on time and index
	seed := uint32(time.Now().UnixNano()) + uint32(index)
	for i := 0; i < 8; i++ {
		seed = seed*1103515245 + 12345 // Simple LCG
		frame.Data[i] = uint8(seed >> 16)
	}

	return frame
}

// generateFixedFrame creates a frame with fixed test pattern
func generateFixedFrame(index int, baseID uint32) can.Frame {
	frame := can.Frame{
		ID:     baseID + uint32(index),
		Length: 8,
	}

	// Fixed test pattern
	testPattern := []uint8{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}
	copy(frame.Data[:], testPattern)
	frame.Data[0] = uint8(index) // Keep index in first byte

	return frame
}

// generateIncrementFrame creates a frame with incrementing data
func generateIncrementFrame(index int, baseID uint32) can.Frame {
	frame := can.Frame{
		ID:     baseID + uint32(index),
		Length: 8,
	}

	// Fill with incrementing data
	frame.Data[0] = uint8(index)                    // Frame counter
	frame.Data[1] = uint8(index >> 8)               // High byte of counter
	frame.Data[2] = 0xAA                            // Test pattern
	frame.Data[3] = 0x55                            // Test pattern
	frame.Data[4] = uint8(time.Now().Unix() & 0xFF) // Timestamp byte
	frame.Data[5] = 0x12                            // Fixed test data
	frame.Data[6] = 0x34                            // Fixed test data
	frame.Data[7] = uint8((index * 3) & 0xFF)       // Calculated value

	return frame
}
