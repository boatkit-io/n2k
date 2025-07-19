package nodesim

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/sirupsen/logrus"
)

// ResponseInjector handles dynamic response generation based on network triggers
type ResponseInjector struct {
	// Configuration
	triggers []ResponseTrigger
	nodeID   uint8
	log      *logrus.Logger

	// Network integration
	publisher *pgn.Publisher

	// State management
	triggerStates map[string]*TriggerState
	responseQueue chan *TimedResponse

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mutex  sync.RWMutex
}

// ResponseTrigger defines conditions that generate responses
type ResponseTrigger struct {
	Name        string
	Description string

	// Trigger conditions
	MatchPGN    uint32
	MatchSource uint8
	MatchTarget uint8
	MatchData   []byte // Optional data pattern

	// Response generation
	ResponsePGN    uint32
	ResponseSource uint8
	ResponseTarget uint8
	ResponseData   []byte
	ResponseDelay  time.Duration

	// Trigger behavior
	TriggerOnce  bool
	MaxTriggers  int
	ActiveWindow time.Duration // Time window when trigger is active
}

// TriggerState tracks the state of a response trigger
type TriggerState struct {
	TriggerCount  int
	LastTriggered time.Time
	Enabled       bool
}

// TimedResponse represents a response to be sent at a specific time
type TimedResponse struct {
	PGN         uint32
	Source      uint8
	Target      uint8
	Data        []byte
	SendTime    time.Time
	TriggerName string
}

