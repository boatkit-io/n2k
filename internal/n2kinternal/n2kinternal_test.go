package n2kinternal

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type queueTestEndpoint struct{}

func (queueTestEndpoint) Start(context.Context) error       { return nil }
func (queueTestEndpoint) Run(context.Context) error         { return nil }
func (queueTestEndpoint) Close() error                      { return nil }
func (queueTestEndpoint) SetOutput(endpoint.MessageHandler) {}
func (queueTestEndpoint) WriteFrame(can.Frame)              {}

type lagTestEndpoint struct {
	queueTestEndpoint
	lag time.Duration
}

func (e lagTestEndpoint) OutboundQueueLag() time.Duration { return e.lag }

type startupTestEndpoint struct {
	startErr  error
	runCalled chan struct{}
}

func (e *startupTestEndpoint) Start(context.Context) error { return e.startErr }
func (e *startupTestEndpoint) Run(ctx context.Context) error {
	close(e.runCalled)
	<-ctx.Done()
	return ctx.Err()
}
func (*startupTestEndpoint) Close() error                      { return nil }
func (*startupTestEndpoint) SetOutput(endpoint.MessageHandler) {}
func (*startupTestEndpoint) WriteFrame(can.Frame)              {}

type lifecycleTestEndpoint struct {
	mu             sync.Mutex
	output         endpoint.MessageHandler
	startEntered   chan struct{}
	startEnteredMu sync.Once
	startRelease   chan struct{}
	runEntered     chan struct{}
	runEnteredMu   sync.Once
	closeCh        chan struct{}
	closeOnce      sync.Once
	runExited      chan struct{}
	emitOnClose    bool
	closeErr       error
	runCalls       atomic.Int32
}

func newLifecycleTestEndpoint() *lifecycleTestEndpoint {
	return &lifecycleTestEndpoint{
		startEntered: make(chan struct{}),
		runEntered:   make(chan struct{}),
		closeCh:      make(chan struct{}),
		runExited:    make(chan struct{}),
	}
}

func (e *lifecycleTestEndpoint) Start(context.Context) error {
	e.startEnteredMu.Do(func() { close(e.startEntered) })
	if e.startRelease != nil {
		<-e.startRelease
	}
	return nil
}

func (e *lifecycleTestEndpoint) Run(context.Context) error {
	e.runCalls.Add(1)
	e.runEnteredMu.Do(func() { close(e.runEntered) })
	<-e.closeCh
	if e.emitOnClose {
		e.mu.Lock()
		output := e.output
		e.mu.Unlock()
		if output != nil {
			output.HandleMessage(&can.Frame{})
		}
	}
	close(e.runExited)
	return nil
}

func (e *lifecycleTestEndpoint) Close() error {
	e.closeOnce.Do(func() { close(e.closeCh) })
	return e.closeErr
}

func (e *lifecycleTestEndpoint) SetOutput(output endpoint.MessageHandler) {
	e.mu.Lock()
	e.output = output
	e.mu.Unlock()
}

func (*lifecycleTestEndpoint) WriteFrame(can.Frame) {}

type teardownTestEndpoint struct {
	runEntered   chan struct{}
	runReturn    chan struct{}
	closeEntered chan struct{}
	closeRelease chan struct{}
	closeOnce    sync.Once
}

func newTeardownTestEndpoint() *teardownTestEndpoint {
	return &teardownTestEndpoint{
		runEntered:   make(chan struct{}),
		runReturn:    make(chan struct{}),
		closeEntered: make(chan struct{}),
		closeRelease: make(chan struct{}),
	}
}

func (*teardownTestEndpoint) Start(context.Context) error { return nil }
func (e *teardownTestEndpoint) Run(context.Context) error {
	close(e.runEntered)
	<-e.runReturn
	return errors.New("endpoint run failed")
}
func (e *teardownTestEndpoint) Close() error {
	e.closeOnce.Do(func() {
		close(e.closeEntered)
		<-e.closeRelease
	})
	return nil
}
func (*teardownTestEndpoint) SetOutput(endpoint.MessageHandler) {}
func (*teardownTestEndpoint) WriteFrame(can.Frame)              {}

