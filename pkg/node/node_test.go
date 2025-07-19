package node

import (
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/stretchr/testify/assert"
)

func uint32Ptr(v uint32) *uint32 {
	return &v
}

func uint8Ptr(v uint8) *uint8 {
	return &v
}

func TestNewNode(t *testing.T) {
	n := NewNode(nil, nil, nil).(*node)

	assert.Nil(t, n.subscriber)
	assert.Nil(t, n.publisher)
	assert.Equal(t, uint64(0), n.name)
	assert.Equal(t, uint8(255), n.networkAddress)
	assert.Equal(t, uint8(128), n.preferredAddress)
	assert.False(t, n.addressClaimed)
	assert.False(t, n.heartbeatEnabled)
	assert.Equal(t, 60*time.Second, n.heartbeatInterval)
	assert.False(t, n.started)
}

func TestSetters(t *testing.T) {
	n := NewNode(nil, nil, nil).(*node)

	// Test SetProductInfo
	productInfo := ProductInfo{ProductCode: 1234, ModelID: "Test"}
	n.SetProductInfo(productInfo)
	assert.Equal(t, productInfo, n.productInfo)

	// Test SetSupportedPGNs
	tx := []uint32{1, 2}
	rx := []uint32{3, 4}
	n.SetSupportedPGNs(tx, rx)
	assert.Equal(t, tx, n.transmitPGNs)
	assert.Equal(t, rx, n.receivePGNs)

	// Test SetHeartbeatInterval
	interval := 10 * time.Second
	n.SetHeartbeatInterval(interval)
	assert.Equal(t, interval, n.heartbeatInterval)

	// Test EnableHeartbeat
	n.EnableHeartbeat(true)
	assert.True(t, n.heartbeatEnabled)
}

func TestComputeName(t *testing.T) {
	validInfo := DeviceInfo{
		UniqueNumber:            1,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          130,
		DeviceClass:             60, // An example device class
		DeviceInstanceLower:     0,
		DeviceInstanceUpper:     0,
		SystemInstance:          0,
		IndustryGroup:           pgn.Marine,
		ArbitraryAddressCapable: true,
	}

	t.Run("ValidInfo", func(t *testing.T) {
		_, err := computeName(validInfo)
		assert.NoError(t, err)
	})

	// Test case 2: Invalid fields
	testCases := []struct {
		name          string
		mutator       func(d *DeviceInfo)
		expectedError string
	}{
		{"UniqueNumberTooLarge", func(d *DeviceInfo) { d.UniqueNumber = 0x200000 }, "exceeds 21-bit limit"},
		{"ManufacturerCodeTooLarge", func(d *DeviceInfo) { d.ManufacturerCode = 0x800 }, "exceeds 11-bit limit"},
		{"DeviceInstanceLowerTooLarge", func(d *DeviceInfo) { d.DeviceInstanceLower = 8 }, "exceeds 3-bit limit"},
		{"DeviceInstanceUpperTooLarge", func(d *DeviceInfo) { d.DeviceInstanceUpper = 32 }, "exceeds 5-bit limit"},
		{"DeviceClassTooLarge", func(d *DeviceInfo) { d.DeviceClass = 128 }, "exceeds 7-bit limit"},
		{"SystemInstanceTooLarge", func(d *DeviceInfo) { d.SystemInstance = 16 }, "exceeds 4-bit limit"},
		{"IndustryGroupTooLarge", func(d *DeviceInfo) { d.IndustryGroup = 8 }, "exceeds 3-bit limit"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			invalidInfo := validInfo // Start with a valid struct
			tc.mutator(&invalidInfo)
			_, err := computeName(invalidInfo)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
	}

	t.Run("SetDeviceInfo", func(t *testing.T) {
		n := NewNode(nil, nil, nil)
		err := n.SetDeviceInfo(validInfo)
		assert.NoError(t, err)
		err = n.SetDeviceInfo(DeviceInfo{UniqueNumber: 0x2FFFFF})
		assert.Error(t, err)
	})
}

func TestLifecycleAndResponses(t *testing.T) {
	// Setup
	sub := newMockSubscriber()
	pub := newMockPublisher()
	clock := newMockClock()
	n := NewNode(sub, pub, clock)
	_ = n.SetDeviceInfo(DeviceInfo{UniqueNumber: 1})

	err := n.Start()
	assert.NoError(t, err)
	assert.True(t, n.(*node).started)
	assert.Len(t, sub.subscriptions, 3, "should have 3 subscriptions after start")

	err = n.Stop()
	assert.NoError(t, err)
	assert.False(t, n.(*node).started)
	assert.Len(t, sub.subscriptions, 0, "should have 0 subscriptions after stop")

	// Restart the node for response tests
	err = n.Start()
	assert.NoError(t, err)

	// An address must be claimed before the node can write responses.
	pub.expectWrite() // 1. Expect the claim PGN to be written
	_ = n.ClaimAddress(50)
	pub.waitForWrite() // 2. Wait for the node to send the claim

	// 3. Now, advance the clock to allow the 250ms claim period to complete
	clock.Advance()
	// Add a small sleep to allow the process goroutine to run and update the state
	time.Sleep(10 * time.Millisecond)

	assert.True(t, n.IsAddressClaimed())
	pub.clear()

	t.Run("ProductInfoRequest", func(t *testing.T) {
		pub.clear()
		productInfo := ProductInfo{ProductCode: 1234, ModelID: "Test"}
		n.SetProductInfo(productInfo)

		requestPgn := &pgn.IsoRequest{
			Info: pgn.MessageInfo{SourceId: 10},
			Pgn:  uint32Ptr(126996),
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.ProductInformation)
		assert.True(t, ok)
		assert.Equal(t, uint16(1234), *response.ProductCode)
		assert.Equal(t, "Test", response.ModelId)
	})

	t.Run("PgnListRequest", func(t *testing.T) {
		pub.clear()
		n.SetSupportedPGNs([]uint32{1, 2}, []uint32{3, 4})

		requestPgn := &pgn.IsoRequest{
			Info: pgn.MessageInfo{SourceId: 20},
			Pgn:  uint32Ptr(126464),
		}
		pub.expectWrite()
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()
		pub.waitForWrite()

		assert.Len(t, pub.written, 2)
	})

	_ = n.Stop()
}
