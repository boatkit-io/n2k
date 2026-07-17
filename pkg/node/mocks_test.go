package node

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/pkg/pgn"
)

// mockSubscriber is a mock implementation of the Subscriber interface.
type mockSubscriber struct {
	subscriptions map[SubscriptionID]bool
	handlers      map[string]any // Store handlers of any func type
	nextSubID     SubscriptionID
	wg            sync.WaitGroup
}

func newMockSubscriber() *mockSubscriber {
	return &mockSubscriber{
		subscriptions: make(map[SubscriptionID]bool),
		handlers:      make(map[string]any),
		nextSubID:     1,
		wg:            sync.WaitGroup{},
	}
}

func (m *mockSubscriber) SubscribeToStruct(t, callback any) (SubscriptionID, error) {
	structName := ""
	switch t.(type) {
	case pgn.ISOAcknowledgement:
		structName = "ISOAcknowledgement"
	case pgn.ISORequest:
		structName = "ISORequest"
	case pgn.ISOAddressClaim:
		structName = "ISOAddressClaim"
	case pgn.ISOCommandedAddress:
		structName = "ISOCommandedAddress"
	case pgn.NMEARequestGroupFunction:
		structName = "NMEARequestGroupFunction"
	case pgn.NMEACommandGroupFunction:
		structName = "NMEACommandGroupFunction"
	case pgn.NMEAWriteFieldsGroupFunction:
		structName = "NMEAWriteFieldsGroupFunction"
	case pgn.ProductInformation:
		structName = "ProductInformation"
	case pgn.ConfigurationInformation:
		structName = "ConfigurationInformation"
	case pgn.PGNListTransmitAndReceive:
		structName = "PGNListTransmitAndReceive"
	default:
		return 0, fmt.Errorf("mockSubscriber does not support type %T", t)
	}

	m.subscriptions[m.nextSubID] = true
	m.handlers[structName] = callback
	m.nextSubID++
	return m.nextSubID - 1, nil
}

func (m *mockSubscriber) Unsubscribe(subID SubscriptionID) error {
	if _, ok := m.subscriptions[subID]; !ok {
		return fmt.Errorf("subscription not found")
	}
	delete(m.subscriptions, subID)
	return nil
}

// Helper to simulate a PGN arriving from the network
func (m *mockSubscriber) simulatePGN(pgnStruct any) {
	var structName string
	switch pgnStruct.(type) {
	case pgn.ISOAcknowledgement, *pgn.ISOAcknowledgement:
		structName = "ISOAcknowledgement"
	case pgn.ISORequest, *pgn.ISORequest:
		structName = "ISORequest"
	case pgn.ISOAddressClaim, *pgn.ISOAddressClaim:
		structName = "ISOAddressClaim"
	case pgn.ISOCommandedAddress, *pgn.ISOCommandedAddress:
		structName = "ISOCommandedAddress"
	case pgn.NMEARequestGroupFunction, *pgn.NMEARequestGroupFunction:
		structName = "NMEARequestGroupFunction"
	case pgn.NMEACommandGroupFunction, *pgn.NMEACommandGroupFunction:
		structName = "NMEACommandGroupFunction"
	case pgn.NMEAWriteFieldsGroupFunction, *pgn.NMEAWriteFieldsGroupFunction:
		structName = "NMEAWriteFieldsGroupFunction"
	case pgn.ProductInformation, *pgn.ProductInformation:
		structName = "ProductInformation"
	case pgn.ConfigurationInformation, *pgn.ConfigurationInformation:
		structName = "ConfigurationInformation"
	case pgn.PGNListTransmitAndReceive, *pgn.PGNListTransmitAndReceive:
		structName = "PGNListTransmitAndReceive"
	default:
		return
	}

	if handler, ok := m.handlers[structName]; ok {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			v := reflect.ValueOf(handler)
			arg := reflect.ValueOf(pgnStruct)
			if arg.Kind() == reflect.Pointer {
				arg = arg.Elem()
			}
			v.Call([]reflect.Value{arg})
		}()
	}
}

// Helper to wait for handlers to complete
func (m *mockSubscriber) waitForHandler() {
	m.wg.Wait()
}

// mockPublisher is a mock implementation of the Publisher interface.
type mockPublisher struct {
	written []any
	wg      sync.WaitGroup
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{
		written: make([]any, 0),
		wg:      sync.WaitGroup{},
	}
}

// Write is the mock implementation of the Publisher interface.
func (m *mockPublisher) Write(pgnStruct any) error {
	m.written = append(m.written, pgnStruct)
	m.wg.Done() // Signal that a write has occurred
	return nil
}

// Helper to get the last written PGN
func (m *mockPublisher) lastWritten() any {
	if len(m.written) == 0 {
		return nil
	}
	return m.written[len(m.written)-1]
}

// Helper to clear the written PGNs
func (m *mockPublisher) clear() {
	m.written = make([]any, 0)
}

// Helper to wait for a write to occur
func (m *mockPublisher) waitForWrite() {
	m.wg.Wait()
}

// Helper to prepare for a write
func (m *mockPublisher) expectWrite() {
	m.wg.Add(1)
}

// --- Mock Clock ---

type mockTicker struct {
	c    chan time.Time
	stop chan struct{}
}

func (mt *mockTicker) C() <-chan time.Time {
	return mt.c
}

func (mt *mockTicker) Stop() {
	close(mt.stop)
}

type mockClock struct {
	tickers []*mockTicker
}

func newMockClock() *mockClock {
	return &mockClock{
		tickers: make([]*mockTicker, 0),
	}
}

func (mc *mockClock) NewTicker(_ time.Duration) Ticker {
	ticker := &mockTicker{
		c:    make(chan time.Time, 1),
		stop: make(chan struct{}),
	}
	mc.tickers = append(mc.tickers, ticker)
	return ticker
}

// Advance simulates the passage of time, triggering any active tickers.
func (mc *mockClock) Advance() {
	for _, ticker := range mc.tickers {
		select {
		case <-ticker.stop:
			// Ticker was stopped, do nothing
		default:
			ticker.c <- time.Now()
		}
	}
}
