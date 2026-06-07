// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package node provides standard NMEA 2000 node behavior.
package node

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

// Node represents a generic NMEA 2000 device, handling standard behaviors
// required for any device on the network.
type Node interface {
	Start() error
	Stop() error
	ClaimAddress(preferredAddress uint8) error
	GetNetworkAddress() uint8
	IsAddressClaimed() bool
	KnownDevices() []KnownDevice
	SubscribeToDeviceChanges(callback func(DeviceChange)) SubscriptionID
	UnsubscribeDeviceChanges(subID SubscriptionID) error
	Write(pgnStruct any) error
	WriteTo(pgnStruct any, destination uint8) error
	SetDeviceInfo(info DeviceInfo) error
	SetProductInfo(info ProductInfo) //nolint:gocritic // API accepts value configuration.
	SetConfigurationProvider(provider ConfigurationProvider)
	SetSupportedPGNs(transmit, receive []uint32)
	SetHeartbeatInterval(interval time.Duration)
	EnableHeartbeat(enable bool)
}

// Subscriber is an interface that abstracts bus subscriptions for testing.
type Subscriber interface {
	SubscribeToStruct(t, callback any) (SubscriptionID, error)
	Unsubscribe(subID SubscriptionID) error
}

// Publisher is an interface that abstracts the pgn.Publisher for testing.
type Publisher interface {
	Write(pgnStruct any) error
}

// DeviceInfo contains the fields required to compute the NMEA 2000 NAME,
// which uniquely identifies a device on the network.
type DeviceInfo struct {
	UniqueNumber            uint32
	ManufacturerCode        pgn.ManufacturerCodeConst
	DeviceFunction          pgn.DeviceFunctionConst
	DeviceClass             pgn.DeviceClassConst
	DeviceInstanceLower     uint8
	DeviceInstanceUpper     uint8
	SystemInstance          uint8
	IndustryGroup           pgn.IndustryCodeConst
	ArbitraryAddressCapable bool
}

// ProductInfo contains the product details for a device.
type ProductInfo struct {
	NMEA2000Version     uint16
	ProductCode         uint16
	ModelID             string
	SoftwareVersionCode string
	ModelVersion        string
	ModelSerialCode     string
	CertificationLevel  uint8
	LoadEquivalency     uint8
}

// ConfigurationInfo contains device-specific configuration metadata.
type ConfigurationInfo struct {
	InstallationDescription1 string
	InstallationDescription2 string
	ManufacturerInformation  string
}

// ConfigurationProvider lets device-specific software provide current
// configuration data and accept or reject configuration updates.
type ConfigurationProvider interface {
	GetConfigurationInfo() (ConfigurationInfo, error)
	SetConfigurationInfo(ConfigurationInfo) error
}

// KnownDevice describes a device observed on the NMEA 2000 network.
type KnownDevice struct {
	Address      uint8
	Name         uint64
	LastSeen     time.Time
	ProductInfo  *ProductInfo
	ConfigInfo   *ConfigurationInfo
	TransmitPGNs []uint32
	ReceivePGNs  []uint32
}

// DeviceChangeKind identifies why an observed device changed.
type DeviceChangeKind int

const (
	// DeviceChangeObserved indicates a device was observed for the first time.
	DeviceChangeObserved DeviceChangeKind = iota
	// DeviceChangeAddressChanged indicates an existing device NAME moved to a different source address.
	DeviceChangeAddressChanged
	// DeviceChangeProductInfoChanged indicates observed Product Information changed.
	DeviceChangeProductInfoChanged
	// DeviceChangeConfigurationInfoChanged indicates observed Configuration Information changed.
	DeviceChangeConfigurationInfoChanged
	// DeviceChangePGNListsChanged indicates observed transmit or receive PGN lists changed.
	DeviceChangePGNListsChanged
	// DeviceChangeExpired indicates an observed device expired from current network state.
	DeviceChangeExpired
)

// DeviceChange describes a node-observed device state change.
type DeviceChange struct {
	Kind        DeviceChangeKind
	Device      KnownDevice
	OldAddress  *uint8
	ChangedPGNs []uint32
}

type addressState uint8

const (
	stateUnclaimed addressState = iota
	stateClaiming
	stateClaimed
	stateLost
)

// ReadOnlyAddress can be passed to ClaimAddress to keep the node in passive
// monitor mode. A read-only node receives bus traffic and tracks known devices
// but never writes to the bus or responds to requests.
const ReadOnlyAddress uint8 = 255

// node is the internal implementation of the Node interface.
type node struct {
	subscriber                     Subscriber
	publisher                      Publisher
	clock                          Clock
	deviceInfo                     DeviceInfo
	deviceInfoSet                  bool
	productInfo                    ProductInfo
	configProvider                 ConfigurationProvider
	name                           uint64
	networkAddress                 uint8
	preferredAddress               uint8
	addressClaimed                 bool
	addressState                   addressState
	readOnly                       bool
	transmitPGNs                   []uint32
	receivePGNs                    []uint32
	knownDevices                   map[uint64]KnownDevice
	knownDeviceNamesByAddress      map[uint8]uint64
	unknownKnownDevicesByAddress   map[uint8]KnownDevice
	deviceChangeSubscribers        map[SubscriptionID]func(DeviceChange)
	nextDeviceChangeSubscriptionID SubscriptionID
	heartbeatEnabled               bool
	heartbeatInterval              time.Duration
	heartbeatSeq                   uint8
	started                        bool
	ctx                            context.Context
	cancel                         context.CancelFunc
	wg                             sync.WaitGroup
	subscriptions                  []SubscriptionID
	pgnIn                          chan any
	mutex                          sync.RWMutex
	wakeUp                         chan struct{}
	logger                         *logrus.Logger
}

type toSend struct {
	pgn  any
	dest uint8
}

