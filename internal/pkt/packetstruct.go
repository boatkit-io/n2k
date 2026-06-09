// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package pkt

import (
	"fmt"

	"github.com/boatkit-io/n2k/internal/pgn"
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

// HandlePacket is how you tell PacketStruct to start processing a new
// packet into a PGN
//
//nolint:gocritic // Why: Breaking change to change.
func (ps *PacketStruct) HandlePacket(pkt Packet) {
	stream := pgn.NewDataStream(pkt.Data)
	decoder, err := pgn.FindDecoder(stream, pkt.Info.PGN)
	if err != nil {
		pkt.ParseErrors = append(pkt.ParseErrors, fmt.Errorf("no matching decoder for PGN %d: %w", pkt.Info.PGN, err))
		ps.pgnReady(pkt.UnknownPGN())
		return
	}

	ret, err := decoder(pkt.Info, stream)
	if err != nil {
		pkt.ParseErrors = append(pkt.ParseErrors, err)
		ps.pgnReady(pkt.UnknownPGN())
		return
	}

	ps.pgnReady(ret)
}

// pgnReady is a helper to call when a PGN is ready to run it through the handler
func (ps *PacketStruct) pgnReady(fullPGN any) {
	if ps.handler != nil {
		ps.handler.HandleStruct(fullPGN)
	}
}
