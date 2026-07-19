package node

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uint32Ptr(v uint32) *uint32 {
	return &v
}

func pgnListValues(list []pgn.PGNListTransmitAndReceiveRepeating1) []uint32 {
	values := make([]uint32, 0, len(list))
	for _, item := range list {
		if item.PGN != nil {
			values = append(values, *item.PGN)
		}
	}
	return values
}

type mockConfigurationProvider struct {
	info ConfigurationInfo
	err  error
	set  []ConfigurationInfo
}

func (m *mockConfigurationProvider) GetConfigurationInfo() (ConfigurationInfo, error) {
	return m.info, m.err
}

func (m *mockConfigurationProvider) SetConfigurationInfo(info ConfigurationInfo) error {
	m.set = append(m.set, info)
	return m.err
}

func TestNewNode(t *testing.T) {
	n := NewNode(nil, nil, nil)

	assert.Nil(t, n.subscriber)
	assert.Nil(t, n.publisher)
	assert.Equal(t, uint64(0), n.name)
	assert.Equal(t, uint8(255), n.networkAddress)
	assert.Equal(t, uint8(128), n.preferredAddress)
	assert.False(t, n.addressClaimed)
	assert.True(t, n.readOnly)
	assert.False(t, n.heartbeatEnabled)
	assert.Equal(t, 60*time.Second, n.heartbeatInterval)
	assert.False(t, n.started)
}

