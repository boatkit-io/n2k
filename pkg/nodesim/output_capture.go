package nodesim

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

// OutputCapture records outgoing messages for test validation
type OutputCapture struct {
	// Configuration
	nodeID uint8
	log    *logrus.Logger

	// Captured messages
	messages []CapturedMessage
	mutex    sync.RWMutex

	// Filtering
	captureAll  bool
	capturePGNs map[uint32]bool

	// Control
	ctx    context.Context
	cancel context.CancelFunc

	// Statistics
	totalMessages   uint64
	droppedMessages uint64
	maxMessages     int
}

// CapturedMessage represents a captured outgoing message
type CapturedMessage struct {
	Timestamp   time.Time
	PGN         uint32
	Source      uint8
	Target      uint8
	Priority    uint8
	Data        []byte
	MessageInfo pgn.MessageInfo
}

// NewOutputCapture creates a new OutputCapture instance
func NewOutputCapture(nodeID uint8, log *logrus.Logger) *OutputCapture {
	ctx, cancel := context.WithCancel(context.Background())

	return &OutputCapture{
		nodeID:      nodeID,
		log:         log,
		messages:    make([]CapturedMessage, 0),
		captureAll:  true,
		capturePGNs: make(map[uint32]bool),
		ctx:         ctx,
		cancel:      cancel,
		maxMessages: 10000, // Default limit to prevent memory issues
	}
}

// SetCaptureFilter configures which PGNs to capture
func (oc *OutputCapture) SetCaptureFilter(pgnList []uint32) {
	oc.mutex.Lock()
	defer oc.mutex.Unlock()

	if len(pgnList) == 0 {
		oc.captureAll = true
		oc.capturePGNs = make(map[uint32]bool)
	} else {
		oc.captureAll = false
		oc.capturePGNs = make(map[uint32]bool)
		for _, pgn := range pgnList {
			oc.capturePGNs[pgn] = true
		}
	}

	oc.log.Debugf("Output capture filter updated: captureAll=%t, PGNs=%v", oc.captureAll, pgnList)
}

// SetMaxMessages sets the maximum number of messages to capture
func (oc *OutputCapture) SetMaxMessages(max int) {
	oc.mutex.Lock()
	defer oc.mutex.Unlock()

	oc.maxMessages = max

	// Trim existing messages if necessary
	if len(oc.messages) > max {
		oc.messages = oc.messages[len(oc.messages)-max:]
		oc.log.Debugf("Trimmed captured messages to %d", max)
	}
}

// Start begins capturing messages
func (oc *OutputCapture) Start() error {
	oc.log.Info("Starting output capture")
	return nil
}

// Stop stops capturing messages
func (oc *OutputCapture) Stop() {
	oc.log.Info("Stopping output capture")
	oc.cancel()
}

// WritePgn implements the PgnWriter interface to capture outgoing messages
func (oc *OutputCapture) WritePgn(info pgn.MessageInfo, data []uint8) error {
	oc.mutex.Lock()
	defer oc.mutex.Unlock()

	// Check if we should capture this PGN
	if !oc.shouldCapture(info.PGN) {
		return nil
	}

	// Create captured message
	message := CapturedMessage{
		Timestamp:   time.Now(),
		PGN:         info.PGN,
		Source:      info.SourceId,
		Target:      info.TargetId,
		Priority:    info.Priority,
		Data:        make([]byte, len(data)),
		MessageInfo: info,
	}
	copy(message.Data, data)

	// Add to collection (with size limit)
	if len(oc.messages) >= oc.maxMessages {
		// Remove oldest message
		oc.messages = oc.messages[1:]
		oc.droppedMessages++
	}

	oc.messages = append(oc.messages, message)
	oc.totalMessages++

	oc.log.Debugf("Captured message: PGN %d, Source %d, Target %d, %d bytes",
		info.PGN, info.SourceId, info.TargetId, len(data))

	return nil
}

// shouldCapture determines if a PGN should be captured
func (oc *OutputCapture) shouldCapture(pgn uint32) bool {
	if oc.captureAll {
		return true
	}
	return oc.capturePGNs[pgn]
}

// GetMessages returns all captured messages
func (oc *OutputCapture) GetMessages() []CapturedMessage {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	// Return a copy to prevent external modification
	messages := make([]CapturedMessage, len(oc.messages))
	copy(messages, oc.messages)
	return messages
}

