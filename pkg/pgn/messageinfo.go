// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package pgn

import (
	"time"
)

// MessageInfo contains context needed to process an NMEA 2000 message.
//
//nolint:revive // Why: Breaking change to refactor.
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
