package socketcanendpoint

import (
	"context"
	"io"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type retryTestChannel struct {
	failures int32
	writes   atomic.Int32
}

func (c *retryTestChannel) Run(context.Context) error { return nil }
func (c *retryTestChannel) Close() error              { return nil }
func (c *retryTestChannel) WriteFrame(can.Frame) error {
	writes := c.writes.Add(1)
	if writes <= c.failures {
		return syscall.ENOBUFS
	}
	return nil
}

type requeueTestChannel struct {
	failedLow atomic.Bool
	writes    chan uint32
}

func (c *requeueTestChannel) Run(context.Context) error { return nil }
func (c *requeueTestChannel) Close() error              { return nil }
func (c *requeueTestChannel) WriteFrame(frame can.Frame) error {
	c.writes <- converter.DecodeCanID(frame.ID).PGN
	if isLowPrioritySocketCANFrame(frame) && !c.failedLow.Swap(true) {
		return syscall.ENOBUFS
	}
	return nil
}

func TestWriteFrameRetriesSocketCANBufferFull(t *testing.T) {
	log := discardLogger()
	channel := &retryTestChannel{failures: 2}
	endpoint := &SocketCANEndpoint{
		log:     log,
		channel: channel,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go endpoint.runOutboundWriter(ctx)

	endpoint.WriteFrame(can.Frame{})

	assert.Eventually(t, func() bool {
		return channel.writes.Load() == 3
	}, time.Second, time.Millisecond)
}

func TestLowPriorityBackoffDoesNotBlockHighPriorityFrames(t *testing.T) {
	channel := &requeueTestChannel{writes: make(chan uint32, 4)}
	endpoint := &SocketCANEndpoint{
		log:     discardLogger(),
		channel: channel,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go endpoint.runOutboundWriter(ctx)

	endpoint.WriteFrame(isoRequestFrame(pgn.ProductInformationPGN))
	assert.Equal(t, uint32(pgn.ISORequestPGN), <-channel.writes)

	endpoint.WriteFrame(pgnFrame(pgn.HeartbeatPGN))
	select {
	case writtenPGN := <-channel.writes:
		assert.Equal(t, uint32(pgn.HeartbeatPGN), writtenPGN)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("high-priority frame was blocked behind low-priority retry backoff")
	}
}

func TestWriteFrameQueuesDiscoveryRequestsAsLowPriority(t *testing.T) {
	endpoint := &SocketCANEndpoint{
		log:     discardLogger(),
		channel: &retryTestChannel{},
	}

	endpoint.WriteFrame(isoRequestFrame(pgn.ProductInformationPGN))

	assert.Len(t, endpoint.outboundHigh, 0)
	assert.Len(t, endpoint.outboundLow, 1)
}

func TestWriteFrameQueuesOperationalFramesAsHighPriority(t *testing.T) {
	endpoint := &SocketCANEndpoint{
		log:     discardLogger(),
		channel: &retryTestChannel{},
	}

	endpoint.WriteFrame(pgnFrame(pgn.HeartbeatPGN))

	assert.Len(t, endpoint.outboundHigh, 1)
	assert.Len(t, endpoint.outboundLow, 0)
}

func TestRetryPolicyUsesLongerBackoffForDiscoveryFrames(t *testing.T) {
	assert.Equal(t, time.Millisecond, retryPolicyForSocketCANFrame(pgnFrame(pgn.HeartbeatPGN)).delay(0))
	assert.Equal(t, 250*time.Millisecond, retryPolicyForSocketCANFrame(isoRequestFrame(pgn.ProductInformationPGN)).delay(0))
}

func TestOutboundQueueLagRecordsSendLatency(t *testing.T) {
	endpoint := &SocketCANEndpoint{
		log:     discardLogger(),
		channel: &retryTestChannel{},
	}

	endpoint.writeQueuedFrame(context.Background(), outboundSocketCANFrame{
		frame:      pgnFrame(pgn.HeartbeatPGN),
		enqueuedAt: time.Now().Add(-1500 * time.Millisecond),
	})

	assert.GreaterOrEqual(t, endpoint.OutboundQueueLag(), time.Second)
}

func TestOutboundQueueLagExpires(t *testing.T) {
	endpoint := &SocketCANEndpoint{
		log: discardLogger(),
	}
	endpoint.outboundLagNano.Store(int64(2 * time.Second))
	endpoint.outboundLagUpdatedNano.Store(time.Now().Add(-socketCANOutboundLagTTL - time.Second).UnixNano())

	assert.Zero(t, endpoint.OutboundQueueLag())
}

func discardLogger() *logrus.Logger {
	log := logrus.New()
	log.SetOutput(io.Discard)
	return log
}

func isoRequestFrame(requestedPGN uint32) can.Frame {
	frame := pgnFrame(pgn.ISORequestPGN)
	frame.Length = 3
	frame.Data[0] = uint8(requestedPGN)
	frame.Data[1] = uint8(requestedPGN >> 8)
	frame.Data[2] = uint8(requestedPGN >> 16)
	return frame
}

func pgnFrame(pgnNum uint32) can.Frame {
	return can.Frame{
		ID:     converter.CanIDFromData(pgnNum, 110, 6, 255),
		Length: 8,
	}
}
