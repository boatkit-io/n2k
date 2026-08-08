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
	"errors"
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
	endpoint       endpoint.Endpoint
	endpointOutput *serviceEndpointOutput
	adapter        *canadapter.CANAdapter
	replayAdapter  *canadapter.CANAdapter
	packetStruct   *pkt.PacketStruct
	subscriber     *subscribe.SubscribeManager
	publisher      *pgn.Publisher
	log            *logrus.Logger

	lifecycleOpMu sync.Mutex
	lifecycleMu   sync.Mutex
	endpointRun   *serviceEndpointRun
	lastRun       *serviceEndpointRun

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

type serviceEndpointRun struct {
	ep        endpoint.Endpoint
	output    *serviceEndpointOutput
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	doneOnce  sync.Once
	closeOnce sync.Once
	closeErr  error
	stopping  bool
	result    error
}

// RunHandle identifies one endpoint generation and can be retained while a
// later generation is installed or started.
type RunHandle struct {
	run *serviceEndpointRun
}

type serviceEndpointOutput struct {
	mu      sync.RWMutex
	service *N2kService
	active  bool
}

func (o *serviceEndpointOutput) HandleMessage(message endpoint.Message) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if !o.active {
		return
	}
	o.service.HandleMessage(message)
}

func (o *serviceEndpointOutput) activate() {
	o.mu.Lock()
	o.active = true
	o.mu.Unlock()
}

