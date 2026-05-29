package node

import "github.com/boatkit-io/n2k/pkg/n2k"

// SubscriptionId identifies a subscription managed by a Subscriber.
type SubscriptionId uint

// NewFromService creates a Node backed by the public N2kService API.
// This is the intended entry point for clients such as goatkit.
func NewFromService(svc *n2k.N2kService) Node {
	return NewNode(newN2kServiceSubscriber(svc), newN2kServicePublisher(svc), nil)
}

type n2kServiceSubscriber struct {
	svc *n2k.N2kService
}

func newN2kServiceSubscriber(svc *n2k.N2kService) *n2kServiceSubscriber {
	return &n2kServiceSubscriber{svc: svc}
}

func (s *n2kServiceSubscriber) SubscribeToStruct(t any, callback any) (SubscriptionId, error) {
	id, err := s.svc.SubscribeToStruct(t, callback)
	return SubscriptionId(id), err
}

func (s *n2kServiceSubscriber) Unsubscribe(subId SubscriptionId) error {
	return s.svc.Unsubscribe(uint(subId))
}

type n2kServicePublisher struct {
	svc *n2k.N2kService
}

func newN2kServicePublisher(svc *n2k.N2kService) *n2kServicePublisher {
	return &n2kServicePublisher{svc: svc}
}

func (p *n2kServicePublisher) Write(pgnStruct any) error {
	return p.svc.Write(pgnStruct)
}
