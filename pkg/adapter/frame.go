package adapter

import (
	"time"

	"github.com/boatkit-io/n2k/pkg/pgn"
)

// This data structure is copied from
// https://github.com/brutella/can/blob/master/frame.go
// licensed under the MIT License, following

/*
The MIT License (MIT)

Copyright (c) 2016 Matthias Hochgatterer

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

*/

type Frame struct {
	// bit 0-28: CAN identifier (11/29 bit)
	// bit 29: error message flag (ERR)
	// bit 30: remote transmision request (RTR)
	// bit 31: extended frame format (EFF)
	ID     uint32
	Length uint8
	Flags  uint8
	Res0   uint8
	Res1   uint8
	Data   [8]uint8
}

func NewPacketInfo(message Frame) pgn.MessageInfo {
	p := pgn.MessageInfo{
		Timestamp: time.Now(),
		SourceId:  uint8(message.ID & 0xFF),
		PGN:       (message.ID & 0x3FFFF00) >> 8,
		Priority:  uint8((message.ID & 0x1C000000) >> 26),
	}

	pduFormat := uint8((p.PGN & 0xFF00) >> 8)
	if pduFormat < 240 {
		// This is a targeted packet, and the lower PS has the address
		p.TargetId = uint8(p.PGN & 0xFF)
		p.PGN &= 0xFFF00
	}
	return p
}