func TestSetters(t *testing.T) {
	n := NewNode(nil, nil, nil)

	// Test SetProductInfo
	productInfo := ProductInfo{ProductCode: 1234, ModelID: "Test"}
	n.SetProductInfo(productInfo)
	assert.Equal(t, productInfo, n.productInfo)

	// Test SetConfigurationProvider
	configProvider := &mockConfigurationProvider{}
	n.SetConfigurationProvider(configProvider)
	assert.Equal(t, configProvider, n.configProvider)

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

func TestWriteFieldsUpdatesConfigurationInformation(t *testing.T) {
	provider := &mockConfigurationProvider{info: ConfigurationInfo{
		InstallationDescription1: "old helm",
		InstallationDescription2: "old port",
		ManufacturerInformation:  "boatkit",
	}}
	n := NewNode(nil, nil, nil)
	n.configProvider = provider
	n.networkAddress = 44
	n.addressClaimed = true
	n.readOnly = false
	parameter := uint8(1)
	parameterCount := uint8(1)
	selectionCount := uint8(0)
	targetPGN := uint32(pgn.ConfigurationInformationPGN)
	value := append([]byte{uint8(len("new helm") + 3), 1}, []byte("new helm")...)
	value = append(value, 0)

	responses := n.processNmeaWriteFieldsGroupFunction(&pgn.NMEAWriteFieldsGroupFunction{
		Info:                   pgn.MessageInfo{SourceId: 33, TargetId: 44},
		FunctionCode:           pgn.WriteFields,
		PGN:                    &targetPGN,
		NumberOfSelectionPairs: &selectionCount,
		NumberOfParameters:     &parameterCount,
		Repeating2: []pgn.NMEAWriteFieldsGroupFunctionRepeating2{{
			Parameter: &parameter,
			Value:     value,
		}},
	})

	assert.Len(t, provider.set, 1)
	assert.Equal(t, "new helm", provider.set[0].InstallationDescription1)
	assert.Equal(t, "old port", provider.set[0].InstallationDescription2)
	assert.Len(t, responses, 1)
	reply, ok := responses[0].pgn.(*pgn.NMEAWriteFieldsReplyGroupFunction)
	if !assert.True(t, ok) {
		return
	}
	assert.Equal(t, &parameterCount, reply.NumberOfParameters)
	if assert.Len(t, reply.Repeating2, 1) {
		assert.Equal(t, &parameter, reply.Repeating2[0].Parameter)
		assert.Equal(t, value, reply.Repeating2[0].Value)
	}
}

func TestWriteFieldsDecodesUTF16ConfigurationInformation(t *testing.T) {
	provider := &mockConfigurationProvider{info: ConfigurationInfo{InstallationDescription1: "old"}}
	n := NewNode(nil, nil, nil)
	n.configProvider = provider
	n.networkAddress = 44
	n.addressClaimed = true
	n.readOnly = false
	parameter, parameterCount, selectionCount := uint8(1), uint8(1), uint8(0)
	targetPGN := uint32(pgn.ConfigurationInformationPGN)
	value := []byte{6, 0, 0x4f, 0x60, 0x59, 0x7d}

	responses := n.processNmeaWriteFieldsGroupFunction(&pgn.NMEAWriteFieldsGroupFunction{
		Info:                   pgn.MessageInfo{SourceId: 33, TargetId: 44},
		FunctionCode:           pgn.WriteFields,
		PGN:                    &targetPGN,
		NumberOfSelectionPairs: &selectionCount,
		NumberOfParameters:     &parameterCount,
		Repeating2: []pgn.NMEAWriteFieldsGroupFunctionRepeating2{{
			Parameter: &parameter,
			Value:     value,
		}},
	})

	if assert.Len(t, provider.set, 1) {
		assert.Equal(t, "你好", provider.set[0].InstallationDescription1)
	}
	assert.Len(t, responses, 1)
}

func TestCommandUpdatesConfigurationInformationAndAcknowledgesEachParameter(t *testing.T) {
	provider := &mockConfigurationProvider{info: ConfigurationInfo{
		InstallationDescription1: "old helm",
		InstallationDescription2: "old port",
		ManufacturerInformation:  "boatkit",
	}}
	n := NewNode(nil, nil, nil)
	n.configProvider = provider
	n.networkAddress = 44
	n.addressClaimed = true
	n.readOnly = false
	targetPGN := uint32(pgn.ConfigurationInformationPGN)
	parameterCount := uint8(2)
	parameter1, parameter2 := uint8(1), uint8(2)
	value1, err := pgn.EncodeStringLAU("new helm")
	require.NoError(t, err)
	value2, err := pgn.EncodeStringLAU("new port")
	require.NoError(t, err)

	responses := n.processNmeaCommandGroupFunction(&pgn.NMEACommandGroupFunction{
		Info:               pgn.MessageInfo{SourceId: 33, TargetId: 44},
		FunctionCode:       pgn.Command,
		PGN:                &targetPGN,
		Priority:           pgn.LeaveUnchanged,
		NumberOfParameters: &parameterCount,
		Repeating1: []pgn.NMEACommandGroupFunctionRepeating1{
			{Parameter: &parameter1, Value: value1},
			{Parameter: &parameter2, Value: value2},
		},
	})

	if assert.Len(t, provider.set, 1) {
		assert.Equal(t, "new helm", provider.set[0].InstallationDescription1)
		assert.Equal(t, "new port", provider.set[0].InstallationDescription2)
	}
	if !assert.Len(t, responses, 1) {
		return
	}
	ack, ok := responses[0].pgn.(*pgn.NMEAAcknowledgeGroupFunction)
	if !assert.True(t, ok) {
		return
	}
	if assert.NotNil(t, ack.NumberOfParameters) {
		assert.Equal(t, parameterCount, *ack.NumberOfParameters)
	}
	if assert.Len(t, ack.Repeating1, int(parameterCount)) {
		assert.Equal(t, pgn.Acknowledge_3, ack.Repeating1[0].Parameter)
		assert.Equal(t, pgn.Acknowledge_3, ack.Repeating1[1].Parameter)
	}
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
		IndustryGroup:           pgn.MarineIndustry,
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
		{"UniqueNumberTooLarge", func(d *DeviceInfo) { d.UniqueNumber = 0x200000 }, "unique number 2097152 is too large"},
		{"ManufacturerCodeTooLarge", func(d *DeviceInfo) { d.ManufacturerCode = 0x800 }, "manufacturer code 2048 is too large"},
		{"DeviceInstanceLowerTooLarge", func(d *DeviceInfo) { d.DeviceInstanceLower = 8 }, "device instance lower 8 is too large"},
		{"DeviceInstanceUpperTooLarge", func(d *DeviceInfo) { d.DeviceInstanceUpper = 32 }, "device instance upper 32 is too large"},
		{"DeviceClassTooLarge", func(d *DeviceInfo) { d.DeviceClass = 128 }, "device class 128 is too large"},
		{"SystemInstanceTooLarge", func(d *DeviceInfo) { d.SystemInstance = 16 }, "system instance 16 is too large"},
		{"IndustryGroupTooLarge", func(d *DeviceInfo) { d.IndustryGroup = 8 }, "industry group 8 is too large"},
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

func TestClaimAddressRequiresDeviceInfo(t *testing.T) {
	n := NewNode(nil, nil, nil)

	err := n.ClaimAddress(50)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "device info has not been set")
}

func TestClaimAddressReadOnly(t *testing.T) {
	sub := newMockSubscriber()
	pub := newMockPublisher()
	n := NewNode(sub, pub, newMockClock())

	err := n.ClaimAddress(ReadOnlyAddress)
	assert.NoError(t, err)
	assert.Equal(t, ReadOnlyAddress, n.GetNetworkAddress())
	assert.False(t, n.IsAddressClaimed())
	assert.True(t, n.readOnly)

	err = n.Start()
	assert.NoError(t, err)
	defer func() { _ = n.Stop() }()

	time.Sleep(10 * time.Millisecond)
	assert.Empty(t, pub.written)

	err = n.Write(&pgn.ISORequest{PGN: uint32Ptr(pgn.ISOAddressClaimPGN)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node is read-only")
	assert.Empty(t, pub.written)

	responses := n.processIsoRequest(pgn.ISORequest{
		Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
		PGN:  uint32Ptr(pgn.ProductInformationPGN),
	})
	assert.Empty(t, responses)

	sub.simulatePGN(testKnownDeviceClaim(23))
	sub.waitForHandler()
	assert.Eventually(t, func() bool {
		devices := n.KnownDevices()
		return len(devices) == 1 && devices[0].Address == 23
	}, time.Second, time.Millisecond)
	assert.Empty(t, pub.written)
}

func TestDefaultReadOnlyIgnoresCommandedAddress(t *testing.T) {
	sub := newMockSubscriber()
	pub := newMockPublisher()
	n := NewNode(sub, pub, newMockClock())
	err := n.SetDeviceInfo(DeviceInfo{UniqueNumber: 1})
	assert.NoError(t, err)

	err = n.Start()
	assert.NoError(t, err)
	defer func() { _ = n.Stop() }()

	cmd := pgn.ISOCommandedAddress{
		Info:             pgn.MessageInfo{SourceId: 10, TargetId: 255},
		UniqueNumber:     []uint8{1, 0, 0},
		NewSourceAddress: ptrUint8(44),
	}
	sub.simulatePGN(cmd)
	sub.waitForHandler()

	time.Sleep(10 * time.Millisecond)
	assert.False(t, n.IsAddressClaimed())
	assert.Equal(t, ReadOnlyAddress, n.GetNetworkAddress())
	assert.Empty(t, pub.written)
}

func TestClaimAddressZeroRemainsClaimable(t *testing.T) {
	sub := newMockSubscriber()
	pub := newMockPublisher()
	clock := newMockClock()
	n := NewNode(sub, pub, clock)
	_ = n.SetDeviceInfo(DeviceInfo{UniqueNumber: 1})

	err := n.Start()
	assert.NoError(t, err)
	defer func() { _ = n.Stop() }()

	pub.expectWrite()
	err = n.ClaimAddress(0)
	assert.NoError(t, err)
	pub.waitForWrite()

	clock.Advance()
	assert.Eventually(t, n.IsAddressClaimed, time.Second, time.Millisecond)
	assert.Equal(t, uint8(0), n.GetNetworkAddress())
	assert.False(t, n.readOnly)
}

func TestEnableHeartbeatAfterClaimWakesProcess(t *testing.T) {
	sub := newMockSubscriber()
	pub := newMockPublisher()
	clock := newMockClock()
	n := NewNode(sub, pub, clock)
	_ = n.SetDeviceInfo(DeviceInfo{UniqueNumber: 1})

	err := n.Start()
	assert.NoError(t, err)
	defer func() { _ = n.Stop() }()

	pub.expectWrite()
	err = n.ClaimAddress(50)
	assert.NoError(t, err)
	pub.waitForWrite()

	clock.Advance()
	assert.Eventually(t, n.IsAddressClaimed, time.Second, time.Millisecond)
	pub.clear()

	pub.expectWrite()
	n.EnableHeartbeat(true)
	pub.waitForWrite()

	_, ok := pub.lastWritten().(*pgn.Heartbeat)
	assert.True(t, ok)
}

func TestManagedTransmitPGNsIncludesConditionalNodePGNs(t *testing.T) {
	result := managedTransmitPGNs([]uint32{pgn.ISOAddressClaimPGN, 1}, true, true)

	assert.ElementsMatch(t,
		[]uint32{
			1,
			pgn.ISOAcknowledgementPGN,
			pgn.ISOAddressClaimPGN,
			pgn.NMEAAcknowledgeGroupFunctionPGN,
			pgn.PGNListTransmitAndReceivePGN,
			pgn.ProductInformationPGN,
			pgn.ConfigurationInformationPGN,
			pgn.HeartbeatPGN,
		},
		result,
	)
}

func TestComputeNameFromClaimIncludesArbitraryAddressBit(t *testing.T) {
	uniqueNumber := uint32(1)
	deviceInstanceLower := uint8(2)
	deviceInstanceUpper := uint8(3)
	systemInstance := uint8(4)

	info := DeviceInfo{
		UniqueNumber:            uniqueNumber,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          130,
		DeviceClass:             60,
		DeviceInstanceLower:     deviceInstanceLower,
		DeviceInstanceUpper:     deviceInstanceUpper,
		SystemInstance:          systemInstance,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: true,
	}
	expectedName, err := computeName(info)
	assert.NoError(t, err)

	claimName := computeNameFromClaim(&pgn.ISOAddressClaim{
		UniqueNumber:            &uniqueNumber,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          130,
		DeviceClass:             60,
		DeviceInstanceLower:     &deviceInstanceLower,
		DeviceInstanceUpper:     &deviceInstanceUpper,
		SystemInstance:          &systemInstance,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: pgn.Yes_2,
	})
	assert.Equal(t, expectedName, claimName)
	assert.NotZero(t, claimName&(1<<63))
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
	assert.True(t, n.started)
	assert.Len(t, sub.subscriptions, 11, "should have 11 subscriptions after start")

	err = n.Stop()
	assert.NoError(t, err)
	assert.False(t, n.started)
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

		requestPgn := pgn.ISORequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
			PGN:  uint32Ptr(126996),
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.ProductInformation)
		assert.True(t, ok)
		assert.Equal(t, uint16(1234), *response.ProductCode)
		assert.Equal(t, "Test", response.ModelID)
	})

	t.Run("AddressClaimRequest", func(t *testing.T) {
		pub.clear()

		requestPgn := pgn.ISORequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
			PGN:  uint32Ptr(pgn.ISOAddressClaimPGN),
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.ISOAddressClaim)
		assert.True(t, ok)
		assert.Equal(t, uint8(50), response.Info.SourceId)
		assert.Equal(t, uint32(1), *response.UniqueNumber)
	})

	t.Run("PgnListRequest", func(t *testing.T) {
		pub.clear()
		n.SetConfigurationProvider(&mockConfigurationProvider{})
		n.SetSupportedPGNs([]uint32{1, 2}, []uint32{3, 4})

		requestPgn := &pgn.ISORequest{
			Info: pgn.MessageInfo{SourceId: 20, TargetId: 255},
			PGN:  uint32Ptr(126464),
		}
		pub.expectWrite()
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()
		pub.waitForWrite()

		assert.Len(t, pub.written, 2)
		txResponse, ok := pub.written[0].(*pgn.PGNListTransmitAndReceive)
		assert.True(t, ok)
		assert.Equal(t, pgn.TransmitPGNList, txResponse.FunctionCode)
		assert.ElementsMatch(t,
			[]uint32{
				1,
				2,
				pgn.ISOAcknowledgementPGN,
				pgn.ISOAddressClaimPGN,
				pgn.NMEAAcknowledgeGroupFunctionPGN,
				pgn.PGNListTransmitAndReceivePGN,
				pgn.ProductInformationPGN,
				pgn.ConfigurationInformationPGN,
			},
			pgnListValues(txResponse.Repeating1),
		)
		rxResponse, ok := pub.written[1].(*pgn.PGNListTransmitAndReceive)
		assert.True(t, ok)
		assert.Equal(t, pgn.ReceivePGNList, rxResponse.FunctionCode)
		assert.ElementsMatch(t,
			[]uint32{
				3,
				4,
				pgn.ISOAcknowledgementPGN,
				pgn.ISORequestPGN,
				pgn.ISOAddressClaimPGN,
				pgn.ISOCommandedAddressPGN,
				pgn.NMEARequestGroupFunctionPGN,
			},
			pgnListValues(rxResponse.Repeating1),
		)
	})

	t.Run("ConfigurationInfoRequest", func(t *testing.T) {
		pub.clear()
		n.SetConfigurationProvider(&mockConfigurationProvider{
			info: ConfigurationInfo{
				InstallationDescription1: "helm",
				InstallationDescription2: "port",
				ManufacturerInformation:  "boatkit",
			},
		})

		requestPgn := pgn.ISORequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
			PGN:  uint32Ptr(pgn.ConfigurationInformationPGN),
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.ConfigurationInformation)
		assert.True(t, ok)
		assert.Equal(t, "helm", response.InstallationDescription1)
		assert.Equal(t, "port", response.InstallationDescription2)
		assert.Equal(t, "boatkit", response.ManufacturerInformation)
	})

	t.Run("ConfigurationInfoRequestWithoutProviderNaks", func(t *testing.T) {
		pub.clear()
		n.SetConfigurationProvider(nil)

		requestPgn := pgn.ISORequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
			PGN:  uint32Ptr(pgn.ConfigurationInformationPGN),
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.ISOAcknowledgement)
		assert.True(t, ok)
		assert.Equal(t, pgn.Nak, response.Control)
		assert.Equal(t, uint32(pgn.ConfigurationInformationPGN), *response.PGN)
	})

	t.Run("NmeaRequestGroupFunctionRoutesToIsoRequestHandling", func(t *testing.T) {
		pub.clear()
		productInfo := ProductInfo{ProductCode: 5678, ModelID: "Group"}
		n.SetProductInfo(productInfo)
		zeroParameters := uint8(0)

		requestPgn := pgn.NMEARequestGroupFunction{
			Info:               pgn.MessageInfo{SourceId: 10, TargetId: 255},
			FunctionCode:       pgn.Request,
			PGN:                uint32Ptr(pgn.ProductInformationPGN),
			NumberOfParameters: &zeroParameters,
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.ProductInformation)
		assert.True(t, ok)
		assert.Equal(t, uint16(5678), *response.ProductCode)
		assert.Equal(t, "Group", response.ModelID)
	})

	t.Run("UnsupportedDirectedNmeaCommandGroupFunctionNaks", func(t *testing.T) {
		pub.clear()
		zeroParameters := uint8(0)

		commandPgn := pgn.NMEACommandGroupFunction{
			Info:               pgn.MessageInfo{SourceId: 10, TargetId: 50},
			FunctionCode:       pgn.Command,
			PGN:                uint32Ptr(pgn.ConfigurationInformationPGN),
			NumberOfParameters: &zeroParameters,
		}
		pub.expectWrite()
		sub.simulatePGN(commandPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.NMEAAcknowledgeGroupFunction)
		assert.True(t, ok)
		assert.Equal(t, pgn.Acknowledge_5, response.FunctionCode)
		assert.Equal(t, pgn.PGNNotSupported, response.PGNErrorCode)
		assert.Equal(t, pgn.NotSupported, response.TransmissionIntervalPriorityErrorCode)
		assert.Equal(t, uint32(pgn.ConfigurationInformationPGN), *response.PGN)
		assert.Len(t, response.Repeating1, 1)
		assert.Equal(t, pgn.ReadOrWriteNotSupported, response.Repeating1[0].Parameter)
	})

	t.Run("UnsupportedGlobalNmeaCommandGroupFunctionIgnored", func(t *testing.T) {
		commandPgn := pgn.NMEACommandGroupFunction{
			Info:         pgn.MessageInfo{SourceId: 10, TargetId: 255},
			FunctionCode: pgn.Command,
			PGN:          uint32Ptr(pgn.ConfigurationInformationPGN),
		}
		responses := n.processNmeaCommandGroupFunction(&commandPgn)
		assert.Empty(t, responses)
	})

	t.Run("IgnoresRequestDirectedToAnotherNode", func(t *testing.T) {
		requestPgn := pgn.ISORequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 51},
			PGN:  uint32Ptr(126996),
		}
		responses := n.processIsoRequest(requestPgn)
		assert.Empty(t, responses)
	})

	_ = n.Stop()
}