type completedRunTestEndpoint struct {
	closeCalls atomic.Int32
}

func (*completedRunTestEndpoint) Start(context.Context) error { return nil }
func (*completedRunTestEndpoint) Run(context.Context) error   { return nil }
func (e *completedRunTestEndpoint) Close() error {
	e.closeCalls.Add(1)
	return nil
}
func (*completedRunTestEndpoint) SetOutput(endpoint.MessageHandler) {}
func (*completedRunTestEndpoint) WriteFrame(can.Frame)              {}

func TestStartReturnsEndpointStartupFailure(t *testing.T) {
	wantErr := errors.New("startup failed")
	ep := &startupTestEndpoint{startErr: wantErr, runCalled: make(chan struct{})}
	service := NewN2kService(ep, logrus.New())

	err := service.Start(context.Background())

	assert.ErrorIs(t, err, wantErr)
	select {
	case <-ep.runCalled:
		t.Fatal("Run called after startup failed")
	default:
	}
}

func TestWaitReturnsMostRecentEndpointResult(t *testing.T) {
	service := NewN2kService(queueTestEndpoint{}, logrus.New())

	assert.ErrorContains(t, service.Wait(context.Background()), "has not been started")
	assert.NoError(t, service.Start(context.Background()))
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.NoError(t, service.Wait(waitCtx))
}

