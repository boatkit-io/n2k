package pgn

// UnknownPGN is returned when we fail to recognize the PGN.
// This can be because canboat.json is incomplete, an error in data transmission, or even a bug?
type UnknownPGN struct {
	Info             MessageInfo
	Data             []uint8
	ManufacturerCode ManufacturerCodeConst
	IndustryCode     IndustryCodeConst
	Reason           error
	WasUnseen        bool // Marked as not seen in log files by Canboat.
}

func (p *UnknownPGN) Encode(stream *DataStream) (*MessageInfo, error) {
	if stream != nil {
		// use to avoid compile error. Don't care.
	}
	return &p.Info, nil
}
