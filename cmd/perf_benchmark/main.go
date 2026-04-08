// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package main runs local performance benchmarks against replay data.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"time"

	"github.com/boatkit-io/n2k/internal/adapter/canadapter"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/boatkit-io/n2k/internal/subscribe"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/sirupsen/logrus"
)

func main() {
	// Disable logging for performance
	logrus.SetLevel(logrus.ErrorLevel)

	// Get all test files from integration directory
	// Using absolute path as seen in the test file
	integrationDir := "/home/russ/dev/n2k/n2kreplays/integration"
	files, err := filepath.Glob(filepath.Join(integrationDir, "*.n2k"))
	if err != nil {
		fmt.Printf("Error finding files: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Println("No .n2k files found in integration directory")
		os.Exit(1)
	}

	fmt.Printf("Found %d test files to profile\n", len(files))

	// Track overall statistics
	var totalMessages int
	var totalTime time.Duration
	var totalAllocBytes uint64

	fmt.Println("filename,messages,time_sec,rate_msg_sec,alloc_mb,total_alloc_mb,sys_mb,gc_cycles")

	// Process each file
	for _, testFile := range files {
		fileName := filepath.Base(testFile)

		// Force GC before starting to get a clean baseline
		runtime.GC()
		var m1, m2 runtime.MemStats
		runtime.ReadMemStats(&m1)

		// Setup the file endpoint
		ca := canadapter.NewCANAdapter(logrus.New())
		ep := n2kfileendpoint.NewN2kFileEndpoint(testFile, logrus.New())

		// Create subscriber
		subs := subscribe.New()
		ps := pkt.NewPacketStruct()
		ps.SetOutput(subs)
		ca.SetOutput(ps)
		ep.SetOutput(ca)

		// Track performance metrics for this file
		var messageCount int
		startTime := time.Now()

		// Subscribe to all structs
		_, err := subs.SubscribeToAllStructs(func(p any) {
			messageCount++

			// Simple type check to ensure we're doing the work
			_ = reflect.TypeOf(p).String()
		})
		if err != nil {
			fmt.Printf("Error subscribing: %v\n", err)
			continue
		}

		// Run the endpoint
		ctx := context.Background()
		err = ep.Run(ctx)
		if err != nil {
			fmt.Printf("Error running endpoint for %s: %v\n", fileName, err)
		}

		elapsed := time.Since(startTime)

		// Capture memory stats after processing
		runtime.ReadMemStats(&m2)

		totalMessages += messageCount
		totalTime += elapsed
		totalAllocBytes += m2.TotalAlloc - m1.TotalAlloc

		rate := float64(messageCount) / elapsed.Seconds()
		allocMb := float64(m2.Alloc) / 1024 / 1024
		totalAllocMb := float64(m2.TotalAlloc-m1.TotalAlloc) / 1024 / 1024
		sysMb := float64(m2.Sys) / 1024 / 1024
		gcCycles := m2.NumGC - m1.NumGC

		fmt.Printf("%s,%d,%.4f,%.2f,%.2f,%.2f,%.2f,%d\n",
			fileName, messageCount, elapsed.Seconds(), rate, allocMb, totalAllocMb, sysMb, gcCycles)
	}

	fmt.Printf("TOTAL,%d,%.4f,%.2f,%.2f,%.2f,%.2f,0\n",
		totalMessages, totalTime.Seconds(), float64(totalMessages)/totalTime.Seconds(),
		0.0, float64(totalAllocBytes)/1024/1024, 0.0)
}
