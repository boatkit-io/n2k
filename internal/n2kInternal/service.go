// Package internal provides the internal implementation of the N2K service
package n2kInternal

import (
	"context"
	"time"

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
func NewN2kService(ep endpoint.Endpoint, log *logrus.Logger) *N2kService {
	adapter := canadapter.NewCANAdapter(log) // TODO: pass logger
	subscriber := subscribe.New()

	pub := pgn.NewPublisher(adapter)
	ps := pkt.NewPacketStruct()
	ps.SetOutput(subscriber)
	adapter.SetOutput(ps)

	// Connect the endpoint to the adapter
	ep.SetOutput(adapter)

	// Connect the adapter to the endpoint for writing frames
	adapter.SetWriter(ep)

	return &N2kService{
		endpoint:   ep,
		adapter:    adapter,
		subscriber: subscriber,
		publisher:  &pub,
	}
}

func (s *N2kService) SubscribeToStruct(t any, callback any) (uint, error) {
	id, err := s.subscriber.SubscribeToStruct(t, callback)
	return uint(id), err
}

func (s *N2kService) SubscribeToAllStructs(callback any) (uint, error) {
	id, err := s.subscriber.SubscribeToAllStructs(callback)
	return uint(id), err
}

func (s *N2kService) Unsubscribe(id uint) error {
	return s.subscriber.Unsubscribe(subscribe.SubscriptionId(id))
}

// Write sends a PGN struct to the bus
func (s *N2kService) Write(pgnStruct pgn.PgnStruct) error {
	return s.publisher.Write(pgnStruct)
}

// Start begins processing messages from the endpoint
func (s *N2kService) Start(ctx context.Context) error {
	// Start the endpoint in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.endpoint.Run(ctx)
	}()

	// Wait a moment for the endpoint to initialize
	// This ensures the CAN bus is connected before we start writing frames
	select {
	case err := <-errChan:
		// Endpoint failed to start
		return err
	case <-time.After(500 * time.Millisecond):
		// Endpoint should be ready now
		return nil
	}
}

// Stop stops processing messages
func (s *N2kService) Stop() error {
	return s.endpoint.Close()
}

// UpdateEndpoint updates the endpoint used by the service
func (s *N2kService) UpdateEndpoint(ep endpoint.Endpoint) error {
	// Close the current endpoint if it's running
	if err := s.endpoint.Close(); err != nil {
		return err
	}

	// Set the new endpoint
	s.endpoint = ep

	// Connect the new endpoint to the adapter
	s.endpoint.SetOutput(s.adapter)

	// Connect the adapter to the new endpoint for writing frames
	s.adapter.SetWriter(s.endpoint)

	return nil
}
