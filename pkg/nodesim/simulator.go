package nodesim

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// NodeSimulator simulates NMEA 2000 device behavior based on YAML scenarios
type NodeSimulator struct {
	scenarioFile string
	nodeID       uint8
	log          *logrus.Logger
	output       io.Writer
	scenarios    *ScenarioConfig
}

// ScenarioConfig represents the top-level YAML configuration
type ScenarioConfig struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Version     string     `yaml:"version"`
	Scenarios   []Scenario `yaml:"scenarios"`
}

// Scenario represents a single test scenario
type Scenario struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	NodeType    string              `yaml:"node_type"`
	Triggers    []Trigger           `yaml:"triggers"`
	Responses   map[string]Response `yaml:"responses"`
	Defaults    DefaultBehavior     `yaml:"defaults"`
}

// Trigger defines what network events cause responses
type Trigger struct {
	Type      string            `yaml:"type"`
	PGN       uint32            `yaml:"pgn,omitempty"`
	Source    uint8             `yaml:"source,omitempty"`
	Data      string            `yaml:"data,omitempty"`
	Condition map[string]string `yaml:"condition,omitempty"`
	Response  string            `yaml:"response"`
}

// Response defines what to send back
type Response struct {
	Type     string `yaml:"type"`
	PGN      uint32 `yaml:"pgn"`
	Priority uint8  `yaml:"priority"`
	Data     string `yaml:"data"`
	Delay    string `yaml:"delay,omitempty"`
}

// DefaultBehavior defines automatic behaviors
type DefaultBehavior struct {
	Heartbeat HeartbeatConfig `yaml:"heartbeat,omitempty"`
}

// HeartbeatConfig defines periodic message sending
type HeartbeatConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
	PGN      uint32 `yaml:"pgn"`
	Data     string `yaml:"data"`
}

// ScenarioInfo provides summary information about a scenario
type ScenarioInfo struct {
	Name        string
	Description string
}

// NewNodeSimulator creates a new node simulator instance
func NewNodeSimulator(scenarioFile string, nodeID uint8, log *logrus.Logger) (*NodeSimulator, error) {
	simulator := &NodeSimulator{
		scenarioFile: scenarioFile,
		nodeID:       nodeID,
		log:          log,
		output:       os.Stdout,
	}

	// Load and parse scenario file
	if err := simulator.loadScenarios(); err != nil {
		return nil, fmt.Errorf("failed to load scenarios: %w", err)
	}

	return simulator, nil
}

// SetOutput sets the output writer for generated responses
func (ns *NodeSimulator) SetOutput(output io.Writer) {
	ns.output = output
}

// ListScenarios returns a list of available scenarios
func (ns *NodeSimulator) ListScenarios() []ScenarioInfo {
	var scenarios []ScenarioInfo
	for _, scenario := range ns.scenarios.Scenarios {
		scenarios = append(scenarios, ScenarioInfo{
			Name:        scenario.Name,
			Description: scenario.Description,
		})
	}
	return scenarios
}

// Validate checks the scenario file for errors
func (ns *NodeSimulator) Validate() error {
	// Basic validation of loaded scenarios
	if ns.scenarios == nil {
		return fmt.Errorf("no scenarios loaded")
	}

	if len(ns.scenarios.Scenarios) == 0 {
		return fmt.Errorf("no scenarios defined")
	}

	for i, scenario := range ns.scenarios.Scenarios {
		if scenario.Name == "" {
			return fmt.Errorf("scenario %d missing name", i)
		}

		// Validate triggers
		for j, trigger := range scenario.Triggers {
			if trigger.Type == "" {
				return fmt.Errorf("scenario %s trigger %d missing type", scenario.Name, j)
			}
			if trigger.Response == "" {
				return fmt.Errorf("scenario %s trigger %d missing response", scenario.Name, j)
			}

			// Check that referenced response exists
			if _, exists := scenario.Responses[trigger.Response]; !exists {
				return fmt.Errorf("scenario %s trigger %d references unknown response '%s'", scenario.Name, j, trigger.Response)
			}
		}

		// Validate responses
		for name, response := range scenario.Responses {
			if response.Type == "" {
				return fmt.Errorf("scenario %s response %s missing type", scenario.Name, name)
			}
			if response.PGN == 0 {
				return fmt.Errorf("scenario %s response %s missing PGN", scenario.Name, name)
			}
		}
	}

	return nil
}