// NewNode creates a new Node instance with the given dependencies.
func NewNode(subscriber Subscriber, publisher Publisher, clock Clock) Node {
	if clock == nil {
		clock = NewRealClock()
	}
	return &node{
		subscriber:                     subscriber,
		publisher:                      publisher,
		clock:                          clock,
		name:                           0,
		networkAddress:                 255,
		preferredAddress:               128,
		addressClaimed:                 false,
		readOnly:                       true,
		knownDevices:                   make(map[uint64]KnownDevice),
		knownDeviceNamesByAddress:      make(map[uint8]uint64),
		unknownKnownDevicesByAddress:   make(map[uint8]KnownDevice),
		deviceChangeSubscribers:        make(map[SubscriptionID]func(DeviceChange)),
		nextDeviceChangeSubscriptionID: 1,
		heartbeatEnabled:               false,
		heartbeatInterval:              60 * time.Second,
		started:                        false,
		subscriptions:                  make([]SubscriptionID, 0),
		pgnIn:                          make(chan any, 10),
		mutex:                          sync.RWMutex{},
		wakeUp:                         make(chan struct{}, 1),
		logger:                         logrus.New(),
	}
}

// SetLogger allows overriding the default logger for debugging.
// This method is not part of the Node interface.
func (n *node) SetLogger(logger *logrus.Logger) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.logger = logger
}

func (n *node) handleIsoRequest(p pgn.IsoRequest) {
	n.enqueuePgn(p)
}

func (n *node) handleIsoAddressClaim(p pgn.IsoAddressClaim) { //nolint:gocritic // Subscriber callbacks must accept value PGNs.
	n.logger.Infof("handleIsoAddressClaim: received address claim from source %d", p.Info.SourceId)
	n.enqueuePgn(p)
}

func (n *node) handleIsoCommandedAddress(p pgn.IsoCommandedAddress) { //nolint:gocritic // Subscriber callbacks must accept value PGNs.
	n.enqueuePgn(p)
}

func (n *node) handleIsoAcknowledgement(p pgn.IsoAcknowledgement) {
	n.enqueuePgn(p)
}

//nolint:gocritic // Subscriber callbacks must accept value PGNs.
func (n *node) handleNmeaRequestGroupFunction(p pgn.NmeaRequestGroupFunction) {
	n.enqueuePgn(p)
}

//nolint:gocritic // Subscriber callbacks must accept value PGNs.
func (n *node) handleNmeaCommandGroupFunction(p pgn.NmeaCommandGroupFunction) {
	n.enqueuePgn(p)
}

func (n *node) handleProductInformation(p pgn.ProductInformation) { //nolint:gocritic // Subscriber callbacks must accept value PGNs.
	n.enqueuePgn(p)
}

//nolint:gocritic // Subscriber callbacks must accept value PGNs.
func (n *node) handleConfigurationInformation(p pgn.ConfigurationInformation) {
	n.enqueuePgn(p)
}

func (n *node) handlePgnListTransmitAndReceive(p pgn.PgnListTransmitAndReceive) {
	n.enqueuePgn(p)
}

