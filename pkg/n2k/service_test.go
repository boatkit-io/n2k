package n2k

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func makeFloat(v float32) *float32 {
	return &v
}

func TestSubscriptions(t *testing.T) {
	s, err1 := NewService(context.Background(), logrus.StandardLogger(), "")
	assert.NoError(t, err1)

	// Basic PGN sub
	var ov *CogSogRapidUpdate
	cf := func(s CogSogRapidUpdate) {
		ov = &s
	}
	subId, err := s.SubscribeToPgn(CogSogRapidUpdate{}, cf)
	assert.NoError(t, err)
	assert.Equal(t, SubscriptionId(1), subId)

	toSend := CogSogRapidUpdate{
		Cog: makeFloat(1.0),
	}

	s.handlePGNStruct(toSend)

	assert.NotNil(t, ov)
	assert.Equal(t, float32(1.0), *ov.Cog)
	ov = nil

	// Add global sub
	var ov2 *CogSogRapidUpdate
	cf2 := func(s interface{}) {
		s2 := s.(CogSogRapidUpdate)
		assert.NotNil(t, s2)
		ov2 = &s2
	}
	subId2, err := s.SubscribeToAllPgns(cf2)
	assert.NoError(t, err)
	assert.Equal(t, SubscriptionId(2), subId2)

	toSend.Cog = makeFloat(2.0)
	s.handlePGNStruct(toSend)

	assert.NotNil(t, ov)
	assert.Equal(t, float32(2.0), *ov.Cog)
	assert.NotNil(t, ov2)
	assert.Equal(t, float32(2.0), *ov2.Cog)
	ov = nil
	ov2 = nil

	// unsubscribe pgn sub
	err = s.Unsubscribe(subId)
	assert.NoError(t, err)

	toSend.Cog = makeFloat(3.0)
	s.handlePGNStruct(toSend)

	assert.Nil(t, ov)
	assert.NotNil(t, ov2)
	assert.Equal(t, float32(3.0), *ov2.Cog)
	ov2 = nil

	// unsubscribe global sub
	err = s.Unsubscribe(subId2)
	assert.NoError(t, err)

	toSend.Cog = makeFloat(4.0)
	s.handlePGNStruct(toSend)

	assert.Nil(t, ov)
	assert.Nil(t, ov2)
}

func TestSubscriptionErrors(t *testing.T) {
	s, err1 := NewService(context.Background(), logrus.StandardLogger(), "")
	assert.NoError(t, err1)

	// PGN subs that shouldn't work
	cf := func(s CogSogRapidUpdate) {}
	_, err := s.SubscribeToPgn(4, cf)
	assert.Error(t, err)

	cf2 := func(s int) {}
	_, err = s.SubscribeToPgn(4, cf2)
	assert.Error(t, err)
	_, err = s.SubscribeToPgn(CogSogRapidUpdate{}, cf2)
	assert.Error(t, err)

	cf3 := func(s PositionDeltaRapidUpdate) {}
	_, err = s.SubscribeToPgn(CogSogRapidUpdate{}, cf3)
	assert.Error(t, err)

	subId, err := s.SubscribeToPgn(CogSogRapidUpdate{}, cf)
	assert.NoError(t, err)
	assert.Equal(t, SubscriptionId(1), subId)

	assert.Error(t, s.Unsubscribe(SubscriptionId(0)))
	assert.Error(t, s.Unsubscribe(SubscriptionId(4)))
	assert.NoError(t, s.Unsubscribe(subId))

	// second time should break
	assert.Error(t, s.Unsubscribe(subId))
}
