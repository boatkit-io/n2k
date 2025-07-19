//go:build integration

package node

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/boatkit-io/n2k/pkg/subscribe"
	"github.com/sirupsen/logrus"
)

var canInterface string

func TestMain(m *testing.M) {
	flag.StringVar(&canInterface, "iface", "", "CAN interface name for integration tests")
	flag.Parse()
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

	// 1. Build the pipeline
	subs := subscribe.New()
	adapter := canadapter.NewCANAdapter(log)
	publisher := pgn.NewPublisher(adapter)
	packetStruct := pkt.NewPacketStruct()

	endpoint, err := socketcanendpoint.NewSocketCANEndpoint(canInterface, log)
	if err != nil {
		t.Fatalf("failed to create socketcan endpoint: %v", err)
	}

	// Wire it all up
	endpoint.SetOutput(adapter)
	adapter.SetOutput(packetStruct)
	packetStruct.SetOutput(subs)
	adapter.SetWriter(endpoint)

	// 2. Create a PGN dumper to see all traffic
	_, err = subs.SubscribeToAllStructs(func(p any) {
		log.Infof("PGN DUMP: %s", pgn.DebugDumpPGN(p))
	})
	if err != nil {
		t.Fatalf("failed to subscribe to all structs: %v", err)
	}

	// 3. Start the pipeline
	go func() {
		if err := endpoint.Run(ctx); err != nil {
			log.Errorf("endpoint exited with error: %v", err)
		}
	}()
	log.Infof("Pipeline started on interface %s", canInterface)

	// 4. Instantiate and configure the node
	node := New(subs, publisher)
	node.SetLogger(log)

	deviceInfo := DeviceInfo{
		UniqueNumber:            123456,
		ManufacturerCode:        pgn.ManufacturerCode_Garmin,
		DeviceFunction:          140, // GPS
		DeviceClass:             pgn.DeviceClass_Navigation,
		DeviceInstanceLower:     0,
		DeviceInstanceUpper:     0,
		SystemInstance:          0,
		IndustryGroup:           pgn.IndustryGroup_Marine,
		ArbitraryAddressCapable: true,
	}
	if err := node.SetDeviceInfo(deviceInfo); err != nil {
		t.Fatalf("failed to set device info: %v", err)
	}

	node.SetProductInfo(ProductInfo{
		NMEA2000Version:     2100,
		ProductCode:         101,
		ModelID:             "Test Node v1",
		SoftwareVersionCode: "1.0.0",
		ModelVersion:        "v1",
		ModelSerialCode:     "SN-001",
		CertificationLevel:  1,
		LoadEquivalency:     1,
	})

	// 5. Start the node and claim an address
	if err := node.Start(); err != nil {
		t.Fatalf("failed to start node: %v", err)
	}
	defer node.Stop()

	if err := node.ClaimAddress(55); err != nil {
		t.Fatalf("failed to claim address: %v", err)
	}
	log.Info("Node started and address claim initiated for address 55.")

	// Give the node a moment to claim its address
	time.Sleep(2 * time.Second)

	// 6. Send ISO requests to the bus
	log.Info("Sending ISO Request for Address Claim (PGN 60928)")
	isoRequestAddrClaim := &pgn.IsoRequest{
		PGN:          60928, // ISO Address Claim
		SourceNodeId: node.GetNetworkAddress(),
	}
	if err := publisher.Write(isoRequestAddrClaim); err != nil {
		t.Errorf("failed to write ISO request for address claim: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.Info("Sending ISO Request for Product Info (PGN 126996)")
	isoRequestProdInfo := &pgn.IsoRequest{
		PGN:          126996, // Product Information
		SourceNodeId: node.GetNetworkAddress(),
	}
	if err := publisher.Write(isoRequestProdInfo); err != nil {
		t.Errorf("failed to write ISO request for product info: %v", err)
	}

	// 7. Run for a while to observe traffic
	log.Info("Running for 10 seconds to observe traffic...")
	time.Sleep(10 * time.Second)
	log.Info("Integration test finished.")
}
