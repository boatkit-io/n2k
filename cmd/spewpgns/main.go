// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package main sends sample NMEA 2000 PGNs on a SocketCAN interface.
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"

	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/boatkit-io/tugboat/pkg/units"

	"github.com/sirupsen/logrus"
)

// bus is the N2K service instance for sending and receiving PGNs.
var bus *n2k.N2kService

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
	bus = n2k.NewN2kService(endpoint, log)
	if err := bus.Start(ctx); err != nil {
		cancel()
		log.Errorf("Failed to start bus: %v", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: cancel() runs before exit
	}

	// 2. Create a PGN dumper to see all traffic
	_, err := bus.SubscribeToAllStructs(func(p any) {
		log.Infof("PGN DUMP: %s", n2k.DebugDumpPGN(p))
	})
	if err != nil {
		cancel()
		log.Errorf("failed to subscribe to all structs: %v", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: cancel() runs before exit
	}

	// Give the channel time to start up
	log.Infof("Starting to send test PGNs...")

	sendTestPGNs(log)

	// Wait for context cancellation
	<-ctx.Done()
	log.Info("Shutting down...")
	if err := bus.Stop(); err != nil {
		log.Errorf("Error stopping bus: %v", err)
	}
}

// sendTestPGNs sends a batch of test PGNs.
func sendTestPGNs(log *logrus.Logger) {
	sourceID := uint8(254) // Use reserved address for nodes unable to claim an address
	var counter float32

	// Generate and send different types of PGNs
	if err := sendEngineData(sourceID, counter, log); err != nil {
		log.Errorf("Failed to send engine data: %v", err)
	}

	if err := sendSpeedData(sourceID, counter, log); err != nil {
		log.Errorf("Failed to send speed data: %v", err)
	}

	if err := sendPositionData(sourceID, counter, log); err != nil {
		log.Errorf("Failed to send position data: %v", err)
	}
	if err := sendEngineInfo(sourceID, log); err != nil {
		log.Errorf("Failed to send ebgine data: %v", err)
	}

	log.Infof("Sent test PGNs batch %d", int(counter))
}

// sendEngineData sends an EngineParametersRapidUpdate PGN.
func sendEngineData(sourceID uint8, counter float32, log *logrus.Logger) error {
	// Generate realistic engine RPM (1000-2500 RPM with some variation)
	rpm := float32(1500.0 + 500.0*math.Sin(float64(counter)*0.1))
	// Boost pressure in Pascal (150 kPa = 150000 Pa)
	boostPressurePa := float32(150000.0 + 30000.0*math.Sin(float64(counter)*0.2))
	boostPressure := units.NewPressure(units.Pa, boostPressurePa)

	enginePgn := n2k.EngineParametersRapidUpdate{
		Info: n2k.MessageInfo{
			SourceId: sourceID,
			TargetId: 255, // Broadcast
		},
		Instance:      n2k.EngineInstanceConst(1),
		Speed:         &rpm,
		BoostPressure: &boostPressure,
		TiltTrim:      nil, // Not applicable for this test
	}

	log.Debugf("Sending engine data: RPM=%.1f, Boost=%.1f kPa", rpm, boostPressurePa/1000.0)
	return bus.Write(&enginePgn)
}

// sendSpeedData sends a Speed PGN.
func sendSpeedData(sourceID uint8, counter float32, log *logrus.Logger) error {
	// Generate realistic boat speeds (5-15 knots)
	speedKnots := 10.0 + 3.0*math.Sin(float64(counter)*0.15)
	speedMs := float32(speedKnots * 0.514444) // Convert knots to m/s

	waterSpeed := units.NewVelocity(units.MetersPerSecond, speedMs)
	groundSpeed := units.NewVelocity(units.MetersPerSecond, speedMs+0.2) // Slight current
	sid := uint8(1)
	direction := uint8(0) // Forward

	speedPgn := n2k.Speed{
		Info: n2k.MessageInfo{
			SourceId: sourceID,
			Priority: 0,   // Explicitly set priority to 0
			TargetId: 255, // Broadcast
		},
		Sid:                      &sid,
		SpeedWaterReferenced:     &waterSpeed,
		SpeedGroundReferenced:    &groundSpeed,
		SpeedWaterReferencedType: n2k.PaddleWheel,
		SpeedDirection:           &direction,
	}

	log.Debugf("Sending speed data: %.1f knots (%.2f m/s)", speedKnots, speedMs)
	return bus.Write(&speedPgn)
}

// sendEngineInfo sends a single frame EngineParametersRapidUpdate PGN for testing.
func sendEngineInfo(sourceID uint8, log *logrus.Logger) error {
	log.Info("Testing single frame PGN...")
	info1 := n2k.MessageInfo{
		PGN:      n2k.EngineParametersRapidUpdatePgn, // Engine Parameters Rapid Update
		SourceId: sourceID,
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
	return err
}

// sendPositionData sends a PositionRapidUpdate PGN.
func sendPositionData(sourceID uint8, counter float32, log *logrus.Logger) error {
	// Generate a position that moves in a small circle (simulating boat movement)
	// Starting position: approximately San Francisco Bay
	baseLat := 37.7749
	baseLon := -122.4194

	// Create a small circular motion (radius ~100 meters)
	angle := float64(counter) * 0.1
	deltaLat := 0.001 * math.Cos(angle) // About 111 meters per 0.001 degrees
	deltaLon := 0.001 * math.Sin(angle)

	latitude := baseLat + deltaLat
	longitude := baseLon + deltaLon

	positionPgn := n2k.PositionRapidUpdate{
		Info: n2k.MessageInfo{
			PGN:      n2k.PositionRapidUpdatePgn,
			SourceId: sourceID,
			TargetId: 0, // Broadcast
			Priority: 0x3,
		},
		Latitude:  &latitude,
		Longitude: &longitude,
	}

	log.Debugf("Sending position data: %.6f, %.6f", latitude, longitude)
	if err := bus.Write(&positionPgn); err != nil {
		return err
	}
	return nil
}
