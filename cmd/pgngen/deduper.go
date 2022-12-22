package main

import (
	"strconv"
)

type DeDuper struct {
	used map[string]int
}

func NewDeDuper() *DeDuper {
	return &DeDuper{
		used: make(map[string]int),
	}
}

func (deduper *DeDuper) isUnique(name string) bool {
	_, exists := deduper.used[name]
	return !exists
}

func (duduper *DeDuper) unique(name string) string {
	count := duduper.used[name]
	count++
	duduper.used[name] = count
	if count > 1 {
		name += strconv.FormatInt(int64(count), 10)
	}
	return name
}
