package usbcanendpoint

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type lifecycleTestChannel struct {
	startCalls atomic.Int32
	runEntered chan struct{}
	closed     chan struct{}
	closeOnce  sync.Once
}

func (c *lifecycleTestChannel) Start(context.Context) error {
	c.startCalls.Add(1)
	return nil
}

func (c *lifecycleTestChannel) Run(context.Context) error {
	close(c.runEntered)
	<-c.closed
	return nil
}

func (c *lifecycleTestChannel) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (*lifecycleTestChannel) WriteFrame(can.Frame) error { return nil }

func TestRunRejectsConcurrentReaderBeforeStartingChannelAgain(t *testing.T) {
	channel := &lifecycleTestChannel{
		runEntered: make(chan struct{}),
		closed:     make(chan struct{}),
	}
	ep := &USBCANEndpoint{log: logrus.New(), channel: channel}
	runDone := make(chan error, 1)
	go func() {
		runDone <- ep.Run(context.Background())
	}()

	select {
	case <-channel.runEntered:
	case <-time.After(time.Second):
		t.Fatal("USBCAN Run did not start")
	}
	require.ErrorContains(t, ep.Run(context.Background()), "already running")
	require.Equal(t, int32(1), channel.startCalls.Load())
	require.NoError(t, ep.Close())
	select {
	case err := <-runDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("USBCAN Close did not stop Run")
	}
}

func TestRunReleasesRunningStateAfterStartupFailure(t *testing.T) {
	wantErr := errors.New("startup failed")
	ep := &USBCANEndpoint{
		log: logrus.New(),
		channel: startErrorTestChannel{
			err: wantErr,
		},
	}

	require.ErrorIs(t, ep.Run(context.Background()), wantErr)
	ep.runMu.Lock()
	running := ep.running
	ep.runMu.Unlock()
	require.False(t, running)
}

type startErrorTestChannel struct {
	err error
}

func (c startErrorTestChannel) Start(context.Context) error { return c.err }
func (startErrorTestChannel) Run(context.Context) error     { return nil }
func (startErrorTestChannel) Close() error                  { return nil }
func (startErrorTestChannel) WriteFrame(can.Frame) error    { return nil }
