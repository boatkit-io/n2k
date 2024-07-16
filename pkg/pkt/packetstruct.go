package pkt

import (
	"fmt"

	"github.com/boatkit-io/n2k/pkg/pgn"
)

// StructHandler is an interface for a handler of the output of a PacketStruct
type StructHandler interface {
	HandleStruct(any)
}

// PacketStruct methods convert Packets to golang structs and sends them on.
type PacketStruct struct {
	handler StructHandler
}

// NewPacketStruct initializes and returns a new PacketStruct instance.
func NewPacketStruct() *PacketStruct {
	return &PacketStruct{}
}

// SetOutput hooks up the output from packetstruct processing into a handler
func (ps *PacketStruct) SetOutput(sh StructHandler) {
	ps.handler = sh
}

// HandlePacket is how you tell PacketStruct to start processing a new packet into a PGN
func (ps *PacketStruct) HandlePacket(pkt Packet) {
	if len(pkt.Decoders) > 0 {
		// call frame decoders, send valid return on.
		for _, decoder := range pkt.Decoders {
			stream := pgn.NewDataStream(pkt.Data)
			ret, err := decoder(pkt.Info, stream)
			if err != nil {
				pkt.ParseErrors = append(pkt.ParseErrors, err)
				continue
			} else {
				ps.pgnReady(ret)
				return
			}
		}

		// no decoder succeeded
		ps.pgnReady(pkt.UnknownPGN())
	} else {
		// No valid decoder, so send on an UnknownPGN.
		pkt.ParseErrors = append(pkt.ParseErrors, fmt.Errorf("no matching decoder"))
		ps.pgnReady(pkt.UnknownPGN())
	}
}

// pgnReady is a helper to call when a PGN is ready to run it through the handler
func (ps *PacketStruct) pgnReady(fullPGN any) {
	if ps.handler != nil {
		ps.handler.HandleStruct(fullPGN)
	}
}
