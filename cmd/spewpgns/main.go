package main

import (
	"context"
	"flag"
	"math"
	"os"
	"time"

	"github.com/boatkit-io/n2k/internal/adapter/canadapter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/boatkit-io/n2k/internal/subscribe"
	"github.com/boatkit-io/n2k/pkg/endpoint/socketcanendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/boatkit-io/tugboat/pkg/units"
	"github.com/sirupsen/logrus"
)

// canInterface is the CAN interface name for integration tests.
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
		log.Infof("PGN DUMP: %s", n2k.DebugDumpPGN(p))
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

	// 4. Generate and send PGNs
	log.Infof("Starting to send test PGNs...")
	sendTestPGNs(ctx, publisher, log)
}

// sendTestPGNs sends a batch of test PGNs.
func sendTestPGNs(ctx context.Context, publisher pgn.Publisher, log *logrus.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	sourceId := uint8(254) // Use reserved address for nodes unable to claim an address
	var counter float32 = 0

	for {
		select {
		case <-ctx.Done():
			log.Infof("Context cancelled, stopping PGN generation")
			return
		case <-ticker.C:
			counter++

			// Generate and send different types of PGNs
			if err := sendEngineData(&publisher, sourceId, counter, log); err != nil {
				log.Errorf("Failed to send engine data: %v", err)
			}

			if err := sendSpeedData(&publisher, sourceId, counter, log); err != nil {
				log.Errorf("Failed to send speed data: %v", err)
			}

			if err := sendPositionData(&publisher, sourceId, counter, log); err != nil {
				log.Errorf("Failed to send position data: %v", err)
			}

			log.Infof("Sent test PGNs batch %d", int(counter))
		}
	}
}

// sendEngineData sends an EngineParametersRapidUpdate PGN.
func sendEngineData(publisher *pgn.Publisher, sourceId uint8, counter float32, log *logrus.Logger) error {
	// Generate realistic engine RPM (1000-2500 RPM with some variation)
	rpm := float32(1500.0 + 500.0*math.Sin(float64(counter)*0.1))
	// Boost pressure in Pascal (150 kPa = 150000 Pa)
	boostPressurePa := float32(150000.0 + 30000.0*math.Sin(float64(counter)*0.2))
	boostPressure := units.NewPressure(units.Pa, boostPressurePa)

	enginePgn := pgn.EngineParametersRapidUpdate{
		Info: pgn.MessageInfo{
			SourceId: sourceId,
			TargetId: 255, // Broadcast
		},
		Instance:      pgn.SingleEngineOrDualEnginePort,
		Speed:         &rpm,
		BoostPressure: &boostPressure,
		TiltTrim:      nil, // Not applicable for this test
	}

	log.Debugf("Sending engine data: RPM=%.1f, Boost=%.1f kPa", rpm, boostPressurePa/1000.0)
	return publisher.Write(&enginePgn)
}

// sendSpeedData sends a Speed PGN.
func sendSpeedData(publisher *pgn.Publisher, sourceId uint8, counter float32, log *logrus.Logger) error {
	// Generate realistic boat speeds (5-15 knots)
	speedKnots := 10.0 + 3.0*math.Sin(float64(counter)*0.15)
	speedMs := float32(speedKnots * 0.514444) // Convert knots to m/s

	waterSpeed := units.NewVelocity(units.MetersPerSecond, speedMs)
	groundSpeed := units.NewVelocity(units.MetersPerSecond, speedMs+0.2) // Slight current
	sid := uint8(1)
	direction := uint8(0) // Forward

	speedPgn := pgn.Speed{
		Info: pgn.MessageInfo{
			SourceId: sourceId,
			TargetId: 255, // Broadcast
		},
		Sid:                      &sid,
		SpeedWaterReferenced:     &waterSpeed,
		SpeedGroundReferenced:    &groundSpeed,
		SpeedWaterReferencedType: pgn.PaddleWheel,
		SpeedDirection:           &direction,
	}

	log.Debugf("Sending speed data: %.1f knots (%.2f m/s)", speedKnots, speedMs)
	return publisher.Write(&speedPgn)
}

// sendPositionData sends a PositionRapidUpdate PGN.
func sendPositionData(publisher *pgn.Publisher, sourceId uint8, counter float32, log *logrus.Logger) error {
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

	positionPgn := pgn.PositionRapidUpdate{
		Info: pgn.MessageInfo{
			PGN:      pgn.PositionRapidUpdatePgn,
			SourceId: sourceId,
			TargetId: 255, // Broadcast
		},
		Latitude:  &latitude,
		Longitude: &longitude,
	}

	log.Debugf("Sending position data: %.6f, %.6f", latitude, longitude)
	return publisher.Write(&positionPgn)
}
