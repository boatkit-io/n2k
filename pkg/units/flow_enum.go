package units

import (
	"errors"
	"fmt"
)

const (
	// LitersPerHour is a FlowUnit of type LitersPerHour.
	LitersPerHour FlowUnit = iota
	// GallonsPerMinute is a FlowUnit of type GallonsPerMinute.
	GallonsPerMinute
	// GallonsPerHour is a FlowUnit of type GallonsPerHour.
	GallonsPerHour
)

var ErrInvalidFlowUnit = errors.New("not a valid FlowUnit")

const _FlowUnitName = "LitersPerHourGallonsPerMinuteGallonsPerHour"

// FlowUnitValues returns a list of the values for FlowUnit
func FlowUnitValues() []FlowUnit {
	return []FlowUnit{
		LitersPerHour,
		GallonsPerMinute,
		GallonsPerHour,
	}
}

var _FlowUnitMap = map[FlowUnit]string{
	LitersPerHour:    _FlowUnitName[0:13],
	GallonsPerMinute: _FlowUnitName[13:29],
	GallonsPerHour:   _FlowUnitName[29:43],
}

// String implements the Stringer interface.
func (x FlowUnit) String() string {
	if str, ok := _FlowUnitMap[x]; ok {
		return str
	}
	return fmt.Sprintf("FlowUnit(%d)", x)
}

// IsValid provides a quick way to determine if the typed value is
// part of the allowed enumerated values
func (x FlowUnit) IsValid() bool {
	_, ok := _FlowUnitMap[x]
	return ok
}

var _FlowUnitValue = map[string]FlowUnit{
	_FlowUnitName[0:13]:  LitersPerHour,
	_FlowUnitName[13:29]: GallonsPerMinute,
	_FlowUnitName[29:43]: GallonsPerHour,
}

// ParseFlowUnit attempts to convert a string to a FlowUnit.
func ParseFlowUnit(name string) (FlowUnit, error) {
	if x, ok := _FlowUnitValue[name]; ok {
		return x, nil
	}
	return FlowUnit(0), fmt.Errorf("%s is %w", name, ErrInvalidFlowUnit)
}
