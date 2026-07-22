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
	"sync/atomic"
	"time"

	internalpgn "github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/sirupsen/logrus"
)

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

// nodePGNQueueSize accommodates discovery responses from a full 254-address NMEA 2000 network.
const nodePGNQueueSize = 2048

// Node represents a generic NMEA 2000 device, handling standard behaviors
// required for any device on the network.
type Node struct {
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
	pgnQueueDropped                atomic.Uint64
	mutex                          sync.RWMutex
	wakeUp                         chan struct{}
	logger                         *logrus.Logger
}

type toSend struct {
	pgn  any
	dest uint8
}

// NewNode creates a new Node instance with the given dependencies.
func NewNode(subscriber Subscriber, publisher Publisher, clock Clock) *Node {
	if clock == nil {
		clock = NewRealClock()
	}
	return &Node{
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
		pgnIn:                          make(chan any, nodePGNQueueSize),
		mutex:                          sync.RWMutex{},
		wakeUp:                         make(chan struct{}, 1),
		logger:                         logrus.New(),
	}
}

// SetLogger allows overriding the default logger for debugging.
func (n *Node) SetLogger(logger *logrus.Logger) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.logger = logger
}

func (n *Node) handleIsoRequest(p pgn.ISORequest) {
	n.enqueuePgn(p)
}

func (n *Node) handleIsoAddressClaim(p pgn.ISOAddressClaim) { //nolint:gocritic // Subscriber callbacks must accept value PGNs.
	n.enqueuePgn(p)
}

func (n *Node) handleIsoCommandedAddress(p pgn.ISOCommandedAddress) { //nolint:gocritic // Subscriber callbacks must accept value PGNs.
	n.enqueuePgn(p)
}

func (n *Node) handleIsoAcknowledgement(p pgn.ISOAcknowledgement) {
	n.enqueuePgn(p)
}

//nolint:gocritic // Subscriber callbacks must accept value PGNs.
func (n *Node) handleNmeaRequestGroupFunction(p pgn.NMEARequestGroupFunction) {
	n.enqueuePgn(p)
}

//nolint:gocritic // Subscriber callbacks must accept value PGNs.
func (n *Node) handleNmeaCommandGroupFunction(p pgn.NMEACommandGroupFunction) {
	requestedPGN := uint32(0)
	if p.PGN != nil {
		requestedPGN = *p.PGN
	}
	n.logger.Infof(
		"received NMEA Command Group Function: source=0x%02x target=0x%02x pgn=%d parameters=%d",
		p.Info.SourceId, p.Info.TargetId, requestedPGN, len(p.Repeating1),
	)
	n.enqueuePgn(p)
}

//nolint:gocritic // Subscriber callbacks must accept value PGNs.
func (n *Node) handleNmeaWriteFieldsGroupFunction(p pgn.NMEAWriteFieldsGroupFunction) {
	n.logger.Infof(
		"received NMEA Write Fields: source=0x%02x target=0x%02x pgn=%v parameters=%d",
		p.Info.SourceId, p.Info.TargetId, p.PGN, len(p.Repeating2),
	)
	n.enqueuePgn(p)
}

//nolint:gocritic // Subscriber callbacks must accept value PGNs.
func (n *Node) handleNmeaReadFieldsGroupFunction(p pgn.NMEAReadFieldsGroupFunction) {
	n.logger.Infof(
		"received NMEA Read Fields: source=0x%02x target=0x%02x pgn=%v parameters=%d",
		p.Info.SourceId, p.Info.TargetId, p.PGN, len(p.Repeating2),
	)
	n.enqueuePgn(p)
}

func (n *Node) handleProductInformation(p pgn.ProductInformation) { //nolint:gocritic // Subscriber callbacks must accept value PGNs.
	n.enqueuePgn(p)
}

//nolint:gocritic // Subscriber callbacks must accept value PGNs.
func (n *Node) handleConfigurationInformation(p pgn.ConfigurationInformation) {
	n.enqueuePgn(p)
}

func (n *Node) handlePgnListTransmitAndReceive(p pgn.PGNListTransmitAndReceive) {
	n.enqueuePgn(p)
}

