package pgn

import (
	"encoding/hex"
	"testing"

	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
)

func TestGarminAISShortPayloadsDecode(t *testing.T) {
	tests := []struct {
		name       string
		pgn        uint32
		payloadHex string
		assert     func(*testing.T, any)
	}{
		{
			name:       "class A position without sequence ID",
			pgn:        publicpgn.AISClassAPositionReportPGN,
			payloadHex: "03b423f415002199b500e1d81de18921ed01800d08fe1d191a00fe",
			assert: func(t *testing.T, decoded any) {
				message, ok := decoded.(publicpgn.AISClassAPositionReport)
				if !ok {
					t.Fatalf("decoded type = %T, want AISClassAPositionReport", decoded)
				}
				if message.UserID == nil || *message.UserID != 368321460 {
					t.Fatalf("UserID = %v, want 368321460", message.UserID)
				}
				if message.SequenceID != nil {
					t.Fatalf("SequenceID = %v, want nil", message.SequenceID)
				}
			},
		},
		{
			name:       "class B static data part A without trailing metadata",
			pgn:        publicpgn.AISClassBStaticDataMsg24PartAPGN,
			payloadHex: "1814cdef1552414242495420544f4f40404040404040404040",
			assert: func(t *testing.T, decoded any) {
				message, ok := decoded.(publicpgn.AISClassBStaticDataMsg24PartA)
				if !ok {
					t.Fatalf("decoded type = %T, want AISClassBStaticDataMsg24PartA", decoded)
				}
				if message.UserID == nil || *message.UserID != 368037140 {
					t.Fatalf("UserID = %v, want 368037140", message.UserID)
				}
				if message.Name != "RABBIT TOO" {
					t.Fatalf("Name = %q, want %q", message.Name, "RABBIT TOO")
				}
				if message.SequenceID != nil {
					t.Fatalf("SequenceID = %v, want nil", message.SequenceID)
				}
			},
		},
		{
			name:       "class B static data part B without transceiver or sequence ID",
			pgn:        publicpgn.AISClassBStaticDataMsg24PartBPGN,
			payloadHex: "18f8402c14254e56433130323440404040404040ffffffff00000000ffffffff03",
			assert: func(t *testing.T, decoded any) {
				message, ok := decoded.(publicpgn.AISClassBStaticDataMsg24PartB)
				if !ok {
					t.Fatalf("decoded type = %T, want AISClassBStaticDataMsg24PartB", decoded)
				}
				if message.UserID == nil || *message.UserID != 338444536 {
					t.Fatalf("UserID = %v, want 338444536", message.UserID)
				}
				if message.VendorID != "NVC1024" {
					t.Fatalf("VendorID = %q, want %q", message.VendorID, "NVC1024")
				}
				if message.SequenceID != nil {
					t.Fatalf("SequenceID = %v, want nil", message.SequenceID)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := hex.DecodeString(test.payloadHex)
			if err != nil {
				t.Fatalf("hex.DecodeString() error = %v", err)
			}
			stream := NewDataStream(payload)
			decoder, err := FindDecoder(stream, test.pgn)
			if err != nil {
				t.Fatalf("FindDecoder() error = %v", err)
			}
			decoded, err := decoder(publicpgn.MessageInfo{PGN: test.pgn}, stream)
			if err != nil {
				t.Fatalf("decoder() error = %v", err)
			}
			test.assert(t, decoded)
		})
	}
}
