package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/boatkit-io/n2k/pkg/endpoint/n2kfileendpoint"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/boatkit-io/n2k/pkg/subscribe"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// decodeFuncs maps PGN struct names to their decode functions
var decodeFuncs map[string]interface{}

func init() {
	decodeFuncs = make(map[string]interface{})
	for _, pgnInfo := range pgn.PgnList {
		// Skip NMEA types as they're handled differently
		/* if strings.HasPrefix(pgnInfo.Id, "Nmea") {
			continue
		} */

		typeName := pgnInfo.Id
		decodeFuncs[typeName] = pgnInfo.Decoder
	}
}

// findDecodeFunc finds the appropriate Decode function for a given PGN struct
func findDecodeFunc(pgnStruct pgn.PgnStruct) (reflect.Value, error) {
	typeName := reflect.TypeOf(pgnStruct).String()
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		typeName = typeName[idx+1:]
	}

	fn, ok := decodeFuncs[typeName]
	if !ok {
		return reflect.Value{}, fmt.Errorf("decode function not found for type %s", typeName)
	}

	return reflect.ValueOf(fn), nil
}

func TestPGNSerializationFromN2K(t *testing.T) {
	// Get path to test data file
	testFile := filepath.Join("/home/russ/dev/n2k/n2kreplays/n2k", "longdump.n2k")

	// Setup the file endpoint
	ca := canadapter.NewCANAdapter(logrus.New())
	ep := n2kfileendpoint.NewN2kFileEndpoint(testFile, logrus.New())

	// Create subscriber
	subs := subscribe.New()
	//	pub := pgn.NewPublisher(ca)
	ps := pkt.NewPacketStruct()
	ps.SetOutput(subs)
	ca.SetOutput(ps)
	ep.SetOutput(ca)

	// Process each PGN
	_, err := subs.SubscribeToAllStructs(func(p any) {
		pgnStruct, ok := p.(pgn.PgnStruct)
		if !ok {
			return // Skip non-PGN structs
		}

		// Create a datastream for serialization
		stream := pgn.NewDataStream(make([]uint8, 254))

		// Encode the PGN
		info, err := pgnStruct.Encode(stream)
		assert.NoError(t, err)
		if err != nil {
			return
		}

		// Trim stream data to actual length and reset position
		inStream := pgn.NewDataStream(stream.GetData())

		// Find and call the appropriate decode function
		decodeFunc, err := findDecodeFunc(pgnStruct)
		assert.NoError(t, err)
		if err != nil {
			return
		}

		// Call the decode function with the MessageInfo and DataStream
		results := decodeFunc.Call([]reflect.Value{
			reflect.ValueOf(*info),
			reflect.ValueOf(inStream),
		})

		// Check for error from decode function
		if !results[1].IsNil() {
			assert.NoError(t, results[1].Interface().(error))
			fmt.Printf("While decoding %T, got error: %v\n", pgnStruct, results[1].Interface().(error))
			return
		}

		// Get the decoded PGN struct
		decoded := results[0].Interface()

		// Compare original and decoded PGNs
		opts := cmp.Options{
			cmpopts.EquateEmpty(),
			cmpopts.EquateApprox(0.001, 0.001),
		}

		diff := cmp.Diff(pgnStruct, decoded, opts)
		assert.Empty(t, diff, "PGN roundtrip failed for %T", pgnStruct)
	})
	assert.NoError(t, err)

	// Run the endpoint to process the file
	ctx := context.Background()
	err = ep.Run(ctx)
	assert.NoError(t, err)
}
