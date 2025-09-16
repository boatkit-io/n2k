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

	for _, pgnId := range pgns {
		fmt.Printf("PGN 0x%X (%d): IsFast = %v\n", pgnId, pgnId, pgn.IsFast(pgnId))
	}
}
