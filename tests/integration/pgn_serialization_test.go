package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/internal/adapter/canadapter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/boatkit-io/n2k/internal/subscribe"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// pgnCount represents a PGN type and its count
type pgnCount struct {
	name  string
	count int
}

func TestPGNSerializationFromN2K(t *testing.T) {
	// Get path to test data file
	testFile := testReplaySusterrana2020

	// Setup the file endpoint
	ca := canadapter.NewCANAdapter(logrus.New())
	ep := n2kfileendpoint.NewN2kFileEndpoint(testFile, logrus.New())

	// Create subscriber
	subs := subscribe.New()
	ps := pkt.NewPacketStruct()
	ps.SetOutput(subs)
	ca.SetOutput(ps)
	ep.SetOutput(ca)

	// Process each PGN
	var messageCount int
	pgnCounts := make(map[string]int)
	startTime := time.Now()
	_, err := subs.SubscribeToAllStructs(func(p any) {
		messageCount++

		// Track PGN types
		typeName := reflect.TypeOf(p).String()
		if idx := strings.LastIndex(typeName, "."); idx >= 0 {
			typeName = typeName[idx+1:]
		}
		pgnCounts[typeName]++

		if messageCount%1000 == 0 {
			elapsed := time.Since(startTime)
			fmt.Printf("Processed %d messages in %v (%.2f msg/sec)\n",
				messageCount, elapsed, float64(messageCount)/elapsed.Seconds())
		}
		// Handle both UnknownPGN and regular PGN structs
		var pgnStruct pgn.PgnStruct
		var ok bool

		// Skip UnknownPGN structs early - they can't be round-trip tested
		if _, isUnknownPtr := p.(*pgn.UnknownPGN); isUnknownPtr {
			return
		}

		// Since HandleStruct now passes pointers, try direct cast to PgnStruct
		if pgnStruct, ok = p.(pgn.PgnStruct); !ok {
			return // Skip non-PGN structs
		}

		// Create a datastream for serialization
		stream := pgn.NewDataStream(make([]uint8, 254))

		// Encode the PGN
		info, err := pgnStruct.Encode(stream)
		assert.NoError(t, err)
		if err != nil {
			return
		}

		// Trim stream data to actual length and reset position
		inStream := pgn.NewDataStream(stream.GetData())

		// Use the discriminator system to find and call the appropriate decode function
		decoder, err := pgn.FindDecoder(inStream, info.PGN)
		assert.NoError(t, err)
		if err != nil {
			fmt.Printf("While finding decoder for %T (PGN %d), got error: %v\n", pgnStruct, info.PGN, err)
			return
		}

		// Call the decode function with the MessageInfo and DataStream
		decoded, err := decoder(*info, inStream)
		if err != nil {
			assert.NoError(t, err)
			fmt.Printf("While decoding %T, got error: %v\n", pgnStruct, err)
			return
		}

		// Compare original and decoded PGNs
		// Since the original is a pointer and decoded is a value, dereference the original
		originalValue := reflect.ValueOf(pgnStruct).Elem().Interface()

		opts := cmp.Options{
			cmpopts.EquateEmpty(),
			cmpopts.EquateApprox(0.001, 0.001),
		}

		diff := cmp.Diff(originalValue, decoded, opts)
		assert.Empty(t, diff, "PGN roundtrip failed for %T", pgnStruct)
	})
	assert.NoError(t, err)

	// Run the endpoint to process the file
	ctx := context.Background()
	err = ep.Run(ctx)
	assert.NoError(t, err)

	// Print final statistics
	totalElapsed := time.Since(startTime)
	fmt.Printf("\n=== Final Statistics ===\n")
	fmt.Printf("Total messages: %d\n", messageCount)
	fmt.Printf("Total time: %v\n", totalElapsed)
	fmt.Printf("Average rate: %.2f msg/sec\n", float64(messageCount)/totalElapsed.Seconds())
	fmt.Printf("\nTop PGN types by count:\n")

	// Sort PGN counts by frequency
	var sorted []pgnCount
	for name, count := range pgnCounts {
		sorted = append(sorted, pgnCount{name, count})
	}
	// Simple sort by count (descending)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Show top 10
	limit := 10
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for i := 0; i < limit; i++ {
		fmt.Printf("  %s: %d\n", sorted[i].name, sorted[i].count)
	}
}