func (o *serviceEndpointOutput) deactivate() {
	o.mu.Lock()
	o.active = false
	o.mu.Unlock()
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
	endpointOutput := &serviceEndpointOutput{service: s}
	s.endpointOutput = endpointOutput

	ps.SetOutput(s)
	adapter.SetOutput(s)
	subscriber.SetCallbackObserver(s)

	ep.SetOutput(endpointOutput)
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
	if err := ctx.Err(); err != nil {
		return err
	}

	s.lifecycleOpMu.Lock()
	defer s.lifecycleOpMu.Unlock()

	s.lifecycleMu.Lock()
	if s.endpointRun != nil {
		stopping := s.endpointRun.stopping || s.endpointRun.ctx.Err() != nil
		s.lifecycleMu.Unlock()
		if stopping {
			return errors.New("N2K service is stopping")
		}
		return nil
	}
	ep := s.endpoint
	output := s.endpointOutput
	runCtx, cancel := context.WithCancel(ctx)
	run := &serviceEndpointRun{
		ep:     ep,
		output: output,
		ctx:    runCtx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	s.endpointRun = run
	s.lastRun = run
	s.lifecycleMu.Unlock()

	if err := ep.Start(runCtx); err != nil {
		closeErr := run.closeEndpoint()
		result := errors.Join(err, closeErr)
		s.finishEndpointRun(run, result)
		return result
	}

	s.lifecycleMu.Lock()
	stopping := run.stopping || runCtx.Err() != nil || s.endpointRun != run
	if !stopping {
		output.activate()
		s.startMessageProcessor(runCtx)
		go s.closeEndpointOnContext(run)
		go s.runEndpoint(run)
	}
	s.lifecycleMu.Unlock()
	if stopping {
		closeErr := run.closeEndpoint()
		result := errors.Join(runCtx.Err(), closeErr)
		s.finishEndpointRun(run, result)
		return result
	}

	return nil
}

func (s *N2kService) runEndpoint(run *serviceEndpointRun) {
	err := run.ep.Run(run.ctx)
	s.lifecycleMu.Lock()
	run.stopping = true
	s.lifecycleMu.Unlock()
	run.output.deactivate()
	expectedStop := run.ctx.Err() != nil
	var drainErr error
	if !expectedStop {
		drainErr = s.waitForMessageQueueDrain(run.ctx)
	}
	closeErr := run.closeEndpoint()
	s.stopMessageProcessor()
	expectedStop = run.ctx.Err() != nil
	result := errors.Join(err, drainErr, closeErr)
	if expectedStop {
		result = closeErr
	}
	s.finishEndpointRun(run, result)

	if err != nil && !expectedStop {
		s.log.WithError(err).Error("N2K endpoint stopped unexpectedly")
	}
	if closeErr != nil && !expectedStop {
		s.log.WithError(closeErr).Warn("Failed to close stopped N2K endpoint")
	}
}

func (s *N2kService) closeEndpointOnContext(run *serviceEndpointRun) {
	select {
	case <-run.ctx.Done():
		s.lifecycleMu.Lock()
		run.stopping = true
		s.lifecycleMu.Unlock()
		run.output.deactivate()
		if err := run.closeEndpoint(); err != nil {
			s.log.WithError(err).Warn("Failed to close canceled N2K endpoint")
		}
	case <-run.done:
	}
}

func (run *serviceEndpointRun) closeEndpoint() error {
	run.closeOnce.Do(func() {
		run.closeErr = run.ep.Close()
	})
	return run.closeErr
}

// CurrentRun returns a handle for the most recently started endpoint
// generation. The handle remains bound to that generation after replacement.
func (s *N2kService) CurrentRun() (*RunHandle, error) {
	s.lifecycleMu.Lock()
	run := s.lastRun
	s.lifecycleMu.Unlock()
	if run == nil {
		return nil, errors.New("N2K service has not been started")
	}
	return &RunHandle{run: run}, nil
}

// Wait waits for this endpoint generation to finish.
func (h *RunHandle) Wait(ctx context.Context) error {
	if h == nil || h.run == nil {
		return errors.New("N2K endpoint run handle is nil")
	}
	select {
	case <-h.run.done:
		return h.run.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wait waits for the most recently started endpoint generation to finish.
func (s *N2kService) Wait(ctx context.Context) error {
	handle, err := s.CurrentRun()
	if err != nil {
		return err
	}
	return handle.Wait(ctx)
}

// Stop stops processing messages
func (s *N2kService) Stop() error {
	s.cancelCurrentEndpointRun()
	s.lifecycleOpMu.Lock()
	defer s.lifecycleOpMu.Unlock()
	return s.stopCurrentEndpoint()
}

// UpdateEndpoint updates the endpoint used by the service
func (s *N2kService) UpdateEndpoint(ep endpoint.Endpoint) error {
	if ep == nil {
		return errors.New("N2K endpoint is nil")
	}

	s.cancelCurrentEndpointRun()
	s.lifecycleOpMu.Lock()
	defer s.lifecycleOpMu.Unlock()
	closeErr := s.stopCurrentEndpoint()

	output := &serviceEndpointOutput{service: s}
	ep.SetOutput(output)

	s.lifecycleMu.Lock()
	s.endpoint = ep
	s.endpointOutput = output
	s.lastRun = nil
	s.lifecycleMu.Unlock()
	s.adapter.SetWriter(ep)
	if closeErr != nil {
		s.log.WithError(closeErr).Warn("Replaced N2K endpoint after its close returned an error")
	}

	return nil
}

func (s *N2kService) cancelCurrentEndpointRun() {
	s.lifecycleMu.Lock()
	run := s.endpointRun
	if run != nil {
		run.stopping = true
		run.cancel()
	}
	s.lifecycleMu.Unlock()
	if run != nil {
		run.output.deactivate()
	}
}

func (s *N2kService) stopCurrentEndpoint() error {
	s.lifecycleMu.Lock()
	run := s.endpointRun
	if run == nil {
		// A naturally completed or failed generation has already closed its
		// endpoint. Keep using that generation's closeOnce so a later Stop or
		// UpdateEndpoint does not close the same transport a second time.
		run = s.lastRun
	}
	ep := s.endpoint
	output := s.endpointOutput
	if run != nil {
		run.stopping = true
		run.cancel()
	}
	s.lifecycleMu.Unlock()

	output.deactivate()
	var err error
	if run != nil {
		err = run.closeEndpoint()
	} else {
		err = ep.Close()
	}
	if run != nil {
		<-run.done
	}
	s.stopMessageProcessor()
	return err
}

func (s *N2kService) finishEndpointRun(run *serviceEndpointRun, result error) {
	run.cancel()
	s.lifecycleMu.Lock()
	run.result = result
	if s.endpointRun == run {
		s.endpointRun = nil
	}
	run.doneOnce.Do(func() {
		close(run.done)
	})
	s.lifecycleMu.Unlock()
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
	s.lifecycleMu.Lock()
	ep := s.endpoint
	s.lifecycleMu.Unlock()
	reporter, ok := ep.(endpoint.OutboundLagReporter)
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
