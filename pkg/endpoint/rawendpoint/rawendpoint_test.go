package rawendpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type blockingHandler struct {
	entered chan struct{}
	release chan struct{}
}

func (h *blockingHandler) HandleMessage(endpoint.Message) {
	close(h.entered)
	<-h.release
}

func TestRawEndpointCloseStopsRunAndIsTerminal(t *testing.T) {
	ep := NewRawEndpoint("", logrus.New())
	runDone := make(chan error, 1)
	go func() {
		runDone <- ep.Run(context.Background())
	}()
	require.Eventually(t, func() bool {
		ep.mu.Lock()
		defer ep.mu.Unlock()
		return ep.running
	}, time.Second, time.Millisecond)

	require.NoError(t, ep.Close())
	select {
	case err := <-runDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not stop Run")
	}
	require.ErrorContains(t, ep.Start(context.Background()), "closed")
}

func TestRawFileRunRejectsConcurrentPlayback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.raw")
	frame := can.Frame{ID: can.MaskEff | 0x19f80123, Length: 1, Data: [8]byte{1}}
	require.NoError(t, os.WriteFile(path, []byte(converter.RawFromCanFrame(frame)), 0o600))
	handler := &blockingHandler{entered: make(chan struct{}), release: make(chan struct{})}
	ep := NewRawFileEndpoint(path, logrus.New())
	ep.SetOutput(handler)

	runDone := make(chan error, 1)
	go func() {
		runDone <- ep.Run(context.Background())
	}()
	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("playback did not deliver a frame")
	}
	require.ErrorContains(t, ep.Run(context.Background()), "already running")
	close(handler.release)
	require.NoError(t, <-runDone)
}

func TestRawFileClosePreventsRunFromReopeningFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.raw")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	ep := NewRawFileEndpoint(path, logrus.New())

	require.NoError(t, ep.Start(context.Background()))
	require.NoError(t, ep.Close())
	require.ErrorContains(t, ep.Run(context.Background()), "closed")
}
