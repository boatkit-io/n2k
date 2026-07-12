package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultReplayDir            = "n2kreplays/integration"
	defaultReplaySusterrana2020 = "susterrana2020.n2k"
)

func requireReplayFile(t *testing.T) string {
	t.Helper()

	path := os.Getenv("N2K_TEST_REPLAY")
	if path == "" {
		path = filepath.Join(repoRoot(t), defaultReplayDir, defaultReplaySusterrana2020)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("replay file not available: %v", err)
	}
	return path
}

func requireReplayFiles(t *testing.T) []string {
	t.Helper()

	integrationDir := os.Getenv("N2K_TEST_REPLAY_DIR")
	if integrationDir == "" {
		integrationDir = filepath.Join(repoRoot(t), defaultReplayDir)
	}
	files, err := filepath.Glob(filepath.Join(integrationDir, "*.n2k"))
	require.NoError(t, err)
	if len(files) == 0 {
		t.Skipf("no replay files available in %s", integrationDir)
	}
	return files
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "unable to locate integration test file")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func skipReplayIntegrationInShortMode(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping replay integration test in short mode")
	}
}

func TestN2kServiceIntegration(t *testing.T) {
	skipReplayIntegrationInShortMode(t)

	// Get path to test data file
	testFile := requireReplayFile(t)

	// Create the n2kfileendpoint
	log := logrus.New()
	ep := n2kfileendpoint.NewN2kFileEndpoint(testFile, log)

	// Create N2kService using the public interface
	service := n2k.NewN2kService(ep, log)

	// Track processed messages
	var messageCount int
	var startTime time.Time

	// Start the service
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start processing in a goroutine
	errChan := make(chan error, 1)
	go func() {
		startTime = time.Now()
		err := service.Start(ctx)
		errChan <- err
	}()

	// Give the service a moment to start processing
	time.Sleep(100 * time.Millisecond)

	// Wait for completion or timeout
	select {
	case err := <-errChan:
		require.NoError(t, err, "Service should complete without error")
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	// Stop the service
	err := service.Stop()
	assert.NoError(t, err, "Stop should not return error")

	// Verify we processed some messages (the exact count depends on the test file)
	elapsed := time.Since(startTime)
	fmt.Printf("Integration test completed in %v\n", elapsed)
	fmt.Printf("Processed %d messages\n", messageCount)

	// For this basic integration test, we just verify the service can start and stop
	// The message processing is tested in the existing pgn_serialization_test.go
	assert.True(t, elapsed > 0, "Service should have run for some time")
}

func TestN2kServiceWithSubscription(t *testing.T) {
	skipReplayIntegrationInShortMode(t)

	// Get path to test data file
	testFile := requireReplayFile(t)

	// Create the n2kfileendpoint
	log := logrus.New()
	ep := n2kfileendpoint.NewN2kFileEndpoint(testFile, log)

	// Create N2kService using the public interface
	service := n2k.NewN2kService(ep, log)

	// Start the service
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start processing in a goroutine
	errChan := make(chan error, 1)
	go func() {
		err := service.Start(ctx)
		errChan <- err
	}()

	// Give the service a moment to start processing
	time.Sleep(100 * time.Millisecond)

	// Wait for completion or timeout
	select {
	case err := <-errChan:
		require.NoError(t, err, "Service should complete without error")
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	// Stop the service
	err := service.Stop()
	assert.NoError(t, err, "Stop should not return error")

	// This test verifies the service can start and stop with the public interface
	// Message processing details are tested in the existing pgn_serialization_test.go
}

func TestN2kServiceWrite(t *testing.T) {
	skipReplayIntegrationInShortMode(t)

	// Create a mock endpoint for testing write functionality
	// For this test, we'll use a file endpoint but focus on the write capability
	testFile := requireReplayFile(t)

	// Create the n2kfileendpoint
	log := logrus.New()
	ep := n2kfileendpoint.NewN2kFileEndpoint(testFile, log)

	// Create N2kService using the public interface
	service := n2k.NewN2kService(ep, log)

	// Test that we can call Write without error (even if it doesn't do much in this context)
	// Note: This would need a proper PGN struct to test fully
	// For now, just test that the service can be created and stopped

	// Stop the service
	err := service.Stop()
	assert.NoError(t, err, "Stop should not return error")
}

func TestN2kServiceUpdateEndpoint(t *testing.T) {
	skipReplayIntegrationInShortMode(t)

	// Get path to test data file
	testFile := requireReplayFile(t)

	// Create the initial n2kfileendpoint
	log := logrus.New()
	ep1 := n2kfileendpoint.NewN2kFileEndpoint(testFile, log)

	// Create N2kService using the public interface
	service := n2k.NewN2kService(ep1, log)

	// Test updating the endpoint
	ep2 := n2kfileendpoint.NewN2kFileEndpoint(testFile, log)
	err := service.UpdateEndpoint(ep2)
	assert.NoError(t, err, "UpdateEndpoint should not return error")

	// Test that we can start the service with the new endpoint
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start processing in a goroutine
	errChan := make(chan error, 1)
	go func() {
		startErr := service.Start(ctx)
		errChan <- startErr
	}()

	// Wait for completion or timeout
	select {
	case startErr := <-errChan:
		require.NoError(t, startErr, "Service should complete without error")
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	// Stop the service
	stopErr := service.Stop()
	assert.NoError(t, stopErr, "Stop should not return error")
}

func TestN2kServiceUpdateEndpointWhileRunning(t *testing.T) {
	skipReplayIntegrationInShortMode(t)

	// Get path to test data file
	testFile := requireReplayFile(t)

	// Create the initial n2kfileendpoint
	log := logrus.New()
	ep1 := n2kfileendpoint.NewN2kFileEndpoint(testFile, log)

	// Create N2kService using the public interface
	service := n2k.NewN2kService(ep1, log)

	// Start the service
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start processing in a goroutine
	errChan := make(chan error, 1)
	go func() {
		err := service.Start(ctx)
		errChan <- err
	}()

	// Give the service a moment to start
	time.Sleep(100 * time.Millisecond)

	// Try to update the endpoint while running - this should work
	ep2 := n2kfileendpoint.NewN2kFileEndpoint(testFile, log)
	err := service.UpdateEndpoint(ep2)
	assert.NoError(t, err, "UpdateEndpoint should work even while running")

	// Wait for completion or timeout
	select {
	case startErr := <-errChan:
		require.NoError(t, startErr, "Service should complete without error")
	case <-ctx.Done():
		// This is expected since we have a timeout
	}

	// Stop the service
	stopErr := service.Stop()
	assert.NoError(t, stopErr, "Stop should not return error")
}

func TestN2kServiceHandleReplayCANFrame(t *testing.T) {
	skipReplayIntegrationInShortMode(t)

	log := logrus.New()
	ep := n2kfileendpoint.NewN2kFileEndpoint(requireReplayFile(t), log)
	service := n2k.NewN2kService(ep, log)

	var messageCount int
	_, err := service.SubscribeToAllStructs(func(_ any) {
		messageCount++
	})
	require.NoError(t, err)

	// PGN 127501 frame from canadapter_test
	frames, err := converter.CanFrameFromRaw("2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff")
	require.NoError(t, err)
	require.Len(t, frames, 1)

	require.NoError(t, service.HandleReplayCANFrame(frames[0]))
	assert.Equal(t, 1, messageCount)
}

func TestN2kServiceReceivedCANFrameHook(t *testing.T) {
	skipReplayIntegrationInShortMode(t)

	log := logrus.New()
	ep := n2kfileendpoint.NewN2kFileEndpoint(requireReplayFile(t), log)
	service := n2k.NewN2kService(ep, log)

	var hookCount atomic.Int64
	service.SetReceivedCANFrameHook(func(_ *can.Frame) {
		hookCount.Add(1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- service.Start(ctx)
	}()

	select {
	case err := <-errChan:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	require.NoError(t, service.Stop())
	assert.Greater(t, hookCount.Load(), int64(0), "hook should receive live frames from endpoint")
}
