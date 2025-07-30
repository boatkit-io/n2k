package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/node"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/boatkit-io/n2k/pkg/subscribe"
	"github.com/sirupsen/logrus"
)

var canInterface string

func main() {
	flag.StringVar(&canInterface, "iface", "", "CAN interface name for integration tests")
	flag.Parse()

	// Also check environment variable
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

	// 1. Build the pipeline
	subs := subscribe.New()
	adapter := canadapter.NewCANAdapter(log)
	publisher := pgn.NewPublisher(adapter)
	packetStruct := pkt.NewPacketStruct()

	endpoint := socketcanendpoint.NewSocketCANEndpoint(log, canInterface)

	// Wire it all up
	endpoint.SetOutput(adapter)
	adapter.SetOutput(packetStruct)
	packetStruct.SetOutput(subs)
	adapter.SetWriter(endpoint)

	// 2. Create a PGN dumper to see all traffic
	_, err := subs.SubscribeToAllStructs(func(p any) {
		log.Infof("PGN DUMP: %s", pgn.DebugDumpPGN(p))
	})
	if err != nil {
		log.Fatalf("failed to subscribe to all structs: %v", err)
	}

	// 3. Start the pipeline
	go func() {
		if err := endpoint.Run(ctx); err != nil {
			log.Errorf("endpoint exited with error: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	log.Infof("Pipeline started on interface %s", canInterface)

	// 4. Instantiate and configure the node
	nodeImpl := node.NewNode(subs, &publisher, nil)
	// Note: SetLogger is not available on the interface, so we skip it for now

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

	// Log our computed NAME for debugging
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

	// 5. Start the node and claim an address
	if err := nodeImpl.Start(); err != nil {
		log.Fatalf("failed to start node: %v", err)
	}
	defer nodeImpl.Stop()

	if err := nodeImpl.ClaimAddress(110); err != nil {
		log.Fatalf("failed to claim address: %v", err)
	}
	log.Info("Node started and address claim initiated for address 110.")

	// Give the node a moment to claim its address
	time.Sleep(2 * time.Second)

	// Check if address was successfully claimed
	if nodeImpl.GetNetworkAddress() == 110 {
		log.Info("Address 110 successfully claimed")
	} else {
		log.Warnf("Address claim conflict occurred, current address: %d", nodeImpl.GetNetworkAddress())
	}

	// 6. Send ISO requests to the bus
	log.Info("Sending ISO Request for Address Claim (PGN 60928)")
	isoRequestAddrClaim := &pgn.IsoRequest{
		Pgn: ptrUint32(60928), // ISO Address Claim
		Info: pgn.MessageInfo{
			PGN:      59904, // ISO Request PGN
			SourceId: nodeImpl.GetNetworkAddress(),
			Priority: 6,
		},
	}
	if err := publisher.Write(isoRequestAddrClaim); err != nil {
		log.Errorf("failed to write ISO request for address claim: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	log.Info("Sending ISO Request for Product Info (PGN 126996)")
	isoRequestProdInfo := &pgn.IsoRequest{
		Pgn: ptrUint32(126996), // Product Information
		Info: pgn.MessageInfo{
			PGN:      59904, // ISO Request PGN
			SourceId: nodeImpl.GetNetworkAddress(),
			Priority: 6,
		},
	}
	if err := publisher.Write(isoRequestProdInfo); err != nil {
		log.Errorf("failed to write ISO request for product info: %v", err)
	}

	// 7. Run for a while to observe traffic
	log.Info("Running for 30 seconds to observe traffic...")
	time.Sleep(30 * time.Second)
	log.Info("Integration test finished.")
}

func ptrUint32(v uint32) *uint32 {
	return &v
}
