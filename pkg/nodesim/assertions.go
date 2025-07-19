package nodesim

import (
	"fmt"
	"regexp"
	"time"
)

// TestAssertion defines the interface for all test assertions
type TestAssertion interface {
	Validate(results *TestResults) error
	GetDescription() string
}

// TestResults contains the results of a test execution
type TestResults struct {
	// Test metadata
	TestName  string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// Output capture
	OutputCapture *OutputCapture

	// Response injection
	ResponseInjector *ResponseInjector

	// Test-specific data
	NodeAddress uint8
	CustomData  map[string]interface{}
}

// MessageSentAssertion verifies that a specific message was sent
type MessageSentAssertion struct {
	PGN         uint32
	Source      uint8
	Target      uint8
	DataPattern []byte
	MinCount    int
	MaxCount    int
	TimeWindow  time.Duration
	Description string
}

func (a MessageSentAssertion) GetDescription() string {
	if a.Description != "" {
		return a.Description
	}
	return fmt.Sprintf("Message sent: PGN %d from source %d", a.PGN, a.Source)
}

func (a MessageSentAssertion) Validate(results *TestResults) error {
	if results.OutputCapture == nil {
		return fmt.Errorf("no output capture available for assertion")
	}

	// Get messages in time window if specified
	var messages []CapturedMessage
	if a.TimeWindow > 0 {
		start := results.StartTime
		end := results.StartTime.Add(a.TimeWindow)
		messages = results.OutputCapture.GetMessagesInTimeRange(start, end)
	} else {
		messages = results.OutputCapture.GetMessages()
	}

	// Filter by criteria
	var matchingMessages []CapturedMessage
	for _, msg := range messages {
		if a.matchesMessage(msg) {
			matchingMessages = append(matchingMessages, msg)
		}
	}

	count := len(matchingMessages)

	// Check count constraints
	if a.MinCount > 0 && count < a.MinCount {
		return fmt.Errorf("expected at least %d messages, found %d", a.MinCount, count)
	}

	if a.MaxCount > 0 && count > a.MaxCount {
		return fmt.Errorf("expected at most %d messages, found %d", a.MaxCount, count)
	}

	if a.MinCount == 0 && a.MaxCount == 0 && count == 0 {
		return fmt.Errorf("expected at least one message, found none")
	}

	return nil
}

func (a MessageSentAssertion) matchesMessage(msg CapturedMessage) bool {
	// Check PGN
	if a.PGN != 0 && msg.PGN != a.PGN {
		return false
	}

	// Check source
	if a.Source != 0 && msg.Source != a.Source {
		return false
	}

	// Check target
	if a.Target != 0 && a.Target != 255 && msg.Target != a.Target {
		return false
	}

	// Check data pattern
	if len(a.DataPattern) > 0 {
		if !a.matchesDataPattern(msg.Data) {
			return false
		}
	}

	return true
}

func (a MessageSentAssertion) matchesDataPattern(data []byte) bool {
	if len(a.DataPattern) > len(data) {
		return false
	}

	// Simple prefix match for now
	for i, b := range a.DataPattern {
		if data[i] != b {
			return false
		}
	}

	return true
}

// MessageSequenceAssertion verifies that messages are sent in a specific sequence
type MessageSequenceAssertion struct {
	Sequence    []MessagePattern
	MaxGap      time.Duration
	Description string
}

type MessagePattern struct {
	PGN    uint32
	Source uint8
	Target uint8
}

func (a MessageSequenceAssertion) GetDescription() string {
	if a.Description != "" {
		return a.Description
	}
	return fmt.Sprintf("Message sequence with %d messages", len(a.Sequence))
}

func (a MessageSequenceAssertion) Validate(results *TestResults) error {
	if results.OutputCapture == nil {
		return fmt.Errorf("no output capture available for assertion")
	}

	messages := results.OutputCapture.GetMessages()

	// Find the sequence
	sequenceIndex := 0
	var sequenceMessages []CapturedMessage

	for _, msg := range messages {
		if sequenceIndex < len(a.Sequence) {
			pattern := a.Sequence[sequenceIndex]
			if a.matchesPattern(msg, pattern) {
				sequenceMessages = append(sequenceMessages, msg)
				sequenceIndex++
			}
		}
	}

	if sequenceIndex < len(a.Sequence) {
		return fmt.Errorf("incomplete sequence: found %d of %d expected messages",
			sequenceIndex, len(a.Sequence))
	}

	// Check timing gaps if specified
	if a.MaxGap > 0 && len(sequenceMessages) > 1 {
		for i := 1; i < len(sequenceMessages); i++ {
			gap := sequenceMessages[i].Timestamp.Sub(sequenceMessages[i-1].Timestamp)
			if gap > a.MaxGap {
				return fmt.Errorf("gap between message %d and %d exceeds maximum: %v > %v",
					i-1, i, gap, a.MaxGap)
			}
		}
	}

	return nil
}

