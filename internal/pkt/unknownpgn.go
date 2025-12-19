package pkt

import (
	"fmt"
	"strings"

	"github.com/boatkit-io/n2k/internal/pgn"
)

// buildUnknownPGN returns an UnknownPGN with its Reason field set to the merged errors generated.
func buildUnknownPGN(p *Packet) pgn.UnknownPGN {
	ret := pgn.UnknownPGN{
		Info:   p.Info,
		Data:   p.Data,
		Reason: fmt.Errorf("%s", mergeErrorStrings(p.ParseErrors)),
	}
	if pgn.IsProprietaryPGN(ret.Info.PGN) {
		// read manufacturer code from data
		if len(p.Data) > 2 {
			// create a datastream from the data
			ds := pgn.NewDataStream(p.Data)
			// generate a fieldspec for the manufacturer code
			fs := pgn.FieldSpec{
				BitLength:         11,
				BitOffset:         0,
				BitLengthVariable: false,
			}
			// read manufacturer code from data
			manufacturerCode, err := pgn.ReadRaw[pgn.ManufacturerCodeConst](ds, &fs)
			if err != nil {
				ret.Reason = err
			}
			ret.ManufacturerCode = *manufacturerCode
			// read manufacturer code from data
			ret.ManufacturerCode = *manufacturerCode
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
