package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/boatkit-io/n2k/pkg/node"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

var canInterface string

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
	flag.StringVar(&canInterface, "iface", "", "CAN interface name for integration tests")
	flag.Parse()

	if canInterface == "" {
		canInterface = os.Getenv("IFACE")
	}

	if canInterface == "" {
		logrus.Fatal("CAN interface not specified. Use -iface flag or IFACE environment variable")
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
		log.Fatalf("failed to subscribe to all structs: %v", err)
	}

	if err := svc.Start(ctx); err != nil {
		log.Fatalf("failed to start n2k service: %v", err)
	}
	defer svc.Stop()

	log.Infof("Pipeline started on interface %s", canInterface)

	nodeImpl := node.NewFromService(svc)

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
		log.Fatalf("failed to set device info: %v", err)
	}

	log.Infof("Our device NAME: %x", nodeImpl.GetNetworkAddress())

	nodeImpl.SetProductInfo(node.ProductInfo{
		NMEA2000Version:     2100,
		ProductCode:         101,
		ModelID:             "Test Node v1",
		SoftwareVersionCode: "1.0.0",
		ModelVersion:        "v1",
		ModelSerialCode:     "SN-001",
		CertificationLevel:  1,
		LoadEquivalency:     1,
	})
	nodeImpl.SetConfigurationProvider(&staticConfigurationProvider{
		info: node.ConfigurationInfo{
			InstallationDescription1: "integration helm",
			InstallationDescription2: "integration bench",
			ManufacturerInformation:  "boatkit nodeintegration",
		},
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
		log.Fatalf("failed to start node: %v", err)
	}
	defer nodeImpl.Stop()

	if err := nodeImpl.ClaimAddress(110); err != nil {
		log.Fatalf("failed to claim address: %v", err)
	}
	log.Info("Node started and address claim initiated for address 110.")

	time.Sleep(2 * time.Second)

	if nodeImpl.GetNetworkAddress() == 110 {
		log.Info("Address 110 successfully claimed")
	} else {
		log.Warnf("Address claim conflict occurred, current address: %d", nodeImpl.GetNetworkAddress())
	}
	sourceAddress := nodeImpl.GetNetworkAddress()

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

	log.Info("Running for 30 seconds to observe traffic...")
	time.Sleep(30 * time.Second)
	log.Info("Integration test finished.")
}

func ptrUint32(v uint32) *uint32 {
	return &v
}
