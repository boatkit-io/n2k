package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/boatkit-io/n2k/pkg/endpoint/rawendpoint"
	"github.com/boatkit-io/n2k/pkg/n2k"
	"github.com/sirupsen/logrus"
)

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing
	var replayFile string
	var rawReplayFile string
	flag.StringVar(&replayFile, "replayFile", "", "An optional n2k replay file to run")
	flag.StringVar(&rawReplayFile, "rawReplayFile", "", "An optional raw replay file to run")
	var dumpPgns bool
	var checkUnseen bool
	var checkMissingOrInvalid bool
	var writeRaw bool
	var rawOutFile string
	flag.StringVar(&rawOutFile, "rawOutFile", "", "if writePgns, optionally dump into this file")
	flag.BoolVar(&dumpPgns, "dumpPgns", false, "Debug spew all PGNs coming down the pipe")
	flag.BoolVar(&checkUnseen, "checkUnseen", false, "Check if any of the messages are pgns not yet seen")
	flag.BoolVar(&checkMissingOrInvalid, "checkMissingOrInvalid", false, "Check if any numeric values are missing or invalid")
	flag.BoolVar(&writeRaw, "writeRaw", false, "write out PGN structs as RAW canbus frames")
	flag.Parse()

	if replayFile == "" && rawReplayFile == "" {
		fmt.Fprintf(os.Stderr, "Error: either -replayFile or -rawReplayFile must be specified\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -replayFile <file.n2k> OR -rawReplayFile <file.raw>\n", os.Args[0])
		exitCode = 1
		return
	}

	log := logrus.StandardLogger()
	log.Infof("in replayfile, dump:%t, checkUnseen:%t writeRaw:%t file:%s rawFile:%s\n",
		dumpPgns, checkUnseen, writeRaw, replayFile, rawReplayFile)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info("Received shutdown signal, stopping...")
		cancel()
	}()

	// Create the appropriate endpoint
	var endpoint endpoint.Endpoint
	if len(replayFile) > 0 && strings.HasSuffix(replayFile, ".n2k") {
		endpoint = n2kfileendpoint.NewN2kFileEndpoint(replayFile, log)
	} else if len(rawReplayFile) > 0 {
		endpoint = rawendpoint.NewRawFileEndpoint(rawReplayFile, log)
	}

	// Create n2k service
	bus := n2k.NewN2kService(endpoint, log)

	// Start the service to set up the processing pipeline
	if err := bus.Start(ctx); err != nil {
		log.Errorf("n2k service exited with error: %v", err)
		exitCode = 1
		return
	}

	// Set up subscriptions and message processing tracking
	var messageCount int64
	var lastMessageTime time.Time
	var mu sync.Mutex
	var processingStarted bool

	if dumpPgns {
		_, err := bus.SubscribeToAllStructs(func(p any) {
			log.Infof("Handling PGN: %s", n2k.DebugDumpPGN(p))
			mu.Lock()
			messageCount++
			lastMessageTime = time.Now()
			processingStarted = true
			mu.Unlock()
		})
		if err != nil {
			log.Errorf("failed to subscribe to all structs: %v", err)
			exitCode = 1
			return
		}
	}

	if writeRaw {
		// For writing raw output, we need to set up a raw output endpoint
		// and connect it to the bus for writing
		// Note: This would require extending the n2k service to support raw output
		// For now, we'll handle this in the subscription callback
		_, err := bus.SubscribeToAllStructs(func(p any) {
			// Convert PGN back to raw frames and write them
			// This is a simplified approach - in practice you might want to
			// use the internal publisher directly
			log.Debugf("Would write PGN to raw output: %s", n2k.DebugDumpPGN(p))
		})
		if err != nil {
			log.Errorf("failed to subscribe for raw output: %v", err)
			exitCode = 1
			return
		}
	}

	// Run the endpoint directly to process the file
	if err := endpoint.Run(ctx); err != nil {
		log.Errorf("endpoint run error: %v", err)
		exitCode = 1
		return
	}

	// Wait for message processing to complete
	if dumpPgns || writeRaw {
		log.Infof("Waiting for message processing to complete...")

		// Wait for a period of inactivity to ensure all messages are processed
		for {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			started := processingStarted
			timeSinceLastMessage := time.Since(lastMessageTime)
			mu.Unlock()

			// If no messages have been processed yet, or if processing started and it's been quiet for 500ms
			if !started || timeSinceLastMessage > 500*time.Millisecond {
				break
			}
		}

		mu.Lock()
		finalCount := messageCount
		mu.Unlock()
		log.Infof("Message processing complete. Processed %d messages.", finalCount)
	}

	// Stop the bus and exit
	log.Info("Shutting down...")
	bus.Stop()
	log.Info("Shutdown complete")
}
