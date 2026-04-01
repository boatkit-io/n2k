package pgn

import (
	"time"
)

// MessageInfo carries the CAN bus header metadata that accompanies every NMEA 2000 message.
// It is extracted from the CAN frame's 29-bit identifier and timestamp before the payload
// is passed to a PGN decoder. Every generated PGN struct embeds a MessageInfo as its "Info"
// field, making the header data available alongside the decoded payload.
type MessageInfo struct {
	// Timestamp records when the message was received (or replayed). It is set by the
	// transport layer and is not part of the CAN bus wire format.
	Timestamp time.Time

	// Priority is the 3-bit message priority from the CAN identifier (0 = highest, 7 = lowest).
	// Lower-priority messages yield the bus to higher-priority ones during arbitration.
	Priority uint8

	// PGN is the 19-bit Parameter Group Number extracted from the CAN identifier.
	// It identifies the message type and determines which decoder to use.
	// Stored as uint32 because Go lacks a native 19-bit type.
	PGN uint32

	// SourceId is the 8-bit network address of the device that transmitted this message.
	// Device addresses are assigned dynamically via the NMEA 2000 address claim protocol
	// and can change at runtime, so SourceId alone is not a stable device identifier.
	SourceId uint8

	// TargetId is the 8-bit destination address for PDU1-format (addressed) PGNs,
	// where the PDU Format byte (PF) is less than 240. For PDU2-format (broadcast)
	// PGNs (PF >= 240), this field is not meaningful and is typically set to 255.
	TargetId uint8
}