func TestKnownDevices(t *testing.T) {
	sub := newMockSubscriber()
	pub := newMockPublisher()
	n := NewNode(sub, pub, newMockClock())

	err := n.Start()
	assert.NoError(t, err)
	defer func() { _ = n.Stop() }()

	uniqueNumber := uint32(42)
	deviceInstanceLower := uint8(1)
	deviceInstanceUpper := uint8(2)
	systemInstance := uint8(3)
	claim := pgn.ISOAddressClaim{
		Info:                    pgn.MessageInfo{SourceId: 23},
		UniqueNumber:            &uniqueNumber,
		ManufacturerCode:        pgn.Garmin,
		DeviceInstanceLower:     &deviceInstanceLower,
		DeviceInstanceUpper:     &deviceInstanceUpper,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		SystemInstance:          &systemInstance,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: pgn.Yes_2,
	}
	sub.simulatePGN(claim)
	sub.waitForHandler()

	assert.Eventually(t, func() bool {
		return len(n.KnownDevices()) == 1
	}, time.Second, time.Millisecond)

	devices := n.KnownDevices()
	assert.Equal(t, uint8(23), devices[0].Address)
	assert.NotZero(t, devices[0].Name)
	assert.False(t, devices[0].LastSeen.IsZero())

	version := float32(2.1)
	productCode := uint16(101)
	loadEquivalency := uint8(1)
	sub.simulatePGN(pgn.ProductInformation{
		Info:                pgn.MessageInfo{SourceId: 23},
		NMEA2000Version:     &version,
		ProductCode:         &productCode,
		ModelID:             "Remote",
		SoftwareVersionCode: "1.2.3",
		ModelVersion:        "v1",
		ModelSerialCode:     "SN-remote",
		CertificationLevel:  pgn.LevelB,
		LoadEquivalency:     &loadEquivalency,
	})
	sub.simulatePGN(pgn.ConfigurationInformation{
		Info:                     pgn.MessageInfo{SourceId: 23},
		InstallationDescription1: "helm",
		InstallationDescription2: "bench",
		ManufacturerInformation:  "remote maker",
	})
	sub.simulatePGN(pgn.PGNListTransmitAndReceive{
		Info:         pgn.MessageInfo{SourceId: 23},
		FunctionCode: pgn.TransmitPGNList,
		Repeating1: []pgn.PGNListTransmitAndReceiveRepeating1{
			{PGN: uint32Ptr(pgn.ISOAddressClaimPGN)},
			{PGN: uint32Ptr(pgn.ProductInformationPGN)},
		},
	})
	sub.simulatePGN(pgn.PGNListTransmitAndReceive{
		Info:         pgn.MessageInfo{SourceId: 23},
		FunctionCode: pgn.ReceivePGNList,
		Repeating1: []pgn.PGNListTransmitAndReceiveRepeating1{
			{PGN: uint32Ptr(pgn.ISORequestPGN)},
		},
	})
	sub.waitForHandler()

	assert.Eventually(t, func() bool {
		devices = n.KnownDevices()
		return len(devices) == 1 &&
			devices[0].ProductInfo != nil &&
			devices[0].ConfigInfo != nil &&
			len(devices[0].TransmitPGNs) == 2 &&
			len(devices[0].ReceivePGNs) == 1
	}, time.Second, time.Millisecond)

	devices = n.KnownDevices()
	assert.Equal(t, "Remote", devices[0].ProductInfo.ModelID)
	assert.Equal(t, "1.2.3", devices[0].ProductInfo.SoftwareVersionCode)
	assert.Equal(t, "helm", devices[0].ConfigInfo.InstallationDescription1)
	assert.Equal(t, []uint32{pgn.ISOAddressClaimPGN, pgn.ProductInformationPGN}, devices[0].TransmitPGNs)
	assert.Equal(t, []uint32{pgn.ISORequestPGN}, devices[0].ReceivePGNs)

	devices[0].ProductInfo.ModelID = "mutated"
	devices[0].TransmitPGNs[0] = 0
	assert.Equal(t, "Remote", n.KnownDevices()[0].ProductInfo.ModelID)
	assert.Equal(t, uint32(pgn.ISOAddressClaimPGN), n.KnownDevices()[0].TransmitPGNs[0])
}

