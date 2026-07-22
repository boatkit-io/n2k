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

	messageQueue               *messageQueue
	messageQueueMaxAge         time.Duration
	messageQueueProcessingNano atomic.Int64
	messageQueueDropped        atomic.Uint64
	messageQueueBacklogDropped atomic.Uint64
	messageQueueStaleDropped   atomic.Uint64
	messageQueueWG             sync.WaitGroup
	processingMetrics          *processingMetrics

	processorMu     sync.Mutex
	processorCancel context.CancelFunc
	processorDone   chan struct{}

	queueLogMu                 sync.Mutex
	queueLastLog               time.Time
	queueLastDroppedLog        uint64
	queueLastBacklogDroppedLog uint64
	queueLastStaleDroppedLog   uint64
}

const (
	// DefaultMessageQueueMaxAge is the default maximum live CAN message lag allowed
	// before queued messages are dropped.
	DefaultMessageQueueMaxAge = 500 * time.Millisecond
	messageQueueLogInterval   = time.Second
)

type serviceOptions struct {
	messageQueueMaxAge time.Duration
}

// ServiceOption configures an N2K service.
type ServiceOption func(*serviceOptions)

// WithMessageQueueMaxAge sets how stale queued live CAN messages may become before the
// service rejects new messages and discards queued stale messages.
func WithMessageQueueMaxAge(maxAge time.Duration) ServiceOption {
	return func(options *serviceOptions) {
		if maxAge < 0 {
			maxAge = 0
		}
		options.messageQueueMaxAge = maxAge
	}
}

// NewN2kService creates a new internal N2K service with the specified endpoint
func NewN2kService(ep endpoint.Endpoint, log *logrus.Logger, opts ...ServiceOption) *N2kService {
	options := serviceOptions{
		messageQueueMaxAge: DefaultMessageQueueMaxAge,
	}
	for _, opt := range opts {
		opt(&options)
	}

	adapter := canadapter.NewCANAdapter(log)
	subscriber := subscribe.New()

	pub := pgn.NewPublisher(adapter)
	ps := pkt.NewPacketStruct()

	s := &N2kService{
		endpoint:           ep,
		adapter:            adapter,
		packetStruct:       ps,
		subscriber:         subscriber,
		publisher:          &pub,
		log:                log,
		messageQueue:       newMessageQueue(),
		messageQueueMaxAge: options.messageQueueMaxAge,
		processingMetrics:  newProcessingMetrics(),
	}

	ps.SetOutput(s)
	adapter.SetOutput(s)
	subscriber.SetCallbackObserver(s)

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
	queued := queuedMessage{
		message:    message,
		enqueuedAt: time.Now(),
	}

	s.processorMu.Lock()
	if s.processorCancel == nil {
		s.processorMu.Unlock()
		s.processMessage(message)
		return
	}
	s.messageQueueWG.Add(1)
	accepted, queueStats := s.messageQueue.enqueueIfCurrent(
		queued,
		s.messageQueueMaxAge,
		s.messageQueueProcessingAge(queued.enqueuedAt),
	)
	if !accepted {
		s.messageQueueWG.Done()
		s.messageQueueDropped.Add(1)
		s.messageQueueBacklogDropped.Add(1)
		s.processorMu.Unlock()
		s.maybeLogMessageQueueBacklog()
		return
	}

	s.processorMu.Unlock()
	if queueStats.lag > s.messageQueueMaxAge {
		s.maybeLogMessageQueueBacklog()
	}
}

func (s *N2kService) processMessage(message endpoint.Message) {
	pgnNum, hasPGN := messagePGN(message)
	start := time.Now()
	s.adapter.HandleMessage(message)
	s.processingMetrics.observeFrame(pgnNum, hasPGN, time.Since(start))
}

func cloneMessage(message endpoint.Message) endpoint.Message {
	frame, ok := message.(*can.Frame)
	if !ok || frame == nil {
		return message
	}
	frameCopy := *frame
	return &frameCopy
}

type queuedMessage struct {
	message    endpoint.Message
	enqueuedAt time.Time
}

type messageQueue struct {
	mu       sync.Mutex
	messages []queuedMessage
	head     int
	notify   chan struct{}
}

type messageQueueStats struct {
	depth     int
	oldestAge time.Duration
	lag       time.Duration
}

func newMessageQueue() *messageQueue {
	return &messageQueue{
		notify: make(chan struct{}, 1),
	}
}

