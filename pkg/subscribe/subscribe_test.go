package subscribe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeFloat(v float32) *float32 {
	return &v
}

type test1 struct {
	field1 *float32
}

func TestSubscriptions(t *testing.T) {
	s := New()
	//	assert.NoError(t, err1)

	// Basic struct sub
	var ov *test1
	cf := func(s test1) {
		ov = &s
	}
	subId, err := s.SubscribeToStruct(test1{}, cf)
	assert.NoError(t, err)
	assert.Equal(t, SubscriptionId(1), subId)

	toSend := test1{
		field1: makeFloat(1.0),
	}

	s.HandleStruct(toSend)

	assert.NotNil(t, ov)
	assert.Equal(t, float32(1.0), *ov.field1)
	ov = nil

	// Add global sub
	var ov2 *test1
	cf2 := func(s any) {
		s2 := s.(test1)
		assert.NotNil(t, s2)
		ov2 = &s2
	}
	subId2, err := s.SubscribeToAllStructs(cf2)
	assert.NoError(t, err)
	assert.Equal(t, SubscriptionId(2), subId2)

	toSend.field1 = makeFloat(2.0)
	s.HandleStruct(toSend)

	assert.NotNil(t, ov)
	assert.Equal(t, float32(2.0), *ov.field1)
	assert.NotNil(t, ov2)
	assert.Equal(t, float32(2.0), *ov2.field1)
	ov = nil
	ov2 = nil

	// unsubscribe struct sub
	err = s.Unsubscribe(subId)
	assert.NoError(t, err)

	toSend.field1 = makeFloat(3.0)
	s.HandleStruct(toSend)

	assert.Nil(t, ov)
	assert.NotNil(t, ov2)
	assert.Equal(t, float32(3.0), *ov2.field1)
	ov2 = nil

	// unsubscribe global sub
	err = s.Unsubscribe(subId2)
	assert.NoError(t, err)

	toSend.field1 = makeFloat(4.0)
	s.HandleStruct(toSend)

	assert.Nil(t, ov)
	assert.Nil(t, ov2)
}

type test2 struct {
	field1 int
	field2 string
}

func TestSubscriptionErrors(t *testing.T) {
	s := New()
	//	assert.NoError(t, err1)

	// struct subs that shouldn't work
	cf := func(s test1) {}
	_, err := s.SubscribeToStruct(4, cf)
	assert.Error(t, err)

	cf2 := func(s int) {}
	_, err = s.SubscribeToStruct(4, cf2)
	assert.Error(t, err)
	_, err = s.SubscribeToStruct(test1{}, cf2)
	assert.Error(t, err)

	cf3 := func(s *test2) {
		s.field1 = 0
		s.field2 = ""
	}
	_, err = s.SubscribeToStruct(test1{}, cf3)
	assert.Error(t, err)

	subId, err := s.SubscribeToStruct(test1{}, cf)
	assert.NoError(t, err)
	assert.Equal(t, SubscriptionId(1), subId)

	assert.Error(t, s.Unsubscribe(SubscriptionId(0)))
	assert.Error(t, s.Unsubscribe(SubscriptionId(4)))
	assert.NoError(t, s.Unsubscribe(subId))

	// second time should break
	assert.Error(t, s.Unsubscribe(subId))
}