func TestStartReturnsImmediatelyAfterSynchronousStartup(t *testing.T) {
	ep := &startupTestEndpoint{runCalled: make(chan struct{})}
	service := NewN2kService(ep, logrus.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startedAt := time.Now()
	assert.NoError(t, service.Start(ctx))
	assert.Less(t, time.Since(startedAt), 250*time.Millisecond)

	select {
	case <-ep.runCalled:
	case <-time.After(time.Second):
		t.Fatal("Run was not started")
	}
	assert.NoError(t, service.Stop())
}

func TestRepeatedStartDoesNotLaunchAnotherEndpointRun(t *testing.T) {
	ep := newLifecycleTestEndpoint()
	service := NewN2kService(ep, logrus.New())

	assert.NoError(t, service.Start(context.Background()))
	select {
	case <-ep.runEntered:
	case <-time.After(time.Second):
		t.Fatal("Run was not started")
	}
	assert.NoError(t, service.Start(context.Background()))
	assert.Equal(t, int32(1), ep.runCalls.Load())
	assert.NoError(t, service.Stop())
	select {
	case <-ep.runExited:
	default:
		t.Fatal("Stop returned before Run exited")
	}
}

func TestContextCancellationClosesEndpointThatDoesNotObserveContext(t *testing.T) {
	ep := newLifecycleTestEndpoint()
	ep.emitOnClose = true
	service := NewN2kService(ep, logrus.New())
	var received atomic.Int32
	service.SetReceivedCANFrameHook(func(*can.Frame) {
		received.Add(1)
	})
	ctx, cancel := context.WithCancel(context.Background())

	assert.NoError(t, service.Start(ctx))
	<-ep.runEntered
	cancel()
	assert.Error(t, service.Start(context.Background()))

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	assert.NoError(t, service.Wait(waitCtx))
	select {
	case <-ep.runExited:
	default:
		t.Fatal("context cancellation did not close and join the endpoint")
	}
	assert.Zero(t, received.Load())
}

func TestStartRejectsRunThatIsAlreadyTearingDown(t *testing.T) {
	ep := newTeardownTestEndpoint()
	service := NewN2kService(ep, logrus.New())

	assert.NoError(t, service.Start(context.Background()))
	<-ep.runEntered
	close(ep.runReturn)
	<-ep.closeEntered

	assert.ErrorContains(t, service.Start(context.Background()), "stopping")
	close(ep.closeRelease)
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.ErrorContains(t, service.Wait(waitCtx), "endpoint run failed")
}

func TestStopDoesNotCloseCompletedEndpointGenerationTwice(t *testing.T) {
	ep := &completedRunTestEndpoint{}
	service := NewN2kService(ep, logrus.New())

	assert.NoError(t, service.Start(context.Background()))
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.NoError(t, service.Wait(waitCtx))
	assert.NoError(t, service.Stop())
	assert.Equal(t, int32(1), ep.closeCalls.Load())
}

func TestStopBeforeRunIsScheduledDoesNotStartClosedEndpoint(t *testing.T) {
	ep := newLifecycleTestEndpoint()
	ep.startRelease = make(chan struct{})
	service := NewN2kService(ep, logrus.New())
	startErr := make(chan error, 1)
	go func() {
		startErr <- service.Start(context.Background())
	}()
	<-ep.startEntered

	stopErr := make(chan error, 1)
	go func() {
		stopErr <- service.Stop()
	}()
	assert.Eventually(t, func() bool {
		service.lifecycleMu.Lock()
		defer service.lifecycleMu.Unlock()
		return service.endpointRun != nil && service.endpointRun.stopping
	}, time.Second, time.Millisecond)
	close(ep.startRelease)

	assert.ErrorIs(t, <-startErr, context.Canceled)
	assert.NoError(t, <-stopErr)
	assert.Zero(t, ep.runCalls.Load())
}

func TestStopRejectsEndpointMessagesBeforeWaitingForRun(t *testing.T) {
	ep := newLifecycleTestEndpoint()
	ep.emitOnClose = true
	service := NewN2kService(ep, logrus.New())
	var received atomic.Int32
	service.SetReceivedCANFrameHook(func(*can.Frame) {
		received.Add(1)
	})

	assert.NoError(t, service.Start(context.Background()))
	<-ep.runEntered
	assert.NoError(t, service.Stop())
	assert.Zero(t, received.Load())
}

func TestUpdateEndpointWaitsForOldRunBeforeStartingReplacement(t *testing.T) {
	oldEndpoint := newLifecycleTestEndpoint()
	newEndpoint := newLifecycleTestEndpoint()
	service := NewN2kService(oldEndpoint, logrus.New())

	assert.NoError(t, service.Start(context.Background()))
	<-oldEndpoint.runEntered
	assert.NoError(t, service.UpdateEndpoint(newEndpoint))
	select {
	case <-oldEndpoint.runExited:
	default:
		t.Fatal("UpdateEndpoint returned before the old Run exited")
	}
	assert.NoError(t, service.Start(context.Background()))
	<-newEndpoint.runEntered
	assert.NoError(t, service.Stop())
}

func TestUpdateEndpointReplacesEndpointAfterCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	oldEndpoint := newLifecycleTestEndpoint()
	oldEndpoint.closeErr = closeErr
	newEndpoint := newLifecycleTestEndpoint()
	service := NewN2kService(oldEndpoint, logrus.New())

	assert.NoError(t, service.Start(context.Background()))
	<-oldEndpoint.runEntered
	assert.NoError(t, service.UpdateEndpoint(newEndpoint))
	assert.NoError(t, service.Start(context.Background()))
	select {
	case <-newEndpoint.runEntered:
	case <-time.After(time.Second):
		t.Fatal("replacement endpoint did not start after old close error")
	}
	assert.NoError(t, service.Stop())
}

