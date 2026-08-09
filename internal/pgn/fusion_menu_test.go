// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package pgn

import (
	"bytes"
	"reflect"
	"testing"

	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
)

func TestFusionMenuPacketsFromGarminCapture(t *testing.T) {
	tests := []struct {
		name     string
		pgn      uint32
		data     []byte
		wantType any
	}{
		{
			name: "select channel row four", pgn: 126720,
			data:     []byte{0xA3, 0x99, 0x09, 0x00, 0x02, 0x04, 0x00, 0x00, 0x00, 0x02, 0x00},
			wantType: publicpgn.FusionMenuActionCommand{},
		},
		{
			name: "request menu count", pgn: 126720,
			data:     []byte{0xA3, 0x99, 0x0A, 0x00, 0x02, 0x00},
			wantType: publicpgn.FusionRequestMenuCount{},
		},
		{
			name: "request fourteen menu rows", pgn: 126720,
			data:     []byte{0xA3, 0x99, 0x0B, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x0E, 0x00, 0x00, 0x00, 0x00},
			wantType: publicpgn.FusionRequestMenuItems{},
		},
		{
			name: "menu action status", pgn: 130820,
			data:     []byte{0xA3, 0x99, 0x0F, 0x80, 0x02, 0x04, 0x00, 0x00, 0x00, 0x04, 0x00},
			wantType: publicpgn.FusionMenuActionStatus{},
		},
		{
			name: "menu count status", pgn: 130820,
			data:     []byte{0xA3, 0x99, 0x10, 0x80, 0x02, 0x1F, 0x00, 0x00, 0x00, 0x00},
			wantType: publicpgn.FusionMenuCount{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := NewDataStream(test.data)
			decoder, err := FindDecoder(stream, test.pgn)
			if err != nil {
				t.Fatalf("FindDecoder(%d) error = %v", test.pgn, err)
			}
			decoded, err := decoder(publicpgn.MessageInfo{PGN: test.pgn}, stream)
			if err != nil {
				t.Fatalf("decode PGN %d error = %v", test.pgn, err)
			}
			if reflect.TypeOf(decoded) != reflect.TypeOf(test.wantType) {
				t.Fatalf("decoded type = %T, want %T", decoded, test.wantType)
			}
			encoded := NewDataStream(make([]byte, MaxPGNLength))
			if _, err := EncodeStruct(decoded, encoded); err != nil {
				t.Fatalf("EncodeStruct() error = %v", err)
			}
			if got := encoded.GetData(); !bytes.Equal(got, test.data) {
				t.Fatalf("round-trip data = % X, want % X", got, test.data)
			}
		})
	}
}