// GetMessagesByPGN returns captured messages for a specific PGN
func (oc *OutputCapture) GetMessagesByPGN(pgn uint32) []CapturedMessage {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	var filtered []CapturedMessage
	for _, msg := range oc.messages {
		if msg.PGN == pgn {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

// GetMessagesInTimeRange returns messages captured within a time range
func (oc *OutputCapture) GetMessagesInTimeRange(start, end time.Time) []CapturedMessage {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	var filtered []CapturedMessage
	for _, msg := range oc.messages {
		if !msg.Timestamp.Before(start) && !msg.Timestamp.After(end) {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

// CountMessages returns the count of messages matching criteria
func (oc *OutputCapture) CountMessages(pgn uint32, source uint8, target uint8) int {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	count := 0
	for _, msg := range oc.messages {
		if (pgn == 0 || msg.PGN == pgn) &&
			(source == 0 || msg.Source == source) &&
			(target == 0 || target == 255 || msg.Target == target) {
			count++
		}
	}
	return count
}

// FindMessage finds the first message matching criteria
func (oc *OutputCapture) FindMessage(pgn uint32, source uint8, target uint8) *CapturedMessage {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	for _, msg := range oc.messages {
		if (pgn == 0 || msg.PGN == pgn) &&
			(source == 0 || msg.Source == source) &&
			(target == 0 || target == 255 || msg.Target == target) {
			// Return a copy
			msgCopy := msg
			return &msgCopy
		}
	}
	return nil
}

// FindLastMessage finds the most recent message matching criteria
func (oc *OutputCapture) FindLastMessage(pgn uint32, source uint8, target uint8) *CapturedMessage {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	// Search backwards for most recent match
	for i := len(oc.messages) - 1; i >= 0; i-- {
		msg := oc.messages[i]
		if (pgn == 0 || msg.PGN == pgn) &&
			(source == 0 || msg.Source == source) &&
			(target == 0 || target == 255 || msg.Target == target) {
			// Return a copy
			msgCopy := msg
			return &msgCopy
		}
	}
	return nil
}

// WaitForMessage waits for a message matching criteria with timeout
func (oc *OutputCapture) WaitForMessage(pgn uint32, source uint8, target uint8, timeout time.Duration) *CapturedMessage {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if msg := oc.FindLastMessage(pgn, source, target); msg != nil {
			return msg
		}
		time.Sleep(10 * time.Millisecond) // Check every 10ms
	}

	return nil
}

// Clear removes all captured messages
func (oc *OutputCapture) Clear() {
	oc.mutex.Lock()
	defer oc.mutex.Unlock()

	oc.messages = oc.messages[:0]
	oc.totalMessages = 0
	oc.droppedMessages = 0

	oc.log.Debug("Cleared all captured messages")
}

// GetStatistics returns capture statistics
func (oc *OutputCapture) GetStatistics() CaptureStatistics {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	return CaptureStatistics{
		TotalMessages:   oc.totalMessages,
		DroppedMessages: oc.droppedMessages,
		CurrentCount:    uint64(len(oc.messages)),
		MaxMessages:     uint64(oc.maxMessages),
	}
}

// CaptureStatistics provides statistics about message capture
type CaptureStatistics struct {
	TotalMessages   uint64
	DroppedMessages uint64
	CurrentCount    uint64
	MaxMessages     uint64
}

// ExportToRaw exports captured messages to raw format
func (oc *OutputCapture) ExportToRaw() []string {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	var lines []string
	for _, msg := range oc.messages {
		// Build raw format line: timestamp,priority,pgn,source,destination,length,data...
		line := fmt.Sprintf("%s,%d,%d,%d,%d,%d",
			msg.Timestamp.Format("2006-01-02T15:04:05Z"),
			msg.Priority,
			msg.PGN,
			msg.Source,
			msg.Target,
			len(msg.Data))

		// Add data bytes as hex
		for _, b := range msg.Data {
			line += fmt.Sprintf(",%02x", b)
		}

		lines = append(lines, line)
	}

	return lines
}

// ValidateSequence checks if messages follow expected sequence patterns
func (oc *OutputCapture) ValidateSequence(expectedPGNs []uint32, maxGap time.Duration) error {
	oc.mutex.RLock()
	defer oc.mutex.RUnlock()

	if len(expectedPGNs) == 0 {
		return fmt.Errorf("no expected PGNs provided")
	}

	// Find messages for each expected PGN
	var sequenceMessages []CapturedMessage
	for _, pgn := range expectedPGNs {
		found := false
		for _, msg := range oc.messages {
			if msg.PGN == pgn {
				sequenceMessages = append(sequenceMessages, msg)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("expected PGN %d not found in captured messages", pgn)
		}
	}

	// Check timing gaps
	for i := 1; i < len(sequenceMessages); i++ {
		gap := sequenceMessages[i].Timestamp.Sub(sequenceMessages[i-1].Timestamp)
		if gap > maxGap {
			return fmt.Errorf("gap between PGN %d and %d exceeds maximum: %v > %v",
				sequenceMessages[i-1].PGN, sequenceMessages[i].PGN, gap, maxGap)
		}
	}

	return nil
}