func (n *node) Start() error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if n.started {
		return fmt.Errorf("node already started")
	}

	sub, err := n.subscriber.SubscribeToStruct(pgn.IsoRequest{}, n.handleIsoRequest)
	if err != nil {
		return fmt.Errorf("failed to subscribe to IsoRequest: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.IsoAddressClaim{}, n.handleIsoAddressClaim)
	if err != nil {
		return fmt.Errorf("failed to subscribe to IsoAddressClaim: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.IsoCommandedAddress{}, n.handleIsoCommandedAddress)
	if err != nil {
		return fmt.Errorf("failed to subscribe to IsoCommandedAddress: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.IsoAcknowledgement{}, n.handleIsoAcknowledgement)
	if err != nil {
		return fmt.Errorf("failed to subscribe to IsoAcknowledgement: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.NmeaRequestGroupFunction{}, n.handleNmeaRequestGroupFunction)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NmeaRequestGroupFunction: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.NmeaCommandGroupFunction{}, n.handleNmeaCommandGroupFunction)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NmeaCommandGroupFunction: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.ProductInformation{}, n.handleProductInformation)
	if err != nil {
		return fmt.Errorf("failed to subscribe to ProductInformation: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.ConfigurationInformation{}, n.handleConfigurationInformation)
	if err != nil {
		return fmt.Errorf("failed to subscribe to ConfigurationInformation: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.PgnListTransmitAndReceive{}, n.handlePgnListTransmitAndReceive)
	if err != nil {
		return fmt.Errorf("failed to subscribe to PgnListTransmitAndReceive: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	n.ctx, n.cancel = context.WithCancel(context.Background())
	n.started = true

	n.wg.Add(1)
	go n.process()

	return nil
}

func (n *node) Stop() error {
	n.mutex.Lock()
	if !n.started {
		n.mutex.Unlock()
		return fmt.Errorf("node not started")
	}

	for _, sub := range n.subscriptions {
		if err := n.subscriber.Unsubscribe(sub); err != nil {
			n.logger.Warnf("failed to unsubscribe node subscription %d: %v", sub, err)
		}
	}
	n.subscriptions = make([]SubscriptionID, 0)

	if n.cancel != nil {
		n.cancel()
	}
	n.mutex.Unlock()

	n.wg.Wait()

	n.mutex.Lock()
	n.started = false
	n.addressState = stateUnclaimed
	n.addressClaimed = false
	n.networkAddress = 255
	n.readOnly = true
	// Reset the context so the node can be restarted.
	n.ctx = nil
	n.cancel = nil
	n.mutex.Unlock()

	return nil
}

func (n *node) ClaimAddress(preferredAddress uint8) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if preferredAddress == ReadOnlyAddress {
		n.preferredAddress = preferredAddress
		n.networkAddress = ReadOnlyAddress
		n.addressState = stateUnclaimed
		n.addressClaimed = false
		n.readOnly = true
		select {
		case n.wakeUp <- struct{}{}:
		default:
		}
		return nil
	}
	if !n.deviceInfoSet {
		return fmt.Errorf("cannot claim address, device info has not been set")
	}
	if preferredAddress > 253 {
		return fmt.Errorf("preferred address %d is out of range (0-253)", preferredAddress)
	}
	n.preferredAddress = preferredAddress
	n.readOnly = false
	n.addressState = stateClaiming
	select {
	case n.wakeUp <- struct{}{}:
	default:
	}
	return nil
}

func (n *node) GetNetworkAddress() uint8 {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	return n.networkAddress
}

func (n *node) IsAddressClaimed() bool {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	return n.addressClaimed
}

func (n *node) KnownDevices() []KnownDevice {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	devices := make([]KnownDevice, 0, len(n.knownDevices))
	for _, device := range n.knownDevices {
		if device.Address > 253 {
			continue
		}
		devices = append(devices, cloneKnownDevice(&device))
	}
	for _, device := range n.unknownKnownDevicesByAddress {
		devices = append(devices, cloneKnownDevice(&device))
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Address < devices[j].Address
	})

	return devices
}

func (n *node) SubscribeToDeviceChanges(callback func(DeviceChange)) SubscriptionID {
	if callback == nil {
		return 0
	}

	n.mutex.Lock()
	defer n.mutex.Unlock()

	subID := n.nextDeviceChangeSubscriptionID
	n.nextDeviceChangeSubscriptionID++
	n.deviceChangeSubscribers[subID] = callback
	return subID
}

func (n *node) UnsubscribeDeviceChanges(subID SubscriptionID) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if _, ok := n.deviceChangeSubscribers[subID]; !ok {
		return fmt.Errorf("device change subscription not found")
	}
	delete(n.deviceChangeSubscribers, subID)
	return nil
}

func (n *node) Write(pgnStruct any) error {
	return n.write(pgnStruct, 255)
}

func (n *node) WriteTo(pgnStruct any, destination uint8) error {
	return n.write(pgnStruct, destination)
}

func (n *node) write(pgnStruct any, destination uint8) error {
	n.mutex.RLock()
	addressClaimed := n.addressClaimed
	networkAddress := n.networkAddress
	publisher := n.publisher
	readOnly := n.readOnly
	n.mutex.RUnlock()

	if readOnly {
		return fmt.Errorf("cannot write PGN, node is read-only")
	}
	if !addressClaimed {
		return fmt.Errorf("cannot write PGN, address not claimed")
	}

	if err := setMessageInfo(pgnStruct, networkAddress, destination); err != nil {
		return fmt.Errorf("failed to set message info: %w", err)
	}

	return publisher.Write(pgnStruct)
}

func (n *node) SetDeviceInfo(info DeviceInfo) error {
	name, err := computeName(info)
	if err != nil {
		return fmt.Errorf("failed to compute NAME from device info: %w", err)
	}
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.deviceInfo = info
	n.deviceInfoSet = true
	n.name = name
	return nil
}

func (n *node) SetProductInfo(info ProductInfo) { //nolint:gocritic // API accepts value configuration.
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.productInfo = info
}

func (n *node) SetConfigurationProvider(provider ConfigurationProvider) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.configProvider = provider
}

func (n *node) SetSupportedPGNs(transmit, receive []uint32) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.transmitPGNs = transmit
	n.receivePGNs = receive
}

func (n *node) SetHeartbeatInterval(interval time.Duration) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.heartbeatInterval = interval
}

func (n *node) EnableHeartbeat(enable bool) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.heartbeatEnabled = enable
}

func (n *node) enqueuePgn(p any) {
	n.logger.Infof("enqueuePgn: received %T", p)
	select {
	case n.pgnIn <- p:
	case <-n.ctx.Done():
		n.logger.Infof("enqueuePgn: context done, dropping PGN")
	}
}

func (n *node) processIsoRequest(req pgn.IsoRequest) []toSend {
	n.mutex.RLock()
	addressClaimed := n.addressClaimed
	networkAddress := n.networkAddress
	deviceInfoCopy := n.deviceInfo
	productInfoCopy := n.productInfo
	configProvider := n.configProvider
	transmitPGNs := append([]uint32(nil), n.transmitPGNs...)
	receivePGNs := append([]uint32(nil), n.receivePGNs...)
	heartbeatEnabled := n.heartbeatEnabled
	readOnly := n.readOnly
	n.mutex.RUnlock()

	if req.Pgn == nil {
		return nil
	}
	if readOnly {
		return nil
	}
	if !addressClaimed {
		return nil
	}
	if req.Info.TargetId != 255 && req.Info.TargetId != networkAddress {
		return nil
	}

	n.logger.Infof("processIsoRequest: processing request for PGN %d from source %d", *req.Pgn, req.Info.SourceId)

	var responses []toSend

	switch *req.Pgn {
	case pgn.IsoAddressClaimPgn:
		responses = append(responses, toSend{pgn: buildAddressClaim(deviceInfoCopy, networkAddress), dest: req.Info.SourceId})

	case pgn.ProductInformationPgn:
		version := float32(productInfoCopy.NMEA2000Version) / 100.0
		productCode := productInfoCopy.ProductCode
		loadEquivalency := productInfoCopy.LoadEquivalency
		responsePgn := &pgn.ProductInformation{
			Info: pgn.MessageInfo{
				PGN:      pgn.ProductInformationPgn,
				SourceId: networkAddress,
				TargetId: req.Info.SourceId,
			},
			Nmea2000Version:     &version,
			ProductCode:         &productCode,
			ModelId:             productInfoCopy.ModelID,
			SoftwareVersionCode: productInfoCopy.SoftwareVersionCode,
			ModelVersion:        productInfoCopy.ModelVersion,
			ModelSerialCode:     productInfoCopy.ModelSerialCode,
			CertificationLevel:  pgn.CertificationLevelConst(productInfoCopy.CertificationLevel),
			LoadEquivalency:     &loadEquivalency,
		}
		responses = append(responses, toSend{pgn: responsePgn, dest: req.Info.SourceId})

	case pgn.PgnListTransmitAndReceivePgn:
		transmitPGNs = managedTransmitPGNs(transmitPGNs, configProvider != nil, heartbeatEnabled)
		receivePGNs = managedReceivePGNs(receivePGNs)

		txRepeating := make([]pgn.PgnListTransmitAndReceiveRepeating1, len(transmitPGNs))
		for i, pgnNum := range transmitPGNs {
			p := pgnNum
			txRepeating[i] = pgn.PgnListTransmitAndReceiveRepeating1{Pgn: &p}
		}
		txResponse := &pgn.PgnListTransmitAndReceive{
			Info: pgn.MessageInfo{
				PGN:      pgn.PgnListTransmitAndReceivePgn,
				SourceId: networkAddress,
				TargetId: req.Info.SourceId,
			},
			FunctionCode: pgn.TransmitPgnList,
			Repeating1:   txRepeating,
		}
		responses = append(responses, toSend{pgn: txResponse, dest: req.Info.SourceId})

		rxRepeating := make([]pgn.PgnListTransmitAndReceiveRepeating1, len(receivePGNs))
		for i, pgnNum := range receivePGNs {
			p := pgnNum
			rxRepeating[i] = pgn.PgnListTransmitAndReceiveRepeating1{Pgn: &p}
		}
		rxResponse := &pgn.PgnListTransmitAndReceive{
			Info: pgn.MessageInfo{
				PGN:      pgn.PgnListTransmitAndReceivePgn,
				SourceId: networkAddress,
				TargetId: req.Info.SourceId,
			},
			FunctionCode: pgn.ReceivePgnList,
			Repeating1:   rxRepeating,
		}
		responses = append(responses, toSend{pgn: rxResponse, dest: req.Info.SourceId})

	case pgn.ConfigurationInformationPgn:
		if configProvider == nil {
			responses = append(responses, toSend{pgn: buildIsoNak(networkAddress, req.Info.SourceId, *req.Pgn), dest: req.Info.SourceId})
			break
		}
		configInfo, err := configProvider.GetConfigurationInfo()
		if err != nil {
			n.logger.Errorf("processIsoRequest: failed to get configuration information: %v", err)
			responses = append(responses, toSend{pgn: buildIsoNak(networkAddress, req.Info.SourceId, *req.Pgn), dest: req.Info.SourceId})
			break
		}
		responsePgn := &pgn.ConfigurationInformation{
			Info: pgn.MessageInfo{
				PGN:      pgn.ConfigurationInformationPgn,
				SourceId: networkAddress,
				TargetId: req.Info.SourceId,
			},
			InstallationDescription1: configInfo.InstallationDescription1,
			InstallationDescription2: configInfo.InstallationDescription2,
			ManufacturerInformation:  configInfo.ManufacturerInformation,
		}
		responses = append(responses, toSend{pgn: responsePgn, dest: req.Info.SourceId})
	}

	return responses
}

func managedTransmitPGNs(configured []uint32, hasConfigurationProvider, heartbeatEnabled bool) []uint32 {
	managed := []uint32{
		pgn.IsoAcknowledgementPgn,
		pgn.IsoAddressClaimPgn,
		pgn.NmeaAcknowledgeGroupFunctionPgn,
		pgn.PgnListTransmitAndReceivePgn,
		pgn.ProductInformationPgn,
	}
	if hasConfigurationProvider {
		managed = append(managed, pgn.ConfigurationInformationPgn)
	}
	if heartbeatEnabled {
		managed = append(managed, pgn.HeartbeatPgn)
	}
	return mergePGNs(configured, managed)
}

func managedReceivePGNs(configured []uint32) []uint32 {
	return mergePGNs(configured, []uint32{
		pgn.IsoAcknowledgementPgn,
		pgn.IsoRequestPgn,
		pgn.IsoAddressClaimPgn,
		pgn.IsoCommandedAddressPgn,
		pgn.NmeaRequestGroupFunctionPgn,
		pgn.NmeaCommandGroupFunctionPgn,
	})
}

func mergePGNs(configured, managed []uint32) []uint32 {
	seen := make(map[uint32]struct{}, len(configured)+len(managed))
	merged := make([]uint32, 0, len(configured)+len(managed))
	for _, p := range configured {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}
	for _, p := range managed {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}
	return merged
}

func (n *node) processNmeaRequestGroupFunction(req *pgn.NmeaRequestGroupFunction) []toSend {
	if req.Pgn == nil {
		return nil
	}
	if req.NumberOfParameters != nil && *req.NumberOfParameters != 0 {
		return n.processUnsupportedGroupFunction(req.Info, *req.Pgn, pgn.ReadOrWriteNotSupported)
	}

	return n.processIsoRequest(pgn.IsoRequest{
		Info: req.Info,
		Pgn:  req.Pgn,
	})
}

func (n *node) processNmeaCommandGroupFunction(cmd *pgn.NmeaCommandGroupFunction) []toSend {
	if cmd.Pgn == nil {
		return nil
	}
	return n.processUnsupportedGroupFunction(cmd.Info, *cmd.Pgn, pgn.ReadOrWriteNotSupported)
}

func (n *node) processUnsupportedGroupFunction(info pgn.MessageInfo, requestedPgn uint32, parameterError pgn.ParameterFieldConst) []toSend {
	n.mutex.RLock()
	addressClaimed := n.addressClaimed
	networkAddress := n.networkAddress
	readOnly := n.readOnly
	n.mutex.RUnlock()

	if readOnly {
		return nil
	}
	if !addressClaimed {
		return nil
	}
	if info.TargetId != 255 && info.TargetId != networkAddress {
		return nil
	}
	if info.TargetId == 255 {
		return nil
	}

	return []toSend{{
		pgn:  buildNmeaGroupNak(networkAddress, info.SourceId, requestedPgn, parameterError),
		dest: info.SourceId,
	}}
}

func (n *node) processIsoCommandedAddress(cmd *pgn.IsoCommandedAddress) {
	n.mutex.RLock()
	currentName := n.name
	readOnly := n.readOnly
	n.mutex.RUnlock()

	if readOnly {
		return
	}

	cmdName := computeNameFromCommand(cmd)

	ourNameWithoutBit := currentName &^ (1 << 63)
	if cmdName != ourNameWithoutBit {
		return
	}

	if cmd.NewSourceAddress == nil {
		return
	}

	n.mutex.Lock()
	n.preferredAddress = *cmd.NewSourceAddress
	n.addressState = stateClaiming
	n.addressClaimed = false
	n.mutex.Unlock()
}

func (n *node) processIsoAddressClaim(claim *pgn.IsoAddressClaim) {
	incomingName := computeNameFromClaim(claim)
	n.updateKnownDeviceFromClaim(claim, incomingName)

	n.mutex.RLock()
	currentState := n.addressState
	currentAddress := n.networkAddress
	currentName := n.name
	n.mutex.RUnlock()

	n.logger.Infof("processIsoAddressClaim: received claim from source %d, our address %d, state %d",
		claim.Info.SourceId, currentAddress, currentState)

	if currentState != stateClaiming && currentState != stateClaimed {
		n.logger.Infof("processIsoAddressClaim: ignoring claim, not in claiming/claimed state")
		return
	}

	if claim.Info.SourceId != currentAddress {
		n.logger.Infof("processIsoAddressClaim: ignoring claim, different address (%d vs %d)",
			claim.Info.SourceId, currentAddress)
		return
	}

	n.logger.Infof("processIsoAddressClaim: comparing NAMEs - incoming: %x, ours: %x",
		incomingName, currentName)

	if incomingName < currentName {
		n.logger.Infof("processIsoAddressClaim: incoming NAME has higher priority, yielding address")
		n.mutex.Lock()
		n.addressClaimed = false
		if n.deviceInfo.ArbitraryAddressCapable {
			if nextAddress, ok := n.nextAvailableAddressLocked(currentAddress); ok {
				n.preferredAddress = nextAddress
				n.addressState = stateClaiming
				n.logger.Infof("processIsoAddressClaim: retrying address claim with address %d", nextAddress)
			} else {
				n.addressState = stateLost
				n.networkAddress = 255
				n.logger.Warnf("processIsoAddressClaim: no available address found")
			}
		} else {
			n.addressState = stateLost
			n.networkAddress = 255
		}
		n.mutex.Unlock()
		select {
		case n.wakeUp <- struct{}{}:
		default:
		}
	} else {
		n.logger.Infof("processIsoAddressClaim: our NAME has higher priority, keeping address")
		n.sendAddressClaim()
	}
}

func (n *node) updateKnownDeviceFromClaim(claim *pgn.IsoAddressClaim, name uint64) {
	var changes []DeviceChange
	n.mutex.Lock()

	now := time.Now()
	address := claim.Info.SourceId
	device := n.knownDevices[name]
	_, knownName := n.knownDevices[name]
	oldAddress := device.Address
	if unknown, ok := n.unknownKnownDevicesByAddress[address]; ok {
		device = mergeKnownDevice(&device, &unknown)
		delete(n.unknownKnownDevicesByAddress, address)
	}
	if previousName, ok := n.knownDeviceNamesByAddress[address]; ok && previousName != name {
		previousDevice := n.knownDevices[previousName]
		previousDevice.Address = 255
		n.knownDevices[previousName] = previousDevice
	}
	if knownName && oldAddress <= 253 && oldAddress != address {
		delete(n.knownDeviceNamesByAddress, oldAddress)
	}
	device.Address = address
	device.Name = name
	device.LastSeen = now
	n.knownDevices[name] = device
	n.knownDeviceNamesByAddress[address] = name
	if !knownName {
		changes = append(changes, DeviceChange{
			Kind:   DeviceChangeObserved,
			Device: cloneKnownDevice(&device),
		})
	} else if oldAddress != address {
		changes = append(changes, DeviceChange{
			Kind:       DeviceChangeAddressChanged,
			Device:     cloneKnownDevice(&device),
			OldAddress: ptrUint8(oldAddress),
		})
	}
	n.mutex.Unlock()

	n.publishDeviceChanges(changes)
}

func (n *node) updateKnownDeviceFromProductInfo(info *pgn.ProductInformation) {
	var changes []DeviceChange
	n.mutex.Lock()

	device := n.knownDeviceForAddressLocked(info.Info.SourceId)
	device.Address = info.Info.SourceId
	device.LastSeen = time.Now()

	productCode := uint16(0)
	if info.ProductCode != nil {
		productCode = *info.ProductCode
	}
	nmea2000Version := uint16(0)
	if info.Nmea2000Version != nil {
		nmea2000Version = uint16(*info.Nmea2000Version * 100)
	}
	loadEquivalency := uint8(0)
	if info.LoadEquivalency != nil {
		loadEquivalency = *info.LoadEquivalency
	}

	productInfo := ProductInfo{
		NMEA2000Version:     nmea2000Version,
		ProductCode:         productCode,
		ModelID:             info.ModelId,
		SoftwareVersionCode: info.SoftwareVersionCode,
		ModelVersion:        info.ModelVersion,
		ModelSerialCode:     info.ModelSerialCode,
		CertificationLevel:  uint8(info.CertificationLevel),
		LoadEquivalency:     loadEquivalency,
	}
	if device.ProductInfo == nil || !reflect.DeepEqual(*device.ProductInfo, productInfo) {
		device.ProductInfo = &productInfo
		changes = append(changes, DeviceChange{
			Kind:   DeviceChangeProductInfoChanged,
			Device: cloneKnownDevice(&device),
		})
	}
	n.setKnownDeviceForAddressLocked(info.Info.SourceId, &device)
	n.mutex.Unlock()

	n.publishDeviceChanges(changes)
}

func (n *node) updateKnownDeviceFromConfigurationInfo(info *pgn.ConfigurationInformation) {
	var changes []DeviceChange
	n.mutex.Lock()

	device := n.knownDeviceForAddressLocked(info.Info.SourceId)
	device.Address = info.Info.SourceId
	device.LastSeen = time.Now()
	configInfo := ConfigurationInfo{
		InstallationDescription1: info.InstallationDescription1,
		InstallationDescription2: info.InstallationDescription2,
		ManufacturerInformation:  info.ManufacturerInformation,
	}
	if device.ConfigInfo == nil || !reflect.DeepEqual(*device.ConfigInfo, configInfo) {
		device.ConfigInfo = &configInfo
		changes = append(changes, DeviceChange{
			Kind:   DeviceChangeConfigurationInfoChanged,
			Device: cloneKnownDevice(&device),
		})
	}
	n.setKnownDeviceForAddressLocked(info.Info.SourceId, &device)
	n.mutex.Unlock()

	n.publishDeviceChanges(changes)
}

func (n *node) updateKnownDeviceFromPgnList(info *pgn.PgnListTransmitAndReceive) {
	var changes []DeviceChange
	n.mutex.Lock()

	device := n.knownDeviceForAddressLocked(info.Info.SourceId)
	device.Address = info.Info.SourceId
	device.LastSeen = time.Now()

	pgns := knownDevicePGNListValues(info.Repeating1)
	var changedPGNs []uint32
	switch info.FunctionCode {
	case pgn.TransmitPgnList:
		if !reflect.DeepEqual(device.TransmitPGNs, pgns) {
			device.TransmitPGNs = pgns
			changedPGNs = append([]uint32(nil), pgns...)
		}
	case pgn.ReceivePgnList:
		if !reflect.DeepEqual(device.ReceivePGNs, pgns) {
			device.ReceivePGNs = pgns
			changedPGNs = append([]uint32(nil), pgns...)
		}
	default:
		n.mutex.Unlock()
		return
	}
	if changedPGNs != nil {
		changes = append(changes, DeviceChange{
			Kind:        DeviceChangePGNListsChanged,
			Device:      cloneKnownDevice(&device),
			ChangedPGNs: changedPGNs,
		})
	}
	n.setKnownDeviceForAddressLocked(info.Info.SourceId, &device)
	n.mutex.Unlock()

	n.publishDeviceChanges(changes)
}

func (n *node) nextAvailableAddressLocked(after uint8) (uint8, bool) {
	claimed := make(map[uint8]struct{}, len(n.knownDeviceNamesByAddress)+len(n.unknownKnownDevicesByAddress)+1)
	for address := range n.knownDeviceNamesByAddress {
		if address <= 253 {
			claimed[address] = struct{}{}
		}
	}
	for address := range n.unknownKnownDevicesByAddress {
		if address <= 253 {
			claimed[address] = struct{}{}
		}
	}
	claimed[after] = struct{}{}

	for i := 1; i <= 254; i++ {
		candidate := uint8((uint16(after) + uint16(i)) % 254)
		if _, exists := claimed[candidate]; !exists {
			return candidate, true
		}
	}

	return 0, false
}

func (n *node) knownDeviceForAddressLocked(address uint8) KnownDevice {
	if name, ok := n.knownDeviceNamesByAddress[address]; ok {
		return n.knownDevices[name]
	}
	if device, ok := n.unknownKnownDevicesByAddress[address]; ok {
		return device
	}
	return KnownDevice{Address: address}
}

func (n *node) setKnownDeviceForAddressLocked(address uint8, device *KnownDevice) {
	if device.Name != 0 {
		n.knownDevices[device.Name] = *device
		n.knownDeviceNamesByAddress[address] = device.Name
		delete(n.unknownKnownDevicesByAddress, address)
		return
	}
	n.unknownKnownDevicesByAddress[address] = *device
}

func mergeKnownDevice(device, overlay *KnownDevice) KnownDevice {
	ret := *device
	if device.Address == 0 {
		ret.Address = overlay.Address
	}
	if device.LastSeen.IsZero() || overlay.LastSeen.After(device.LastSeen) {
		ret.LastSeen = overlay.LastSeen
	}
	if device.ProductInfo == nil && overlay.ProductInfo != nil {
		productInfo := *overlay.ProductInfo
		ret.ProductInfo = &productInfo
	}
	if device.ConfigInfo == nil && overlay.ConfigInfo != nil {
		configInfo := *overlay.ConfigInfo
		ret.ConfigInfo = &configInfo
	}
	if device.TransmitPGNs == nil && overlay.TransmitPGNs != nil {
		ret.TransmitPGNs = append([]uint32(nil), overlay.TransmitPGNs...)
	}
	if device.ReceivePGNs == nil && overlay.ReceivePGNs != nil {
		ret.ReceivePGNs = append([]uint32(nil), overlay.ReceivePGNs...)
	}
	return ret
}

func cloneKnownDevice(device *KnownDevice) KnownDevice {
	ret := *device
	if ret.ProductInfo != nil {
		productInfo := *ret.ProductInfo
		ret.ProductInfo = &productInfo
	}
	if ret.ConfigInfo != nil {
		configInfo := *ret.ConfigInfo
		ret.ConfigInfo = &configInfo
	}
	ret.TransmitPGNs = append([]uint32(nil), ret.TransmitPGNs...)
	ret.ReceivePGNs = append([]uint32(nil), ret.ReceivePGNs...)
	return ret
}

func knownDevicePGNListValues(list []pgn.PgnListTransmitAndReceiveRepeating1) []uint32 {
	values := make([]uint32, 0, len(list))
	for _, item := range list {
		if item.Pgn != nil {
			values = append(values, *item.Pgn)
		}
	}
	return values
}

func ptrUint8(v uint8) *uint8 {
	return &v
}

func (n *node) publishDeviceChanges(changes []DeviceChange) {
	if len(changes) == 0 {
		return
	}

	n.mutex.RLock()
	subscribers := make([]func(DeviceChange), 0, len(n.deviceChangeSubscribers))
	for _, subscriber := range n.deviceChangeSubscribers {
		subscribers = append(subscribers, subscriber)
	}
	n.mutex.RUnlock()

	for i := range changes {
		for _, subscriber := range subscribers {
			subscriber(changes[i])
		}
	}
}

func (n *node) sendAddressClaim() {
	n.mutex.RLock()
	deviceInfoCopy := n.deviceInfo
	networkAddressCopy := n.networkAddress
	publisher := n.publisher
	readOnly := n.readOnly
	n.mutex.RUnlock()

	if readOnly {
		return
	}

	claim := buildAddressClaim(deviceInfoCopy, networkAddressCopy)
	n.logger.Infof("sendAddressClaim: sending claim for address %d", networkAddressCopy)
	if err := publisher.Write(claim); err != nil {
		n.logger.Errorf("sendAddressClaim: failed to write claim: %v", err)
	}
}

func buildAddressClaim(deviceInfo DeviceInfo, networkAddress uint8) *pgn.IsoAddressClaim {
	arbitraryBit := uint8(0)
	if deviceInfo.ArbitraryAddressCapable {
		arbitraryBit = 1
	}

	return &pgn.IsoAddressClaim{
		Info: pgn.MessageInfo{
			PGN:      pgn.IsoAddressClaimPgn,
			SourceId: networkAddress,
			TargetId: 255,
			Priority: 6,
		},
		UniqueNumber:            &deviceInfo.UniqueNumber,
		ManufacturerCode:        deviceInfo.ManufacturerCode,
		DeviceInstanceLower:     &deviceInfo.DeviceInstanceLower,
		DeviceInstanceUpper:     &deviceInfo.DeviceInstanceUpper,
		DeviceFunction:          deviceInfo.DeviceFunction,
		DeviceClass:             deviceInfo.DeviceClass,
		SystemInstance:          &deviceInfo.SystemInstance,
		IndustryGroup:           deviceInfo.IndustryGroup,
		ArbitraryAddressCapable: pgn.YesNoConst(arbitraryBit),
	}
}

func buildIsoNak(source, destination uint8, requestedPgn uint32) *pgn.IsoAcknowledgement {
	return &pgn.IsoAcknowledgement{
		Info: pgn.MessageInfo{
			PGN:      pgn.IsoAcknowledgementPgn,
			SourceId: source,
			TargetId: destination,
			Priority: 6,
		},
		Control: pgn.Nak,
		Pgn:     &requestedPgn,
	}
}

func buildNmeaGroupNak(
	source, destination uint8,
	requestedPgn uint32,
	parameterError pgn.ParameterFieldConst,
) *pgn.NmeaAcknowledgeGroupFunction {
	return &pgn.NmeaAcknowledgeGroupFunction{
		Info: pgn.MessageInfo{
			PGN:      pgn.NmeaAcknowledgeGroupFunctionPgn,
			SourceId: source,
			TargetId: destination,
			Priority: 3,
		},
		FunctionCode:                          pgn.Acknowledge_4,
		Pgn:                                   &requestedPgn,
		PgnErrorCode:                          pgn.PgnNotSupported,
		TransmissionIntervalPriorityErrorCode: pgn.NotSupported,
		Repeating1: []pgn.NmeaAcknowledgeGroupFunctionRepeating1{
			{Parameter: parameterError},
		},
	}
}

func (n *node) sendHeartbeat() {
	n.mutex.RLock()
	heartbeatSeqCopy := n.heartbeatSeq
	networkAddressCopy := n.networkAddress
	n.mutex.RUnlock()

	hb := &pgn.Heartbeat{
		Info: pgn.MessageInfo{
			PGN:      pgn.HeartbeatPgn,
			SourceId: networkAddressCopy,
		},
		SequenceCounter:  &heartbeatSeqCopy,
		Controller1State: pgn.ErrorActive,
		Controller2State: pgn.ErrorActive,
		EquipmentStatus:  pgn.Operational,
	}

	if err := n.Write(hb); err != nil {
		n.logger.Errorf("sendHeartbeat: failed to write heartbeat: %v", err)
	}

	n.mutex.Lock()
	n.heartbeatSeq++
	n.mutex.Unlock()
}

func (n *node) processPGN(p any) []toSend {
	switch v := p.(type) {
	case pgn.IsoRequest:
		return n.processIsoRequest(v)
	case pgn.NmeaRequestGroupFunction:
		return n.processNmeaRequestGroupFunction(&v)
	case pgn.NmeaCommandGroupFunction:
		return n.processNmeaCommandGroupFunction(&v)
	case pgn.IsoAcknowledgement:
		n.logger.Infof("process: received ISO acknowledgement for PGN %v", v.Pgn)
	case pgn.IsoAddressClaim:
		n.processIsoAddressClaim(&v)
	case pgn.IsoCommandedAddress:
		n.processIsoCommandedAddress(&v)
	case pgn.ProductInformation:
		n.updateKnownDeviceFromProductInfo(&v)
	case pgn.ConfigurationInformation:
		n.updateKnownDeviceFromConfigurationInfo(&v)
	case pgn.PgnListTransmitAndReceive:
		n.updateKnownDeviceFromPgnList(&v)
	default:
		n.logger.Infof("process: received unhandled PGN type %T", p)
	}
	return nil
}

func (n *node) sendProcessResponses(toSendList []toSend) {
	if len(toSendList) == 0 {
		return
	}

	n.mutex.RLock()
	publisher := n.publisher
	readOnly := n.readOnly
	n.mutex.RUnlock()
	if readOnly {
		return
	}
	for _, ts := range toSendList {
		n.logger.Infof("process: sending PGN %+v to %d", ts.pgn, ts.dest)
		if err := publisher.Write(ts.pgn); err != nil {
			n.logger.Errorf("process: failed to write PGN %+v to %d: %v", ts.pgn, ts.dest, err)
		}
	}
}

func (n *node) process() {
	defer n.wg.Done()
	n.logger.Infof("process: goroutine started")
	defer n.logger.Infof("process: goroutine stopped")

	var claimTicker Ticker
	var heartbeatTicker Ticker

	for {
		var initialHeartbeat bool
		var shouldSendClaim bool
		n.mutex.Lock()
		if n.addressState == stateClaiming {
			// We are in the process of claiming an address
			if claimTicker != nil && n.networkAddress != n.preferredAddress {
				claimTicker.Stop()
				claimTicker = nil
			}
			if claimTicker == nil {
				claimTicker = n.clock.NewTicker(250 * time.Millisecond)
				n.networkAddress = n.preferredAddress
				shouldSendClaim = true
			}
		} else if claimTicker != nil {
			claimTicker.Stop()
			claimTicker = nil
		}

		if n.heartbeatEnabled && n.addressClaimed {
			if heartbeatTicker == nil {
				heartbeatTicker = n.clock.NewTicker(n.heartbeatInterval)
				initialHeartbeat = true
			}
		} else if heartbeatTicker != nil {
			heartbeatTicker.Stop()
			heartbeatTicker = nil
		}
		n.mutex.Unlock()

		if shouldSendClaim {
			n.sendAddressClaim()
		}

		if initialHeartbeat {
			n.sendHeartbeat()
		}

		var claimTick <-chan time.Time
		if claimTicker != nil {
			claimTick = claimTicker.C()
		}

		var heartbeatTick <-chan time.Time
		if heartbeatTicker != nil {
			heartbeatTick = heartbeatTicker.C()
		}

		select {
		case p, ok := <-n.pgnIn:
			if !ok {
				n.logger.Infof("process: pgnIn channel closed")
				return
			}
			n.sendProcessResponses(n.processPGN(p))

		case <-claimTick:
			n.mutex.Lock()
			if n.addressState == stateClaiming {
				n.addressState = stateClaimed
				n.addressClaimed = true
				n.logger.Infof("process: address %d claimed", n.networkAddress)
			}
			n.mutex.Unlock()

		case <-heartbeatTick:
			n.sendHeartbeat()

		case <-n.wakeUp:
		// Just loop again to re-evaluate tickers
		case <-n.ctx.Done():
			if claimTicker != nil {
				claimTicker.Stop()
			}
			if heartbeatTicker != nil {
				heartbeatTicker.Stop()
			}
			return
		}
	}
}

func computeName(d DeviceInfo) (uint64, error) {
	if d.UniqueNumber > 0x1FFFFF {
		return 0, fmt.Errorf("unique number %d is too large", d.UniqueNumber)
	}
	if d.ManufacturerCode > 0x7FF {
		return 0, fmt.Errorf("manufacturer code %d is too large", d.ManufacturerCode)
	}
	if d.DeviceInstanceLower > 7 {
		return 0, fmt.Errorf("device instance lower %d is too large", d.DeviceInstanceLower)
	}
	if d.DeviceInstanceUpper > 31 {
		return 0, fmt.Errorf("device instance upper %d is too large", d.DeviceInstanceUpper)
	}
	if d.DeviceFunction > 255 {
		return 0, fmt.Errorf("device function %d is too large", d.DeviceFunction)
	}
	if d.DeviceClass > 127 {
		return 0, fmt.Errorf("device class %d is too large", d.DeviceClass)
	}
	if d.SystemInstance > 15 {
		return 0, fmt.Errorf("system instance %d is too large", d.SystemInstance)
	}
	if d.IndustryGroup > 7 {
		return 0, fmt.Errorf("industry group %d is too large", d.IndustryGroup)
	}

	name := uint64(d.UniqueNumber) |
		(uint64(d.ManufacturerCode) << 21) |
		(uint64(d.DeviceInstanceLower) << 32) |
		(uint64(d.DeviceInstanceUpper) << 35) |
		(uint64(d.DeviceFunction) << 40) |
		(uint64(0) << 48) | // Reserved bit
		(uint64(d.DeviceClass) << 49) |
		(uint64(d.SystemInstance) << 56) |
		(uint64(d.IndustryGroup) << 60)

	if d.ArbitraryAddressCapable {
		name |= 1 << 63
	}

	return name, nil
}

func computeNameFromClaim(claim *pgn.IsoAddressClaim) uint64 {
	var name uint64
	if claim.UniqueNumber != nil {
		name |= uint64(*claim.UniqueNumber)
	}
	if claim.ManufacturerCode != 0 {
		name |= uint64(claim.ManufacturerCode) << 21
	}
	if claim.DeviceInstanceLower != nil {
		name |= uint64(*claim.DeviceInstanceLower) << 32
	}
	if claim.DeviceInstanceUpper != nil {
		name |= uint64(*claim.DeviceInstanceUpper) << 35
	}
	if claim.DeviceFunction != 0 {
		name |= uint64(claim.DeviceFunction) << 40
	}
	// Reserved bit at 48 is 0
	if claim.DeviceClass != 0 {
		name |= uint64(claim.DeviceClass) << 49
	}
	if claim.SystemInstance != nil {
		name |= uint64(*claim.SystemInstance) << 56
	}
	if claim.IndustryGroup != 0 {
		name |= uint64(claim.IndustryGroup) << 60
	}
	if claim.ArbitraryAddressCapable == pgn.Yes {
		name |= 1 << 63
	}
	return name
}

func computeNameFromCommand(cmd *pgn.IsoCommandedAddress) uint64 {
	var name uint64
	if len(cmd.UniqueNumber) >= 3 {
		uniqueNum := uint64(cmd.UniqueNumber[0]) | (uint64(cmd.UniqueNumber[1]) << 8) | (uint64(cmd.UniqueNumber[2]) << 16)
		name |= uniqueNum
	}
	if cmd.ManufacturerCode != 0 {
		name |= uint64(cmd.ManufacturerCode) << 21
	}
	if cmd.DeviceInstanceLower != nil {
		name |= uint64(*cmd.DeviceInstanceLower) << 32
	}
	if cmd.DeviceInstanceUpper != nil {
		name |= uint64(*cmd.DeviceInstanceUpper) << 35
	}
	if cmd.DeviceFunction != 0 {
		name |= uint64(cmd.DeviceFunction) << 40
	}
	// Reserved bit at 48 is 0
	if cmd.DeviceClass != 0 {
		name |= uint64(cmd.DeviceClass) << 49
	}
	if cmd.SystemInstance != nil {
		name |= uint64(*cmd.SystemInstance) << 56
	}
	if cmd.IndustryCode != 0 {
		name |= uint64(cmd.IndustryCode) << 60
	}
	return name
}

// setMessageInfo uses reflection to set the "Info" field on a PGN struct.
// This is a helper to avoid repetitive code when sending PGNs.
func setMessageInfo(s any, source, destination uint8) error {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("expected a pointer to a struct, got %T", s)
	}

	infoField := v.Elem().FieldByName("Info")
	if !infoField.IsValid() || !infoField.CanSet() {
		return fmt.Errorf("struct %T does not have an exported 'Info' field", s)
	}

	if infoField.Type() != reflect.TypeOf(pgn.MessageInfo{}) {
		return fmt.Errorf("'Info' field in struct %T is not of type pgn.MessageInfo", s)
	}

	// Get the current MessageInfo
	info := infoField.Interface().(pgn.MessageInfo)

	// Only update SourceId and TargetId, preserve PGN and Priority
	info.SourceId = source
	info.TargetId = destination

	// Set the updated MessageInfo back
	infoField.Set(reflect.ValueOf(info))

	return nil
}
