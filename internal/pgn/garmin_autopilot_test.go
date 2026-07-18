package pgn

import (
	"reflect"
	"testing"

	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
)

func ptrTo[T any](value T) *T {
	return &value
}

func TestGarminAutopilotTypesAreFullyExportedAndRoutable(t *testing.T) {
	wrapperByte := ptrTo(uint8(4))
	fieldGroup0 := ptrTo(uint8(0))
	fieldGroup1 := ptrTo(uint8(1))
	fieldGroup2 := ptrTo(uint8(2))
	fieldGroup5 := ptrTo(uint8(5))

	tests := []struct {
		name    string
		message any
		pgn     uint32
	}{
		{
			name: "heading to steer",
			message: publicpgn.GarminAutopilotHeadingToSteer{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup0,
				Field:            publicpgn.HeadingToSteer,
				HeadingToSteer:   ptrTo(float32(1.25)),
			},
			pgn: publicpgn.GarminAutopilotHeadingToSteerPGN,
		},
		{
			name: "rate of turn",
			message: publicpgn.GarminAutopilotRateOfTurn{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup0,
				Field:            publicpgn.GarminAutopilotFieldConstRateOfTurn,
				RateOfTurn:       ptrTo(float32(0.5)),
			},
			pgn: publicpgn.GarminAutopilotRateOfTurnPGN,
		},
		{
			name: "rate of turn order",
			message: publicpgn.GarminAutopilotRateOfTurnOrder{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup0,
				Field:            publicpgn.RateOfTurnOrder_2,
				RateOfTurnOrder:  ptrTo(float32(-0.5)),
			},
			pgn: publicpgn.GarminAutopilotRateOfTurnOrderPGN,
		},
		{
			name: "speed",
			message: publicpgn.GarminAutopilotSpeed{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup0,
				Field:            publicpgn.GarminAutopilotFieldConstSpeed,
				Speed:            ptrTo(float32(4.5)),
			},
			pgn: publicpgn.GarminAutopilotSpeedPGN,
		},
		{
			name: "system voltage",
			message: publicpgn.GarminAutopilotSystemVoltage{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup2,
				Field:            publicpgn.SystemVoltage,
				SystemVoltage:    ptrTo(float32(12.6)),
			},
			pgn: publicpgn.GarminAutopilotSystemVoltagePGN,
		},
		{
			name: "turn angle order",
			message: publicpgn.GarminAutopilotTurnAngleOrder{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup2,
				Field:            publicpgn.TurnAngleOrder,
				TurnAngleOrder:   ptrTo(float32(0.25)),
			},
			pgn: publicpgn.GarminAutopilotTurnAngleOrderPGN,
		},
		{
			name: "turn angle measured",
			message: publicpgn.GarminAutopilotTurnAngleMeasured{
				ManufacturerCode:  publicpgn.Garmin,
				IndustryCode:      publicpgn.MarineIndustry,
				SubProtocolID:     publicpgn.AutopilotTransport,
				WrapperByte1:      wrapperByte,
				WrapperByte2:      wrapperByte,
				FieldGroup:        fieldGroup2,
				Field:             publicpgn.TurnAngleMeasured,
				TurnAngleMeasured: ptrTo(float32(-0.25)),
			},
			pgn: publicpgn.GarminAutopilotTurnAngleMeasuredPGN,
		},
		{
			name: "engine RPM A",
			message: publicpgn.GarminAutopilotEngineRPMA{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup5,
				Field:            publicpgn.EngineRPMA,
				EngineSpeed:      ptrTo(uint16(1800)),
			},
			pgn: publicpgn.GarminAutopilotEngineRPMAPGN,
		},
		{
			name: "engine RPM B",
			message: publicpgn.GarminAutopilotEngineRPMB{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup5,
				Field:            publicpgn.EngineRPMB,
				EngineSpeed:      ptrTo(uint16(1750)),
			},
			pgn: publicpgn.GarminAutopilotEngineRPMBPGN,
		},
		{
			name: "response setting",
			message: publicpgn.GarminAutopilotResponseSetting{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup1,
				Field:            publicpgn.ResponseSetting,
				ResponseSetting:  ptrTo(int8(3)),
			},
			pgn: publicpgn.GarminAutopilotResponseSettingPGN,
		},
		{
			name: "mode state",
			message: publicpgn.GarminAutopilotModeState{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       fieldGroup5,
				Field:            publicpgn.ModeState,
				ModeState:        publicpgn.ShadowDrive,
			},
			pgn: publicpgn.GarminAutopilotModeStatePGN,
		},
		{
			name: "heartbeat",
			message: publicpgn.GarminAutopilotHeartbeat{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       ptrTo(uint8(21)),
				Field:            publicpgn.GarminAutopilotFieldConstHeartbeat,
				HeartbeatData:    []uint8{1, 2, 3, 4},
			},
			pgn: publicpgn.GarminAutopilotHeartbeatPGN,
		},
		{
			name: "maneuver",
			message: publicpgn.GarminAutopilotManeuver{
				ManufacturerCode: publicpgn.Garmin,
				IndustryCode:     publicpgn.MarineIndustry,
				SubProtocolID:    publicpgn.AutopilotTransport,
				WrapperByte1:     wrapperByte,
				WrapperByte2:     wrapperByte,
				FieldGroup:       ptrTo(uint8(38)),
				ManeuverCode:     ptrTo(uint8(1)),
				Value:            ptrTo(uint8(2)),
			},
			pgn: publicpgn.GarminAutopilotManeuverPGN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pgn != 126720 {
				t.Fatalf("exported PGN = %d, want 126720", tt.pgn)
			}

			out := NewDataStream(make([]uint8, MaxPGNLength))
			info, err := EncodeStruct(tt.message, out)
			if err != nil {
				t.Fatalf("EncodeStruct() error = %v", err)
			}
			if info.PGN != tt.pgn {
				t.Fatalf("encoded PGN = %d, want %d", info.PGN, tt.pgn)
			}

			in := NewDataStream(out.GetData())
			decoder, err := FindDecoder(in, info.PGN)
			if err != nil {
				t.Fatalf("FindDecoder() error = %v", err)
			}
			decoded, err := decoder(*info, in)
			if err != nil {
				t.Fatalf("decoder() error = %v", err)
			}
			if got, want := reflect.TypeOf(decoded), reflect.TypeOf(tt.message); got != want {
				t.Fatalf("decoded type = %v, want %v", got, want)
			}
		})
	}

	modeStates := []publicpgn.GarminAutopilotModeStateConst{
		publicpgn.Standby_6,
		publicpgn.ShadowDrive,
		publicpgn.Engaged_2,
	}
	if len(modeStates) != 3 {
		t.Fatalf("exported Garmin autopilot mode states = %d, want 3", len(modeStates))
	}
}
