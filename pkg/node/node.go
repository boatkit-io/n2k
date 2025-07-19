package node

import (
	"context"
	"fmt"
	"io"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/subscribe"
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
	SetSupportedPGNs(transmit, receive []uint32)
	SetHeartbeatInterval(interval time.Duration)
	EnableHeartbeat(enable bool)
}

// Subscriber is an interface that abstracts the subscribe.SubscribeManager for testing.
type Subscriber interface {
	SubscribeToStruct(t any, callback any) (subscribe.SubscriptionId, error)
	Unsubscribe(subId subscribe.SubscriptionId) error
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
	productInfo       ProductInfo
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
	subscriptions     []subscribe.SubscriptionId
	pgnIn             chan any
	mutex             sync.RWMutex
	wakeUp            chan struct{}
	logger            *log.Logger
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
		subscriptions:     make([]subscribe.SubscriptionId, 0),
		pgnIn:             make(chan any, 10),
		mutex:             sync.RWMutex{},
		wakeUp:            make(chan struct{}, 1),
		logger:            log.New(io.Discard, "node | ", log.Ltime|log.Lmicroseconds),
	}
}

// SetLogger allows overriding the default logger for debugging.
// This method is not part of the Node interface.
func (n *node) SetLogger(logger *log.Logger) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.logger = logger
}

func (n *node) Start() error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if n.started {
		return fmt.Errorf("node already started")
	}

	sub, err := n.subscriber.SubscribeToStruct(pgn.IsoRequest{}, n.enqueuePgn)
	if err != nil {
		return fmt.Errorf("failed to subscribe to IsoRequest: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.IsoAddressClaim{}, n.enqueuePgn)
	if err != nil {
		return fmt.Errorf("failed to subscribe to IsoAddressClaim: %w", err)
	}
	n.subscriptions = append(n.subscriptions, sub)

	sub, err = n.subscriber.SubscribeToStruct(pgn.IsoCommandedAddress{}, n.enqueuePgn)
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
	n.subscriptions = make([]subscribe.SubscriptionId, 0)

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
		return fmt.Errorf("cannot write PGNs until address is claimed")
	}

	err := setMessageInfo(pgnStruct, networkAddress, destination)
	if err != nil {
		return fmt.Errorf("failed to set message info on PGN: %w", err)
	}

	n.logger.Printf("write: writing PGN %T to %d", pgnStruct, destination)
	return publisher.Write(pgnStruct)
}

func (n *node) SetDeviceInfo(info DeviceInfo) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	name, err := computeName(info)
	if err != nil {
		return err
	}
	n.deviceInfo = info
	n.name = name
	return nil
}

func (n *node) SetProductInfo(info ProductInfo) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.productInfo = info
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
	n.logger.Printf("enqueuePgn: received %T", p)
	select {
	case n.pgnIn <- p:
	case <-n.ctx.Done():
		n.logger.Printf("enqueuePgn: context done, dropping PGN")
	}
}

