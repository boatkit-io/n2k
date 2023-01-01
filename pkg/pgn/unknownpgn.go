package pgn

type UnknownPGN struct {
	Info             MessageInfo
	Data             []uint8
	ManufacturerCode ManufacturerCodeConst
	IndustryCode     IndustryCodeConst
	Reason           error
}
