// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

/*
Filterraw reads a .raw format NMEA 2000 dump file and filters its contents based on PGN.
Supports filtering for:
- Specific PGN
- Unseen PGNs (from UnseenList)
- Unknown PGNs (not in PgnList)

The flags are:

	-input specifies the input .raw file
	-unseen shows only PGNs in the unseen list
	-unknown shows only PGNs not in PgnList
	-pgn shows only a specific PGN number

For multi-frame messages, the frames are reconstructed before output.
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

func main() {
	// Parse command line arguments
	inputFile := flag.String("input", "", "Input .raw file to process")
	showUnseen := flag.Bool("unseen", false, "Show only PGNs in the unseen list")
	showUnknown := flag.Bool("unknown", false, "Show only PGNs not in PgnList")
	specificPgn := flag.String("pgn", "", "Show only specific PGN")
	flag.Parse()

	if *inputFile == "" {
		fmt.Println("Usage: filterraw -input <file.raw> [-unseen] [-unknown] [-pgn <number>]")
		os.Exit(1)
	}

	opts := FilterOptions{
		InputFile: *inputFile,
		Unseen:    *showUnseen,
		Unknown:   *showUnknown,
	}

	if *specificPgn != "" {
		pgn, err := strconv.ParseUint(*specificPgn, 10, 32)
		if err != nil {
			fmt.Printf("Invalid PGN number: %v\n", err)
			os.Exit(1)
		}
		opts.SpecificPGN = uint32(pgn)
	}

	FilterRawFile(opts)
}
