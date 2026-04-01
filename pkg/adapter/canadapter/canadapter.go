// Package canadapter implements the adapter interface for raw CAN bus frame endpoints.
//
// This package converts raw CAN bus frames (as produced by SocketCAN interfaces or
// CAN-to-USB gateways) into complete NMEA 2000 packets. It handles both single-frame
// messages (8 bytes or fewer of payload) and multi-frame "fast packet" messages that
// require assembly of multiple CAN frames into a single logical message.
//
// The processing pipeline is:
//
//	*can.Frame (raw CAN bus frame)
//	  -> NewPacketInfo() extracts PGN, source, priority, destination from CAN ID
//	  -> NewPacket() creates a Packet with candidate decoders looked up from the PGN registry
//	  -> For fast packets: MultiBuilder assembles frames into a complete message
//	  -> AddDecoders() filters candidates by manufacturer for proprietary PGNs
//	  -> Complete packet forwarded to PacketHandler
package canadapter

import (
	"fmt"
	"log/slog"

	"github.com/brutella/can"

	"github.com/open-ships/n2k/pkg/adapter"
	"github.com/open-ships/n2k/pkg/pkt"
)

// CANAdapter reads raw CAN bus frames and outputs complete NMEA 2000 Packets.
// It is the primary adapter implementation for systems that receive NMEA 2000 data
// via a CAN bus interface (e.g., SocketCAN on Linux, CAN-USB adapters).
type CANAdapter struct {
	// multi is the MultiBuilder instance that handles assembly of multi-frame fast packets.
	// It maintains in-progress sequences keyed by source ID, PGN, and sequence ID, and
	// marks packets as Complete when all frames have been received.
	multi *MultiBuilder

	// handler is the downstream consumer that receives complete packets.
	// Typically this is a pkt.PacketStruct that decodes the packet into a typed Go struct.
	handler PacketHandler
}

// PacketHandler is the interface for downstream consumers of complete Packets.
// Implementations receive fully assembled packets (both single-frame and multi-frame)
// that are ready for decoding into typed PGN structs.
type PacketHandler interface {
	// HandlePacket receives a complete Packet for decoding. The packet's Decoders slice
	// is already populated and filtered by manufacturer for proprietary PGNs.
	HandlePacket(pkt.Packet)
}

// NewCANAdapter creates and returns a new CANAdapter with an initialized MultiBuilder
// for fast-packet assembly. Call SetOutput to register a PacketHandler before processing
// messages.
func NewCANAdapter() *CANAdapter {
	return &CANAdapter{
		multi: NewMultiBuilder(),
	}
}

// SetOutput registers the downstream PacketHandler that will receive complete packets.
// This must be called before HandleMessage to ensure packets are forwarded.
//
// Parameters:
//   - ph: The PacketHandler implementation to receive complete packets.
func (c *CANAdapter) SetOutput(ph PacketHandler) {
	c.handler = ph
}

// HandleMessage is the main entry point for processing incoming CAN bus frames.
// It accepts an adapter.Message, type-asserts it to *can.Frame, extracts the NMEA 2000
// metadata from the CAN ID, creates a Packet, and processes it through the pipeline.
//
// Processing flow:
//  1. Type-assert the message to *can.Frame (logs a warning for unexpected types).
//  2. Extract PGN, source, priority, and destination from the 29-bit CAN ID.
//  3. Create a new Packet and look up candidate decoders.
//  4. If there are parse errors (e.g., unknown PGN), forward immediately as-is.
//  5. If the PGN is a fast-packet type, pass to MultiBuilder for assembly.
//     Otherwise, mark as Complete immediately (single-frame message).
//  6. Once Complete, filter decoders by manufacturer and forward to the handler.
//
// Parameters:
//   - message: An adapter.Message that should be a *can.Frame from the brutella/can library.
func (c *CANAdapter) HandleMessage(message adapter.Message) {
	switch f := message.(type) {
	case *can.Frame:
		// Extract NMEA 2000 message metadata (PGN, source, priority, destination) from
		// the 29-bit extended CAN ID.
		pInfo := NewPacketInfo(f)

		// Create a Packet and look up candidate decoders from the canboat PGN registry.
		packet := pkt.NewPacket(pInfo, f.Data[:])

		// Reference: https://endige.com/2050/nmea-2000-pgns-deciphered/

		// If the packet already has parse errors (e.g., PGN not found in registry),
		// forward it immediately -- it will become an UnknownPGN downstream.
		if len(packet.ParseErrors) > 0 {
			c.packetReady(packet)
			return
		}

		if packet.Fast {
			// Fast-packet PGN: pass to MultiBuilder for multi-frame assembly.
			// The MultiBuilder tracks in-progress sequences and sets packet.Complete = true
			// when all frames have arrived.
			c.multi.Add(packet)
		} else {
			// Single-frame PGN: all data fits in one CAN frame, so it's immediately complete.
			packet.Complete = true
		}

		if packet.Complete {
			// Packet is fully assembled. Filter candidate decoders by manufacturer code
			// (for proprietary PGNs) and forward to the downstream handler.
			packet.AddDecoders()
			c.packetReady(packet)
		}
	default:
		// Received an unexpected message type -- log a warning but don't panic.
		// This could happen if the adapter is wired to the wrong endpoint type.
		slog.Warn(fmt.Sprintf("CanAdapter expected *can.Frame, received: %T", f))
	}
}

// packetReady is an internal helper that forwards a complete packet to the registered
// PacketHandler. It safely handles the case where no handler has been set (nil check)
// to avoid panics during testing or initial setup.
//
// Parameters:
//   - packet: The complete Packet to forward (passed by value via dereference).
func (c *CANAdapter) packetReady(packet *pkt.Packet) {
	if c.handler != nil {
		c.handler.HandlePacket(*packet)
	}
}
