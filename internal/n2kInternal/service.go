// Package internal provides the internal implementation of the N2K service
package n2kInternal

import (
	"context"

	"github.com/boatkit-io/n2k/internal/adapter/canadapter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/boatkit-io/n2k/internal/subscribe"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/sirupsen/logrus"
)

// N2kService provides the internal implementation of N2K operations
type N2kService struct {
	endpoint   endpoint.Endpoint
	adapter    *canadapter.CANAdapter
	subscriber *subscribe.SubscribeManager
	publisher  *pgn.Publisher
}

// NewN2kService creates a new internal N2K service with the specified endpoint
func NewN2kService(ep endpoint.Endpoint) *N2kService {
	log := logrus.StandardLogger()

	adapter := canadapter.NewCANAdapter(log) // TODO: pass logger
	subscriber := subscribe.New()

	subs := subscribe.New()
	pub := pgn.NewPublisher(adapter)
	ps := pkt.NewPacketStruct()
	ps.SetOutput(subs)
	adapter.SetOutput(ps)

	return &N2kService{
		endpoint:   ep,
		adapter:    adapter,
		subscriber: subscriber,
		publisher:  pub,
	}
}

// Write sends a PGN struct to the bus
func (s *N2kService) Write(pgnStruct pgn.PgnStruct) error {
	return s.publisher.Write(pgnStruct)
}

// Start begins processing messages from the endpoint
func (s *N2kService) Start(ctx context.Context) error {
	return s.endpoint.Run(ctx)
}

// Stop stops processing messages
func (s *N2kService) Stop() error {
	return s.endpoint.Close()
}
