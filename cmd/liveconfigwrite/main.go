// Copyright (C) 2026 Boatkit
// SPDX-License-Identifier: MIT

// Package main writes Configuration Information installation strings to a live
// NMEA 2000 device.
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/boatkit-io/n2k/pkg/node"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

func main() {
	iface := flag.String("iface", "can0", "SocketCAN interface")
	target := flag.Uint("target", 0x59, "target node address")
	source := flag.Uint("source", 40, "source node address")
	description1 := flag.String("description1", "", "Installation Description 1")
	description2 := flag.String("description2", "", "Installation Description 2")
	flag.Parse()
	if *target > 253 || *source > 253 {
		logrus.Fatal("source and target must be valid claimed node addresses (0-253)")
	}
	if *description1 == "" && *description2 == "" {
		logrus.Fatal("provide -description1 and/or -description2")
	}
	if err := run(*iface, uint8(*source), uint8(*target), *description1, *description2); err != nil { //nolint:gosec // Bounds checked above.
		logrus.Fatal(err)
	}
}

func run(iface string, source, target uint8, description1, description2 string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := logrus.New()
	service := n2k.NewN2kService(socketcanendpoint.NewSocketCANEndpoint(log, iface), log)
	if err := service.Start(ctx); err != nil {
		return err
	}
	defer service.Stop() //nolint:errcheck // Best-effort cleanup.
	n := node.NewFromService(service)
	if err := n.SetDeviceInfo(node.DeviceInfo{
		UniqueNumber:            1001,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: true,
	}); err != nil {
		return err
	}
	if err := n.Start(); err != nil {
		return err
	}
	defer n.Stop() //nolint:errcheck // Best-effort cleanup.
	if err := n.ClaimAddress(source); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	for parameter, value := range map[uint8]string{1: description1, 2: description2} {
		if value == "" {
			continue
		}
		targetPGN, parameters := uint32(pgn.ConfigurationInformationPGN), uint8(1)
		parameterValue := parameter
		encodedValue, err := pgn.EncodeStringLAU(value)
		if err != nil {
			return fmt.Errorf("encode installation description %d: %w", parameter, err)
		}
		write := &pgn.NMEACommandGroupFunction{
			Info:         pgn.MessageInfo{PGN: pgn.NMEACommandGroupFunctionPGN, SourceId: source, TargetId: target, Priority: 3},
			FunctionCode: pgn.Command, PGN: &targetPGN, Priority: pgn.LeaveUnchanged, NumberOfParameters: &parameters,
			Repeating1: []pgn.NMEACommandGroupFunctionRepeating1{{Parameter: &parameterValue, Value: encodedValue}},
		}
		if err := n.WriteTo(write, target); err != nil {
			return fmt.Errorf("write installation description %d: %w", parameter, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Printf("sent configuration write(s) from 0x%02x to 0x%02x on %s\n", source, target, iface)
	return nil
}