func (q *messageQueue) enqueueIfCurrent(
	message queuedMessage,
	maxAge time.Duration,
	processingAge time.Duration,
) (bool, messageQueueStats) {
	q.mu.Lock()
	oldestAge := q.oldestAgeLocked(message.enqueuedAt)
	lag := maxDuration(oldestAge, processingAge)
	if lag > maxAge {
		stats := messageQueueStats{
			depth:     q.depthLocked(),
			oldestAge: oldestAge,
			lag:       lag,
		}
		q.mu.Unlock()
		return false, stats
	}
	q.messages = append(q.messages, message)
	stats := q.statsLocked(message.enqueuedAt, processingAge)
	q.mu.Unlock()
	q.signal()
	return true, stats
}

func (q *messageQueue) dequeue(ctx context.Context) (queuedMessage, bool) {
	for {
		q.mu.Lock()
		if q.depthLocked() > 0 {
			message := q.messages[q.head]
			q.messages[q.head] = queuedMessage{}
			q.head++
			q.compactLocked()
			q.mu.Unlock()
			return message, true
		}
		q.mu.Unlock()

		select {
		case <-q.notify:
		case <-ctx.Done():
			return queuedMessage{}, false
		}
	}
}

func (q *messageQueue) discard() int {
	q.mu.Lock()
	count := q.depthLocked()
	clear(q.messages)
	q.messages = nil
	q.head = 0
	q.mu.Unlock()
	return count
}

func (q *messageQueue) stats(now time.Time) messageQueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.statsLocked(now, 0)
}

func (q *messageQueue) statsLocked(now time.Time, processingAge time.Duration) messageQueueStats {
	oldestAge := q.oldestAgeLocked(now)
	return messageQueueStats{
		depth:     q.depthLocked(),
		oldestAge: oldestAge,
		lag:       maxDuration(oldestAge, processingAge),
	}
}

func (q *messageQueue) oldestAgeLocked(now time.Time) time.Duration {
	if q.depthLocked() == 0 {
		return 0
	}
	age := now.Sub(q.messages[q.head].enqueuedAt)
	if age < 0 {
		return 0
	}
	return age
}

func (q *messageQueue) depthLocked() int {
	return len(q.messages) - q.head
}

func (q *messageQueue) compactLocked() {
	if q.head == 0 {
		return
	}
	if q.head < 1024 && q.head*2 < len(q.messages) {
		return
	}
	copy(q.messages, q.messages[q.head:])
	newLen := len(q.messages) - q.head
	clear(q.messages[newLen:])
	q.messages = q.messages[:newLen]
	q.head = 0
}