func TestRunHandleRemainsBoundToCapturedEndpointGeneration(t *testing.T) {
	oldEndpoint := newLifecycleTestEndpoint()
	newEndpoint := newLifecycleTestEndpoint()
	service := NewN2kService(oldEndpoint, logrus.New())

	assert.NoError(t, service.Start(context.Background()))
	<-oldEndpoint.runEntered
	oldRun, err := service.CurrentRun()
	assert.NoError(t, err)
	assert.NoError(t, service.UpdateEndpoint(newEndpoint))
	assert.NoError(t, service.Start(context.Background()))
	<-newEndpoint.runEntered

	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.NoError(t, oldRun.Wait(waitCtx))
	select {
	case <-newEndpoint.runExited:
		t.Fatal("waiting for old generation stopped the replacement")
	default:
	}
	assert.NoError(t, service.Stop())
}

func TestHandleMessageLogsWhenHandlerQueueLagDrops(t *testing.T) {
	var buf bytes.Buffer
	log := logrus.New()
	log.SetOutput(&buf)
	log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	service := NewN2kService(queueTestEndpoint{}, log, WithMessageQueueMaxAge(50*time.Millisecond))

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.processorCancel = cancel
	service.messageQueueProcessingNano.Store(time.Now().Add(-100 * time.Millisecond).UnixNano())

	frame := &can.Frame{
		ID:     converter.CanIDFromData(publicpgn.PositionRapidUpdatePGN, 42, 3, 255),
		Length: 8,
	}

	service.HandleMessage(frame)

	output := buf.String()
	assert.Contains(t, output, "N2K handler queue is falling behind")
	assert.Contains(t, output, "droppedTotal=1")
	assert.Contains(t, output, "droppedBacklogTotal=1")
	assert.Contains(t, output, "queueMaxAge=50ms")
	assert.Contains(t, output, "stage=n2k-listener-handler-queue")
}

func TestProcessQueuedMessageDropsStaleMessage(t *testing.T) {
	log := logrus.New()
	log.SetOutput(&bytes.Buffer{})

	service := NewN2kService(queueTestEndpoint{}, log, WithMessageQueueMaxAge(50*time.Millisecond))
	frame := &can.Frame{
		ID:     converter.CanIDFromData(publicpgn.PositionRapidUpdatePGN, 42, 3, 255),
		Length: 8,
	}

	service.processQueuedMessage(queuedMessage{
		message:    frame,
		enqueuedAt: time.Now().Add(-100 * time.Millisecond),
	})

	assert.Equal(t, uint64(1), service.messageQueueDropped.Load())
	assert.Equal(t, uint64(1), service.messageQueueStaleDropped.Load())
}

func TestMessageQueueLagIncludesProcessingMessage(t *testing.T) {
	service := NewN2kService(queueTestEndpoint{}, logrus.New(), WithMessageQueueMaxAge(time.Second))
	service.messageQueueProcessingNano.Store(time.Now().Add(-300 * time.Millisecond).UnixNano())

	assert.GreaterOrEqual(t, service.MessageQueueLag(), 250*time.Millisecond)
	assert.Equal(t, time.Second, service.MessageQueueMaxAge())
}

func TestOutboundQueueLagDelegatesToEndpointReporter(t *testing.T) {
	service := NewN2kService(lagTestEndpoint{lag: 1500 * time.Millisecond}, logrus.New())

	assert.Equal(t, 1500*time.Millisecond, service.OutboundQueueLag())
}

func TestProcessingMetricsReportsInFlightSubscriberCallback(t *testing.T) {
	metrics := newProcessingMetrics()
	started := time.Unix(100, 0)
	metrics.callbackStarted("ISORequest", "node.handleIsoRequest", started)

	fields := logrus.Fields{}
	snapshot := metrics.snapshot(started.Add(2 * time.Second))
	snapshot.addFields(fields)
	assert.Equal(t, "ISORequest/node.handleIsoRequest", fields["subscriberCallbackInFlight"])
	assert.Equal(t, "2s", fields["subscriberCallbackInFlightAge"])

	metrics.callbackFinished()
	fields = logrus.Fields{}
	snapshot = metrics.snapshot(started.Add(3 * time.Second))
	snapshot.addFields(fields)
	assert.NotContains(t, fields, "subscriberCallbackInFlight")
}
