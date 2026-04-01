package pgn

// UnknownPGN is the fallback struct returned when a received message cannot be decoded
// into a strongly-typed PGN struct. This happens when:
//   - The PGN number is not present in the canboat database at all
//   - The PGN is known to canboat but was never observed in real log files (unseen)
//   - The proprietary PGN variant cannot be matched to a known manufacturer
//   - A decoding error occurred (malformed data, truncated payload, etc.)
//
// Callers receiving an UnknownPGN can inspect the Reason field for the cause and the
// raw Data field to attempt manual or proprietary decoding.
type UnknownPGN struct {
	// Info contains the CAN bus header metadata (PGN number, source, priority, etc.).
	Info MessageInfo
	// Data holds the raw, undecoded message payload bytes.
	Data []uint8
	// ManufacturerCode is the manufacturer code extracted from the payload if this was
	// a proprietary PGN. It is zero if the PGN is not proprietary or if extraction failed.
	ManufacturerCode ManufacturerCodeConst
	// IndustryCode is the industry code extracted alongside ManufacturerCode for
	// proprietary PGNs (e.g., 4 = Marine). Zero if not applicable.
	IndustryCode IndustryCodeConst
	// Reason describes why decoding failed (e.g., "PGN not found", decode error details).
	Reason error
	// WasUnseen is true when the PGN exists in the canboat database but has no sample data
	// in canboat's log files, meaning the decoder is untested and may be unreliable.
	WasUnseen bool
}
