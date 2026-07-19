package pgn

import (
	"bytes"
	"testing"

	publicpgn "github.com/boatkit-io/n2k/pkg/pgn"
)

func groupFunctionRequestPayload(referencedPGN uint32, numberOfParameters uint8, rawData []byte) []byte {
	payload := []byte{
		uint8(publicpgn.Request),
		uint8(referencedPGN),
		uint8(referencedPGN >> 8),
		uint8(referencedPGN >> 16),
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff,
		numberOfParameters,
	}
	return append(payload, rawData...)
}

func decodeGroupFunctionPayload(t *testing.T, data []byte) any {
	t.Helper()

	stream := NewDataStream(data)
	decoder, err := FindDecoder(stream, publicpgn.NMEARequestGroupFunctionPGN)
	if err != nil {
		t.Fatalf("FindDecoder() error = %v", err)
	}

	decoded, err := decoder(publicpgn.MessageInfo{PGN: publicpgn.NMEARequestGroupFunctionPGN}, stream)
	if err != nil {
		t.Fatalf("decoder() error = %v", err)
	}
	return decoded
}

func TestDecodeNmeaRequestGroupFunctionKnownTargetReturnsCompleteStruct(t *testing.T) {
	decoded := decodeGroupFunctionPayload(t, groupFunctionRequestPayload(publicpgn.ProductInformationPGN, 0, nil))

	request, ok := decoded.(publicpgn.NMEARequestGroupFunction)
	if !ok {
		t.Fatalf("decoded type = %T, want %T", decoded, publicpgn.NMEARequestGroupFunction{})
	}
	if request.PGN == nil || *request.PGN != publicpgn.ProductInformationPGN {
		t.Fatalf("decoded referenced PGN = %v, want %d", request.PGN, publicpgn.ProductInformationPGN)
	}
	if len(request.Repeating1) != 0 {
		t.Fatalf("decoded repeating fields = %d, want 0", len(request.Repeating1))
	}
}

func TestDecodeNmeaRequestGroupFunctionKnownTargetConsumesParameterValues(t *testing.T) {
	rawData := []byte{
		2, 0x34, 0x12, // ProductInformation.ProductCode: 16 bits
		1, 0x08, 0x34, // ProductInformation.NMEA2000Version: 16 bits
	}
	decoded := decodeGroupFunctionPayload(t, groupFunctionRequestPayload(publicpgn.ProductInformationPGN, 2, rawData))

	request, ok := decoded.(publicpgn.NMEARequestGroupFunction)
	if !ok {
		t.Fatalf("decoded type = %T, want %T", decoded, publicpgn.NMEARequestGroupFunction{})
	}
	if len(request.Repeating1) != 2 {
		t.Fatalf("decoded repeating fields = %d, want 2", len(request.Repeating1))
	}
	if request.Repeating1[0].Parameter == nil || *request.Repeating1[0].Parameter != 2 {
		t.Fatalf("first parameter = %v, want 2", request.Repeating1[0].Parameter)
	}
	if !bytes.Equal(request.Repeating1[0].Value, []byte{0x34, 0x12}) {
		t.Fatalf("first value = % X, want 34 12", request.Repeating1[0].Value)
	}
	if request.Repeating1[1].Parameter == nil || *request.Repeating1[1].Parameter != 1 {
		t.Fatalf("second parameter = %v, want 1", request.Repeating1[1].Parameter)
	}
	if !bytes.Equal(request.Repeating1[1].Value, []byte{0x08, 0x34}) {
		t.Fatalf("second value = % X, want 08 34", request.Repeating1[1].Value)
	}
}

func TestDecodeNmeaRequestGroupFunctionProprietaryTargetReturnsPartial(t *testing.T) {
	rawData := []byte{0x09, 0xaa, 0xbb}
	decoded := decodeGroupFunctionPayload(t, groupFunctionRequestPayload(0x1ef00, 1, rawData))

	partial, ok := decoded.(publicpgn.NMEARequestGroupFunctionPartial)
	if !ok {
		t.Fatalf("decoded type = %T, want %T", decoded, publicpgn.NMEARequestGroupFunctionPartial{})
	}
	if partial.PGN == nil || *partial.PGN != 0x1ef00 {
		t.Fatalf("decoded referenced PGN = %v, want %d", partial.PGN, uint32(0x1ef00))
	}
	if !bytes.Equal(partial.RawData, rawData) {
		t.Fatalf("decoded raw data = % X, want % X", partial.RawData, rawData)
	}
}

