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

type capturePGNEndpoint struct {
	captureEndpoint
	messages     []endpoint.PGNMessage
	completePGNs *bool
}

func (c *capturePGNEndpoint) WritePGN(message endpoint.PGNMessage) error {
	c.messages = append(c.messages, message)
	return nil
}

func (c *capturePGNEndpoint) SupportsPGNWrites() bool {
	return c.completePGNs == nil || *c.completePGNs
}

type capturePacketHandler struct {
	packets []pkt.Packet
}

func (c *capturePacketHandler) HandlePacket(packet pkt.Packet) { //nolint:gocritic // PacketHandler requires a value.
	c.packets = append(c.packets, packet)
}

func (c *captureEndpoint) Start(context.Context) error { return nil }
func (c *captureEndpoint) Run(context.Context) error   { return nil }
func (c *captureEndpoint) Close() error                { return nil }
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
	err := adapter.sendFast(35, publicpgn.UserDatumPGN, 0x1F80523, data)
	require.NoError(t, err)
	require.Len(t, writer.frames, 1)

	frame := writer.frames[0]
	assert.Equal(t, uint8(8), frame.Length)
	assert.Equal(t, uint8(0), frame.Data[0])
	assert.Equal(t, uint8(len(data)), frame.Data[1])
	assert.Equal(t, data, frame.Data[2:8])
}

func TestCompletePGNEndpointBypassesCANFragmentation(t *testing.T) {
	writer := &capturePGNEndpoint{}
	adapter := NewCANAdapter(logrus.New())
	adapter.SetWriter(writer)
	info := pgn.MessageInfo{Priority: 3, PGN: publicpgn.ProductInformationPGN, SourceId: 22, TargetId: 255}
	data := make([]byte, 134)
	data[0] = 0x10

	require.NoError(t, adapter.WritePgn(info, data))
	require.Empty(t, writer.frames)
	require.Len(t, writer.messages, 1)
	assert.Equal(t, uint32(publicpgn.ProductInformationPGN), writer.messages[0].PGN)
	assert.Equal(t, uint8(22), writer.messages[0].Source)
	assert.Equal(t, data, writer.messages[0].Data)
}

func TestConditionalPGNEndpointFallsBackToCANFrames(t *testing.T) {
	enabled := false
	writer := &capturePGNEndpoint{completePGNs: &enabled}
	adapter := NewCANAdapter(logrus.New())
	adapter.SetWriter(writer)
	info := pgn.MessageInfo{Priority: 3, PGN: publicpgn.BinarySwitchBankStatusPGN, SourceId: 22, TargetId: 255}

	require.NoError(t, adapter.WritePgn(info, []byte{1, 2}))
	require.Empty(t, writer.messages)
	require.Len(t, writer.frames, 1)
}

func TestCompletePGNMessageIsDeliveredWithoutFastPacketHeaders(t *testing.T) {
	handler := &capturePacketHandler{}
	adapter := NewCANAdapter(logrus.New())
	adapter.SetOutput(handler)
	data := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99}

	adapter.HandleMessage(&endpoint.PGNMessage{
		Priority: 2, PGN: publicpgn.ProductInformationPGN, Source: 41, Destination: 255, Data: data,
	})

	require.Len(t, handler.packets, 1)
	assert.True(t, handler.packets[0].Complete)
	assert.Equal(t, data, handler.packets[0].Data)
	assert.Equal(t, uint8(41), handler.packets[0].Info.SourceId)
}

// TestRawToDataStream was removed as redundant to more comprehensive testing in tests/integration/pgn_serialization_test.go
