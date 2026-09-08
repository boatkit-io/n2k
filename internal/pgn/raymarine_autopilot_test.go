// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package pgn

import (
	"testing"

	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/stretchr/testify/require"
)

func TestSeatalk1KeystrokePreservesAutoKeyComplement(t *testing.T) {
	device := uint8(0x41)
	inverted := uint8(0xfe)
	message := &publicpgn.Seatalk1Keystroke{
		ManufacturerCode: publicpgn.Raymarine,
		IndustryCode:     publicpgn.MarineIndustry,
		ProprietaryID:    publicpgn.Seatalk1Encoded,
		Command:          publicpgn.Seatalk1,
		Seatalk1Command:  publicpgn.Keystroke,
		Device:           &device,
		Key:              publicpgn.Auto_5,
		Keyinverted:      &inverted,
		UnknownData:      make([]uint8, 14),
	}
	stream := NewDataStream(make([]uint8, 22))

	_, err := EncodeStruct(message, stream)
	require.NoError(t, err)
	require.Equal(t, []uint8{0x3b, 0x9f, 0xf0, 0x81, 0x86, 0x41, 0x01, 0xfe}, stream.GetData()[:8])

	stream.resetToStart()
	decoder, err := FindDecoder(stream, publicpgn.Seatalk1KeystrokePGN)
	require.NoError(t, err)
	decoded, err := decoder(publicpgn.MessageInfo{}, stream)
	require.NoError(t, err)
	keystroke := decoded.(publicpgn.Seatalk1Keystroke)
	require.NotNil(t, keystroke.Keyinverted)
	require.Equal(t, inverted, *keystroke.Keyinverted)
}
