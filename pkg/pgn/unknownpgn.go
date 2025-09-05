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

// Encode is a dummy method to allow an UnnownPGN to satisfy the PgnStruct interface
//
//lint:ignore U1000 // we don't encode UnknowPGN, but want it to satisfy the interface
func (p *UnknownPGN) Encode(stream *DataStream) (*MessageInfo, error) {
	return &p.Info, nil
}

func (p *UnknownPGN) GetMessageInfo() *MessageInfo {
	return &p.Info
}

func (p *UnknownPGN) SetMessageInfo(info *MessageInfo) {
	p.Info = *info
}
