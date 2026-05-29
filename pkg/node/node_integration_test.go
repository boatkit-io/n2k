//go:build integration

package node

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

var canInterface string

func TestMain(m *testing.M) {
	flag.StringVar(&canInterface, "iface", "", "CAN interface name for integration tests")
	flag.Parse()

	if canInterface == "" {
		canInterface = os.Getenv("IFACE")
	}

	os.Exit(m.Run())
}

func TestNodeIntegration(t *testing.T) {
	if canInterface == "" {
		t.Skip("skipping integration test: -iface flag not provided")
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
		t.Fatalf("failed to subscribe to all structs: %v", err)
	}

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("failed to start n2k service: %v", err)
	}
	defer svc.Stop()

	log.Infof("Pipeline started on interface %s", canInterface)

	nodeImpl := NewFromService(svc)
	if n, ok := nodeImpl.(*node); ok {
		n.SetLogger(log)
	}

	deviceInfo := DeviceInfo{
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
		t.Fatalf("failed to set device info: %v", err)
	}

	log.Infof("Our device NAME: %x", nodeImpl.(*node).name)

	nodeImpl.SetProductInfo(ProductInfo{
		NMEA2000Version:     2100,
		ProductCode:         101,
		ModelID:             "Test Node v1",
		SoftwareVersionCode: "1.0.0",
		ModelVersion:        "v1",
		ModelSerialCode:     "SN-001",
		CertificationLevel:  1,
		LoadEquivalency:     1,
	})

	if err := nodeImpl.Start(); err != nil {
		t.Fatalf("failed to start node: %v", err)
	}
	defer nodeImpl.Stop()

	if err := nodeImpl.ClaimAddress(110); err != nil {
		t.Fatalf("failed to claim address: %v", err)
	}
	log.Info("Node started and address claim initiated for address 110.")

	time.Sleep(2 * time.Second)

	if nodeImpl.GetNetworkAddress() == 110 {
		log.Info("Address 110 successfully claimed")
	} else {
		log.Warnf("Address claim conflict occurred, current address: %d", nodeImpl.GetNetworkAddress())
	}

	log.Info("Sending ISO Request for Address Claim (PGN 60928)")
	isoRequestAddrClaim := &pgn.IsoRequest{
		Pgn: ptrUint32(60928),
		Info: pgn.MessageInfo{
			PGN:      59904,
			SourceId: nodeImpl.GetNetworkAddress(),
			Priority: 6,
		},
	}
	if err := svc.Write(isoRequestAddrClaim); err != nil {
		t.Errorf("failed to write ISO request for address claim: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.Info("Sending ISO Request for Product Info (PGN 126996)")
	isoRequestProdInfo := &pgn.IsoRequest{
		Pgn: ptrUint32(126996),
		Info: pgn.MessageInfo{
			PGN:      59904,
			SourceId: nodeImpl.GetNetworkAddress(),
			Priority: 6,
		},
	}
	if err := svc.Write(isoRequestProdInfo); err != nil {
		t.Errorf("failed to write ISO request for product info: %v", err)
	}

	log.Info("Running for 10 seconds to observe traffic...")
	time.Sleep(10 * time.Second)
	log.Info("Integration test finished.")
}

func ptrUint32(v uint32) *uint32 {
	return &v
}
