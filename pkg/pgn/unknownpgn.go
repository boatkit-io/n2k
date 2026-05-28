// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package pgn

// UnknownPGN is returned when we fail to recognize the PGN.
// This can be because canboat.json is incomplete, an error in data transmission, or even a bug?
type UnknownPGN struct {
	Info             MessageInfo
	Data             []uint8
	ManufacturerCode ManufacturerCodeConst
	IndustryCode     IndustryCodeConst
	Reason           error
}
