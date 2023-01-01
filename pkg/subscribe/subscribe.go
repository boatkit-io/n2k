package subscribe

import (
	"fmt"
	"reflect"
	"sync"
)

// Track subscriptions
type SubscribeManager struct {
	subMutex sync.Mutex
	// tracked subs by subscriber
	subs map[SubscriptionId]*trackedSub
	// subscriptions for specific stucts
	singles map[string][]*trackedSub
	// subscriptions for all structs
	all       []*trackedSub
	lastSubId SubscriptionId
}

type SubscriptionId uint

type trackedSub struct {
	subId      SubscriptionId
	structName string
	// Will be either func(interface{}) for global handler or func(specific struct) for a struct callback
	callback interface{}
}

func New() *SubscribeManager {
	return &SubscribeManager{
		lastSubId: 0,
		subs:      make(map[SubscriptionId]*trackedSub),
		all:       []*trackedSub{},
		singles:   make(map[string][]*trackedSub),
	}
}

// Callback must be validated already
func (s *SubscribeManager) addSubscription(structName string, callback interface{}) (SubscriptionId, error) {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	s.lastSubId++
	ts := &trackedSub{
		subId:      s.lastSubId,
		structName: structName,
		callback:   callback,
	}

	s.subs[ts.subId] = ts

	if structName == "" {
		s.all = append(s.all, ts)
	} else {
		arr := s.singles[ts.structName]
		if arr == nil {
			arr = make([]*trackedSub, 0)
		}
		s.singles[ts.structName] = append(arr, ts)
	}

	return ts.subId, nil
}

func (s *SubscribeManager) Unsubscribe(subId SubscriptionId) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	ts, exists := s.subs[subId]
	if !exists {
		return fmt.Errorf("subscription %d not found", subId)
	}

	if ts.structName == "" {
		// global sub
		found := false
		for i, sub := range s.all {
			if sub == ts {
				found = true
				s.all = append(s.all[0:i], s.all[i+1:]...)
				break
			}
		}
		if !found {
			return fmt.Errorf("global subscription %d not tracked somehow", subId)
		}
	} else {
		// pgn sub
		subs, exists := s.singles[ts.structName]
		if !exists {
			return fmt.Errorf("struct subscription %d somehow not found in %s", subId, ts.structName)
		}

		found := false
		for i, sub := range subs {
			if sub == ts {
				found = true
				if len(subs) == 1 {
					// now empty -- clean up struct sub list
					delete(s.singles, ts.structName)
				} else {
					s.singles[ts.structName] = append(subs[0:i], subs[i+1:]...)
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("struct subscription %d not tracked somehow in %s", subId, ts.structName)
		}
	}

	return nil
}

func (s *SubscribeManager) ServeStruct(p interface{}) {
	pv := reflect.ValueOf(p)
	sn := pv.Type().Name()

	// Build a call list inside the mutex to call back outside of it, in case the callback unsubscribes
	callList := []reflect.Value{}

	s.subMutex.Lock()

	if single, exists := s.singles[sn]; exists {
		// Copy the single slice in case it changes while we're iterating
		psc := make([]*trackedSub, len(single))
		copy(psc, single)

		for _, sub := range psc {
			t := reflect.ValueOf(sub.callback)
			callList = append(callList, t)
		}
	}

	// Copy the globalSubs slice in case it changes while we're iterating
	gsc := make([]*trackedSub, len(s.all))
	copy(gsc, s.all)
	for _, sub := range gsc {
		t := reflect.ValueOf(sub.callback)
		callList = append(callList, t)
	}

	s.subMutex.Unlock()

	callWith := []reflect.Value{pv}
	for _, t := range callList {
		t.Call(callWith)
	}
}

func (s *SubscribeManager) SubscribeToStruct(t interface{}, callback interface{}) (SubscriptionId, error) {
	e := reflect.ValueOf(t)
	if e.Kind() != reflect.Struct {
		return 0, fmt.Errorf("subscribeToPgn called with non-struct type: %+v", e.Kind())
	}

	ce := reflect.ValueOf(callback)
	if ce.Kind() != reflect.Func {
		return 0, fmt.Errorf("subscribeToPgn called with non-func callback: %+v", ce.Kind())
	}
	if ce.Type().In(0) != e.Type() {
		return 0, fmt.Errorf("subscribeToPgn called with callback type (%+v) not matching passed type (%+v)", ce.Type().In(0).Name(), e.Type().Name())
	}

	return s.addSubscription(e.Type().Name(), callback)
}

func (s *SubscribeManager) SubscribeToAllStructs(callback interface{}) (SubscriptionId, error) {
	ce := reflect.ValueOf(callback)
	if ce.Kind() != reflect.Func {
		return 0, fmt.Errorf("subscribeToAllStructs called with non-func callback: %+v", ce.Kind())
	}
	if ce.Type().In(0).Kind() != reflect.Interface {
		return 0, fmt.Errorf("subscribeToAllStructs called with non-interface{}-taking callback type (%+v)", ce.Type().In(0).Kind())
	}

	return s.addSubscription("", callback)
}
