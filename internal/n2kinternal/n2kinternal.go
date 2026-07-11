// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package n2kinternal provides the internal implementation of the N2K service.
package n2kinternal

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boatkit-io/n2k/internal/adapter/canadapter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/boatkit-io/n2k/internal/subscribe"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
)

// N2kService provides the internal implementation of N2K operations
type N2kService struct {
	endpoint      endpoint.Endpoint
	adapter       *canadapter.CANAdapter
	replayAdapter *canadapter.CANAdapter
	packetStruct  *pkt.PacketStruct
	subscriber    *subscribe.SubscribeManager
	publisher     *pgn.Publisher
	log           *logrus.Logger

	receivedCANFrameHook func(*can.Frame)

	messageQueue        chan endpoint.Message
	messageQueueDropped atomic.Uint64
	messageQueueWG      sync.WaitGroup

	processorMu     sync.Mutex
	processorCancel context.CancelFunc
	processorDone   chan struct{}

	queueLogMu          sync.Mutex
	queueLastLog        time.Time
	queueLastDroppedLog uint64
}

const (
	messageQueueCapacity       = 8192
	messageQueueWarnDepthRatio = 0.75
	messageQueueLogInterval    = time.Second
)

// NewN2kService creates a new internal N2K service with the specified endpoint
func NewN2kService(ep endpoint.Endpoint, log *logrus.Logger) *N2kService {
	adapter := canadapter.NewCANAdapter(log)
	subscriber := subscribe.New()

	pub := pgn.NewPublisher(adapter)
	ps := pkt.NewPacketStruct()
	ps.SetOutput(subscriber)
	adapter.SetOutput(ps)

	s := &N2kService{
		endpoint:     ep,
		adapter:      adapter,
		packetStruct: ps,
		subscriber:   subscriber,
		publisher:    &pub,
		log:          log,
		messageQueue: make(chan endpoint.Message, messageQueueCapacity),
	}

	ep.SetOutput(s)
	adapter.SetWriter(ep)

	return s
}

// SubscribeToStruct subscribes to a specific PGN struct type and calls the callback when messages of that type are received.
func (s *N2kService) SubscribeToStruct(t, callback any) (uint, error) {
	id, err := s.subscriber.SubscribeToStruct(t, callback)
	return uint(id), err
}

// SubscribeToAllStructs subscribes to all PGN struct types and calls the callback when any message is received.
func (s *N2kService) SubscribeToAllStructs(callback any) (uint, error) {
	id, err := s.subscriber.SubscribeToAllStructs(callback)
	return uint(id), err
}

// Unsubscribe removes a subscription by its ID.
func (s *N2kService) Unsubscribe(id uint) error {
	return s.subscriber.Unsubscribe(subscribe.SubscriptionId(id))
}

// SetReceivedCANFrameHook registers a callback invoked for each live CAN frame before decode.
// The hook may be called concurrently from the endpoint goroutine.
func (s *N2kService) SetReceivedCANFrameHook(fn func(*can.Frame)) {
	s.receivedCANFrameHook = fn
}

// HandleMessage implements endpoint.MessageHandler for live endpoint traffic.
func (s *N2kService) HandleMessage(message endpoint.Message) {
	if frame, ok := message.(*can.Frame); ok {
		if s.receivedCANFrameHook != nil {
			s.receivedCANFrameHook(frame)
		}
	}

	message = cloneMessage(message)

	s.processorMu.Lock()
	if s.processorCancel == nil {
		s.processorMu.Unlock()
		s.processMessage(message)
		return
	}

	s.messageQueueWG.Add(1)
	select {
	case s.messageQueue <- message:
		queueFill := float64(len(s.messageQueue)) / float64(cap(s.messageQueue))
		s.processorMu.Unlock()
		if queueFill >= messageQueueWarnDepthRatio {
			s.maybeLogMessageQueueBacklog()
		}
	default:
		s.messageQueueWG.Done()
		s.messageQueueDropped.Add(1)
		s.processorMu.Unlock()
		s.maybeLogMessageQueueBacklog()
	}
}

func (s *N2kService) processMessage(message endpoint.Message) {
	s.adapter.HandleMessage(message)
}

func cloneMessage(message endpoint.Message) endpoint.Message {
	frame, ok := message.(*can.Frame)
	if !ok || frame == nil {
		return message
	}
	frameCopy := *frame
	return &frameCopy
}

