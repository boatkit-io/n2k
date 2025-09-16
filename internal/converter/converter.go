// Package converter provides routines that convert between various text
// data formats and Can frames.
package converter

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brutella/can"
)

// CanFrameFromRaw parses an input string into one or more can.Frames.
// For data longer than 8 bytes, it will create multiple frames according to the ISO-TP protocol.
func CanFrameFromRaw(in string) ([]*can.Frame, error) {
	elems := strings.Split(in, ",")
	if len(elems) < 6 {
		return nil, fmt.Errorf("invalid raw format: insufficient elements")
	}

	priority, err := strconv.ParseUint(elems[1], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("invalid priority: %w", err)
	}
	pgn, err := strconv.ParseUint(elems[2], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid pgn: %w", err)
	}
	source, err := strconv.ParseUint(elems[3], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("invalid source: %w", err)
	}
	destination, err := strconv.ParseUint(elems[4], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("invalid destination: %w", err)
	}
	length, err := strconv.ParseUint(elems[5], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("invalid length: %w", err)
	}

	if int(length) > len(elems)-6 {
		return nil, fmt.Errorf("invalid raw format: data length exceeds available bytes")
	}

	id := CanIdFromData(uint32(pgn), uint8(source), uint8(priority), uint8(destination))

	// TO DO: if this is a fast PGN, we need to handle it differently
	// those must always be formatted as a fast frame, even if it's only 6 bytes (sequence/frame number, length, 6 bytes of data)
	// For data <= 8 bytes, return a single frame
	if length <= 8 {
		frame := &can.Frame{
			ID:     id,
			Length: uint8(length),
		}
		for i := 0; i < int(length); i++ {
			b, err := strconv.ParseUint(elems[i+6], 16, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid data byte at position %d: %w", i, err)
			}
			frame.Data[i] = uint8(b)
		}
		return []*can.Frame{frame}, nil
	}

	// For data > 8 bytes, create multiple frames using NMEA 2000 fast packet format
	var frames []*can.Frame
	remainingBytes := int(length)
	dataIndex := 6 // Start of data in elems

	// Use sequence ID 0 for now (will be overridden by RawFileEndpoint)
	seqId := uint8(0)

	// First frame (frame 0)
	firstFrame := &can.Frame{
		ID:     id,
		Length: 8,
	}
	// First byte: (sequenceId << 5) | frameNumber
	firstFrame.Data[0] = (seqId << 5) // Frame 0
	// Second byte: total data length
	firstFrame.Data[1] = uint8(length)

	// Copy up to 6 bytes of data
	for i := 0; i < min(6, remainingBytes); i++ {
		b, err := strconv.ParseUint(elems[dataIndex+i], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid data byte at position %d: %w", i, err)
		}
		firstFrame.Data[i+2] = uint8(b)
	}
	frames = append(frames, firstFrame)

	remainingBytes -= 6
	dataIndex += 6
	frameNum := uint8(1)

	// Consecutive frames
	for remainingBytes > 0 {
		frame := &can.Frame{
			ID:     id,
			Length: 8,
		}
		// First byte: (sequenceId << 5) | frameNumber
		frame.Data[0] = (seqId << 5) | (frameNum & 0x1F)

		// Copy up to 7 bytes of data (no length byte in continuation frames)
		bytesToCopy := min(7, remainingBytes)
		for i := 0; i < bytesToCopy; i++ {
			b, err := strconv.ParseUint(elems[dataIndex+i], 16, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid data byte at position %d: %w", dataIndex+i, err)
			}
			frame.Data[i+1] = uint8(b)
		}

		frames = append(frames, frame)
		remainingBytes -= bytesToCopy
		dataIndex += bytesToCopy
		frameNum++
	}

	return frames, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CanIdData represents the data needed to construct a CAN ID
type CanIdData struct {
	PGN         uint32
	SourceId    uint8
	Priority    uint8
	Destination uint8
}

// CanIdFromData returns an encoded ID from its inputs.
func CanIdFromData(pgn uint32, sourceId uint8, priority uint8, destination uint8) uint32 {
	// Handle destination encoding based on PDU format
	pduFormat := uint8((pgn & 0xFF00) >> 8)
	if pduFormat < 240 {
		// PDU1 format: always encode destination in lower 8 bits of PGN
		pgn = (pgn & 0xFFF00) | uint32(destination)
	}
	// PDU2 format: destination is always global (255), don't encode it

	// Build the base CAN ID: Priority(3) + Reserved(1) + PGN(18) + Source(8)
	canId := uint32(sourceId) | (pgn << 8) | (uint32(priority) << 26)

	// Set the Extended Frame Format (EFF) bit (bit 31) for 29-bit CAN ID
	// This is required for NMEA 2000 which uses extended CAN IDs
	canId |= 0x80000000 // MaskEff from brutella/can constants

	return canId
}

// CanIdFromStruct returns an encoded ID from a CanIdData struct.
func CanIdFromStruct(data CanIdData) uint32 {
	return CanIdFromData(data.PGN, data.SourceId, data.Priority, data.Destination)
}

// CanIdFromDataWithValidation returns an encoded ID from its inputs with validation.
// Returns an error if a PDU2 PGN is specified with a destination other than 0 or 255.
func CanIdFromDataWithValidation(pgn uint32, sourceId uint8, priority uint8, destination uint8) (uint32, error) {
	pduFormat := uint8((pgn & 0xFF00) >> 8)
	if pduFormat >= 240 {
		// PDU2 format: destination must be 0 or 255
		if destination != 0 && destination != 255 {
			return 0, fmt.Errorf("PDU2 PGN %d requires destination to be 0 or 255, got %d", pgn, destination)
		}
	}

	return CanIdFromData(pgn, sourceId, priority, destination), nil
}

// CanIdFromStructWithValidation returns an encoded ID from a CanIdData struct with validation.
// Returns an error if a PDU2 PGN is specified with a destination other than 0 or 255.
func CanIdFromStructWithValidation(data CanIdData) (uint32, error) {
	return CanIdFromDataWithValidation(data.PGN, data.SourceId, data.Priority, data.Destination)
}

// FrameHeader defines a structure to capture the RAW defined information comprising a CAN Frame ID
// and the recorded timestamp
type FrameHeader struct {
	TimeStamp time.Time
	SourceId  uint8
	PGN       uint32
	Priority  uint8
	TargetId  uint8
}

// DecodeCanId returns a frame header extracted from frame.Id
func DecodeCanId(id uint32) FrameHeader {
	r := FrameHeader{
		TimeStamp: time.Now(),
		SourceId:  uint8(id & 0xFF),
		PGN:       (id & 0x3FFFF00) >> 8,
		Priority:  uint8((id & 0x1C000000) >> 26),
	}

	pduFormat := uint8((r.PGN & 0xFF00) >> 8)
	if pduFormat < 240 {
		// PDU1 format: extract destination from lower 8 bits
		r.TargetId = uint8(r.PGN & 0xFF)
		r.PGN &= 0xFFF00
	} else {
		// PDU2 format: destination is always global
		r.TargetId = 255
	}
	return r
}

// RawFromCanFrame returns a string in RAW format encoding the frame
func RawFromCanFrame(f can.Frame) string {
	h := DecodeCanId(f.ID)
	return fmt.Sprintf("%s,%d,%d,%d,%d,%d,%02x,%02x,%02x,%02x,%02x,%02x,%02x,%02x\n", h.TimeStamp.Format("2006-01-02T15:04:05Z"), h.Priority, h.PGN, h.SourceId, h.TargetId, f.Length, f.Data[0], f.Data[1], f.Data[2], f.Data[3], f.Data[4], f.Data[5], f.Data[6], f.Data[7])

}
