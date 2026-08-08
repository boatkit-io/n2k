package n2kfileendpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint"
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

func TestRunRejectsConcurrentPlayback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.n2k")
	require.NoError(t, os.WriteFile(path, []byte(" (0.000000)  can1  08FF0401   [1]  00\n"), 0o600))
	handler := &blockingHandler{entered: make(chan struct{}), release: make(chan struct{})}
	ep := NewN2kFileEndpoint(path, logrus.New())
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

func TestClosePreventsRunFromReopeningFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.n2k")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	ep := NewN2kFileEndpoint(path, logrus.New())

	require.NoError(t, ep.Start(context.Background()))
	require.NoError(t, ep.Close())
	require.ErrorContains(t, ep.Run(context.Background()), "closed")
}