func TestKnownDevicesTracksNameAcrossAddressChanges(t *testing.T) {
	sub := newMockSubscriber()
	pub := newMockPublisher()
	n := NewNode(sub, pub, newMockClock())

	var changesMu sync.Mutex
	var changes []DeviceChange
	subID := n.SubscribeToDeviceChanges(func(change DeviceChange) {
		changesMu.Lock()
		defer changesMu.Unlock()
		changes = append(changes, change)
	})

	err := n.Start()
	assert.NoError(t, err)
	defer func() { _ = n.Stop() }()

	claim := testKnownDeviceClaim(23)
	sub.simulatePGN(claim)
	sub.waitForHandler()

	assert.Eventually(t, func() bool {
		devices := n.KnownDevices()
		return len(devices) == 1 && devices[0].Address == 23
	}, time.Second, time.Millisecond)

	claim.Info.SourceId = 24
	sub.simulatePGN(claim)
	sub.waitForHandler()

	assert.Eventually(t, func() bool {
		devices := n.KnownDevices()
		return len(devices) == 1 && devices[0].Address == 24
	}, time.Second, time.Millisecond)

	assert.Eventually(t, func() bool {
		changesMu.Lock()
		defer changesMu.Unlock()
		if len(changes) < 2 {
			return false
		}
		last := changes[len(changes)-1]
		return last.Kind == DeviceChangeAddressChanged &&
			last.Device.Address == 24 &&
			last.OldAddress != nil &&
			*last.OldAddress == 23
	}, time.Second, time.Millisecond)

	err = n.UnsubscribeDeviceChanges(subID)
	assert.NoError(t, err)
	sub.simulatePGN(pgn.ProductInformation{
		Info:    pgn.MessageInfo{SourceId: 24},
		ModelID: "No event",
	})
	sub.waitForHandler()
	changesMu.Lock()
	changeCount := len(changes)
	changesMu.Unlock()
	assert.Never(t, func() bool {
		changesMu.Lock()
		defer changesMu.Unlock()
		return len(changes) != changeCount
	}, 50*time.Millisecond, time.Millisecond)
}

