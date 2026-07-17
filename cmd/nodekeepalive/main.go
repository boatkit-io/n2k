// Copyright (C) 2026 Boatkit
// SPDX-License-Identifier: MIT

// Package main keeps an NMEA 2000 node connected and claimed until stopped.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/boatkit-io/n2k/pkg/node"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

type configurationProvider struct {
	mutex sync.Mutex
	info  node.ConfigurationInfo
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
	logrus.Infof("configuration updated: description1=%q description2=%q manufacturer=%q", info.InstallationDescription1, info.InstallationDescription2, info.ManufacturerInformation)
	return nil
}

func main() {
	iface := flag.String("iface", "can0", "SocketCAN interface")
	address := flag.Uint("address", 40, "NMEA 2000 source address to claim (0-253), or 255 for read-only")
	flag.Parse()
	if *address > 253 && *address != uint(node.ReadOnlyAddress) {
		logrus.Fatalf("invalid address %d: use 0-253 or 255", *address)
	}
	if err := run(*iface, uint8(*address)); err != nil {
		logrus.Fatal(err)
	}
}

func run(iface string, address uint8) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := logrus.New()
	service := n2k.NewN2kService(socketcanendpoint.NewSocketCANEndpoint(log, iface), log)
	if err := service.Start(ctx); err != nil {
		return fmt.Errorf("start N2K service: %w", err)
	}
	defer service.Stop() //nolint:errcheck // Best-effort shutdown.

	n := node.NewFromService(service)
	n.SetProductInfo(node.ProductInfo{
		NMEA2000Version:     1,
		ProductCode:         1001,
		ModelID:             "n2k keepalive",
		SoftwareVersionCode: "dev",
		ModelVersion:        "dev",
		ModelSerialCode:     "keepalive",
		CertificationLevel:  1,
		LoadEquivalency:     1,
	})
	n.SetConfigurationProvider(&configurationProvider{info: node.ConfigurationInfo{
		InstallationDescription1: "keepalive installation 1",
		InstallationDescription2: "keepalive installation 2",
		ManufacturerInformation:  "boatkit n2k keepalive",
	}})
	if err := n.SetDeviceInfo(node.DeviceInfo{
		UniqueNumber:            1001,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: true,
	}); err != nil {
		return fmt.Errorf("set device info: %w", err)
	}
	if err := n.Start(); err != nil {
		return fmt.Errorf("start node: %w", err)
	}
	defer n.Stop() //nolint:errcheck // Best-effort shutdown.
	if err := n.ClaimAddress(address); err != nil {
		return fmt.Errorf("claim address: %w", err)
	}
	if address == node.ReadOnlyAddress {
		log.Infof("node is monitoring %s in read-only mode; press Ctrl-C to stop", iface)
	} else {
		log.Infof("node claimed address 0x%02x on %s; press Ctrl-C to stop", address, iface)
	}
	<-ctx.Done()
	return nil
}
