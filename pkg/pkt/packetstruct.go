package pkt

import (
	"fmt"

	"github.com/open-ships/n2k/pkg/pgn"
)

// StructHandler is an interface for consuming decoded PGN structs produced by PacketStruct.
// Implementations receive fully decoded Go structs (e.g., pgn.VesselHeading, pgn.WindData)
// or pgn.UnknownPGN when decoding fails. This is the final output stage of the decoding
// pipeline.
type StructHandler interface {
	// HandleStruct receives a decoded PGN struct. The concrete type will be one of the
	// generated PGN types (e.g., pgn.VesselHeading) or pgn.UnknownPGN if decoding failed.
	HandleStruct(any)
}

// PacketStruct is responsible for converting complete Packets into typed Go structs.
// It sits at the end of the decoding pipeline: adapters produce Packets, and PacketStruct
// tries each candidate decoder until one succeeds, then forwards the resulting struct
// to the registered StructHandler. If no decoder succeeds, it produces an UnknownPGN.
type PacketStruct struct {
	// handler is the downstream consumer that receives decoded PGN structs.
	// May be nil, in which case decoded results are silently discarded.
	handler StructHandler
}

// NewPacketStruct creates and returns a new PacketStruct instance with no handler set.
// Call SetOutput to register a StructHandler before sending packets.
func NewPacketStruct() *PacketStruct {
	return &PacketStruct{}
}

// SetOutput registers the downstream StructHandler that will receive decoded PGN structs.
// This must be called before HandlePacket to ensure decoded results are forwarded.
//
// Parameters:
//   - sh: The StructHandler implementation to receive decoded structs.
func (ps *PacketStruct) SetOutput(sh StructHandler) {
	ps.handler = sh
}

// HandlePacket attempts to decode a complete Packet into a typed Go struct using its
// list of candidate Decoders. Each decoder is tried in order; the first one that succeeds
// produces the output struct. If a decoder fails, the error is appended to ParseErrors
// and the next decoder is tried.
//
// If all decoders fail, or if no decoders are available, the packet is converted to an
// UnknownPGN and sent downstream instead.
//
// Parameters:
//   - pkt: A complete Packet with Data assembled and Decoders populated.
//
// The decoded struct (or UnknownPGN fallback) is forwarded to the registered StructHandler.
func (ps *PacketStruct) HandlePacket(pkt Packet) {
	if len(pkt.Decoders) > 0 {
		// Try each candidate decoder in order. Decoders are already filtered by manufacturer
		// for proprietary PGNs, so typically only one or a few remain.
		for _, decoder := range pkt.Decoders {
			// Create a fresh data stream for each decoder attempt so each starts reading
			// from the beginning of the payload.
			stream := pgn.NewPgnDataStream(pkt.Data)
			ret, err := decoder(pkt.Info, stream)
			if err != nil {
				// This decoder couldn't parse the data -- record the error and try the next one.
				pkt.ParseErrors = append(pkt.ParseErrors, err)
				continue
			} else {
				// Decoder succeeded -- forward the typed struct and return.
				ps.pgnReady(ret)
				return
			}
		}

		// All decoders were attempted but none succeeded. Fall back to UnknownPGN,
		// which preserves the raw data and all accumulated errors for debugging.
		ps.pgnReady(pkt.UnknownPGN())
	} else {
		// No valid decoders available (e.g., unknown PGN or all candidates were filtered out).
		// Send an UnknownPGN so downstream consumers still receive notification of the message.
		pkt.ParseErrors = append(pkt.ParseErrors, fmt.Errorf("no matching decoder"))
		ps.pgnReady(pkt.UnknownPGN())
	}
}

// pgnReady is an internal helper that forwards a decoded PGN struct to the registered
// StructHandler. It safely handles the case where no handler has been set (nil check)
// to avoid panics during testing or when output is intentionally discarded.
//
// Parameters:
//   - fullPGN: The decoded PGN struct (typed or UnknownPGN) to forward.
func (ps *PacketStruct) pgnReady(fullPGN any) {
	if ps.handler != nil {
		ps.handler.HandleStruct(fullPGN)
	}
}
