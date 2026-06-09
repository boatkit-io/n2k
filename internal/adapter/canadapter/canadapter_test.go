package canadapter

import (
	"context"
	"testing"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/brutella/can"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgn127501(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f, err := converter.CanFrameFromRaw(raw)
	assert.Nil(t, err)
	pInfo := ExtractMessageInfo(f[0])
	p := pkt.NewPacket(pInfo, f[0].Data[:])
	stream := pgn.NewDataStream(p.Data)
	decoder, err := pgn.FindDecoder(stream, p.Info.PGN)
	assert.Nil(t, err)
	ret, err := decoder(p.Info, stream)
	assert.Nil(t, err)
	assert.IsType(t, publicpgn.BinarySwitchBankStatus{}, ret)
}

func TestPgn127501Write(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f, err := converter.CanFrameFromRaw(raw)
	assert.Nil(t, err)
	pInfo := ExtractMessageInfo(f[0])
	p := pkt.NewPacket(pInfo, f[0].Data[:])
	stream := pgn.NewDataStream(p.Data)
	decoder, err := pgn.FindDecoder(stream, p.Info.PGN)
	assert.Nil(t, err)
	ret, err := decoder(p.Info, stream)
	assert.Nil(t, err)
	assert.IsType(t, publicpgn.BinarySwitchBankStatus{}, ret)
}

type captureEndpoint struct {
	frames []can.Frame
}

func (c *captureEndpoint) Run(context.Context) error { return nil }
func (c *captureEndpoint) Close() error              { return nil }
func (c *captureEndpoint) SetOutput(_ endpoint.MessageHandler) {
}
func (c *captureEndpoint) WriteFrame(frame can.Frame) {
	c.frames = append(c.frames, frame)
}

func TestCalcFramesRequired(t *testing.T) {
	tests := []struct {
		length int
		want   int
	}{
		{length: 0, want: 0},
		{length: 6, want: 0},
		{length: 7, want: 1},
		{length: 13, want: 1},
		{length: 14, want: 2},
		{length: 223, want: 31},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, calcFramesRequired(tt.length))
	}
}

func TestSendFastShortPayloadWritesOneFrame(t *testing.T) {
	writer := &captureEndpoint{}
	adapter := NewCANAdapter(logrus.New())
	adapter.SetWriter(writer)

	data := []uint8{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
	err := adapter.sendFast(35, publicpgn.UserDatumPgn, 0x1F80523, data)
	require.NoError(t, err)
	require.Len(t, writer.frames, 1)

	frame := writer.frames[0]
	assert.Equal(t, uint8(8), frame.Length)
	assert.Equal(t, uint8(0), frame.Data[0])
	assert.Equal(t, uint8(len(data)), frame.Data[1])
	assert.Equal(t, data, frame.Data[2:8])
}

// TestRawToDataStream was removed as redundant to more comprehensive testing in tests/integration/pgn_serialization_test.go
