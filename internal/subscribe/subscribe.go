// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package subscribe manages subscriptions to all or specific go structs.
package subscribe

import (
	"fmt"
	"reflect"
	"sync"
)

// SubscribeManager maintains lists of subscribers to specific or all
// structs.
//
//nolint:revive // Why: Breaking change to refactor.
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

// SubscriptionId identifies a specific subscriber.
//
//nolint:revive // Why: Breaking change to refactor.
type SubscriptionId uint

// trackedSub connects a  subscriber with a function that fulfills a
// specific subscription.
//
//nolint:revive // Why: Breaking change to refactor.
type trackedSub struct {
	subId      SubscriptionId
	structName string
	// Will be either func(any) for global handler or func(specific struct) for a struct callback
	callback any
}

// New returns a pointer to a new SubscribeManager.
func New() *SubscribeManager {
	return &SubscribeManager{
		lastSubId: 0,
		subs:      make(map[SubscriptionId]*trackedSub),
		all:       []*trackedSub{},
		singles:   make(map[string][]*trackedSub),
	}
}

// addSubscription adds a subscription. It's called internally by routines that validate its arguments.
// Callback must be validated already
func (s *SubscribeManager) addSubscription(structName string, callback any) (SubscriptionId, error) {
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

// Unsubscribe cancels a subscription.
//
//nolint:revive // Why: Breaking change to refactor.
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

// HandleStruct calls registered subscriber callbacks for a struct.
// It calls all specific subscribers and all subscribers.
func (s *SubscribeManager) HandleStruct(p any) {
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

	// Call each callback with the appropriate value type
	for _, t := range callList {
		// Check what type the callback expects
		funcType := t.Type()
		if funcType.NumIn() == 0 {
			continue // Skip callbacks with no parameters
		}

		paramType := funcType.In(0)
		var callWith []reflect.Value

		if paramType.Kind() == reflect.Ptr && paramType.Elem() == pv.Type() {
			// Callback expects a pointer to the struct
			if pv.CanAddr() {
				callWith = []reflect.Value{pv.Addr()}
			} else {
				ptrValue := reflect.New(pv.Type())
				ptrValue.Elem().Set(pv)
				callWith = []reflect.Value{ptrValue}
			}
		} else if paramType == pv.Type() {
			// Callback expects the struct value directly
			callWith = []reflect.Value{pv}
		} else if paramType.Kind() == reflect.Interface {
			// Callback expects an interface (like 'any')
			// Pass the struct value directly
			callWith = []reflect.Value{pv}
		} else {
			// Type mismatch, skip this callback
			continue
		}

		t.Call(callWith)
	}
}

// SubscribeToStruct registers a subscription to the specified struct.
// It validates the callback is a function with a matching argument
// type.
//
//nolint:revive // Why: Breaking change to refactor.
func (s *SubscribeManager) SubscribeToStruct(t, callback any) (SubscriptionId, error) {
	e := reflect.ValueOf(t)
	if e.Kind() != reflect.Struct {
		return 0, fmt.Errorf("subscribeToPgn called with non-struct type: %+v", e.Kind())
	}

	ce := reflect.ValueOf(callback)
	if ce.Kind() != reflect.Func {
		return 0, fmt.Errorf("subscribeToPgn called with non-func callback: %+v", ce.Kind())
	}
	if ce.Type().In(0) != e.Type() {
		return 0, fmt.Errorf(
			"subscribeToPgn called with callback type (%+v) not matching passed type (%+v)",
			ce.Type().In(0).Name(), e.Type().Name(),
		)
	}

	return s.addSubscription(e.Type().Name(), callback)
}

// SubscribeToAllStructs registers a subscription to all structs.
// It validates the callback is a func with an any argument.
// Note: callbacks now receive pointers to structs, not struct values.
func (s *SubscribeManager) SubscribeToAllStructs(callback any) (SubscriptionId, error) {
	ce := reflect.ValueOf(callback)
	if ce.Kind() != reflect.Func {
		return 0, fmt.Errorf("subscribeToAllStructs called with non-func callback: %+v", ce.Kind())
	}
	if ce.Type().In(0).Kind() != reflect.Interface {
		return 0, fmt.Errorf("subscribeToAllStructs called with non-any-taking callback type (%+v)", ce.Type().In(0).Kind())
	}

	return s.addSubscription("", callback)
}