// HandleReplayCANFrame feeds a captured CAN frame through a dedicated adapter into the
// shared decode pipeline so existing subscribers receive replay traffic alongside live data.
func (s *N2kService) HandleReplayCANFrame(frame *can.Frame) error {
	if s.replayAdapter == nil {
		s.replayAdapter = canadapter.NewCANAdapter(s.log)
		s.replayAdapter.SetOutput(s.packetStruct)
	}
	s.replayAdapter.HandleMessage(frame)
	return nil
}

// Write sends a PGN struct to the bus
func (s *N2kService) Write(pgnStruct any) error {
	return s.publisher.Write(pgnStruct)
}

// Start begins processing messages from the endpoint
func (s *N2kService) Start(ctx context.Context) error {
	s.startMessageProcessor(ctx)

	// Start the endpoint in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.endpoint.Run(ctx)
	}()

	// Wait a moment for the endpoint to initialize
	// This ensures the CAN bus is connected before we start writing frames
	select {
	case err := <-errChan:
		if waitErr := s.waitForMessageQueueDrain(ctx); waitErr != nil && err == nil {
			err = waitErr
		}
		s.stopMessageProcessor()
		// Endpoint failed to start
		return err
	case <-time.After(500 * time.Millisecond):
		// Endpoint should be ready now
		return nil
	}
}

// Stop stops processing messages
func (s *N2kService) Stop() error {
	err := s.endpoint.Close()
	s.stopMessageProcessor()
	return err
}

// UpdateEndpoint updates the endpoint used by the service
func (s *N2kService) UpdateEndpoint(ep endpoint.Endpoint) error {
	// Close the current endpoint if it's running
	if err := s.endpoint.Close(); err != nil {
		return err
	}
	s.stopMessageProcessor()

	// Set the new endpoint
	s.endpoint = ep

	// Connect the new endpoint to this service
	s.endpoint.SetOutput(s)

	// Connect the adapter to the new endpoint for writing frames
	s.adapter.SetWriter(s.endpoint)

	return nil
}

func (s *N2kService) startMessageProcessor(ctx context.Context) {
	s.processorMu.Lock()
	defer s.processorMu.Unlock()

	if s.processorCancel != nil {
		return
	}

	processorCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.processorCancel = cancel
	s.processorDone = done

	go s.runMessageProcessor(processorCtx, done)
}

func (s *N2kService) stopMessageProcessor() {
	s.processorMu.Lock()
	cancel := s.processorCancel
	done := s.processorDone
	s.processorCancel = nil
	s.processorDone = nil
	if cancel != nil {
		cancel()
	}
	s.processorMu.Unlock()

	if done != nil {
		<-done
	}
}

func (s *N2kService) runMessageProcessor(ctx context.Context, done chan<- struct{}) {
	defer func() {
		s.processorMu.Lock()
		if s.processorDone == done {
			s.processorCancel = nil
			s.processorDone = nil
		}
		s.processorMu.Unlock()
		close(done)
	}()
	ticker := time.NewTicker(messageQueueLogInterval)
	defer ticker.Stop()

	for {
		select {
		case message := <-s.messageQueue:
			s.processMessage(message)
			s.messageQueueWG.Done()
		case <-ticker.C:
			s.maybeLogMessageQueueBacklog()
		case <-ctx.Done():
			s.discardQueuedMessages()
			return
		}
	}
}

func (s *N2kService) discardQueuedMessages() {
	for {
		select {
		case <-s.messageQueue:
			s.messageQueueWG.Done()
		default:
			return
		}
	}
}

func (s *N2kService) waitForMessageQueueDrain(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.messageQueueWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *N2kService) maybeLogMessageQueueBacklog() {
	if s.log == nil {
		return
	}

	now := time.Now()
	s.queueLogMu.Lock()
	if !s.queueLastLog.IsZero() && now.Sub(s.queueLastLog) < messageQueueLogInterval {
		s.queueLogMu.Unlock()
		return
	}

	depth := len(s.messageQueue)
	capacity := cap(s.messageQueue)
	var fill float64
	if capacity > 0 {
		fill = float64(depth) / float64(capacity)
	}
	droppedTotal := s.messageQueueDropped.Load()
	droppedInterval := droppedTotal - s.queueLastDroppedLog
	if fill < messageQueueWarnDepthRatio && droppedInterval == 0 {
		s.queueLogMu.Unlock()
		return
	}
	s.queueLastLog = now
	s.queueLastDroppedLog = droppedTotal
	s.queueLogMu.Unlock()

	s.log.WithFields(logrus.Fields{
		"queueDepth":      depth,
		"queueCapacity":   capacity,
		"queueFill":       fill,
		"droppedInterval": droppedInterval,
		"droppedTotal":    droppedTotal,
		"stage":           "n2k-listener-handler-queue",
	}).Warn("N2K handler queue is falling behind")
}
