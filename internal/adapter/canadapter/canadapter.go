// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package canadapter implements the adapter interface for n2k endpoints.
package canadapter

import (
	"fmt"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"

	"github.com/boatkit-io/n2k/internal/adapter"
	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/boatkit-io/n2k/pkg/endpoint"
)

// CANAdapter instances on input read canbus frames from its input and outputs complete Packets.
// On output it
type CANAdapter struct {
	multi *MultiBuilder // combines multiple frames into a complete Packet.
	log   *logrus.Logger

	handler     PacketHandler
	frameWriter endpoint.Endpoint
	seqIDMap    map[uint8]map[uint32]uint8 //sourceID:PGN:last used sequenceID
}

// PacketHandler is an interface for the output handler for a CANAdapter
type PacketHandler interface {
	HandlePacket(pkt.Packet)
}

// FrameWriter is an interface for the endpoint frame writer for a CANAdapter
type FrameWriter interface {
	WriteFrame(can.Frame)
}

// NewCANAdapter instantiates a new CanAdapter
func NewCANAdapter(log *logrus.Logger) *CANAdapter {
	return &CANAdapter{
		multi:    NewMultiBuilder(log),
		log:      log,
		seqIDMap: make(map[uint8]map[uint32]uint8), // SourceID, PGN, most recently used sequenceId
	}
}

// SetWriter assigns the argument to the frameWriter field
func (c *CANAdapter) SetWriter(writer endpoint.Endpoint) {
	c.frameWriter = writer
}

// SetOutput assigns a handler for any ready packets
func (c *CANAdapter) SetOutput(ph PacketHandler) {
	c.handler = ph
}

// HandleMessage is how you tell CanAdapter to start processing a new message into a packet
func (c *CANAdapter) HandleMessage(message adapter.Message) {
	switch f := message.(type) {
	case *can.Frame:
		pInfo := ExtractMessageInfo(f)
		p := pkt.NewPacket(pInfo, f.Data[:])

		// https://endige.com/2050/nmea-2000-pgns-deciphered/

		if len(p.ParseErrors) > 0 {
			c.packetReady(p)
			return
		}

		if pgn.IsFast(p.Info.PGN) {
			c.multi.Add(p)
		} else {
			p.Complete = true
		}

		if p.Complete {
			c.packetReady(p)
		}
	default:
		c.log.Warnf("CanAdapter expected *can.Frame, received: %T", f)
	}
}

// packetReady is a helper for fanning out completed packets to the handler
func (c *CANAdapter) packetReady(p *pkt.Packet) {
	if c.handler != nil {
		c.handler.HandlePacket(*p)
	}
}

// WritePgn generates one or more frames from its input and writes them to its configured endpoint.
func (c *CANAdapter) WritePgn(info pgn.MessageInfo, data []uint8) error {
	var err error
	canIDData := converter.CanIDData{
		PGN:         info.PGN,
		SourceID:    info.SourceId,
		Priority:    info.Priority,
		Destination: info.TargetId,
	}
	canID := converter.CanIDFromStruct(canIDData)
	if pgn.IsFast((info.PGN)) {
		err = c.sendFast(info.SourceId, info.PGN, canID, data)
	} else {
		err = c.sendSingle(canID, data)
	}
	return err
}

// calcFramesRequired calculates the number of CAN frames required to transmit data of the specified length.
func calcFramesRequired(length int) int {
	var count int
	if length < 7 {
		count++
	} else {
		count += (length - 6) / 7
		if (length-6)%7 > 0 {
			count++
		}
	}
	return count
}

// sendFast breaks the data up into the required number of packets, provides a sequenceID,
// and passes the resulting frames on.
func (c *CANAdapter) sendFast(sourceID uint8, pgnNum, canID uint32, data []uint8) error {
	var buffer [8]uint8
	total := len(data)
	framesRequired := calcFramesRequired(total)
	if framesRequired > MaxFrameNum {
		return fmt.Errorf("exceeds maximum data length for Fast PGN (223): %d", total)
	}
	if _, t := c.seqIDMap[sourceID]; !t {
		c.seqIDMap[sourceID] = make(map[uint32]uint8)
	}
	if _, t := c.seqIDMap[sourceID][pgnNum]; !t {
		c.seqIDMap[sourceID][pgnNum] = 0
	}
	seqID := c.seqIDMap[sourceID][pgnNum]
	nextID := (seqID + 1) % 7
	c.seqIDMap[sourceID][pgnNum] = nextID
	index := 0
	for frameNum := 0; frameNum <= framesRequired; frameNum++ {
		offset := 0
		buffer[offset] = seqID<<5 | uint8(frameNum)
		offset++
		if frameNum == 0 {
			buffer[offset] = uint8(total)
			offset++
		}
		for {
			if index >= total { // index is zero based, total is # of bytes
				buffer[offset] = 0xFF
			} else {
				buffer[offset] = data[index]
			}
			index++
			offset++
			if offset == can.MaxFrameDataLength {
				frame := can.Frame{
					ID:     canID,
					Length: uint8(can.MaxFrameDataLength),
					Data:   buffer,
				}
				// invoke endpoint handler
				if c.frameWriter != nil {
					c.log.Debugf("Writing CAN frame: ID=0x%X, Length=%d, Data=%02X", frame.ID, frame.Length, frame.Data[:frame.Length])
					c.frameWriter.WriteFrame(frame)
				} else {
					c.log.Warn("frameWriter is nil, cannot write frame")
				}
				offset = 0
				if index >= total {
					break
				}
				break
			}
			if index >= total {
				continue
			}
		}
	}
	return nil
}

// sendSingle creates a CAN frame for the message and sends it on.
func (c *CANAdapter) sendSingle(canID uint32, data []uint8) error {
	length := len(data)
	if length > 8 {
		return fmt.Errorf("attempt to send single PGN with data length %d; max is 8", length)
	}
	frame := can.Frame{
		ID:     canID,
		Length: uint8(length),
	}
	copy(frame.Data[:], data)
	i := frame.Length
	for i < 8 {
		frame.Data[i] = 0xFF
		i++
	}
	// invoke endpoint handler
	if c.frameWriter != nil {
		c.frameWriter.WriteFrame(frame)
	}
	return nil
}

// ExtractMessageInfo extracts MessageInfo from a CAN frame
func ExtractMessageInfo(message *can.Frame) pgn.MessageInfo {
	h := converter.DecodeCanID(message.ID)
	return pgn.MessageInfo{
		PGN:      h.PGN,
		SourceId: h.SourceID,
		TargetId: h.TargetID,
		Priority: h.Priority,
	}
}
