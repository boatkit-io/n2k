package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/boatkit-io/n2k/pkg/nodesim"
	"github.com/sirupsen/logrus"
)

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	// Command-line parsing
	var scenarioFile string
	var nodeID uint8
	var outputFile string
	var verbose bool
	var listScenarios bool
	var validateOnly bool

	flag.StringVar(&scenarioFile, "scenario", "", "YAML scenario file path (required)")
	var nodeIDFlag uint
	flag.UintVar(&nodeIDFlag, "node-id", 0, "Node ID to simulate (0-253, required)")
	flag.StringVar(&outputFile, "output", "", "Output file for generated responses (default: stdout)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&listScenarios, "list", false, "List available scenarios in the file and exit")
	flag.BoolVar(&validateOnly, "validate", false, "Validate scenario file and exit")
	flag.Parse()

	// Setup logging
	log := logrus.New()
	if verbose {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}

	// Validate required arguments
	if scenarioFile == "" {
		log.Fatal("--scenario flag is required")
	}

	nodeID = uint8(nodeIDFlag)
	if nodeID == 0 && !listScenarios && !validateOnly {
		log.Fatal("--node-id flag is required (1-253)")
	}

	if nodeID > 253 {
		log.Fatal("--node-id must be between 1 and 253")
	}

	// Create node simulator
	simulator, err := nodesim.NewNodeSimulator(scenarioFile, nodeID, log)
	if err != nil {
		log.WithError(err).Fatal("Failed to create node simulator")
	}

	// Handle special flags
	if listScenarios {
		scenarios := simulator.ListScenarios()
		fmt.Println("Available scenarios:")
		for _, scenario := range scenarios {
			fmt.Printf("  - %s: %s\n", scenario.Name, scenario.Description)
		}
		return
	}

	if validateOnly {
		if err := simulator.Validate(); err != nil {
			log.WithError(err).Fatal("Scenario validation failed")
		}
		log.Info("Scenario file validation passed")
		return
	}

	// Setup output
	if outputFile != "" {
		file, err := os.Create(outputFile)
		if err != nil {
			log.WithError(err).Fatal("Failed to create output file")
		}
		defer file.Close()
		simulator.SetOutput(file)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Received shutdown signal, stopping simulator...")
		cancel()
	}()

	// Start simulator
	log.WithFields(logrus.Fields{
		"scenario": scenarioFile,
		"nodeID":   nodeID,
		"output":   outputFile,
	}).Info("Starting node simulator")

	if err := simulator.Run(ctx); err != nil {
		log.WithError(err).Fatal("Simulator failed")
	}

	log.Info("Node simulator stopped")
}
