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

func pgnListValues(list []pgn.PgnListTransmitAndReceiveRepeating1) []uint32 {
	values := make([]uint32, 0, len(list))
	for _, item := range list {
		if item.Pgn != nil {
			values = append(values, *item.Pgn)
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

func TestManagedTransmitPGNsIncludesConditionalNodePGNs(t *testing.T) {
	result := managedTransmitPGNs([]uint32{pgn.IsoAddressClaimPgn, 1}, true, true)

	assert.ElementsMatch(t,
		[]uint32{
			1,
			pgn.IsoAcknowledgementPgn,
			pgn.IsoAddressClaimPgn,
			pgn.NmeaAcknowledgeGroupFunctionPgn,
			pgn.PgnListTransmitAndReceivePgn,
			pgn.ProductInformationPgn,
			pgn.ConfigurationInformationPgn,
			pgn.HeartbeatPgn,
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

	claimName, err := computeNameFromClaim(&pgn.IsoAddressClaim{
		UniqueNumber:            &uniqueNumber,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          130,
		DeviceClass:             60,
		DeviceInstanceLower:     &deviceInstanceLower,
		DeviceInstanceUpper:     &deviceInstanceUpper,
		SystemInstance:          &systemInstance,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: pgn.Yes,
	})
	assert.NoError(t, err)
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
	assert.True(t, n.(*node).started)
	assert.Len(t, sub.subscriptions, 8, "should have 8 subscriptions after start")

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

		requestPgn := pgn.IsoRequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
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

	t.Run("AddressClaimRequest", func(t *testing.T) {
		pub.clear()

		requestPgn := pgn.IsoRequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
			Pgn:  uint32Ptr(pgn.IsoAddressClaimPgn),
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.IsoAddressClaim)
		assert.True(t, ok)
		assert.Equal(t, uint8(50), response.Info.SourceId)
		assert.Equal(t, uint32(1), *response.UniqueNumber)
	})

	t.Run("PgnListRequest", func(t *testing.T) {
		pub.clear()
		n.SetConfigurationProvider(&mockConfigurationProvider{})
		n.SetSupportedPGNs([]uint32{1, 2}, []uint32{3, 4})

		requestPgn := &pgn.IsoRequest{
			Info: pgn.MessageInfo{SourceId: 20, TargetId: 255},
			Pgn:  uint32Ptr(126464),
		}
		pub.expectWrite()
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()
		pub.waitForWrite()

		assert.Len(t, pub.written, 2)
		txResponse, ok := pub.written[0].(*pgn.PgnListTransmitAndReceive)
		assert.True(t, ok)
		assert.Equal(t, pgn.TransmitPgnList, txResponse.FunctionCode)
		assert.ElementsMatch(t,
			[]uint32{
				1,
				2,
				pgn.IsoAcknowledgementPgn,
				pgn.IsoAddressClaimPgn,
				pgn.NmeaAcknowledgeGroupFunctionPgn,
				pgn.PgnListTransmitAndReceivePgn,
				pgn.ProductInformationPgn,
				pgn.ConfigurationInformationPgn,
			},
			pgnListValues(txResponse.Repeating1),
		)
		rxResponse, ok := pub.written[1].(*pgn.PgnListTransmitAndReceive)
		assert.True(t, ok)
		assert.Equal(t, pgn.ReceivePgnList, rxResponse.FunctionCode)
		assert.ElementsMatch(t,
			[]uint32{
				3,
				4,
				pgn.IsoAcknowledgementPgn,
				pgn.IsoRequestPgn,
				pgn.IsoAddressClaimPgn,
				pgn.IsoCommandedAddressPgn,
				pgn.NmeaRequestGroupFunctionPgn,
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

		requestPgn := pgn.IsoRequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
			Pgn:  uint32Ptr(pgn.ConfigurationInformationPgn),
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

		requestPgn := pgn.IsoRequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 255},
			Pgn:  uint32Ptr(pgn.ConfigurationInformationPgn),
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.IsoAcknowledgement)
		assert.True(t, ok)
		assert.Equal(t, pgn.Nak, response.Control)
		assert.Equal(t, uint32(pgn.ConfigurationInformationPgn), *response.Pgn)
	})

	t.Run("NmeaRequestGroupFunctionRoutesToIsoRequestHandling", func(t *testing.T) {
		pub.clear()
		productInfo := ProductInfo{ProductCode: 5678, ModelID: "Group"}
		n.SetProductInfo(productInfo)
		zeroParameters := uint8(0)

		requestPgn := pgn.NmeaRequestGroupFunction{
			Info:               pgn.MessageInfo{SourceId: 10, TargetId: 255},
			FunctionCode:       pgn.Request,
			Pgn:                uint32Ptr(pgn.ProductInformationPgn),
			NumberOfParameters: &zeroParameters,
		}
		pub.expectWrite()
		sub.simulatePGN(requestPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.ProductInformation)
		assert.True(t, ok)
		assert.Equal(t, uint16(5678), *response.ProductCode)
		assert.Equal(t, "Group", response.ModelId)
	})

	t.Run("UnsupportedDirectedNmeaCommandGroupFunctionNaks", func(t *testing.T) {
		pub.clear()
		zeroParameters := uint8(0)

		commandPgn := pgn.NmeaCommandGroupFunction{
			Info:               pgn.MessageInfo{SourceId: 10, TargetId: 50},
			FunctionCode:       pgn.Command,
			Pgn:                uint32Ptr(pgn.ConfigurationInformationPgn),
			NumberOfParameters: &zeroParameters,
		}
		pub.expectWrite()
		sub.simulatePGN(commandPgn)
		pub.waitForWrite()

		assert.Len(t, pub.written, 1)
		response, ok := pub.lastWritten().(*pgn.NmeaAcknowledgeGroupFunction)
		assert.True(t, ok)
		assert.Equal(t, pgn.Acknowledge_4, response.FunctionCode)
		assert.Equal(t, pgn.PgnNotSupported, response.PgnErrorCode)
		assert.Equal(t, pgn.NotSupported, response.TransmissionIntervalPriorityErrorCode)
		assert.Equal(t, uint32(pgn.ConfigurationInformationPgn), *response.Pgn)
		assert.Len(t, response.Repeating1, 1)
		assert.Equal(t, pgn.ReadOrWriteNotSupported, response.Repeating1[0].Parameter)
	})

	t.Run("UnsupportedGlobalNmeaCommandGroupFunctionIgnored", func(t *testing.T) {
		commandPgn := pgn.NmeaCommandGroupFunction{
			Info:         pgn.MessageInfo{SourceId: 10, TargetId: 255},
			FunctionCode: pgn.Command,
			Pgn:          uint32Ptr(pgn.ConfigurationInformationPgn),
		}
		responses := n.(*node).processNmeaCommandGroupFunction(commandPgn)
		assert.Empty(t, responses)
	})

	t.Run("IgnoresRequestDirectedToAnotherNode", func(t *testing.T) {
		requestPgn := pgn.IsoRequest{
			Info: pgn.MessageInfo{SourceId: 10, TargetId: 51},
			Pgn:  uint32Ptr(126996),
		}
		responses := n.(*node).processIsoRequest(requestPgn)
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
	claim := pgn.IsoAddressClaim{
		Info:                    pgn.MessageInfo{SourceId: 23},
		UniqueNumber:            &uniqueNumber,
		ManufacturerCode:        pgn.Garmin,
		DeviceInstanceLower:     &deviceInstanceLower,
		DeviceInstanceUpper:     &deviceInstanceUpper,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		SystemInstance:          &systemInstance,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: pgn.Yes,
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
		Nmea2000Version:     &version,
		ProductCode:         &productCode,
		ModelId:             "Remote",
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
	sub.waitForHandler()

	assert.Eventually(t, func() bool {
		devices = n.KnownDevices()
		return len(devices) == 1 && devices[0].ProductInfo != nil && devices[0].ConfigInfo != nil
	}, time.Second, time.Millisecond)

	devices = n.KnownDevices()
	assert.Equal(t, "Remote", devices[0].ProductInfo.ModelID)
	assert.Equal(t, "1.2.3", devices[0].ProductInfo.SoftwareVersionCode)
	assert.Equal(t, "helm", devices[0].ConfigInfo.InstallationDescription1)

	devices[0].ProductInfo.ModelID = "mutated"
	assert.Equal(t, "Remote", n.KnownDevices()[0].ProductInfo.ModelID)
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
	sub.simulatePGN(pgn.IsoAddressClaim{
		Info:                    pgn.MessageInfo{SourceId: 50},
		UniqueNumber:            &winningUniqueNumber,
		ManufacturerCode:        pgn.Garmin,
		DeviceFunction:          140,
		DeviceClass:             pgn.Navigation,
		IndustryGroup:           pgn.MarineIndustry,
		ArbitraryAddressCapable: pgn.Yes,
	})
	pub.waitForWrite()

	claim, ok := pub.lastWritten().(*pgn.IsoAddressClaim)
	assert.True(t, ok)
	assert.Equal(t, uint8(51), claim.Info.SourceId)
	assert.Equal(t, uint8(51), n.GetNetworkAddress())
	assert.False(t, n.IsAddressClaimed())

	clock.Advance()
	assert.Eventually(t, n.IsAddressClaimed, time.Second, time.Millisecond)
	assert.Equal(t, uint8(51), n.GetNetworkAddress())
}
