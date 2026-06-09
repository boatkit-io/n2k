// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package main prints whether sample PGN numbers use the fast-packet protocol.
package main

import (
	"fmt"

	"github.com/boatkit-io/n2k/internal/pgn"
)

func main() {
	pgns := []uint32{
		0x1F200, // Engine Parameters Rapid Update
		0x1F503, // Speed
		0x1F801, // Position Rapid Update
	}

	for _, pgnNum := range pgns {
		fmt.Printf("PGN 0x%X (%d): IsFast = %v\n", pgnNum, pgnNum, pgn.IsFast(pgnNum))
	}
}