// Run starts the node simulator
func (ns *NodeSimulator) Run(ctx context.Context) error {
	ns.log.Info("Node simulator started")

	// For now, we'll focus on heartbeat functionality and response generation
	// In a full implementation, this would integrate with the network stack

	// Start heartbeat goroutines for scenarios that have them enabled
	for _, scenario := range ns.scenarios.Scenarios {
		if scenario.Defaults.Heartbeat.Enabled {
			go ns.runHeartbeat(ctx, scenario)
		}
	}

	// TODO: In a full implementation, this would:
	// 1. Listen for incoming network messages (via rawendpoint or similar)
	// 2. Match against scenario trigger conditions
	// 3. Generate appropriate responses

	// For demonstration, let's generate some sample responses
	ns.log.Info("Generating sample responses for demonstration...")

	// Generate responses for the first scenario
	if len(ns.scenarios.Scenarios) > 0 {
		scenario := ns.scenarios.Scenarios[0]
		for name, response := range scenario.Responses {
			rawOutput, err := ns.generateRawResponse(response)
			if err != nil {
				ns.log.WithError(err).Warnf("Failed to generate response %s", name)
				continue
			}

			ns.log.Infof("Generated response %s: %s", name, strings.TrimSpace(rawOutput))
			fmt.Fprint(ns.output, rawOutput)
		}
	}

	// Wait for context cancellation
	<-ctx.Done()

	ns.log.Info("Node simulator stopping")
	return nil
}

// runHeartbeat sends periodic heartbeat messages for a scenario
func (ns *NodeSimulator) runHeartbeat(ctx context.Context, scenario Scenario) {
	interval, err := time.ParseDuration(scenario.Defaults.Heartbeat.Interval)
	if err != nil {
		ns.log.WithError(err).Warnf("Invalid heartbeat interval for scenario %s", scenario.Name)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ns.log.Infof("Starting heartbeat for scenario %s (interval: %s)", scenario.Name, interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			response := Response{
				Type:     "data",
				PGN:      scenario.Defaults.Heartbeat.PGN,
				Priority: 3, // Default priority for heartbeat
				Data:     scenario.Defaults.Heartbeat.Data,
			}

			rawOutput, err := ns.generateRawResponse(response)
			if err != nil {
				ns.log.WithError(err).Warnf("Failed to generate heartbeat for scenario %s", scenario.Name)
				continue
			}

			ns.log.Debugf("Heartbeat %s: %s", scenario.Name, strings.TrimSpace(rawOutput))
			fmt.Fprint(ns.output, rawOutput)
		}
	}
}

// generateRawResponse converts a Response definition to raw format output
func (ns *NodeSimulator) generateRawResponse(response Response) (string, error) {
	// Parse the hex data string
	dataBytes, err := ns.parseHexData(response.Data)
	if err != nil {
		return "", fmt.Errorf("failed to parse response data: %w", err)
	}

	// Create a timestamp (current time)
	timestamp := time.Now().Format("2006-01-02T15:04:05Z")

	// Build the raw format string
	// Format: timestamp,priority,pgn,source,destination,length,data...
	var rawParts []string
	rawParts = append(rawParts, timestamp)
	rawParts = append(rawParts, fmt.Sprintf("%d", response.Priority))
	rawParts = append(rawParts, fmt.Sprintf("%d", response.PGN))
	rawParts = append(rawParts, fmt.Sprintf("%d", ns.nodeID))
	rawParts = append(rawParts, "255") // Broadcast destination
	rawParts = append(rawParts, fmt.Sprintf("%d", len(dataBytes)))

	// Add data bytes as hex
	for _, b := range dataBytes {
		rawParts = append(rawParts, fmt.Sprintf("%02x", b))
	}

	// Pad to 8 bytes if needed
	for len(rawParts) < 14 { // timestamp + 5 fields + 8 data bytes = 14 total
		rawParts = append(rawParts, "ff")
	}

	return strings.Join(rawParts, ",") + "\n", nil
}

// parseHexData converts a comma-separated hex string to bytes
func (ns *NodeSimulator) parseHexData(hexStr string) ([]byte, error) {
	if hexStr == "" {
		return nil, fmt.Errorf("empty data string")
	}

	parts := strings.Split(hexStr, ",")
	var bytes []byte

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		b, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid hex byte at position %d: %s", i, part)
		}
		bytes = append(bytes, uint8(b))
	}

	return bytes, nil
}

// loadScenarios reads and parses the YAML scenario file
func (ns *NodeSimulator) loadScenarios() error {
	data, err := os.ReadFile(ns.scenarioFile)
	if err != nil {
		return fmt.Errorf("failed to read scenario file: %w", err)
	}

	ns.scenarios = &ScenarioConfig{}
	if err := yaml.Unmarshal(data, ns.scenarios); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	return nil
}
