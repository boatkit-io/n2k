package main

import (
	"strconv"
)

// DeDuper assures uniqueness of names.
// If presented a name that isn't unique it returns a unique name by appending an incrementing number.
type DeDuper struct {
	used map[string]int
}

// NewDeDuper create a new instance with an initialized map
func NewDeDuper() *DeDuper {
	return &DeDuper{
		used: make(map[string]int),
	}
}

// isUnique returns true if the name hasn't been seen
func (deduper *DeDuper) isUnique(name string) bool {
	_, exists := deduper.used[name]
	return !exists
}

// unique returns the name (if not already used) or appends an incrementing counter to name
func (duduper *DeDuper) unique(name string) string {
	count := duduper.used[name]
	count++
	duduper.used[name] = count
	if count > 1 {
		name += strconv.FormatInt(int64(count), 10)
	}
	return name
}