func TestKnownDevicesMergesPreClaimMetadata(t *testing.T) {
	sub := newMockSubscriber()
	pub := newMockPublisher()
	n := NewNode(sub, pub, newMockClock())

	err := n.Start()
	assert.NoError(t, err)
	defer func() { _ = n.Stop() }()

	sub.simulatePGN(pgn.ProductInformation{
		Info:    pgn.MessageInfo{SourceId: 23},
		ModelID: "Before Claim",
	})
	sub.waitForHandler()

	assert.Eventually(t, func() bool {
		devices := n.KnownDevices()
		return len(devices) == 1 && devices[0].Name == 0 && devices[0].ProductInfo != nil
	}, time.Second, time.Millisecond)

	sub.simulatePGN(testKnownDeviceClaim(23))
	sub.waitForHandler()

	assert.Eventually(t, func() bool {
		devices := n.KnownDevices()
		return len(devices) == 1 &&
			devices[0].Name != 0 &&
			devices[0].ProductInfo != nil &&
			devices[0].ProductInfo.ModelID == "Before Claim"
	}, time.Second, time.Millisecond)
}

func TestOtherAddressClaimDoesNotLogInfoNoise(t *testing.T) {
	var logOutput bytes.Buffer
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetOutput(&logOutput)

	n := NewNode(newMockSubscriber(), newMockPublisher(), newMockClock())
	n.SetLogger(logger)
	err := n.SetDeviceInfo(DeviceInfo{
		UniqueNumber:            100,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: true,
	})
	assert.NoError(t, err)

	n.addressState = stateClaimed
	n.addressClaimed = true
	n.networkAddress = 110

	claim := testKnownDeviceClaim(229)
	n.processIsoAddressClaim(&claim)

	assert.Empty(t, logOutput.String())
}

