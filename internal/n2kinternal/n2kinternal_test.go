package n2kinternal

import (
	"bytes"
	"context"
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

func (queueTestEndpoint) Run(context.Context) error         { return nil }
func (queueTestEndpoint) Close() error                      { return nil }
func (queueTestEndpoint) SetOutput(endpoint.MessageHandler) {}
func (queueTestEndpoint) WriteFrame(can.Frame)              {}

type lagTestEndpoint struct {
	queueTestEndpoint
	lag time.Duration
}

func (e lagTestEndpoint) OutboundQueueLag() time.Duration { return e.lag }

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