func TestComprehensivePerformanceProfiling(t *testing.T) {
	// Get all test files from integration directory
	integrationDir := "/home/russ/dev/n2k/n2kreplays/integration"
	files, err := filepath.Glob(filepath.Join(integrationDir, "*.n2k"))
	assert.NoError(t, err)
	assert.NotEmpty(t, files, "No .n2k files found in integration directory")

	fmt.Printf("Found %d test files to profile:\n", len(files))
	for _, file := range files {
		fmt.Printf("  %s\n", filepath.Base(file))
	}
	fmt.Println()

	// Track overall statistics
	overallStats := make(map[string]map[string]int) // filename -> pgn type -> count
	overallTimes := make(map[string]time.Duration)  // filename -> duration

	// Process each file
	for _, testFile := range files {
		fileName := filepath.Base(testFile)
		fmt.Printf("=== Processing %s ===\n", fileName)

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
		pgnCounts := make(map[string]int)
		startTime := time.Now()

		// Subscribe to all structs and track performance
		_, err := subs.SubscribeToAllStructs(func(p any) {
			messageCount++

			// Track PGN types
			typeName := reflect.TypeOf(p).String()
			if idx := strings.LastIndex(typeName, "."); idx >= 0 {
				typeName = typeName[idx+1:]
			}
			pgnCounts[typeName]++

			if messageCount%1000 == 0 {
				elapsed := time.Since(startTime)
				fmt.Printf("  Processed %d messages in %v (%.2f msg/sec)\n",
					messageCount, elapsed, float64(messageCount)/elapsed.Seconds())
			}
		})
		assert.NoError(t, err)

		// Run the endpoint
		ctx := context.Background()
		err = ep.Run(ctx)
		assert.NoError(t, err)

		// Calculate final statistics for this file
		totalElapsed := time.Since(startTime)
		overallTimes[fileName] = totalElapsed
		overallStats[fileName] = pgnCounts

		// Print statistics for this file
		fmt.Printf("  Total messages: %d\n", messageCount)
		fmt.Printf("  Total time: %v\n", totalElapsed)
		fmt.Printf("  Average rate: %.2f msg/sec\n", float64(messageCount)/totalElapsed.Seconds())

		// Show top 5 PGN types for this file
		var sortedCounts []pgnCount
		for name, count := range pgnCounts {
			sortedCounts = append(sortedCounts, pgnCount{name, count})
		}
		sort.Slice(sortedCounts, func(i, j int) bool {
			return sortedCounts[i].count > sortedCounts[j].count
		})

		fmt.Printf("  Top PGN types:\n")
		limit := 5
		if len(sortedCounts) < limit {
			limit = len(sortedCounts)
		}
		for i := 0; i < limit; i++ {
			fmt.Printf("    %s: %d\n", sortedCounts[i].name, sortedCounts[i].count)
		}
		fmt.Println()
	}

	// Print overall summary
	fmt.Printf("=== OVERALL SUMMARY ===\n")
	fmt.Printf("%-30s %10s %12s %15s\n", "File", "Messages", "Time", "Rate (msg/sec)")
	fmt.Printf("%-30s %10s %12s %15s\n", "----", "--------", "----", "-------------")

	var totalMessages int
	var totalTime time.Duration
	for _, testFile := range files {
		fileName := filepath.Base(testFile)
		fileStats := overallStats[fileName]
		fileTime := overallTimes[fileName]

		// Count total messages for this file
		fileMessageCount := 0
		for _, count := range fileStats {
			fileMessageCount += count
		}
		totalMessages += fileMessageCount
		totalTime += fileTime

		rate := float64(fileMessageCount) / fileTime.Seconds()
		fmt.Printf("%-30s %10d %12v %15.2f\n", fileName, fileMessageCount, fileTime, rate)
	}

	fmt.Printf("%-30s %10s %12s %15s\n", "----", "--------", "----", "-------------")
	fmt.Printf("%-30s %10d %12v %15.2f\n", "TOTAL", totalMessages, totalTime, float64(totalMessages)/totalTime.Seconds())
	fmt.Println()

	// Find performance outliers
	fmt.Printf("=== PERFORMANCE ANALYSIS ===\n")
	var fileRates []struct {
		name string
		rate float64
	}
	for _, testFile := range files {
		fileName := filepath.Base(testFile)
		fileStats := overallStats[fileName]
		fileTime := overallTimes[fileName]

		fileMessageCount := 0
		for _, count := range fileStats {
			fileMessageCount += count
		}

		rate := float64(fileMessageCount) / fileTime.Seconds()
		fileRates = append(fileRates, struct {
			name string
			rate float64
		}{fileName, rate})
	}

	// Sort by rate (slowest first)
	sort.Slice(fileRates, func(i, j int) bool {
		return fileRates[i].rate < fileRates[j].rate
	})

	fmt.Printf("Files sorted by processing rate (slowest first):\n")
	for i, fr := range fileRates {
		fmt.Printf("  %d. %s: %.2f msg/sec\n", i+1, fr.name, fr.rate)
	}

	// Calculate performance ratio between slowest and fastest
	if len(fileRates) > 1 {
		slowest := fileRates[0].rate
		fastest := fileRates[len(fileRates)-1].rate
		ratio := fastest / slowest
		fmt.Printf("\nPerformance ratio (fastest/slowest): %.1fx\n", ratio)
	}

	// Analyze PGN type distribution across files
	fmt.Printf("\n=== PGN TYPE DISTRIBUTION ANALYSIS ===\n")

	// Aggregate PGN counts across all files
	globalPgnCounts := make(map[string]int)
	for _, fileStats := range overallStats {
		for pgnType, count := range fileStats {
			globalPgnCounts[pgnType] += count
		}
	}

	// Sort global PGN counts
	var globalSorted []pgnCount
	for name, count := range globalPgnCounts {
		globalSorted = append(globalSorted, pgnCount{name, count})
	}
	sort.Slice(globalSorted, func(i, j int) bool {
		return globalSorted[i].count > globalSorted[j].count
	})

	fmt.Printf("Global PGN type distribution (top 15):\n")
	globalLimit := 15
	if len(globalSorted) < globalLimit {
		globalLimit = len(globalSorted)
	}
	for i := 0; i < globalLimit; i++ {
		fmt.Printf("  %s: %d\n", globalSorted[i].name, globalSorted[i].count)
	}

	// Find files with high concentration of slow PGN types
	fmt.Printf("\n=== SLOW PGN TYPE CONCENTRATION ===\n")

	// Define known slow PGN types (based on our earlier analysis)
	slowPgnTypes := map[string]bool{
		"NmeaAcknowledgeGroupFunctionPartial": true,
		"NmeaRequestGroupFunctionPartial":     true,
		"NmeaCommandGroupFunctionPartial":     true,
		"UnknownPGN":                          true, // These might be slow due to processing overhead
	}

	for _, testFile := range files {
		fileName := filepath.Base(testFile)
		fileStats := overallStats[fileName]
		fileTime := overallTimes[fileName]

		// Count slow PGN types in this file
		slowCount := 0
		totalCount := 0
		var slowPgnsInFile []pgnCount

		for pgnType, count := range fileStats {
			totalCount += count
			if slowPgnTypes[pgnType] {
				slowCount += count
				slowPgnsInFile = append(slowPgnsInFile, pgnCount{pgnType, count})
			}
		}

		slowPercentage := float64(slowCount) / float64(totalCount) * 100
		rate := float64(totalCount) / fileTime.Seconds()

		fmt.Printf("  %s: %.1f%% slow PGNs, %.2f msg/sec\n", fileName, slowPercentage, rate)

		// Show which slow PGN types are in this file
		if len(slowPgnsInFile) > 0 {
			// Sort by count (descending)
			sort.Slice(slowPgnsInFile, func(i, j int) bool {
				return slowPgnsInFile[i].count > slowPgnsInFile[j].count
			})

			fmt.Printf("    Slow PGNs in %s:\n", fileName)
			for _, slowPgn := range slowPgnsInFile {
				percentage := float64(slowPgn.count) / float64(totalCount) * 100
				fmt.Printf("      %s: %d (%.1f%%)\n", slowPgn.name, slowPgn.count, percentage)
			}
		} else {
			fmt.Printf("    No slow PGNs detected in %s\n", fileName)
		}
		fmt.Println()
	}
}
