// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package main provides a SocketCAN integration exerciser for node behavior.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/boatkit-io/n2k/pkg/node"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

var canInterface string
var claimAddress = node.ReadOnlyAddress

type deviceListDumper struct {
	log         *logrus.Logger
	node        *node.Node
	localDevice *node.KnownDevice
	lastDump    string
}

func newDeviceListDumper(log *logrus.Logger, nodeImpl *node.Node) *deviceListDumper {
	return &deviceListDumper{
		log:  log,
		node: nodeImpl,
	}
}

func (d *deviceListDumper) setLocalDevice(
	address uint8,
	productInfo *node.ProductInfo,
	configurationInfo *node.ConfigurationInfo,
) {
	d.localDevice = &node.KnownDevice{
		Address: address,
		ProductInfo: &node.ProductInfo{
			NMEA2000Version:     productInfo.NMEA2000Version,
			ProductCode:         productInfo.ProductCode,
			ModelID:             productInfo.ModelID,
			SoftwareVersionCode: productInfo.SoftwareVersionCode,
			ModelVersion:        productInfo.ModelVersion,
			ModelSerialCode:     productInfo.ModelSerialCode,
			CertificationLevel:  productInfo.CertificationLevel,
			LoadEquivalency:     productInfo.LoadEquivalency,
		},
		ConfigInfo: &node.ConfigurationInfo{
			InstallationDescription1: configurationInfo.InstallationDescription1,
			InstallationDescription2: configurationInfo.InstallationDescription2,
			ManufacturerInformation:  configurationInfo.ManufacturerInformation,
		},
	}
}

func (d *deviceListDumper) DumpIfChanged(reason string) {
	dump := d.render()
	if dump == d.lastDump {
		return
	}
	d.lastDump = dump
	d.log.Infof("DEVICE LIST (%s):\n%s", reason, dump)
}

func (d *deviceListDumper) Dump(reason string) {
	d.log.Infof("DEVICE LIST (%s):\n%s", reason, d.render())
}

func (d *deviceListDumper) render() string {
	devices := d.node.KnownDevices()
	if d.localDevice != nil {
		devices = upsertLocalDevice(devices, d.localDevice)
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Address < devices[j].Address
	})

	if len(devices) == 0 {
		return "no devices discovered"
	}

	lines := make([]string, 0, len(devices))
	for i := range devices {
		lines = append(lines, formatKnownDevice(&devices[i]))
	}
	return strings.Join(lines, "\n")
}

func upsertLocalDevice(devices []node.KnownDevice, localDevice *node.KnownDevice) []node.KnownDevice {
	for i := range devices {
		if devices[i].Address == localDevice.Address {
			devices[i] = mergeKnownDevice(&devices[i], localDevice)
			return devices
		}
	}
	return append(devices, *localDevice)
}

func mergeKnownDevice(observed, localDevice *node.KnownDevice) node.KnownDevice {
	merged := *observed
	if merged.ProductInfo == nil {
		merged.ProductInfo = localDevice.ProductInfo
	}
	if merged.ConfigInfo == nil {
		merged.ConfigInfo = localDevice.ConfigInfo
	}
	return merged
}

func formatKnownDevice(device *node.KnownDevice) string {
	parts := []string{
		fmt.Sprintf("address=0x%02x(%d)", device.Address, device.Address),
	}
	if device.Name != 0 {
		parts = append(parts, fmt.Sprintf("name=0x%016x", device.Name))
	}
	if device.ProductInfo != nil {
		if device.ProductInfo.ModelID != "" {
			parts = append(parts, "model="+device.ProductInfo.ModelID)
		}
		if device.ProductInfo.ModelSerialCode != "" {
			parts = append(parts, "serial="+device.ProductInfo.ModelSerialCode)
		}
		if device.ProductInfo.SoftwareVersionCode != "" {
			parts = append(parts, "software="+device.ProductInfo.SoftwareVersionCode)
		}
	}
	if device.ConfigInfo != nil {
		if device.ConfigInfo.InstallationDescription1 != "" {
			parts = append(parts, "installation1="+device.ConfigInfo.InstallationDescription1)
		}
		if device.ConfigInfo.InstallationDescription2 != "" {
			parts = append(parts, "installation2="+device.ConfigInfo.InstallationDescription2)
		}
		if device.ConfigInfo.ManufacturerInformation != "" {
			parts = append(parts, "manufacturerInfo="+device.ConfigInfo.ManufacturerInformation)
		}
	}
	return "  - " + strings.Join(parts, ", ")
}

type staticConfigurationProvider struct {
	info node.ConfigurationInfo
}

func (p *staticConfigurationProvider) GetConfigurationInfo() (node.ConfigurationInfo, error) {
	return p.info, nil
}

