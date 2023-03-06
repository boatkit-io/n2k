package pkt

import (
	"fmt"
	"strings"

	"github.com/boatkit-io/n2k/pkg/pgn"
)

// buildUnknownPGN returns an UnknownPGN with its Reason field set to the merged errors generated.
func buildUnknownPGN(p *Packet) pgn.UnknownPGN {
	ret := pgn.UnknownPGN{
		Info:   p.Info,
		Data:   p.Data,
		Reason: fmt.Errorf("%s", mergeErrorStrings(p.ParseErrors)),
	}

	if pgn.IsProprietaryPGN(ret.Info.PGN) {
		if p.Manufacturer != 0 {
			ret.ManufacturerCode = p.Manufacturer
		} else {
			// Proprietary-range PGNS all are required to have the manufacturer code/industry code for the first
			// 2 bytes of the packet, so pull those out for info display and later debugging.
			ret.ManufacturerCode, ret.IndustryCode, _ = pgn.GetProprietaryInfo(p.Data)
		}
	}

	return ret
}

// mergeErrorStrings merges the error strings in its argument.
func mergeErrorStrings(errs []error) string {
	sErrs := make([]string, 0, len(errs))
	for i := range errs {
		sErrs = append(sErrs, errs[i].Error())
	}
	return strings.Join(sErrs, ", ")
}
