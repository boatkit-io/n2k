package n2k

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

	"github.com/boatkit-io/canbus"

	"github.com/boatkit-io/tugboat/pkg/service"
	"github.com/brutella/can"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Service is a management structure that implements a n2k canbus channel and
// manages processing of canbus data into usable structures and events for the
// rest of the system.
//
// This type implements the brutella.CanHandler interface.
type Service struct {
	log              *logrus.Logger
	n2kInterfaceName string

	shuttingDown bool
	canbus       *canbus.Channel
	pgnBuilder   *PGNBuilder

	// Track subscriptions
	subMutex   sync.Mutex
	subs       map[SubscriptionId]*trackedSub
	pgnSubs    map[string][]*trackedSub
	globalSubs []*trackedSub
	lastSubId  SubscriptionId
}

type SubscriptionId uint

type trackedSub struct {
	subId      SubscriptionId
	structName string
	// Will be either func(interface{}) for global handler or func(specific struct) for a struct callback
	callback interface{}
}

var _ service.Activity = &Service{}

// NewService returns a new instance of a n2k Service.
func NewService(ctx context.Context, log *logrus.Logger, interfaceName string) (*Service, error) {
	s := Service{
		log:              log,
		n2kInterfaceName: interfaceName,

		shuttingDown: false,
		canbus:       nil,

		lastSubId:  0,
		subs:       make(map[SubscriptionId]*trackedSub),
		globalSubs: []*trackedSub{},
		pgnSubs:    make(map[string][]*trackedSub),
	}

	s.pgnBuilder = NewPGNBuilder(log, s.handlePGNStruct)

	return &s, nil
}

// Name returns the name of the Service service.Activity (helps implement the interface).
func (*Service) Name() string {
	return "n2k"
}

func (s *Service) Run(ctx context.Context) error {
	if s.n2kInterfaceName != "" {
		canbusOpts := canbus.ChannelOptions{
			CanInterfaceName: s.n2kInterfaceName,
			MessageHandler:   s.pgnBuilder.ProcessFrame,
		}

		if cc, err := canbus.NewChannel(ctx, s.log, canbusOpts); err != nil {
			s.log.WithError(err).Warn("n2k channel creation failed")
		} else {
			s.canbus = cc
		}
	}

	// Wait for shutdown
	<-ctx.Done()

	return nil
}

// Kill attempts to gracefully shutdown the Service.
func (s *Service) Shutdown(ctx context.Context) error {
	s.shuttingDown = true

	var errs []error

	if s.canbus != nil {
		if err := s.canbus.Close(ctx); err != nil {
			errs = append(errs, errors.Wrap(err, "closing n2k canbus channel"))
		}
	}

	if len(errs) > 0 {
		err := errs[0]
		for i := 1; i < len(errs); i++ {
			err = errors.Wrap(err, errs[i].Error())
		}
		return err
	}
	return nil
}

// Kill attempts to forcefully shutdown the Service.
func (s *Service) Kill() error {
	return nil
}

func (s *Service) ReplayFile(filename string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	file, err := os.Open(path.Join(wd, "n2kreplays", filename))
	if err != nil {
		return err
	}

	go func() {
		defer file.Close()

		startTime := time.Now()

		s.log.Info("starting n2k file playback")

		replayPGNBuilder := NewPGNBuilder(s.log, s.handlePGNStruct)

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			// Sample line:
			// (010.139585)  can1  08FF0401   [8]  AC 98 21 FC 5E FD 64 FF
			line := scanner.Text()
			var frame can.Frame
			var canDead string
			var timeDelta float32
			fmt.Sscanf(line, " (%f)  %s  %8X   [%d]  %X %X %X %X %X %X %X %X", &timeDelta, &canDead, &frame.ID, &frame.Length, &frame.Data[0], &frame.Data[1], &frame.Data[2], &frame.Data[3], &frame.Data[4], &frame.Data[5], &frame.Data[6], &frame.Data[7])
			// Pause until the timeDelta has expired, so this all replays in "real-time" (relative to start, obvs)
			for {
				if s.shuttingDown {
					return
				}

				curDelta := time.Since(startTime).Seconds()
				nextTime := timeDelta - float32(curDelta)
				// Make sure we wait at most 0.5 seconds to gracefully quit as needed
				time.Sleep(time.Duration(math.Min(500, float64(nextTime)*1000.0)) * time.Millisecond)

				if time.Since(startTime) > time.Duration(timeDelta) {
					break
				}
			}

			replayPGNBuilder.ProcessFrame(frame)
		}

		s.log.Info("n2k file playback complete")

		if err := scanner.Err(); err != nil {
			s.log.Warn(errors.Wrap(err, "error while scanning n2k replay file"))
		}
	}()

	return nil
}

