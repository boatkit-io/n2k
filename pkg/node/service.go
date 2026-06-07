// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package node provides standard NMEA 2000 node behavior.
package node

import "github.com/boatkit-io/n2k/pkg/n2k"

// SubscriptionID identifies a subscription managed by a Subscriber.
type SubscriptionID uint

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

func (s *n2kServiceSubscriber) SubscribeToStruct(t, callback any) (SubscriptionID, error) {
	id, err := s.svc.SubscribeToStruct(t, callback)
	return SubscriptionID(id), err
}

func (s *n2kServiceSubscriber) Unsubscribe(subID SubscriptionID) error {
	return s.svc.Unsubscribe(uint(subID))
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
