// Copyright (C) 2026 Boatkit
// SPDX-License-Identifier: MIT

// Package main validates a configuration-field write between two NMEA 2000
// nodes connected to the same SocketCAN interface.
package main

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/boatkit-io/n2k/pkg/node"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

type configurationProvider struct {
	mutex   sync.Mutex
	info    node.ConfigurationInfo
	updated chan node.ConfigurationInfo
}

func (p *configurationProvider) GetConfigurationInfo() (node.ConfigurationInfo, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.info, nil
}

func (p *configurationProvider) SetConfigurationInfo(info node.ConfigurationInfo) error {
	p.mutex.Lock()
	p.info = info
	p.mutex.Unlock()
	p.updated <- info
	return nil
}

func main() {
	iface := flag.String("iface", "vcan0", "SocketCAN interface shared by both nodes")
	flag.Parse()
	if err := run(*iface); err != nil {
		logrus.Fatal(err)
	}
}

func run(iface string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)

	senderService := n2k.NewN2kService(socketcanendpoint.NewSocketCANEndpoint(log, iface), log)
	receiverService := n2k.NewN2kService(socketcanendpoint.NewSocketCANEndpoint(log, iface), log)
	for _, service := range []*n2k.N2kService{senderService, receiverService} {
		if err := service.Start(ctx); err != nil {
			return err
		}
		defer service.Stop() //nolint:errcheck // Best-effort integration cleanup.
	}

	sender := node.NewFromService(senderService)
	receiver := node.NewFromService(receiverService)
	provider := &configurationProvider{
		info:    node.ConfigurationInfo{InstallationDescription1: "before", InstallationDescription2: "bench"},
		updated: make(chan node.ConfigurationInfo, 1),
	}
	receiver.SetConfigurationProvider(provider)
	if err := sender.SetDeviceInfo(deviceInfo(1001)); err != nil {
		return err
	}
	if err := receiver.SetDeviceInfo(deviceInfo(1002)); err != nil {
		return err
	}
	for _, n := range []*node.Node{sender, receiver} {
		if err := n.Start(); err != nil {
			return err
		}
		defer n.Stop() //nolint:errcheck // Best-effort integration cleanup.
	}
	if err := sender.ClaimAddress(40); err != nil {
		return err
	}
	if err := receiver.ClaimAddress(41); err != nil {
		return err
	}
	time.Sleep(750 * time.Millisecond)

	targetPGN := uint32(pgn.ConfigurationInformationPGN)
	selectionCount, parameterCount, parameter := uint8(0), uint8(1), uint8(1)
	write := &pgn.NMEAWriteFieldsGroupFunction{
		Info:         pgn.MessageInfo{PGN: pgn.NMEAWriteFieldsGroupFunctionPGN, SourceId: 40, TargetId: 41, Priority: 3},
		FunctionCode: pgn.WriteFields, PGN: &targetPGN,
		NumberOfSelectionPairs: &selectionCount, NumberOfParameters: &parameterCount,
		Repeating2: []pgn.NMEAWriteFieldsGroupFunctionRepeating2{{Parameter: &parameter, Value: encodeLAU("written over vcan")}},
	}
	if err := sender.WriteTo(write, 41); err != nil {
		return fmt.Errorf("send configuration write: %w", err)
	}

	select {
	case updated := <-provider.updated:
		if updated.InstallationDescription1 != "written over vcan" {
			return fmt.Errorf("unexpected configuration value %q", updated.InstallationDescription1)
		}
		fmt.Printf("PASS: node 40 changed node 41 InstallationDescription1 from %q to %q via %s\n", "before", updated.InstallationDescription1, iface)
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for configuration update")
	}
}

func deviceInfo(unique uint32) node.DeviceInfo {
	return node.DeviceInfo{UniqueNumber: unique, ManufacturerCode: pgn.Garmin, DeviceFunction: 140, DeviceClass: pgn.Navigation, IndustryGroup: pgn.MarineIndustry, ArbitraryAddressCapable: true}
}

func encodeLAU(value string) []byte {
	encoded := []byte{uint8(len(value) + 3), 1}
	encoded = append(encoded, value...)
	return append(encoded, 0)
}