func (s *Service) handlePGNStruct(p interface{}) {
	pv := reflect.ValueOf(p)
	sn := pv.Type().Name()

	// Build a call list inside the mutex to call back outside of it, in case the callback unsubscribes
	callList := []reflect.Value{}

	s.subMutex.Lock()

	if pgnSubs, exists := s.pgnSubs[sn]; exists {
		// Copy the pgnSubs slice in case it changes while we're iterating
		psc := make([]*trackedSub, len(pgnSubs))
		copy(psc, pgnSubs)

		for _, sub := range psc {
			t := reflect.ValueOf(sub.callback)
			callList = append(callList, t)
		}
	}

	// Copy the globalSubs slice in case it changes while we're iterating
	gsc := make([]*trackedSub, len(s.globalSubs))
	copy(gsc, s.globalSubs)
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

func (s *Service) SubscribeToPgn(t interface{}, callback interface{}) (SubscriptionId, error) {
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

	return s.addPgnSubscription(e.Type().Name(), callback)
}

func (s *Service) SubscribeToAllPgns(callback interface{}) (SubscriptionId, error) {
	ce := reflect.ValueOf(callback)
	if ce.Kind() != reflect.Func {
		return 0, fmt.Errorf("subscribeToAllPgns called with non-func callback: %+v", ce.Kind())
	}
	if ce.Type().In(0).Kind() != reflect.Interface {
		return 0, fmt.Errorf("subscribeToAllPgns called with non-interface{}-taking callback type (%+v)", ce.Type().In(0).Kind())
	}

	return s.addPgnSubscription("", callback)
}

// Callback must be validated already
func (s *Service) addPgnSubscription(structName string, callback interface{}) (SubscriptionId, error) {
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
		s.globalSubs = append(s.globalSubs, ts)
	} else {
		arr := s.pgnSubs[ts.structName]
		if arr == nil {
			arr = make([]*trackedSub, 0)
		}
		s.pgnSubs[ts.structName] = append(arr, ts)
	}

	return ts.subId, nil
}

func (s *Service) Unsubscribe(subId SubscriptionId) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	ts, exists := s.subs[subId]
	if !exists {
		return fmt.Errorf("subscription %d not found", subId)
	}

	if ts.structName == "" {
		// global sub
		found := false
		for i, sub := range s.globalSubs {
			if sub == ts {
				found = true
				s.globalSubs = append(s.globalSubs[0:i], s.globalSubs[i+1:]...)
				break
			}
		}
		if !found {
			return fmt.Errorf("global subscription %d not tracked somehow", subId)
		}
	} else {
		// pgn sub
		subs, exists := s.pgnSubs[ts.structName]
		if !exists {
			return fmt.Errorf("pgn subscription %d somehow not found in %s", subId, ts.structName)
		}

		found := false
		for i, sub := range subs {
			if sub == ts {
				found = true
				if len(subs) == 1 {
					// now empty -- clean up struct sub list
					delete(s.pgnSubs, ts.structName)
				} else {
					s.pgnSubs[ts.structName] = append(subs[0:i], subs[i+1:]...)
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("pgn subscription %d not tracked somehow in %s", subId, ts.structName)
		}
	}

	return nil
}
