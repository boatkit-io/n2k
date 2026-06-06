package node

import (
	"context"
	"fmt"
	"reflect"
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
	Write(pgnStruct any) error
	WriteTo(pgnStruct any, destination uint8) error
	SetDeviceInfo(info DeviceInfo) error
	SetProductInfo(info ProductInfo)
	SetConfigurationProvider(provider ConfigurationProvider)
	SetSupportedPGNs(transmit, receive []uint32)
	SetHeartbeatInterval(interval time.Duration)
	EnableHeartbeat(enable bool)
}

// Subscriber is an interface that abstracts bus subscriptions for testing.
type Subscriber interface {
	SubscribeToStruct(t any, callback any) (SubscriptionId, error)
	Unsubscribe(subId SubscriptionId) error
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

type addressState uint8

const (
	stateUnclaimed addressState = iota
	stateClaiming
	stateClaimed
	stateLost
)

// node is the internal implementation of the Node interface.
type node struct {
	subscriber        Subscriber
	publisher         Publisher
	clock             Clock
	deviceInfo        DeviceInfo
	deviceInfoSet     bool
	productInfo       ProductInfo
	configProvider    ConfigurationProvider
	name              uint64
	networkAddress    uint8
	preferredAddress  uint8
	addressClaimed    bool
	addressState      addressState
	transmitPGNs      []uint32
	receivePGNs       []uint32
	heartbeatEnabled  bool
	heartbeatInterval time.Duration
	heartbeatSeq      uint8
	started           bool
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	subscriptions     []SubscriptionId
	pgnIn             chan any
	mutex             sync.RWMutex
	wakeUp            chan struct{}
	logger            *logrus.Logger
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
		subscriber:        subscriber,
		publisher:         publisher,
		clock:             clock,
		name:              0,
		networkAddress:    255,
		preferredAddress:  128,
		addressClaimed:    false,
		heartbeatEnabled:  false,
		heartbeatInterval: 60 * time.Second,
		started:           false,
		subscriptions:     make([]SubscriptionId, 0),
		pgnIn:             make(chan any, 10),
		mutex:             sync.RWMutex{},
		wakeUp:            make(chan struct{}, 1),
		logger:            logrus.New(),
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

func (n *node) handleIsoAddressClaim(p pgn.IsoAddressClaim) {
	n.logger.Infof("handleIsoAddressClaim: received address claim from source %d", p.Info.SourceId)
	n.enqueuePgn(p)
}

func (n *node) handleIsoCommandedAddress(p pgn.IsoCommandedAddress) {
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
		_ = n.subscriber.Unsubscribe(sub)
	}
	n.subscriptions = make([]SubscriptionId, 0)

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
	// Reset the context so the node can be restarted.
	n.ctx = nil
	n.cancel = nil
	n.mutex.Unlock()

	return nil
}

func (n *node) ClaimAddress(preferredAddress uint8) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if !n.deviceInfoSet {
		return fmt.Errorf("cannot claim address, device info has not been set")
	}
	if preferredAddress > 253 {
		return fmt.Errorf("preferred address %d is out of range (0-253)", preferredAddress)
	}
	n.preferredAddress = preferredAddress
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
	n.mutex.RUnlock()

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

func (n *node) SetProductInfo(info ProductInfo) {
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
	n.mutex.RUnlock()

	if req.Pgn == nil {
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
		txRepeating := make([]pgn.PgnListTransmitAndReceiveRepeating1, len(transmitPGNs))
		for i, pgn_num := range transmitPGNs {
			p := pgn_num
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
		for i, pgn_num := range receivePGNs {
			p := pgn_num
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

func (n *node) processIsoCommandedAddress(cmd pgn.IsoCommandedAddress) {
	n.mutex.RLock()
	currentName := n.name
	n.mutex.RUnlock()

	cmdName, err := computeNameFromCommand(&cmd)
	if err != nil {
		return
	}

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

func (n *node) processIsoAddressClaim(claim pgn.IsoAddressClaim) {
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

	incomingName, err := computeNameFromClaim(&claim)
	if err != nil {
		n.logger.Infof("processIsoAddressClaim: failed to compute incoming NAME: %v", err)
		return
	}

	n.logger.Infof("processIsoAddressClaim: comparing NAMEs - incoming: %x, ours: %x",
		incomingName, currentName)

	if incomingName < currentName {
		n.logger.Infof("processIsoAddressClaim: incoming NAME has higher priority, yielding address")
		n.mutex.Lock()
		n.addressState = stateLost
		n.addressClaimed = false
		n.mutex.Unlock()
	} else {
		n.logger.Infof("processIsoAddressClaim: our NAME has higher priority, keeping address")
	}
}

func (n *node) sendAddressClaim() {
	n.mutex.RLock()
	deviceInfoCopy := n.deviceInfo
	networkAddressCopy := n.networkAddress
	publisher := n.publisher
	n.mutex.RUnlock()

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
			if claimTicker == nil {
				claimTicker = n.clock.NewTicker(250 * time.Millisecond)
				n.networkAddress = n.preferredAddress
				shouldSendClaim = true
			}
		} else {
			if claimTicker != nil {
				claimTicker.Stop()
				claimTicker = nil
			}
		}

		if n.heartbeatEnabled && n.addressClaimed {
			if heartbeatTicker == nil {
				heartbeatTicker = n.clock.NewTicker(n.heartbeatInterval)
				initialHeartbeat = true
			}
		} else {
			if heartbeatTicker != nil {
				heartbeatTicker.Stop()
				heartbeatTicker = nil
			}
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
			var toSendList []toSend
			switch v := p.(type) {
			case pgn.IsoRequest:
				toSendList = n.processIsoRequest(v)
			case pgn.IsoAddressClaim:
				n.processIsoAddressClaim(v)
			case pgn.IsoCommandedAddress:
				n.processIsoCommandedAddress(v)
			default:
				n.logger.Infof("process: received unhandled PGN type %T", p)
			}

			if len(toSendList) > 0 {
				n.mutex.RLock()
				publisher := n.publisher
				n.mutex.RUnlock()
				for _, ts := range toSendList {
					n.logger.Infof("process: sending PGN %+v to %d", ts.pgn, ts.dest)
					_ = publisher.Write(ts.pgn)
				}
			}

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

func computeNameFromClaim(claim *pgn.IsoAddressClaim) (uint64, error) {
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
	return name, nil
}

func computeNameFromCommand(cmd *pgn.IsoCommandedAddress) (uint64, error) {
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
	return name, nil
}

// setMessageInfo uses reflection to set the "Info" field on a PGN struct.
// This is a helper to avoid repetitive code when sending PGNs.
func setMessageInfo(s any, source, destination uint8) error {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
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
