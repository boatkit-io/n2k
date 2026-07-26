// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package pgn

import (
	"bytes"
	"math"
	"testing"

	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
)

func TestDecodeObservedYanmarEngineDataA(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		engineInstance publicpgn.EngineInstanceConst
		throttle       float32
		gear           publicpgn.GearStatusConst
		engineSpeed    uint16
		unknownData    []byte
	}{
		{
			name:           "port at 22 percent",
			data:           []byte{0xAC, 0x98, 0x01, 0xDC, 0x00, 0x03, 0x05, 0x64},
			engineInstance: publicpgn.SingleEngineOrDualEnginePort,
			throttle:       22.0,
			gear:           publicpgn.Forward,
			engineSpeed:    1283,
			unknownData:    []byte{0x64},
		},
		{
			name:           "starboard at 18 percent",
			data:           []byte{0xAC, 0x98, 0x21, 0xB8, 0x00, 0xEE, 0x04, 0x64},
			engineInstance: publicpgn.DualEngineStarboard,
			throttle:       18.4,
			gear:           publicpgn.Forward,
			engineSpeed:    1262,
			unknownData:    []byte{0x64},
		},
		{
			name:           "ten bit throttle crosses byte boundary",
			data:           []byte{0xAC, 0x98, 0x01, 0x55, 0x01, 0x62, 0x06, 0x64},
			engineInstance: publicpgn.SingleEngineOrDualEnginePort,
			throttle:       34.1,
			gear:           publicpgn.Forward,
			engineSpeed:    1634,
			unknownData:    []byte{0x64},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded := decodeObservedYanmar(t, publicpgn.YanmarEngineDataAPGN, test.data)
			message, ok := decoded.(publicpgn.YanmarEngineDataA)
			if !ok {
				t.Fatalf("decoded type = %T, want pgn.YanmarEngineDataA", decoded)
			}
			if message.EngineInstance != test.engineInstance {
				t.Fatalf("EngineInstance = %v, want %v", message.EngineInstance, test.engineInstance)
			}
			assertFloat32Pointer(t, "ThrottlePosition", message.ThrottlePosition, test.throttle)
			if message.TransmissionGear != test.gear {
				t.Fatalf("TransmissionGear = %v, want %v", message.TransmissionGear, test.gear)
			}
			assertUint16Pointer(t, "EngineSpeed", message.EngineSpeed, test.engineSpeed)
			if !bytes.Equal(message.UnknownData, test.unknownData) {
				t.Fatalf("UnknownData = % X, want % X", message.UnknownData, test.unknownData)
			}
			assertYanmarRoundTrip(t, decoded, test.data)
		})
	}
}

func TestDecodeObservedYanmarThrottleControl(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		engineInstance publicpgn.EngineInstanceConst
		throttle       float32
		gear           publicpgn.GearStatusConst
	}{
		{
			name:           "port at 22 percent",
			data:           []byte{0xAC, 0x98, 0x01, 0xFC, 0xDC, 0xFC, 0x64, 0xFF},
			engineInstance: publicpgn.SingleEngineOrDualEnginePort,
			throttle:       22.0,
			gear:           publicpgn.Forward,
		},
		{
			name:           "starboard at 18 percent",
			data:           []byte{0xAC, 0x98, 0x21, 0xFC, 0xB8, 0xFC, 0x64, 0xFF},
			engineInstance: publicpgn.DualEngineStarboard,
			throttle:       18.4,
			gear:           publicpgn.Forward,
		},
		{
			name:           "ten bit throttle crosses byte boundary",
			data:           []byte{0xAC, 0x98, 0x21, 0xFC, 0x50, 0xFD, 0x64, 0xFF},
			engineInstance: publicpgn.DualEngineStarboard,
			throttle:       33.6,
			gear:           publicpgn.Forward,
		},
		{
			name:           "neutral",
			data:           []byte{0xAC, 0x98, 0x01, 0xF9, 0x00, 0xFC, 0x64, 0xFF},
			engineInstance: publicpgn.SingleEngineOrDualEnginePort,
			throttle:       0,
			gear:           publicpgn.Neutral,
		},
		{
			name:           "reverse",
			data:           []byte{0xAC, 0x98, 0x21, 0xFA, 0x00, 0xFC, 0x64, 0xFF},
			engineInstance: publicpgn.DualEngineStarboard,
			throttle:       0,
			gear:           publicpgn.Reverse,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded := decodeObservedYanmar(t, publicpgn.YanmarThrottleControlPGN, test.data)
			message, ok := decoded.(publicpgn.YanmarThrottleControl)
			if !ok {
				t.Fatalf("decoded type = %T, want pgn.YanmarThrottleControl", decoded)
			}
			if message.EngineInstance != test.engineInstance {
				t.Fatalf("EngineInstance = %v, want %v", message.EngineInstance, test.engineInstance)
			}
			assertFloat32Pointer(t, "ThrottlePosition", message.ThrottlePosition, test.throttle)
			if message.TransmissionGear != test.gear {
				t.Fatalf("TransmissionGear = %v, want %v", message.TransmissionGear, test.gear)
			}
			if !bytes.Equal(message.UnknownData, []byte{0x64, 0xFF}) {
				t.Fatalf("UnknownData = % X, want 64 FF", message.UnknownData)
			}
			assertYanmarRoundTrip(t, decoded, test.data)
		})
	}
}

func decodeObservedYanmar(t *testing.T, pgnNumber uint32, data []byte) any {
	t.Helper()

	stream := NewDataStream(data)
	decoder, err := FindDecoder(stream, pgnNumber)
	if err != nil {
		t.Fatalf("FindDecoder(%d) error = %v", pgnNumber, err)
	}
	decoded, err := decoder(publicpgn.MessageInfo{PGN: pgnNumber}, stream)
	if err != nil {
		t.Fatalf("decode PGN %d error = %v", pgnNumber, err)
	}
	return decoded
}

func assertYanmarRoundTrip(t *testing.T, message any, want []byte) {
	t.Helper()

	stream := NewDataStream(make([]byte, MaxPGNLength))
	if _, err := EncodeStruct(message, stream); err != nil {
		t.Fatalf("EncodeStruct() error = %v", err)
	}
	if got := stream.GetData(); !bytes.Equal(got, want) {
		t.Fatalf("round-trip data = % X, want % X", got, want)
	}
}

func assertUint16Pointer(t *testing.T, name string, got *uint16, want uint16) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %d", name, got, want)
	}
}

func assertFloat32Pointer(t *testing.T, name string, got *float32, want float32) {
	t.Helper()
	if got == nil || math.Abs(float64(*got-want)) > 0.001 {
		t.Fatalf("%s = %v, want %.1f", name, got, want)
	}
}
