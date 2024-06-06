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

// unique returns true if the name is encountered for the first time
// if false the name has "_"+ incrementing digit(s) appended
func (deduper *DeDuper) unique(name string) (bool, string) {
	firstTime := true
	count := deduper.used[name]
	count++
	deduper.used[name] = count
	if count > 1 {
		name += "_" + strconv.FormatInt(int64(count), 10)
		firstTime = false
	}
	return firstTime, name
}