// NewResponseInjector creates a new ResponseInjector instance
func NewResponseInjector(nodeID uint8, publisher *pgn.Publisher, log *logrus.Logger) *ResponseInjector {
	ctx, cancel := context.WithCancel(context.Background())

	return &ResponseInjector{
		nodeID:        nodeID,
		publisher:     publisher,
		log:           log,
		triggerStates: make(map[string]*TriggerState),
		responseQueue: make(chan *TimedResponse, 100),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// AddTrigger adds a new response trigger
func (ri *ResponseInjector) AddTrigger(trigger ResponseTrigger) {
	ri.mutex.Lock()
	defer ri.mutex.Unlock()

	ri.triggers = append(ri.triggers, trigger)
	ri.triggerStates[trigger.Name] = &TriggerState{
		TriggerCount: 0,
		Enabled:      true,
	}

	ri.log.Debugf("Added response trigger: %s", trigger.Name)
}

// Start begins processing network messages and generating responses
func (ri *ResponseInjector) Start() error {
	ri.log.Info("Starting response injector")

	// Start response queue processor
	ri.wg.Add(1)
	go ri.processResponseQueue()

	return nil
}

// Stop shuts down the response injector
func (ri *ResponseInjector) Stop() {
	ri.log.Info("Stopping response injector")
	ri.cancel()
	close(ri.responseQueue)
	ri.wg.Wait()
}

// HandlePacket processes incoming network packets and checks for triggers
func (ri *ResponseInjector) HandlePacket(packet pkt.Packet) {
	ri.mutex.RLock()
	defer ri.mutex.RUnlock()

	// Check each trigger for matches
	for _, trigger := range ri.triggers {
		state := ri.triggerStates[trigger.Name]
		if !state.Enabled {
			continue
		}

		// Check if trigger conditions are met
		if ri.matchesTrigger(packet, trigger, state) {
			ri.log.Debugf("Trigger matched: %s for PGN %d", trigger.Name, packet.Info.PGN)

			// Schedule response
			response := &TimedResponse{
				PGN:         trigger.ResponsePGN,
				Source:      trigger.ResponseSource,
				Target:      trigger.ResponseTarget,
				Data:        trigger.ResponseData,
				SendTime:    time.Now().Add(trigger.ResponseDelay),
				TriggerName: trigger.Name,
			}

			// Update trigger state
			state.TriggerCount++
			state.LastTriggered = time.Now()

			// Disable trigger if it's a one-time trigger or max count reached
			if trigger.TriggerOnce || (trigger.MaxTriggers > 0 && state.TriggerCount >= trigger.MaxTriggers) {
				state.Enabled = false
				ri.log.Debugf("Disabled trigger %s after %d activations", trigger.Name, state.TriggerCount)
			}

			// Queue the response
			select {
			case ri.responseQueue <- response:
				ri.log.Debugf("Queued response for trigger %s (delay: %v)", trigger.Name, trigger.ResponseDelay)
			default:
				ri.log.Warnf("Response queue full, dropping response for trigger %s", trigger.Name)
			}
		}
	}
}

// matchesTrigger checks if a packet matches a trigger's conditions
func (ri *ResponseInjector) matchesTrigger(packet pkt.Packet, trigger ResponseTrigger, state *TriggerState) bool {
	// Check PGN match
	if trigger.MatchPGN != 0 && packet.Info.PGN != trigger.MatchPGN {
		return false
	}

	// Check source match
	if trigger.MatchSource != 0 && packet.Info.SourceId != trigger.MatchSource {
		return false
	}

	// Check target match (255 = broadcast)
	if trigger.MatchTarget != 0 && trigger.MatchTarget != 255 && packet.Info.TargetId != trigger.MatchTarget {
		return false
	}

	// Check data pattern match (if specified)
	if len(trigger.MatchData) > 0 {
		if !ri.matchesDataPattern(packet.Data, trigger.MatchData) {
			return false
		}
	}

	// Check active window (if specified)
	if trigger.ActiveWindow > 0 {
		timeSinceLastTrigger := time.Since(state.LastTriggered)
		if timeSinceLastTrigger < trigger.ActiveWindow {
			return false // Too soon since last trigger
		}
	}

	return true
}

// matchesDataPattern checks if packet data matches the trigger pattern
func (ri *ResponseInjector) matchesDataPattern(packetData, pattern []byte) bool {
	if len(pattern) > len(packetData) {
		return false
	}

	// Simple prefix match for now
	// Could be enhanced to support wildcards, regex, etc.
	for i, b := range pattern {
		if packetData[i] != b {
			return false
		}
	}

	return true
}

// processResponseQueue handles sending queued responses at the appropriate time
func (ri *ResponseInjector) processResponseQueue() {
	defer ri.wg.Done()

	ri.log.Debug("Response queue processor started")

	for {
		select {
		case <-ri.ctx.Done():
			ri.log.Debug("Response queue processor stopping")
			return

		case response, ok := <-ri.responseQueue:
			if !ok {
				ri.log.Debug("Response queue closed")
				return
			}

			// Wait for the scheduled send time
			waitTime := time.Until(response.SendTime)
			if waitTime > 0 {
				select {
				case <-ri.ctx.Done():
					return
				case <-time.After(waitTime):
					// Continue to send
				}
			}

			// Send the response
			err := ri.sendResponse(response)
			if err != nil {
				ri.log.WithError(err).Warnf("Failed to send response for trigger %s", response.TriggerName)
			} else {
				ri.log.Debugf("Sent response for trigger %s: PGN %d", response.TriggerName, response.PGN)
			}
		}
	}
}

// sendResponse sends a response using the publisher
func (ri *ResponseInjector) sendResponse(response *TimedResponse) error {
	// Create a PGN struct based on the response data
	// This is a simplified approach - in a full implementation,
	// you might want to create proper PGN structs based on the PGN type

	// For now, we'll create a generic message
	// This would need to be enhanced to support specific PGN types

	if ri.publisher == nil {
		return fmt.Errorf("no publisher configured")
	}

	// TODO: This is where you would create the appropriate PGN struct
	// based on the response.PGN and response.Data
	// For now, we'll log the response that would be sent

	ri.log.Infof("Would send response: PGN %d, Source %d, Target %d, Data %v",
		response.PGN, response.Source, response.Target, response.Data)

	return nil
}

// GetTriggerStates returns the current state of all triggers
func (ri *ResponseInjector) GetTriggerStates() map[string]*TriggerState {
	ri.mutex.RLock()
	defer ri.mutex.RUnlock()

	// Return a copy to prevent external modification
	states := make(map[string]*TriggerState)
	for name, state := range ri.triggerStates {
		states[name] = &TriggerState{
			TriggerCount:  state.TriggerCount,
			LastTriggered: state.LastTriggered,
			Enabled:       state.Enabled,
		}
	}

	return states
}

// EnableTrigger enables or disables a specific trigger
func (ri *ResponseInjector) EnableTrigger(name string, enabled bool) error {
	ri.mutex.Lock()
	defer ri.mutex.Unlock()

	state, exists := ri.triggerStates[name]
	if !exists {
		return fmt.Errorf("trigger %s not found", name)
	}

	state.Enabled = enabled
	ri.log.Infof("Trigger %s enabled: %t", name, enabled)

	return nil
}

// ResetTrigger resets the state of a specific trigger
func (ri *ResponseInjector) ResetTrigger(name string) error {
	ri.mutex.Lock()
	defer ri.mutex.Unlock()

	state, exists := ri.triggerStates[name]
	if !exists {
		return fmt.Errorf("trigger %s not found", name)
	}

	state.TriggerCount = 0
	state.LastTriggered = time.Time{}
	state.Enabled = true

	ri.log.Infof("Reset trigger %s", name)

	return nil
}