func (n *node) processIsoRequest(req *pgn.IsoRequest) []toSend {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	if req.Pgn == nil {
		return nil
	}

	n.logger.Printf("processIsoRequest: processing request for PGN %d from source %d", *req.Pgn, req.Info.SourceId)

	var responses []toSend

	switch *req.Pgn {
	case 126996:
		version := float32(n.productInfo.NMEA2000Version) / 100.0
		responsePgn := &pgn.ProductInformation{
			Info: pgn.MessageInfo{
				PGN:      126996,
				SourceId: n.networkAddress,
				TargetId: req.Info.SourceId,
			},
			Nmea2000Version:     &version,
			ProductCode:         &n.productInfo.ProductCode,
			ModelId:             n.productInfo.ModelID,
			SoftwareVersionCode: n.productInfo.SoftwareVersionCode,
			ModelVersion:        n.productInfo.ModelVersion,
			ModelSerialCode:     n.productInfo.ModelSerialCode,
			CertificationLevel:  &n.productInfo.CertificationLevel,
			LoadEquivalency:     &n.productInfo.LoadEquivalency,
		}
		responses = append(responses, toSend{pgn: responsePgn, dest: req.Info.SourceId})

	case 126464:
		txRepeating := make([]pgn.PgnListTransmitAndReceiveRepeating1, len(n.transmitPGNs))
		for i, pgn_num := range n.transmitPGNs {
			p := pgn_num
			txRepeating[i] = pgn.PgnListTransmitAndReceiveRepeating1{Pgn: &p}
		}
		txResponse := &pgn.PgnListTransmitAndReceive{
			Info: pgn.MessageInfo{
				PGN:      126464,
				SourceId: n.networkAddress,
				TargetId: req.Info.SourceId,
			},
			FunctionCode: pgn.TransmitPGNList,
			Repeating1:   txRepeating,
		}
		responses = append(responses, toSend{pgn: txResponse, dest: req.Info.SourceId})

		rxRepeating := make([]pgn.PgnListTransmitAndReceiveRepeating1, len(n.receivePGNs))
		for i, pgn_num := range n.receivePGNs {
			p := pgn_num
			rxRepeating[i] = pgn.PgnListTransmitAndReceiveRepeating1{Pgn: &p}
		}
		rxResponse := &pgn.PgnListTransmitAndReceive{
			Info: pgn.MessageInfo{
				PGN:      126464,
				SourceId: n.networkAddress,
				TargetId: req.Info.SourceId,
			},
			FunctionCode: pgn.ReceivePGNList,
			Repeating1:   rxRepeating,
		}
		responses = append(responses, toSend{pgn: rxResponse, dest: req.Info.SourceId})
	}

	return responses
}

