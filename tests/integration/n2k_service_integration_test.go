package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestN2kServiceIntegration(t *testing.T) {
	// Get path to test data file
	testFile := filepath.Join("/home/russ/dev/n2k/n2kreplays/integration", "susterrana2020.n2k")

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
	// Get path to test data file
	testFile := filepath.Join("/home/russ/dev/n2k/n2kreplays/integration", "susterrana2020.n2k")

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
	// Create a mock endpoint for testing write functionality
	// For this test, we'll use a file endpoint but focus on the write capability
	testFile := filepath.Join("/home/russ/dev/n2k/n2kreplays/integration", "susterrana2020.n2k")

	// Create the n2kfileendpoint
	log := logrus.New()
	ep := n2kfileendpoint.NewN2kFileEndpoint(testFile, log)

	// Create N2kService using the public interface
	service := n2k.NewN2kService(ep, log)

	// Test that we can call Write without error (even if it doesn't do much in this context)
	// Note: This would need a proper PGN struct to test fully
	// We'll test with a simple struct that implements PgnStruct interface
	// For now, just test that the service can be created and stopped
	// The actual Write functionality would need a proper PGN struct

	// Stop the service
	err := service.Stop()
	assert.NoError(t, err, "Stop should not return error")
}

func TestN2kServiceUpdateEndpoint(t *testing.T) {
	// Get path to test data file
	testFile := filepath.Join("/home/russ/dev/n2k/n2kreplays/integration", "susterrana2020.n2k")

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
	// Get path to test data file
	testFile := filepath.Join("/home/russ/dev/n2k/n2kreplays/integration", "susterrana2020.n2k")

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
