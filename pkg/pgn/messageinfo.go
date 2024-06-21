package pgn

import (
	"time"
)

// MessageInfo contains context needed to process an NMEA 2000 message.
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

	// Length of Frame data: max (and almost always) 8
	Length uint8
}