func (q *messageQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// HandlePacket implements canadapter.PacketHandler and records packet-to-struct processing time.
//
//nolint:gocritic // Why: canadapter.PacketHandler currently passes packets by value.
func (s *N2kService) HandlePacket(packet pkt.Packet) {
	start := time.Now()
	s.packetStruct.HandlePacket(packet)
	s.processingMetrics.observePacket(time.Since(start))
}

// HandleStruct implements pkt.StructHandler and records subscriber fanout time.
func (s *N2kService) HandleStruct(p any) {
	start := time.Now()
	s.subscriber.HandleStruct(p)
	s.processingMetrics.observeSubscriber(time.Since(start))
}

// ObserveCallback records individual subscriber callback time for backlog diagnostics.
func (s *N2kService) ObserveCallback(structName, callbackName string, duration time.Duration) {
	s.processingMetrics.observeCallback(structName, callbackName, duration)
}

// CallbackStarted records which synchronous callback currently owns the serial handler.
func (s *N2kService) CallbackStarted(structName, callbackName string) {
	s.processingMetrics.callbackStarted(structName, callbackName, time.Now())
}

// CallbackFinished clears the current synchronous callback diagnostic.
func (s *N2kService) CallbackFinished() {
	s.processingMetrics.callbackFinished()
}

// HandleReplayCANFrame feeds a captured CAN frame through a dedicated adapter into the
// shared decode pipeline so existing subscribers receive replay traffic alongside live data.
func (s *N2kService) HandleReplayCANFrame(frame *can.Frame) error {
	if s.replayAdapter == nil {
		s.replayAdapter = canadapter.NewCANAdapter(s.log)
		s.replayAdapter.SetOutput(s)
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
		queued, ok := s.messageQueue.dequeue(ctx)
		if !ok {
			s.discardQueuedMessages()
			return
		}
		s.processQueuedMessage(queued)
		s.messageQueueWG.Done()

		select {
		case <-ticker.C:
			s.maybeLogMessageQueueBacklog()
		default:
		}
	}
}

func (s *N2kService) discardQueuedMessages() {
	count := s.messageQueue.discard()
	for i := 0; i < count; i++ {
		s.messageQueueWG.Done()
	}
}

func (s *N2kService) processQueuedMessage(queued queuedMessage) {
	s.messageQueueProcessingNano.Store(queued.enqueuedAt.UnixNano())
	defer s.messageQueueProcessingNano.Store(0)

	queueWait := time.Since(queued.enqueuedAt)
	s.processingMetrics.observeQueueWait(queueWait)
	if queueWait > s.messageQueueMaxAge {
		s.messageQueueDropped.Add(1)
		s.messageQueueStaleDropped.Add(1)
		s.maybeLogMessageQueueBacklog()
		return
	}
	s.processMessage(queued.message)
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

	queueStats := s.messageQueueStats(now)
	droppedTotal := s.messageQueueDropped.Load()
	droppedInterval := droppedTotal - s.queueLastDroppedLog
	backlogDroppedTotal := s.messageQueueBacklogDropped.Load()
	backlogDroppedInterval := backlogDroppedTotal - s.queueLastBacklogDroppedLog
	staleDroppedTotal := s.messageQueueStaleDropped.Load()
	staleDroppedInterval := staleDroppedTotal - s.queueLastStaleDroppedLog
	if droppedInterval == 0 && queueStats.lag <= s.messageQueueMaxAge {
		s.queueLogMu.Unlock()
		return
	}
	s.queueLastLog = now
	s.queueLastDroppedLog = droppedTotal
	s.queueLastBacklogDroppedLog = backlogDroppedTotal
	s.queueLastStaleDroppedLog = staleDroppedTotal
	s.queueLogMu.Unlock()

	fields := logrus.Fields{
		"queueDepth":             queueStats.depth,
		"queueLag":               queueStats.lag.String(),
		"queueMaxAge":            s.messageQueueMaxAge.String(),
		"queueOldestAge":         queueStats.oldestAge.String(),
		"queueProcessingAge":     queueStats.processingAge.String(),
		"droppedInterval":        droppedInterval,
		"droppedTotal":           droppedTotal,
		"droppedBacklogInterval": backlogDroppedInterval,
		"droppedBacklogTotal":    backlogDroppedTotal,
		"staleDroppedInterval":   staleDroppedInterval,
		"staleDroppedTotal":      staleDroppedTotal,
		"stage":                  "n2k-listener-handler-queue",
	}
	metricsSnapshot := s.processingMetrics.snapshot(now)
	metricsSnapshot.addFields(fields)
	s.log.WithFields(fields).Warn("N2K handler queue is falling behind")
}

// MessageQueueLag returns the current age of the oldest live CAN message waiting
// in or moving through the serial handler path.
func (s *N2kService) MessageQueueLag() time.Duration {
	return s.messageQueueStats(time.Now()).lag
}

// MessageQueueMaxAge returns the configured maximum tolerated live CAN message queue lag.
func (s *N2kService) MessageQueueMaxAge() time.Duration {
	return s.messageQueueMaxAge
}

// OutboundQueueLag returns recent outbound endpoint queue/send latency.
func (s *N2kService) OutboundQueueLag() time.Duration {
	reporter, ok := s.endpoint.(endpoint.OutboundLagReporter)
	if !ok {
		return 0
	}
	return reporter.OutboundQueueLag()
}

type messageQueueSnapshot struct {
	depth         int
	oldestAge     time.Duration
	processingAge time.Duration
	lag           time.Duration
}

func (s *N2kService) messageQueueStats(now time.Time) messageQueueSnapshot {
	queueStats := s.messageQueue.stats(now)
	processingAge := s.messageQueueProcessingAge(now)
	return messageQueueSnapshot{
		depth:         queueStats.depth,
		oldestAge:     queueStats.oldestAge,
		processingAge: processingAge,
		lag:           maxDuration(queueStats.oldestAge, processingAge),
	}
}

func (s *N2kService) messageQueueProcessingAge(now time.Time) time.Duration {
	processingNano := s.messageQueueProcessingNano.Load()
	if processingNano == 0 {
		return 0
	}
	age := now.Sub(time.Unix(0, processingNano))
	if age < 0 {
		return 0
	}
	return age
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