func (a MessageSequenceAssertion) matchesPattern(msg CapturedMessage, pattern MessagePattern) bool {
	if pattern.PGN != 0 && msg.PGN != pattern.PGN {
		return false
	}
	if pattern.Source != 0 && msg.Source != pattern.Source {
		return false
	}
	if pattern.Target != 0 && pattern.Target != 255 && msg.Target != pattern.Target {
		return false
	}
	return true
}

// AddressClaimedAssertion verifies that a node claims a specific address
type AddressClaimedAssertion struct {
	ExpectedAddress uint8
	TimeoutToClaim  time.Duration
	Description     string
}

func (a AddressClaimedAssertion) GetDescription() string {
	if a.Description != "" {
		return a.Description
	}
	return fmt.Sprintf("Address %d claimed within %v", a.ExpectedAddress, a.TimeoutToClaim)
}

func (a AddressClaimedAssertion) Validate(results *TestResults) error {
	if results.OutputCapture == nil {
		return fmt.Errorf("no output capture available for assertion")
	}

	// Look for ISO Address Claim messages (PGN 60928)
	messages := results.OutputCapture.GetMessagesByPGN(60928)

	deadline := results.StartTime.Add(a.TimeoutToClaim)

	for _, msg := range messages {
		if msg.Timestamp.After(deadline) {
			continue
		}

		if msg.Source == a.ExpectedAddress {
			return nil // Found the address claim
		}
	}

	return fmt.Errorf("expected address %d not claimed within %v",
		a.ExpectedAddress, a.TimeoutToClaim)
}

// HeartbeatIntervalAssertion verifies heartbeat timing
type HeartbeatIntervalAssertion struct {
	ExpectedInterval time.Duration
	Tolerance        time.Duration
	MinBeats         int
	PGN              uint32
	Description      string
}

func (a HeartbeatIntervalAssertion) GetDescription() string {
	if a.Description != "" {
		return a.Description
	}
	return fmt.Sprintf("Heartbeat interval %v (±%v) for PGN %d",
		a.ExpectedInterval, a.Tolerance, a.PGN)
}

func (a HeartbeatIntervalAssertion) Validate(results *TestResults) error {
	if results.OutputCapture == nil {
		return fmt.Errorf("no output capture available for assertion")
	}

	pgn := a.PGN
	if pgn == 0 {
		pgn = 126993 // Default heartbeat PGN
	}

	messages := results.OutputCapture.GetMessagesByPGN(pgn)

	if len(messages) < a.MinBeats {
		return fmt.Errorf("expected at least %d heartbeat messages, found %d",
			a.MinBeats, len(messages))
	}

	// Check intervals between consecutive messages
	for i := 1; i < len(messages); i++ {
		interval := messages[i].Timestamp.Sub(messages[i-1].Timestamp)
		expectedMin := a.ExpectedInterval - a.Tolerance
		expectedMax := a.ExpectedInterval + a.Tolerance

		if interval < expectedMin || interval > expectedMax {
			return fmt.Errorf("heartbeat interval %v outside expected range %v to %v",
				interval, expectedMin, expectedMax)
		}
	}

	return nil
}

// TriggerActivatedAssertion verifies that a response trigger was activated
type TriggerActivatedAssertion struct {
	TriggerName   string
	ExpectedCount int
	Description   string
}

func (a TriggerActivatedAssertion) GetDescription() string {
	if a.Description != "" {
		return a.Description
	}
	return fmt.Sprintf("Trigger '%s' activated %d times", a.TriggerName, a.ExpectedCount)
}

func (a TriggerActivatedAssertion) Validate(results *TestResults) error {
	if results.ResponseInjector == nil {
		return fmt.Errorf("no response injector available for assertion")
	}

	states := results.ResponseInjector.GetTriggerStates()
	state, exists := states[a.TriggerName]

	if !exists {
		return fmt.Errorf("trigger '%s' not found", a.TriggerName)
	}

	if state.TriggerCount != a.ExpectedCount {
		return fmt.Errorf("trigger '%s' activated %d times, expected %d",
			a.TriggerName, state.TriggerCount, a.ExpectedCount)
	}

	return nil
}