// Start subscribes to required management PGNs and starts the node processing loop.
func (n *Node) Start() error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if n.started {
		return fmt.Errorf("node already started")
	}

	sub, err := n.subscriber.SubscribeToStruct(pgn.ISORequest{}, n.handleIsoRequest)
	if err != nil {
		return fmt.Errorf("failed to subscribe to ISORequest: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.ISOAddressClaim{}, n.handleIsoAddressClaim)
	if err != nil {
		return fmt.Errorf("failed to subscribe to ISOAddressClaim: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.ISOCommandedAddress{}, n.handleIsoCommandedAddress)
	if err != nil {
		return fmt.Errorf("failed to subscribe to ISOCommandedAddress: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.ISOAcknowledgement{}, n.handleIsoAcknowledgement)
	if err != nil {
		return fmt.Errorf("failed to subscribe to ISOAcknowledgement: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.NMEARequestGroupFunction{}, n.handleNmeaRequestGroupFunction)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NMEARequestGroupFunction: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.NMEACommandGroupFunction{}, n.handleNmeaCommandGroupFunction)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NMEACommandGroupFunction: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.NMEAWriteFieldsGroupFunction{}, n.handleNmeaWriteFieldsGroupFunction)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NMEAWriteFieldsGroupFunction: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.NMEAReadFieldsGroupFunction{}, n.handleNmeaReadFieldsGroupFunction)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NMEAReadFieldsGroupFunction: %w", err)
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

	sub, err = n.subscriber.SubscribeToStruct(pgn.PGNListTransmitAndReceive{}, n.handlePgnListTransmitAndReceive)
	if err != nil {
		return fmt.Errorf("failed to subscribe to PGNListTransmitAndReceive: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	n.ctx, n.cancel = context.WithCancel(context.Background())
	n.started = true

	n.wg.Add(1)
	go n.process()

	return nil
}

// Stop unsubscribes from bus traffic and stops the node processing loop.
func (n *Node) Stop() error {
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

// ClaimAddress begins claiming preferredAddress or enters read-only mode for ReadOnlyAddress.
func (n *Node) ClaimAddress(preferredAddress uint8) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if preferredAddress == ReadOnlyAddress {
		n.logger.Infof("N2K address claim disabled; entering passive mode")
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
	n.logger.Infof("starting N2K address claim for preferred address %d", preferredAddress)
	n.preferredAddress = preferredAddress
	n.readOnly = false
	n.addressState = stateClaiming
	select {
	case n.wakeUp <- struct{}{}:
	default:
	}
	return nil
}

// GetNetworkAddress returns the node's current NMEA 2000 source address.
func (n *Node) GetNetworkAddress() uint8 {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	return n.networkAddress
}

// IsAddressClaimed reports whether the node currently owns its source address.
func (n *Node) IsAddressClaimed() bool {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	return n.addressClaimed
}

// KnownDevices returns the devices currently observed by this node.
func (n *Node) KnownDevices() []KnownDevice {
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

// SubscribeToDeviceChanges registers callback for observed device changes.
func (n *Node) SubscribeToDeviceChanges(callback func(DeviceChange)) SubscriptionID {
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

// UnsubscribeDeviceChanges removes a device-change subscription.
func (n *Node) UnsubscribeDeviceChanges(subID SubscriptionID) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if _, ok := n.deviceChangeSubscribers[subID]; !ok {
		return fmt.Errorf("device change subscription not found")
	}
	delete(n.deviceChangeSubscribers, subID)
	return nil
}

// Write publishes pgnStruct using the node's claimed source address.
func (n *Node) Write(pgnStruct any) error {
	return n.write(pgnStruct, 255)
}

// WriteTo publishes pgnStruct to destination using the node's claimed source address.
func (n *Node) WriteTo(pgnStruct any, destination uint8) error {
	return n.write(pgnStruct, destination)
}

func (n *Node) write(pgnStruct any, destination uint8) error {
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

// SetDeviceInfo configures the fields used to compute this node's NAME.
func (n *Node) SetDeviceInfo(info DeviceInfo) error {
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

// SetProductInfo configures the product metadata returned for standard requests.
func (n *Node) SetProductInfo(info ProductInfo) { //nolint:gocritic // API accepts value configuration.
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.productInfo = info
}

// SetConfigurationProvider configures the source for device configuration metadata.
func (n *Node) SetConfigurationProvider(provider ConfigurationProvider) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.configProvider = provider
}

// SetSupportedPGNs configures the PGN lists reported by this node.
func (n *Node) SetSupportedPGNs(transmit, receive []uint32) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.transmitPGNs = transmit
	n.receivePGNs = receive
}

// SetHeartbeatInterval configures how often enabled heartbeat messages are sent.
func (n *Node) SetHeartbeatInterval(interval time.Duration) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.heartbeatInterval = interval
	select {
	case n.wakeUp <- struct{}{}:
	default:
	}
}

// EnableHeartbeat controls whether this node sends heartbeat messages.
func (n *Node) EnableHeartbeat(enable bool) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.heartbeatEnabled = enable
	select {
	case n.wakeUp <- struct{}{}:
	default:
	}
}

func (n *Node) enqueuePgn(p any) {
	if n.ctx != nil {
		select {
		case <-n.ctx.Done():
			n.logger.Debugf("node context done, dropping PGN %T", p)
			return
		default:
		}
	}
	select {
	case n.pgnIn <- p:
	default:
		dropped := n.pgnQueueDropped.Add(1)
		if dropped == 1 || dropped%100 == 0 {
			n.logger.WithFields(logrus.Fields{
				"capacity":     cap(n.pgnIn),
				"depth":        len(n.pgnIn),
				"droppedTotal": dropped,
				"pgnType":      fmt.Sprintf("%T", p),
			}).Warn("dropping N2K node-management PGN because its queue is full")
		}
	}
}

func (n *Node) processIsoRequest(req pgn.ISORequest) []toSend {
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

	if req.PGN == nil {
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

	var responses []toSend

	switch *req.PGN {
	case pgn.ISOAddressClaimPGN:
		responses = append(responses, toSend{pgn: buildAddressClaim(deviceInfoCopy, networkAddress), dest: req.Info.SourceId})

	case pgn.ProductInformationPGN:
		version := float32(productInfoCopy.NMEA2000Version) / 100.0
		productCode := productInfoCopy.ProductCode
		loadEquivalency := productInfoCopy.LoadEquivalency
		responsePgn := &pgn.ProductInformation{
			Info: pgn.MessageInfo{
				PGN:      pgn.ProductInformationPGN,
				SourceId: networkAddress,
				TargetId: req.Info.SourceId,
			},
			NMEA2000Version:     &version,
			ProductCode:         &productCode,
			ModelID:             productInfoCopy.ModelID,
			SoftwareVersionCode: productInfoCopy.SoftwareVersionCode,
			ModelVersion:        productInfoCopy.ModelVersion,
			ModelSerialCode:     productInfoCopy.ModelSerialCode,
			CertificationLevel:  pgn.CertificationLevelConst(productInfoCopy.CertificationLevel),
			LoadEquivalency:     &loadEquivalency,
		}
		responses = append(responses, toSend{pgn: responsePgn, dest: req.Info.SourceId})

	case pgn.PGNListTransmitAndReceivePGN:
		transmitPGNs = managedTransmitPGNs(transmitPGNs, configProvider != nil, heartbeatEnabled)
		receivePGNs = managedReceivePGNs(receivePGNs)

		txRepeating := make([]pgn.PGNListTransmitAndReceiveRepeating1, len(transmitPGNs))
		for i, pgnNum := range transmitPGNs {
			p := pgnNum
			txRepeating[i] = pgn.PGNListTransmitAndReceiveRepeating1{PGN: &p}
		}
		txResponse := &pgn.PGNListTransmitAndReceive{
			Info: pgn.MessageInfo{
				PGN:      pgn.PGNListTransmitAndReceivePGN,
				SourceId: networkAddress,
				TargetId: req.Info.SourceId,
			},
			FunctionCode: pgn.TransmitPGNList,
			Repeating1:   txRepeating,
		}
		responses = append(responses, toSend{pgn: txResponse, dest: req.Info.SourceId})

		rxRepeating := make([]pgn.PGNListTransmitAndReceiveRepeating1, len(receivePGNs))
		for i, pgnNum := range receivePGNs {
			p := pgnNum
			rxRepeating[i] = pgn.PGNListTransmitAndReceiveRepeating1{PGN: &p}
		}
		rxResponse := &pgn.PGNListTransmitAndReceive{
			Info: pgn.MessageInfo{
				PGN:      pgn.PGNListTransmitAndReceivePGN,
				SourceId: networkAddress,
				TargetId: req.Info.SourceId,
			},
			FunctionCode: pgn.ReceivePGNList,
			Repeating1:   rxRepeating,
		}
		responses = append(responses, toSend{pgn: rxResponse, dest: req.Info.SourceId})

	case pgn.ConfigurationInformationPGN:
		if configProvider == nil {
			responses = append(responses, toSend{pgn: buildIsoNak(networkAddress, req.Info.SourceId, *req.PGN), dest: req.Info.SourceId})
			break
		}
		configInfo, err := configProvider.GetConfigurationInfo()
		if err != nil {
			n.logger.Errorf("processIsoRequest: failed to get configuration information: %v", err)
			responses = append(responses, toSend{pgn: buildIsoNak(networkAddress, req.Info.SourceId, *req.PGN), dest: req.Info.SourceId})
			break
		}
		responsePgn := &pgn.ConfigurationInformation{
			Info: pgn.MessageInfo{
				PGN:      pgn.ConfigurationInformationPGN,
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
		pgn.ISOAcknowledgementPGN,
		pgn.ISOAddressClaimPGN,
		pgn.NMEAAcknowledgeGroupFunctionPGN,
		pgn.PGNListTransmitAndReceivePGN,
		pgn.ProductInformationPGN,
	}
	if hasConfigurationProvider {
		managed = append(managed, pgn.ConfigurationInformationPGN)
	}
	if heartbeatEnabled {
		managed = append(managed, pgn.HeartbeatPGN)
	}
	return mergePGNs(configured, managed)
}

func managedReceivePGNs(configured []uint32) []uint32 {
	return mergePGNs(configured, []uint32{
		pgn.ISOAcknowledgementPGN,
		pgn.ISORequestPGN,
		pgn.ISOAddressClaimPGN,
		pgn.ISOCommandedAddressPGN,
		pgn.NMEARequestGroupFunctionPGN,
		pgn.NMEACommandGroupFunctionPGN,
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

func (n *Node) processNmeaRequestGroupFunction(req *pgn.NMEARequestGroupFunction) []toSend {
	if req.PGN == nil {
		return nil
	}
	if req.NumberOfParameters != nil && *req.NumberOfParameters != 0 {
		return n.processUnsupportedGroupFunction(req.Info, *req.PGN, pgn.ReadOrWriteNotSupported)
	}

	return n.processIsoRequest(pgn.ISORequest{
		Info: req.Info,
		PGN:  req.PGN,
	})
}

func (n *Node) processNmeaCommandGroupFunction(cmd *pgn.NMEACommandGroupFunction) []toSend {
	if cmd.PGN == nil {
		n.logger.Warn("ignoring NMEA Command Group Function without PGN")
		return nil
	}
	n.logger.Infof("processing NMEA command for PGN %d with %d parameters", *cmd.PGN, len(cmd.Repeating1))
	if *cmd.PGN == pgn.ConfigurationInformationPGN && len(cmd.Repeating1) > 0 {
		if len(cmd.Repeating1) > 255 {
			return n.processUnsupportedGroupFunction(cmd.Info, *cmd.PGN, pgn.InvalidParameterField)
		}
		parameters := make([]pgn.NMEAWriteFieldsGroupFunctionRepeating2, 0, len(cmd.Repeating1))
		for _, field := range cmd.Repeating1 {
			parameters = append(parameters, pgn.NMEAWriteFieldsGroupFunctionRepeating2(field))
		}
		count := uint8(len(parameters)) //nolint:gosec // Parameter count is bounded above.
		zero := uint8(0)
		write := pgn.NMEAWriteFieldsGroupFunction{
			Info: cmd.Info, FunctionCode: pgn.WriteFields, PGN: cmd.PGN,
			NumberOfSelectionPairs: &zero, NumberOfParameters: &count, Repeating2: parameters,
		}
		responses := n.processNmeaWriteFieldsGroupFunction(&write)
		if len(responses) == 0 {
			return nil
		}
		n.mutex.RLock()
		source := n.networkAddress
		n.mutex.RUnlock()
		ackParameters := make([]pgn.NMEAAcknowledgeGroupFunctionRepeating1, len(parameters))
		for i := range ackParameters {
			ackParameters[i].Parameter = pgn.Acknowledge_3
		}
		ack := &pgn.NMEAAcknowledgeGroupFunction{
			Info:         pgn.MessageInfo{PGN: pgn.NMEAAcknowledgeGroupFunctionPGN, SourceId: source, TargetId: cmd.Info.SourceId, Priority: 3},
			FunctionCode: pgn.Acknowledge_5, PGN: cmd.PGN, PGNErrorCode: pgn.Acknowledge_6, TransmissionIntervalPriorityErrorCode: pgn.Acknowledge_2,
			NumberOfParameters: &count,
			Repeating1:         ackParameters,
		}
		return []toSend{{pgn: ack, dest: cmd.Info.SourceId}}
	}
	return n.processUnsupportedGroupFunction(cmd.Info, *cmd.PGN, pgn.ReadOrWriteNotSupported)
}

func (n *Node) processNmeaWriteFieldsGroupFunction(req *pgn.NMEAWriteFieldsGroupFunction) []toSend {
	if req.PGN == nil || *req.PGN != pgn.ConfigurationInformationPGN {
		if req.PGN == nil {
			return nil
		}
		return n.processUnsupportedGroupFunction(req.Info, *req.PGN, pgn.ReadOrWriteNotSupported)
	}

	n.mutex.RLock()
	provider := n.configProvider
	addressClaimed := n.addressClaimed
	networkAddress := n.networkAddress
	readOnly := n.readOnly
	n.mutex.RUnlock()
	if readOnly || !addressClaimed || provider == nil || req.Info.TargetId != networkAddress || len(req.Repeating1) != 0 {
		n.logger.Infof(
			"rejecting configuration write: readOnly=%t claimed=%t provider=%t target=0x%02x node=0x%02x selections=%d",
			readOnly, addressClaimed, provider != nil, req.Info.TargetId, networkAddress, len(req.Repeating1),
		)
		return nil
	}

	info, err := provider.GetConfigurationInfo()
	if err != nil {
		return n.processUnsupportedGroupFunction(req.Info, *req.PGN, pgn.AccessDenied_2)
	}
	replyFields := make([]pgn.NMEAWriteFieldsReplyGroupFunctionRepeating2, 0, len(req.Repeating2))
	for _, field := range req.Repeating2 {
		if field.Parameter == nil {
			continue
		}
		value, decodeErr := decodeGroupFunctionLAU(field.Value)
		if decodeErr != nil {
			n.logger.Infof("invalid configuration value for parameter %d: bytes=%v error=%v", *field.Parameter, field.Value, decodeErr)
			return n.processUnsupportedGroupFunction(req.Info, *req.PGN, pgn.InvalidParameterField)
		}
		switch *field.Parameter {
		case 1:
			info.InstallationDescription1 = value
		case 2:
			info.InstallationDescription2 = value
		case 3:
			info.ManufacturerInformation = value
		default:
			return n.processUnsupportedGroupFunction(req.Info, *req.PGN, pgn.InvalidParameterField)
		}
		parameter := *field.Parameter
		replyFields = append(replyFields, pgn.NMEAWriteFieldsReplyGroupFunctionRepeating2{Parameter: &parameter, Value: field.Value})
	}
	if err := provider.SetConfigurationInfo(info); err != nil {
		return n.processUnsupportedGroupFunction(req.Info, *req.PGN, pgn.AccessDenied_2)
	}

	reply := &pgn.NMEAWriteFieldsReplyGroupFunction{
		Info: pgn.MessageInfo{
			PGN:      pgn.NMEAWriteFieldsReplyGroupFunctionPGN,
			SourceId: networkAddress, TargetId: req.Info.SourceId, Priority: req.Info.Priority,
		},
		FunctionCode: pgn.WriteFieldsReply, PGN: req.PGN, ManufacturerCode: req.ManufacturerCode,
		IndustryCode: req.IndustryCode, UniqueID: req.UniqueID, NumberOfSelectionPairs: req.NumberOfSelectionPairs,
		NumberOfParameters: req.NumberOfParameters,
		Repeating2:         replyFields,
	}
	return []toSend{{pgn: reply, dest: req.Info.SourceId}}
}

func (n *Node) processNmeaReadFieldsGroupFunction(req *pgn.NMEAReadFieldsGroupFunction) []toSend {
	if req.PGN == nil || *req.PGN != pgn.ConfigurationInformationPGN {
		return nil
	}
	n.mutex.RLock()
	provider, addressClaimed, networkAddress, readOnly := n.configProvider, n.addressClaimed, n.networkAddress, n.readOnly
	n.mutex.RUnlock()
	if readOnly || !addressClaimed || provider == nil || req.Info.TargetId != networkAddress || len(req.Repeating1) != 0 {
		return nil
	}
	info, err := provider.GetConfigurationInfo()
	if err != nil {
		return nil
	}
	values := map[uint8]string{1: info.InstallationDescription1, 2: info.InstallationDescription2, 3: info.ManufacturerInformation}
	reply := &pgn.NMEAReadFieldsReplyGroupFunction{
		Info: pgn.MessageInfo{
			PGN:      pgn.NMEAReadFieldsReplyGroupFunctionPGN,
			SourceId: networkAddress, TargetId: req.Info.SourceId, Priority: req.Info.Priority,
		},
		FunctionCode: pgn.ReadFieldsReply, PGN: req.PGN, ManufacturerCode: req.ManufacturerCode, IndustryCode: req.IndustryCode,
		UniqueID: req.UniqueID, NumberOfSelectionPairs: req.NumberOfSelectionPairs, NumberOfParameters: req.NumberOfParameters,
	}
	for _, field := range req.Repeating2 {
		if field.Parameter == nil {
			continue
		}
		value, ok := values[*field.Parameter]
		if !ok {
			return nil
		}
		parameter := *field.Parameter
		encoded, encodeErr := pgn.EncodeStringLAU(value)
		if encodeErr != nil {
			return n.processUnsupportedGroupFunction(req.Info, *req.PGN, pgn.InvalidParameterField)
		}
		reply.Repeating2 = append(reply.Repeating2, pgn.NMEAReadFieldsReplyGroupFunctionRepeating2{
			Parameter: &parameter,
			Value:     encoded,
		})
	}
	return []toSend{{pgn: reply, dest: req.Info.SourceId}}
}

func decodeGroupFunctionLAU(value []byte) (string, error) {
	return internalpgn.DecodeStringLAU(value)
}

func (n *Node) processUnsupportedGroupFunction(info pgn.MessageInfo, requestedPgn uint32, parameterError pgn.ParameterFieldConst) []toSend {
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

func (n *Node) processIsoCommandedAddress(cmd *pgn.ISOCommandedAddress) {
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

	n.logger.Infof("received commanded N2K address %d; restarting address claim", *cmd.NewSourceAddress)

	n.mutex.Lock()
	n.preferredAddress = *cmd.NewSourceAddress
	n.addressState = stateClaiming
	n.addressClaimed = false
	n.mutex.Unlock()
}

func (n *Node) processIsoAddressClaim(claim *pgn.ISOAddressClaim) {
	incomingName := computeNameFromClaim(claim)
	n.updateKnownDeviceFromClaim(claim, incomingName)

	n.mutex.RLock()
	currentState := n.addressState
	currentAddress := n.networkAddress
	currentName := n.name
	n.mutex.RUnlock()

	if currentState != stateClaiming && currentState != stateClaimed {
		return
	}

	if claim.Info.SourceId != currentAddress {
		return
	}

	if incomingName < currentName {
		n.logger.Warnf("N2K address %d conflict: device NAME %016x has higher priority than ours %016x; yielding",
			currentAddress, incomingName, currentName)
		n.mutex.Lock()
		n.addressClaimed = false
		if n.deviceInfo.ArbitraryAddressCapable {
			if nextAddress, ok := n.nextAvailableAddressLocked(currentAddress); ok {
				n.preferredAddress = nextAddress
				n.addressState = stateClaiming
				n.logger.Infof("retrying N2K address claim with address %d", nextAddress)
			} else {
				n.addressState = stateLost
				n.networkAddress = 255
				n.logger.Warnf("lost N2K address %d conflict and no available address found", currentAddress)
			}
		} else {
			n.addressState = stateLost
			n.networkAddress = 255
			n.logger.Warnf("lost N2K address %d conflict and this node cannot choose another address", currentAddress)
		}
		n.mutex.Unlock()
		select {
		case n.wakeUp <- struct{}{}:
		default:
		}
	} else {
		n.logger.Infof("N2K address %d conflict: our NAME %016x has higher priority than incoming %016x; reasserting claim",
			currentAddress, currentName, incomingName)
		n.sendAddressClaim()
	}
}

func (n *Node) updateKnownDeviceFromClaim(claim *pgn.ISOAddressClaim, name uint64) {
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

func (n *Node) updateKnownDeviceFromProductInfo(info *pgn.ProductInformation) {
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
	if info.NMEA2000Version != nil {
		nmea2000Version = uint16(*info.NMEA2000Version * 100)
	}
	loadEquivalency := uint8(0)
	if info.LoadEquivalency != nil {
		loadEquivalency = *info.LoadEquivalency
	}

	productInfo := ProductInfo{
		NMEA2000Version:     nmea2000Version,
		ProductCode:         productCode,
		ModelID:             info.ModelID,
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

func (n *Node) updateKnownDeviceFromConfigurationInfo(info *pgn.ConfigurationInformation) {
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

func (n *Node) updateKnownDeviceFromPgnList(info *pgn.PGNListTransmitAndReceive) {
	var changes []DeviceChange
	n.mutex.Lock()

	device := n.knownDeviceForAddressLocked(info.Info.SourceId)
	device.Address = info.Info.SourceId
	device.LastSeen = time.Now()

	pgns := knownDevicePGNListValues(info.Repeating1)
	var changedPGNs []uint32
	switch info.FunctionCode {
	case pgn.TransmitPGNList:
		if !reflect.DeepEqual(device.TransmitPGNs, pgns) {
			device.TransmitPGNs = pgns
			changedPGNs = append([]uint32(nil), pgns...)
		}
	case pgn.ReceivePGNList:
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

func (n *Node) nextAvailableAddressLocked(after uint8) (uint8, bool) {
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

func (n *Node) knownDeviceForAddressLocked(address uint8) KnownDevice {
	if name, ok := n.knownDeviceNamesByAddress[address]; ok {
		return n.knownDevices[name]
	}
	if device, ok := n.unknownKnownDevicesByAddress[address]; ok {
		return device
	}
	return KnownDevice{Address: address}
}

func (n *Node) setKnownDeviceForAddressLocked(address uint8, device *KnownDevice) {
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

func knownDevicePGNListValues(list []pgn.PGNListTransmitAndReceiveRepeating1) []uint32 {
	values := make([]uint32, 0, len(list))
	for _, item := range list {
		if item.PGN != nil {
			values = append(values, *item.PGN)
		}
	}
	return values
}

func ptrUint8(v uint8) *uint8 {
	return &v
}

func (n *Node) publishDeviceChanges(changes []DeviceChange) {
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

func (n *Node) sendAddressClaim() {
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
	n.logger.Infof("claiming N2K address %d", networkAddressCopy)
	if err := publisher.Write(claim); err != nil {
		n.logger.Errorf("failed to write N2K address claim: %v", err)
	}
}

func buildAddressClaim(deviceInfo DeviceInfo, networkAddress uint8) *pgn.ISOAddressClaim {
	arbitraryBit := uint8(0)
	if deviceInfo.ArbitraryAddressCapable {
		arbitraryBit = 1
	}

	return &pgn.ISOAddressClaim{
		Info: pgn.MessageInfo{
			PGN:      pgn.ISOAddressClaimPGN,
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
		ArbitraryAddressCapable: pgn.YesNo1BitConst(arbitraryBit),
	}
}

func buildIsoNak(source, destination uint8, requestedPgn uint32) *pgn.ISOAcknowledgement {
	return &pgn.ISOAcknowledgement{
		Info: pgn.MessageInfo{
			PGN:      pgn.ISOAcknowledgementPGN,
			SourceId: source,
			TargetId: destination,
			Priority: 6,
		},
		Control: pgn.Nak,
		PGN:     &requestedPgn,
	}
}

func buildNmeaGroupNak(
	source, destination uint8,
	requestedPgn uint32,
	parameterError pgn.ParameterFieldConst,
) *pgn.NMEAAcknowledgeGroupFunction {
	return &pgn.NMEAAcknowledgeGroupFunction{
		Info: pgn.MessageInfo{
			PGN:      pgn.NMEAAcknowledgeGroupFunctionPGN,
			SourceId: source,
			TargetId: destination,
			Priority: 3,
		},
		FunctionCode:                          pgn.Acknowledge_5,
		PGN:                                   &requestedPgn,
		PGNErrorCode:                          pgn.PGNNotSupported,
		TransmissionIntervalPriorityErrorCode: pgn.NotSupported,
		Repeating1: []pgn.NMEAAcknowledgeGroupFunctionRepeating1{
			{Parameter: parameterError},
		},
	}
}

func (n *Node) sendHeartbeat() {
	n.mutex.RLock()
	heartbeatSeqCopy := n.heartbeatSeq
	networkAddressCopy := n.networkAddress
	n.mutex.RUnlock()

	hb := &pgn.Heartbeat{
		Info: pgn.MessageInfo{
			PGN:      pgn.HeartbeatPGN,
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

func (n *Node) processPGN(p any) []toSend {
	switch v := p.(type) {
	case pgn.ISORequest:
		return n.processIsoRequest(v)
	case pgn.NMEARequestGroupFunction:
		return n.processNmeaRequestGroupFunction(&v)
	case pgn.NMEACommandGroupFunction:
		return n.processNmeaCommandGroupFunction(&v)
	case pgn.NMEAWriteFieldsGroupFunction:
		return n.processNmeaWriteFieldsGroupFunction(&v)
	case pgn.NMEAReadFieldsGroupFunction:
		return n.processNmeaReadFieldsGroupFunction(&v)
	case pgn.ISOAcknowledgement:
		n.logger.Debugf("received ISO acknowledgement for PGN %v", v.PGN)
	case pgn.ISOAddressClaim:
		n.processIsoAddressClaim(&v)
	case pgn.ISOCommandedAddress:
		n.processIsoCommandedAddress(&v)
	case pgn.ProductInformation:
		n.updateKnownDeviceFromProductInfo(&v)
	case pgn.ConfigurationInformation:
		n.updateKnownDeviceFromConfigurationInfo(&v)
	case pgn.PGNListTransmitAndReceive:
		n.updateKnownDeviceFromPgnList(&v)
	default:
		n.logger.Debugf("received unhandled PGN type %T", p)
	}
	return nil
}

func (n *Node) sendProcessResponses(toSendList []toSend) {
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
		n.logger.Debugf("sending node response PGN %T to %d", ts.pgn, ts.dest)
		if err := publisher.Write(ts.pgn); err != nil {
			n.logger.Errorf("failed to write node response PGN %T to %d: %v", ts.pgn, ts.dest, err)
		}
	}
}

func (n *Node) process() {
	defer n.wg.Done()
	n.logger.Debugf("node process goroutine started")
	defer n.logger.Debugf("node process goroutine stopped")

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
				n.logger.Debugf("node PGN channel closed")
				return
			}
			n.sendProcessResponses(n.processPGN(p))

		case <-claimTick:
			n.mutex.Lock()
			if n.addressState == stateClaiming {
				n.addressState = stateClaimed
				n.addressClaimed = true
				n.logger.Infof("claimed N2K address %d", n.networkAddress)
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

func computeNameFromClaim(claim *pgn.ISOAddressClaim) uint64 {
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
	if claim.ArbitraryAddressCapable == pgn.Yes_2 {
		name |= 1 << 63
	}
	return name
}

func computeNameFromCommand(cmd *pgn.ISOCommandedAddress) uint64 {
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
