package canadapter

import (
	"time"

	"github.com/open-ships/n2k/pkg/pgn"
	"github.com/brutella/can"
)

// This data structure is copied from
// https://github.com/brutella/can/blob/master/frame.go
// licensed under the MIT License, following

/*
The MIT License (MIT)

Copyright (c) 2016 Matthias Hochgatterer

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

*/

// NewPacketInfo extracts NMEA 2000 message metadata from a raw CAN bus frame's 29-bit
// extended identifier. The CAN ID encodes several fields using the NMEA 2000 / ISO 11783
// bit layout:
//
// CAN ID bit layout (29 bits total, extended frame format):
//
//	Bits 28-26: Priority (3 bits, 0-7, lower = higher priority)
//	Bit  25:    Reserved
//	Bit  24:    Data Page (DP)
//	Bits 23-16: PDU Format (PF) - determines if message is broadcast or addressed
//	Bits 15-8:  PDU Specific (PS) - destination address (if PF < 240) or group extension (if PF >= 240)
//	Bits 7-0:   Source Address (SA) - the sender's address on the bus
//
// The PGN (Parameter Group Number) is extracted from bits 25-8 of the CAN ID. For
// addressed messages (PDU Format < 240), the PS field contains the destination address
// rather than being part of the PGN, so the lower 8 bits are masked off and stored as
// TargetId instead.
//
// Parameters:
//   - message: A pointer to a can.Frame containing the raw CAN bus frame with its 29-bit ID.
//
// Returns a pgn.MessageInfo populated with the extracted PGN, source, priority, target,
// and current timestamp.
func NewPacketInfo(message *can.Frame) pgn.MessageInfo {
	p := pgn.MessageInfo{
		Timestamp: time.Now(),
		// Extract source address from bits 0-7 of the CAN ID.
		SourceId: uint8(message.ID & 0xFF),
		// Extract PGN from bits 8-25 (18 bits). The mask 0x3FFFF00 selects bits 25-8,
		// then right-shift by 8 to get the actual PGN value.
		PGN: (message.ID & 0x3FFFF00) >> 8,
		// Extract priority from bits 26-28 (3 bits). The mask 0x1C000000 selects bits 28-26,
		// then right-shift by 26 to get the 0-7 priority value.
		Priority: uint8((message.ID & 0x1C000000) >> 26),
	}

	// Determine if this is an addressed (point-to-point) or broadcast message by
	// examining the PDU Format (PF) field in bits 15-8 of the PGN.
	// PDU Format < 240: Addressed message (PDU1) -- PS field is destination address.
	// PDU Format >= 240: Broadcast message (PDU2) -- PS field is group extension (part of PGN).
	pduFormat := uint8((p.PGN & 0xFF00) >> 8)
	if pduFormat < 240 {
		// This is an addressed (point-to-point) message. The lower byte of the PGN field
		// actually contains the destination address, not part of the PGN itself.
		// Extract it as TargetId and mask it off the PGN.
		p.TargetId = uint8(p.PGN & 0xFF)
		p.PGN &= 0xFFF00 // Zero out the destination byte to get the true PGN.
	}
	return p
}