func (p *staticConfigurationProvider) SetConfigurationInfo(info node.ConfigurationInfo) error {
	p.info = info
	return nil
}

func main() {
	if err := run(); err != nil {
		logrus.Fatal(err)
	}
}

func run() error {
	flag.StringVar(&canInterface, "iface", "", "CAN interface name for integration tests")
	flag.Func("address", "NMEA 2000 address to claim (0-253), or 255 for read-only monitoring", func(value string) error {
		parsed, err := strconv.ParseUint(value, 0, 8)
		if err != nil {
			return fmt.Errorf("invalid address %q: %w", value, err)
		}
		if parsed == 254 {
			return fmt.Errorf("address 254 is the null address and cannot be claimed")
		}
		claimAddress = uint8(parsed)
		return nil
	})
	flag.Parse()

	if canInterface == "" {
		canInterface = os.Getenv("IFACE")
	}

	if canInterface == "" {
		return fmt.Errorf("CAN interface not specified. Use -iface flag or IFACE environment variable")
	}

	log := logrus.StandardLogger()
	log.SetLevel(logrus.InfoLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint := socketcanendpoint.NewSocketCANEndpoint(log, canInterface)
	svc := n2k.NewN2kService(endpoint, log)

	_, err := svc.SubscribeToAllStructs(func(p any) {
		log.Infof("PGN DUMP: %s", n2k.DebugDumpPGN(p))
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to all structs: %w", err)
	}

	if err := svc.Start(ctx); err != nil {
		return fmt.Errorf("failed to start n2k service: %w", err)
	}
	defer func() {
		if err := svc.Stop(); err != nil {
			log.Errorf("failed to stop n2k service: %v", err)
		}
	}()

	log.Infof("Pipeline started on interface %s", canInterface)

	nodeImpl := node.NewFromService(svc)
	deviceDumper := newDeviceListDumper(log, nodeImpl)

	deviceInfo := node.DeviceInfo{
		UniqueNumber:            123456,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          140, // GPS
		DeviceClass:             pgn.Navigation,
		DeviceInstanceLower:     0,
		DeviceInstanceUpper:     0,
		SystemInstance:          0,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: true,
	}
	if err := nodeImpl.SetDeviceInfo(deviceInfo); err != nil {
		return fmt.Errorf("failed to set device info: %w", err)
	}

	log.Infof("Our device NAME: %x", nodeImpl.GetNetworkAddress())

	productInfo := node.ProductInfo{
		NMEA2000Version:     2100,
		ProductCode:         101,
		ModelID:             "Test Node v1",
		SoftwareVersionCode: "1.0.0",
		ModelVersion:        "v1",
		ModelSerialCode:     "SN-001",
		CertificationLevel:  1,
		LoadEquivalency:     1,
	}
	nodeImpl.SetProductInfo(productInfo)
	configurationInfo := node.ConfigurationInfo{
		InstallationDescription1: "integration helm",
		InstallationDescription2: "integration bench",
		ManufacturerInformation:  "boatkit nodeintegration",
	}
	nodeImpl.SetConfigurationProvider(&staticConfigurationProvider{
		info: configurationInfo,
	})
	nodeImpl.SetSupportedPGNs(
		[]uint32{
			pgn.ProductInformationPgn,
			pgn.ConfigurationInformationPgn,
		},
		[]uint32{
			pgn.IsoRequestPgn,
			pgn.NmeaRequestGroupFunctionPgn,
			pgn.NmeaCommandGroupFunctionPgn,
		},
	)
	nodeImpl.SetHeartbeatInterval(5 * time.Second)
	nodeImpl.EnableHeartbeat(true)

	if err := nodeImpl.Start(); err != nil {
		return fmt.Errorf("failed to start node: %w", err)
	}
	defer func() {
		if err := nodeImpl.Stop(); err != nil {
			log.Errorf("failed to stop node: %v", err)
		}
	}()

	if err := nodeImpl.ClaimAddress(claimAddress); err != nil {
		return fmt.Errorf("failed to claim address: %w", err)
	}

	readOnly := claimAddress == node.ReadOnlyAddress
	if readOnly {
		log.Info("Node started in read-only monitoring mode.")
	} else {
		log.Infof("Node started and address claim initiated for address %d.", claimAddress)
	}

	time.Sleep(2 * time.Second)

	switch {
	case readOnly:
		deviceDumper.DumpIfChanged("changed")
	case nodeImpl.GetNetworkAddress() == claimAddress:
		log.Infof("Address %d successfully claimed", claimAddress)
	default:
		log.Warnf("Address claim conflict occurred, requested address: %d, current address: %d",
			claimAddress, nodeImpl.GetNetworkAddress())
	}
	sourceAddress := nodeImpl.GetNetworkAddress()
	if !readOnly {
		deviceDumper.setLocalDevice(sourceAddress, &productInfo, &configurationInfo)
		deviceDumper.DumpIfChanged("changed")
	}

	if readOnly {
		log.Info("Skipping active request PGNs in read-only mode.")
	} else {
		sendActiveRequests(log, svc, sourceAddress)
	}

	log.Info("Running for 30 seconds to observe traffic...")
	observeDone := time.After(30 * time.Second)
	observeTicker := time.NewTicker(250 * time.Millisecond)
	for observing := true; observing; {
		select {
		case <-observeTicker.C:
			deviceDumper.DumpIfChanged("changed")
		case <-observeDone:
			observing = false
		}
	}
	observeTicker.Stop()
	deviceDumper.Dump("final")
	log.Info("Integration test finished.")
	return nil
}

func sendActiveRequests(log *logrus.Logger, svc *n2k.N2kService, sourceAddress uint8) {
	log.Info("Sending ISO Request for Address Claim (PGN 60928)")
	isoRequestAddrClaim := &pgn.IsoRequest{
		Pgn: ptrUint32(pgn.IsoAddressClaimPgn),
		Info: pgn.MessageInfo{
			PGN:      pgn.IsoRequestPgn,
			SourceId: sourceAddress,
			TargetId: 255,
			Priority: 6,
		},
	}
	if err := svc.Write(isoRequestAddrClaim); err != nil {
		log.Errorf("failed to write ISO request for address claim: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.Info("Sending ISO Request for Product Info (PGN 126996)")
	isoRequestProdInfo := &pgn.IsoRequest{
		Pgn: ptrUint32(pgn.ProductInformationPgn),
		Info: pgn.MessageInfo{
			PGN:      pgn.IsoRequestPgn,
			SourceId: sourceAddress,
			TargetId: 255,
			Priority: 6,
		},
	}
	if err := svc.Write(isoRequestProdInfo); err != nil {
		log.Errorf("failed to write ISO request for product info: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.Info("Sending ISO Request for PGN List (PGN 126464)")
	isoRequestPgnList := &pgn.IsoRequest{
		Pgn: ptrUint32(pgn.PgnListTransmitAndReceivePgn),
		Info: pgn.MessageInfo{
			PGN:      pgn.IsoRequestPgn,
			SourceId: sourceAddress,
			TargetId: 255,
			Priority: 6,
		},
	}
	if err := svc.Write(isoRequestPgnList); err != nil {
		log.Errorf("failed to write ISO request for PGN list: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.Info("Sending ISO Request for Configuration Info (PGN 126998)")
	isoRequestConfigInfo := &pgn.IsoRequest{
		Pgn: ptrUint32(pgn.ConfigurationInformationPgn),
		Info: pgn.MessageInfo{
			PGN:      pgn.IsoRequestPgn,
			SourceId: sourceAddress,
			TargetId: 255,
			Priority: 6,
		},
	}
	if err := svc.Write(isoRequestConfigInfo); err != nil {
		log.Errorf("failed to write ISO request for configuration info: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.Info("Sending NMEA Request Group Function for Product Info (PGN 126996)")
	zeroParameters := uint8(0)
	nmeaRequestProdInfo := &pgn.NmeaRequestGroupFunction{
		FunctionCode:       pgn.Request,
		Pgn:                ptrUint32(pgn.ProductInformationPgn),
		NumberOfParameters: &zeroParameters,
		Info: pgn.MessageInfo{
			PGN:      pgn.NmeaRequestGroupFunctionPgn,
			SourceId: sourceAddress,
			TargetId: sourceAddress,
			Priority: 3,
		},
	}
	if err := svc.Write(nmeaRequestProdInfo); err != nil {
		log.Errorf("failed to write NMEA request group function for product info: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.Info("Sending unsupported NMEA Command Group Function for Configuration Info (expect group-function NAK)")
	nmeaCommandConfigInfo := &pgn.NmeaCommandGroupFunction{
		FunctionCode:       pgn.Command,
		Pgn:                ptrUint32(pgn.ConfigurationInformationPgn),
		Priority:           pgn.Three,
		NumberOfParameters: &zeroParameters,
		Info: pgn.MessageInfo{
			PGN:      pgn.NmeaCommandGroupFunctionPgn,
			SourceId: sourceAddress,
			TargetId: sourceAddress,
			Priority: 3,
		},
	}
	if err := svc.Write(nmeaCommandConfigInfo); err != nil {
		log.Errorf("failed to write NMEA command group function for configuration info: %v", err)
	}
}

func ptrUint32(v uint32) *uint32 {
	return &v
}