// CustomAssertion allows for custom validation logic
type CustomAssertion struct {
	Name         string
	Description  string
	ValidateFunc func(*TestResults) error
}

func (a CustomAssertion) GetDescription() string {
	if a.Description != "" {
		return a.Description
	}
	return a.Name
}

func (a CustomAssertion) Validate(results *TestResults) error {
	if a.ValidateFunc == nil {
		return fmt.Errorf("no validation function provided for custom assertion '%s'", a.Name)
	}

	return a.ValidateFunc(results)
}

// DataPatternAssertion verifies that message data matches a pattern
type DataPatternAssertion struct {
	PGN         uint32
	Source      uint8
	Target      uint8
	Pattern     string // Hex pattern with wildcards (e.g., "01,XX,03,XX")
	Description string
}

func (a DataPatternAssertion) GetDescription() string {
	if a.Description != "" {
		return a.Description
	}
	return fmt.Sprintf("Data pattern match for PGN %d: %s", a.PGN, a.Pattern)
}

func (a DataPatternAssertion) Validate(results *TestResults) error {
	if results.OutputCapture == nil {
		return fmt.Errorf("no output capture available for assertion")
	}

	messages := results.OutputCapture.GetMessages()

	for _, msg := range messages {
		if (a.PGN == 0 || msg.PGN == a.PGN) &&
			(a.Source == 0 || msg.Source == a.Source) &&
			(a.Target == 0 || a.Target == 255 || msg.Target == a.Target) {

			if a.matchesHexPattern(msg.Data) {
				return nil // Found matching message
			}
		}
	}

	return fmt.Errorf("no message found matching data pattern %s", a.Pattern)
}

func (a DataPatternAssertion) matchesHexPattern(data []byte) bool {
	// Parse pattern like "01,XX,03,FF"
	patternParts := regexp.MustCompile(`[,\s]+`).Split(a.Pattern, -1)

	if len(patternParts) > len(data) {
		return false
	}

	for i, part := range patternParts {
		if part == "XX" || part == "xx" {
			continue // Wildcard matches anything
		}

		// Parse hex byte
		var expected byte
		if _, err := fmt.Sscanf(part, "%02x", &expected); err != nil {
			if _, err := fmt.Sscanf(part, "%02X", &expected); err != nil {
				return false // Invalid pattern
			}
		}

		if data[i] != expected {
			return false
		}
	}

	return true
}

// AssertionRunner runs a collection of assertions against test results
type AssertionRunner struct {
	assertions []TestAssertion
	results    []AssertionResult
}

// AssertionResult contains the result of running a single assertion
type AssertionResult struct {
	Assertion TestAssertion
	Passed    bool
	Error     error
	Duration  time.Duration
}

// NewAssertionRunner creates a new assertion runner
func NewAssertionRunner() *AssertionRunner {
	return &AssertionRunner{
		assertions: make([]TestAssertion, 0),
		results:    make([]AssertionResult, 0),
	}
}

// AddAssertion adds an assertion to be run
func (ar *AssertionRunner) AddAssertion(assertion TestAssertion) {
	ar.assertions = append(ar.assertions, assertion)
}

// RunAll runs all assertions against the test results
func (ar *AssertionRunner) RunAll(testResults *TestResults) []AssertionResult {
	ar.results = make([]AssertionResult, 0, len(ar.assertions))

	for _, assertion := range ar.assertions {
		start := time.Now()
		err := assertion.Validate(testResults)
		duration := time.Since(start)

		result := AssertionResult{
			Assertion: assertion,
			Passed:    err == nil,
			Error:     err,
			Duration:  duration,
		}

		ar.results = append(ar.results, result)
	}

	return ar.results
}

// GetResults returns the results of the last run
func (ar *AssertionRunner) GetResults() []AssertionResult {
	return ar.results
}

// GetSummary returns a summary of assertion results
func (ar *AssertionRunner) GetSummary() AssertionSummary {
	summary := AssertionSummary{
		Total:  len(ar.results),
		Passed: 0,
		Failed: 0,
	}

	for _, result := range ar.results {
		if result.Passed {
			summary.Passed++
		} else {
			summary.Failed++
		}
	}

	return summary
}

// AssertionSummary provides a summary of assertion results
type AssertionSummary struct {
	Total  int
	Passed int
	Failed int
}

// AllPassed returns true if all assertions passed
func (s AssertionSummary) AllPassed() bool {
	return s.Failed == 0 && s.Total > 0
}