func testKnownDeviceClaim(sourceID uint8) pgn.ISOAddressClaim {
	uniqueNumber := uint32(42)
	deviceInstanceLower := uint8(1)
	deviceInstanceUpper := uint8(2)
	systemInstance := uint8(3)
	return pgn.ISOAddressClaim{
		Info:                    pgn.MessageInfo{SourceId: sourceID},
		UniqueNumber:            &uniqueNumber,
		ManufacturerCode:        pgn.Garmin,
		DeviceInstanceLower:     &deviceInstanceLower,
		DeviceInstanceUpper:     &deviceInstanceUpper,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		SystemInstance:          &systemInstance,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: pgn.Yes_2,
	}
}

func TestAddressClaimRetriesNextKnownFreeAddress(t *testing.T) {
	sub := newMockSubscriber()
	pub := newMockPublisher()
	clock := newMockClock()
	n := NewNode(sub, pub, clock)
	err := n.SetDeviceInfo(DeviceInfo{
		UniqueNumber:            100,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: true,
	})
	assert.NoError(t, err)
	err = n.Start()
	assert.NoError(t, err)
	defer func() { _ = n.Stop() }()

	pub.expectWrite()
	err = n.ClaimAddress(50)
	assert.NoError(t, err)
	pub.waitForWrite()

	winningUniqueNumber := uint32(1)
	pub.expectWrite()
	sub.simulatePGN(pgn.ISOAddressClaim{
		Info:                    pgn.MessageInfo{SourceId: 50},
		UniqueNumber:            &winningUniqueNumber,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: pgn.Yes_2,
	})
	pub.waitForWrite()

	claim, ok := pub.lastWritten().(*pgn.ISOAddressClaim)
	assert.True(t, ok)
	assert.Equal(t, uint8(51), claim.Info.SourceId)
	assert.Equal(t, uint8(51), n.GetNetworkAddress())
	assert.False(t, n.IsAddressClaimed())

	clock.Advance()
	assert.Eventually(t, n.IsAddressClaimed, time.Second, time.Millisecond)
	assert.Equal(t, uint8(51), n.GetNetworkAddress())
}
