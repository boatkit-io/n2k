package pgn

import (
	"time"
)

// MessageInfo contains info from the incoming message that provides context needed to
// use the decoded PGN
type MessageInfo struct {
	// when did we get the message
	Timestamp time.Time

	// 3-bit
	Priority uint8

	// 19-bit number
	PGN uint32

	// actually 8-bit
	SourceId uint8

	// target address, when relevant (PGNs with PF < 240)
	TargetId uint8
}