func (n *node) processIsoCommandedAddress(cmd *pgn.IsoCommandedAddress) {
	n.mutex.RLock()
	currentName := n.name
	n.mutex.RUnlock()

	cmdName, err := computeNameFromCommand(cmd)
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

func (n *node) processIsoAddressClaim(claim *pgn.IsoAddressClaim) {
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

	incomingName, err := computeNameFromClaim(claim)
	if err != nil {
		return
	}

	if incomingName < currentName {
		n.mutex.Lock()
		n.addressState = stateLost
		n.addressClaimed = false
		n.mutex.Unlock()
	}
}

func (n *node) sendAddressClaim() {
	n.mutex.RLock()
	deviceInfoCopy := n.deviceInfo
	networkAddressCopy := n.networkAddress
	publisher := n.publisher
	n.mutex.RUnlock()

	arbitraryBit := uint8(0)
	if deviceInfoCopy.ArbitraryAddressCapable {
		arbitraryBit = 1
	}

	claim := &pgn.IsoAddressClaim{
		Info: pgn.MessageInfo{
			PGN:      60928,
			SourceId: networkAddressCopy,
			TargetId: 255,
		},
		UniqueNumber:            &deviceInfoCopy.UniqueNumber,
		ManufacturerCode:        deviceInfoCopy.ManufacturerCode,
		DeviceInstanceLower:     &deviceInfoCopy.DeviceInstanceLower,
		DeviceInstanceUpper:     &deviceInfoCopy.DeviceInstanceUpper,
		DeviceFunction:          deviceInfoCopy.DeviceFunction,
		DeviceClass:             deviceInfoCopy.DeviceClass,
		SystemInstance:          &deviceInfoCopy.SystemInstance,
		IndustryGroup:           deviceInfoCopy.IndustryGroup,
		ArbitraryAddressCapable: &arbitraryBit,
	}
	n.logger.Printf("sendAddressClaim: sending claim for address %d", networkAddressCopy)
	_ = publisher.Write(claim)
}

func (n *node) sendHeartbeat() {
	n.mutex.RLock()
	heartbeatSeqCopy := n.heartbeatSeq
	networkAddressCopy := n.networkAddress
	n.mutex.RUnlock()

	hb := &pgn.Heartbeat{
		Info: pgn.MessageInfo{
			PGN:      126993,
			SourceId: networkAddressCopy,
		},
		SequenceCounter:  &heartbeatSeqCopy,
		Controller1State: pgn.ErrorActive,
		Controller2State: pgn.ErrorActive,
		EquipmentStatus:  pgn.Operational,
	}

	_ = n.Write(hb)

	n.mutex.Lock()
	n.heartbeatSeq++
	n.mutex.Unlock()
}

func (n *node) process() {
	defer n.wg.Done()
	n.logger.Printf("process: goroutine started")
	defer n.logger.Printf("process: goroutine stopped")

	var claimTicker Ticker
	var heartbeatTicker Ticker

	for {
		var initialHeartbeat bool
		var shouldSendClaim bool
		n.mutex.Lock()
		n.logger.Printf("process: loop start, state=%d", n.addressState)
		switch n.addressState {
		case stateClaiming:
			if claimTicker == nil {
				claimTicker = n.clock.NewTicker(250 * time.Millisecond)
				n.networkAddress = n.preferredAddress
				shouldSendClaim = true
			}
		case stateLost:
			if claimTicker != nil {
				claimTicker.Stop()
				claimTicker = nil
			}
			n.preferredAddress++
			if n.preferredAddress > 253 {
				n.preferredAddress = 128
			}
			n.addressState = stateClaiming
			n.mutex.Unlock()
			continue
		case stateClaimed:
			if claimTicker != nil {
				claimTicker.Stop()
				claimTicker = nil
			}
			if n.heartbeatEnabled && heartbeatTicker == nil {
				heartbeatTicker = n.clock.NewTicker(n.heartbeatInterval)
				initialHeartbeat = true // Signal to send heartbeat after unlock
			}
			if !n.heartbeatEnabled && heartbeatTicker != nil {
				heartbeatTicker.Stop()
				heartbeatTicker = nil
			}
		}
		n.mutex.Unlock()

		if initialHeartbeat {
			n.sendHeartbeat()
		}
		if shouldSendClaim {
			n.sendAddressClaim()
		}

		var claimTickerC <-chan time.Time
		if claimTicker != nil {
			claimTickerC = claimTicker.C()
		}
		var heartbeatTickerC <-chan time.Time
		if heartbeatTicker != nil {
			heartbeatTickerC = heartbeatTicker.C()
		}

		select {
		case <-n.ctx.Done():
			if claimTicker != nil {
				claimTicker.Stop()
			}
			if heartbeatTicker != nil {
				heartbeatTicker.Stop()
			}
			n.logger.Printf("process: <-ctx.Done()")
			return
		case <-n.wakeUp:
			n.logger.Printf("process: <-wakeUp")
			continue
		case <-claimTickerC:
			n.logger.Printf("process: <-claimTickerC")
			n.mutex.Lock()
			if n.addressState == stateClaiming {
				n.addressState = stateClaimed
				n.addressClaimed = true
			}
			n.mutex.Unlock()
		case <-heartbeatTickerC:
			n.logger.Printf("process: <-heartbeatTickerC")
			n.sendHeartbeat()
		case p := <-n.pgnIn:
			n.logger.Printf("process: <-pgnIn, received %T", p)
			switch pgn := p.(type) {
			case *pgn.IsoRequest:
				responses := n.processIsoRequest(pgn)
				for _, r := range responses {
					_ = n.write(r.pgn, r.dest)
				}
			case *pgn.IsoAddressClaim:
				n.processIsoAddressClaim(pgn)
			case *pgn.IsoCommandedAddress:
				n.processIsoCommandedAddress(pgn)
			}
		}
	}
}

func computeName(d DeviceInfo) (uint64, error) {
	if d.UniqueNumber > 0x1FFFFF {
		return 0, fmt.Errorf("unique number (%d) exceeds 21-bit limit", d.UniqueNumber)
	}
	if d.ManufacturerCode > 0x7FF {
		return 0, fmt.Errorf("manufacturer code (%d) exceeds 11-bit limit", d.ManufacturerCode)
	}
	if d.DeviceInstanceLower > 0x7 {
		return 0, fmt.Errorf("device instance lower (%d) exceeds 3-bit limit", d.DeviceInstanceLower)
	}
	if d.DeviceInstanceUpper > 0x1F {
		return 0, fmt.Errorf("device instance upper (%d) exceeds 5-bit limit", d.DeviceInstanceUpper)
	}
	if d.DeviceFunction > 0xFF {
		return 0, fmt.Errorf("device function (%d) exceeds 8-bit limit", d.DeviceFunction)
	}
	if d.DeviceClass > 0x7F {
		return 0, fmt.Errorf("device class (%d) exceeds 7-bit limit", d.DeviceClass)
	}
	if d.SystemInstance > 0xF {
		return 0, fmt.Errorf("system instance (%d) exceeds 4-bit limit", d.SystemInstance)
	}
	if d.IndustryGroup > 0x7 {
		return 0, fmt.Errorf("industry group (%d) exceeds 3-bit limit", d.IndustryGroup)
	}

	name := uint64(d.UniqueNumber) |
		(uint64(d.ManufacturerCode) << 21) |
		(uint64(d.DeviceInstanceLower) << 32) |
		(uint64(d.DeviceInstanceUpper) << 35) |
		(uint64(d.DeviceFunction) << 40) |
		(uint64(0) << 48) |
		(uint64(d.DeviceClass) << 49) |
		(uint64(d.SystemInstance) << 56) |
		(uint64(d.IndustryGroup) << 60)

	if d.ArbitraryAddressCapable {
		name |= (1 << 63)
	}

	return name, nil
}

func computeNameFromClaim(claim *pgn.IsoAddressClaim) (uint64, error) {
	if claim.UniqueNumber == nil || claim.DeviceInstanceLower == nil || claim.DeviceInstanceUpper == nil || claim.SystemInstance == nil || claim.ArbitraryAddressCapable == nil {
		return 0, fmt.Errorf("invalid claim: missing required fields")
	}
	info := DeviceInfo{
		UniqueNumber:            *claim.UniqueNumber,
		ManufacturerCode:        claim.ManufacturerCode,
		DeviceInstanceLower:     *claim.DeviceInstanceLower,
		DeviceInstanceUpper:     *claim.DeviceInstanceUpper,
		DeviceFunction:          claim.DeviceFunction,
		DeviceClass:             claim.DeviceClass,
		SystemInstance:          *claim.SystemInstance,
		IndustryGroup:           claim.IndustryGroup,
		ArbitraryAddressCapable: *claim.ArbitraryAddressCapable == 1,
	}
	return computeName(info)
}

func computeNameFromCommand(cmd *pgn.IsoCommandedAddress) (uint64, error) {
	if cmd.UniqueNumber == nil || cmd.DeviceInstanceLower == nil || cmd.DeviceInstanceUpper == nil || cmd.SystemInstance == nil {
		return 0, fmt.Errorf("invalid command: missing required fields")
	}

	var uniqueNum uint32
	if len(cmd.UniqueNumber) < 3 {
		return 0, fmt.Errorf("invalid UniqueNumber length in command")
	}
	uniqueNum = uint32(cmd.UniqueNumber[0]) | (uint32(cmd.UniqueNumber[1]) << 8) | (uint32(cmd.UniqueNumber[2]) << 16)
	uniqueNum &= 0x1FFFFF

	name := uint64(uniqueNum) |
		(uint64(cmd.ManufacturerCode) << 21) |
		(uint64(*cmd.DeviceInstanceLower) << 32) |
		(uint64(*cmd.DeviceInstanceUpper) << 35) |
		(uint64(cmd.DeviceFunction) << 40) |
		(uint64(0) << 48) |
		(uint64(cmd.DeviceClass) << 49) |
		(uint64(*cmd.SystemInstance) << 56) |
		(uint64(cmd.IndustryCode) << 60)

	return name, nil
}

func setMessageInfo(s any, source, destination uint8) error {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("input is not a pointer to a struct")
	}

	infoField := v.Elem().FieldByName("Info")
	if !infoField.IsValid() || !infoField.CanSet() {
		return fmt.Errorf("struct does not have a settable 'Info' field")
	}

	if infoField.Type() != reflect.TypeOf(pgn.MessageInfo{}) {
		return fmt.Errorf("'Info' field is not of type pgn.MessageInfo")
	}

	info := infoField.Interface().(pgn.MessageInfo)
	info.SourceId = source
	info.TargetId = destination
	infoField.Set(reflect.ValueOf(info))

	return nil
}
