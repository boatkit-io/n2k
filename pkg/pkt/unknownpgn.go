package pkt

import (
	"fmt"
	"strings"

	"github.com/open-ships/n2k/pkg/pgn"
)

// buildUnknownPGN constructs a pgn.UnknownPGN from a Packet whose data could not be
// decoded by any available decoder. It preserves the raw message info and data payload
// for later inspection, and merges all accumulated parse errors into a single Reason string.
//
// For proprietary PGNs (those in the NMEA 2000 proprietary range), it also extracts
// and stores the manufacturer code and industry code from the first two bytes of the
// data payload. These are useful for identifying which device sent the message even
// when we lack a decoder for that manufacturer's proprietary format.
//
// The WasUnseen flag is set by checking whether this PGN appears on the "unseen" list --
// PGNs that are defined in the canboat registry but have never been observed in real
// captured data, indicating they may be rare or theoretical.
//
// Parameters:
//   - p: The Packet that failed decoding, containing Info, Data, ParseErrors, and Manufacturer.
//
// Returns a fully populated pgn.UnknownPGN ready for downstream consumption.
func buildUnknownPGN(p *Packet) pgn.UnknownPGN {
	ret := pgn.UnknownPGN{
		Info:   p.Info,   // Preserve the original message metadata (PGN, source, priority, etc.)
		Data:   p.Data,   // Preserve the raw payload bytes for debugging
		Reason: fmt.Errorf("%s", mergeErrorStrings(p.ParseErrors)), // Combine all errors into one
	}

	// For proprietary PGNs, extract manufacturer/industry identification from the data.
	if pgn.IsProprietaryPGN(ret.Info.PGN) {
		if p.Manufacturer != 0 {
			// Manufacturer was already extracted during AddDecoders -- reuse it.
			ret.ManufacturerCode = p.Manufacturer
		} else {
			// Manufacturer hasn't been extracted yet (e.g., packet went straight to UnknownPGN
			// without going through AddDecoders). Extract it directly from the raw data.
			// Proprietary PGNs encode the 11-bit manufacturer code and 3-bit industry code
			// in the first 2 bytes of the payload per the NMEA 2000 specification.
			ret.ManufacturerCode, ret.IndustryCode, _ = pgn.GetProprietaryInfo(p.Data)
		}
	}

	// Check if this PGN is on the "unseen" list -- PGNs defined in canboat but never
	// observed in real data captures. Useful for distinguishing truly unknown PGNs from
	// rare-but-defined ones.
	ret.WasUnseen = pgn.SearchUnseenList(ret.Info.PGN)
	return ret
}

// mergeErrorStrings takes a slice of errors and joins their message strings with commas.
// This produces a single human-readable string summarizing all errors encountered during
// packet processing, suitable for inclusion in the UnknownPGN's Reason field.
//
// Parameters:
//   - errs: Slice of errors accumulated during packet validation and decoding attempts.
//
// Returns a comma-separated string of all error messages.
func mergeErrorStrings(errs []error) string {
	sErrs := make([]string, 0, len(errs))
	for i := range errs {
		sErrs = append(sErrs, errs[i].Error())
	}
	return strings.Join(sErrs, ", ")
}
