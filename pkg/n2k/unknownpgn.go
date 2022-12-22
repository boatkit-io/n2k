package n2k

import (
	"fmt"
	"strings"
)

type UnknownPGN struct {
	Info             PacketInfo
	Data             []uint8
	ManufacturerCode ManufacturerCodeConst
	IndustryCode     uint8
	Reason           error
}

func buildUnknownPGN(p *packet) UnknownPGN {
	pkt := UnknownPGN{
		Info:   p.info,
		Data:   p.data,
		Reason: fmt.Errorf("%s", mergeErrorStrings(p.parseErrors)),
	}

	if IsProprietaryPGN(pkt.Info.PGN) {
		if p.manufacturer != 0 {
			pkt.ManufacturerCode = p.manufacturer
		} else {
			// Proprietary-range PGNS all are required to have the manufacturer code/industry code for the first
			// 2 bytes of the packet, so pull those out for info display and later debugging.
			stream := newPgnDataStream(p.data)
			if v, err := stream.readLookupField(11); err == nil {
				pkt.ManufacturerCode = ManufacturerCodeConst(v)
			}
			_ = stream.skipBits(2)
			if v, err := stream.readLookupField(3); err == nil {
				pkt.IndustryCode = uint8(v)
			}
		}
	}

	return pkt
}

func mergeErrorStrings(errs []error) string {
	sErrs := make([]string, 0, len(errs))
	for i := range errs {
		sErrs = append(sErrs, errs[i].Error())
	}
	return strings.Join(sErrs, ", ")
}