func TestDecodeNmeaRequestGroupFunctionUnknownTargetReturnsPartial(t *testing.T) {
	const unknownPGN = uint32(200000)
	rawData := []byte{0x01}
	decoded := decodeGroupFunctionPayload(t, groupFunctionRequestPayload(unknownPGN, 1, rawData))

	partial, ok := decoded.(publicpgn.NMEARequestGroupFunctionPartial)
	if !ok {
		t.Fatalf("decoded type = %T, want %T", decoded, publicpgn.NMEARequestGroupFunctionPartial{})
	}
	if partial.PGN == nil || *partial.PGN != unknownPGN {
		t.Fatalf("decoded referenced PGN = %v, want %d", partial.PGN, unknownPGN)
	}
	if !bytes.Equal(partial.RawData, rawData) {
		t.Fatalf("decoded raw data = % X, want % X", partial.RawData, rawData)
	}
}

func TestEncodeNmeaCommandGroupFunctionMatchesActisenseConfigurationWrite(t *testing.T) {
	targetPGN := uint32(publicpgn.ConfigurationInformationPGN)
	parameterCount := uint8(1)
	parameter := uint8(1)
	value, err := publicpgn.EncodeStringLAU("Port engine fuel test")
	if err != nil {
		t.Fatalf("EncodeStringLAU() error = %v", err)
	}
	command := &publicpgn.NMEACommandGroupFunction{
		Info: publicpgn.MessageInfo{
			PGN:      publicpgn.NMEACommandGroupFunctionPGN,
			Priority: 3,
			TargetId: 32,
		},
		FunctionCode:       publicpgn.Command,
		PGN:                &targetPGN,
		Priority:           publicpgn.LeaveUnchanged,
		NumberOfParameters: &parameterCount,
		Repeating1: []publicpgn.NMEACommandGroupFunctionRepeating1{
			{Parameter: &parameter, Value: value},
		},
	}

	stream := NewDataStream(make([]byte, MaxPGNLength))
	_, err = EncodeNMEACommandGroupFunction(command, stream)
	if err != nil {
		t.Fatalf("EncodeNMEACommandGroupFunction() error = %v", err)
	}
	assertBytes := []byte{
		0x01, 0x16, 0xf0, 0x01, 0xf8, 0x01, 0x01,
		0x17, 0x01, 'P', 'o', 'r', 't', ' ', 'e', 'n', 'g', 'i', 'n', 'e',
		' ', 'f', 'u', 'e', 'l', ' ', 't', 'e', 's', 't',
	}
	if !bytes.Equal(stream.GetData(), assertBytes) {
		t.Fatalf("encoded payload = % X, want % X", stream.GetData(), assertBytes)
	}
}

func TestDecodeNmeaAcknowledgeGroupFunctionMatchesActisenseConfigurationReply(t *testing.T) {
	decoded := decodeGroupFunctionPayload(t, []byte{
		0x02, 0x16, 0xf0, 0x01, 0x00, 0x01, 0xf0,
	})

	ack, ok := decoded.(publicpgn.NMEAAcknowledgeGroupFunction)
	if !ok {
		t.Fatalf("decoded type = %T, want %T", decoded, publicpgn.NMEAAcknowledgeGroupFunction{})
	}
	if ack.PGN == nil || *ack.PGN != publicpgn.ConfigurationInformationPGN {
		t.Fatalf("acknowledged PGN = %v, want %d", ack.PGN, publicpgn.ConfigurationInformationPGN)
	}
	if ack.PGNErrorCode != publicpgn.Acknowledge_6 {
		t.Fatalf("PGN error = %v, want acknowledge", ack.PGNErrorCode)
	}
	if ack.TransmissionIntervalPriorityErrorCode != publicpgn.Acknowledge_2 {
		t.Fatalf("transmission/priority error = %v, want acknowledge", ack.TransmissionIntervalPriorityErrorCode)
	}
	if len(ack.Repeating1) != 1 || ack.Repeating1[0].Parameter != publicpgn.Acknowledge_3 {
		t.Fatalf("parameter acknowledgements = %#v, want one acknowledgement", ack.Repeating1)
	}
}
