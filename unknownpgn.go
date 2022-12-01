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

func BuildUnknownPGN(p *Packet) UnknownPGN {
	pkt := UnknownPGN{
		Info:   p.Info,
		Data:   p.Data,
		Reason: fmt.Errorf("%s", mergeErrorStrings(p.ParseErrors)),
	}

	if IsProprietaryPGN(pkt.Info.PGN) {
		if p.Manufacturer != 0 {
			pkt.ManufacturerCode = p.Manufacturer
		} else {
			// Proprietary-range PGNS all are required to have the manufacturer code/industry code for the first
			// 2 bytes of the packet, so pull those out for info display and later debugging.
			stream := NewPgnDataStream(p.Data)
			if v, err := stream.ReadLookupField(11); err == nil {
				pkt.ManufacturerCode = ManufacturerCodeConst(v)
			}
			_ = stream.SkipBits(2)
			if v, err := stream.ReadLookupField(3); err == nil {
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
