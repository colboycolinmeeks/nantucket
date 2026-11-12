// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// TestEntryListMatchesSlice applies random inserts and deletes to an
// entryList and to a plain sorted slice and checks they always agree.
func TestEntryListMatchesSlice(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	less := func(a, b Entry) bool {
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		return a.Rec < b.Rec
	}
	var l entryList
	var ref []Entry
	check := func(step int) {
		if l.Len() != len(ref) {
			t.Fatalf("step %d: Len %d, want %d", step, l.Len(), len(ref))
		}
		all := l.All()
		for i := range ref {
			if all[i] != ref[i] || l.At(i) != ref[i] {
				t.Fatalf("step %d: position %d is %v, want %v", step, i, l.At(i), ref[i])
			}
		}
	}
	for step := 0; step < 20000; step++ {
		if len(ref) > 0 && rng.Intn(3) == 0 {
			i := rng.Intn(len(ref))
			l.Delete(i)
			ref = append(ref[:i], ref[i+1:]...)
		} else {
			e := Entry{Key: fmt.Sprintf("%06d", rng.Intn(5000)), Rec: uint32(step)}
			i := sort.Search(len(ref), func(k int) bool { return !less(ref[k], e) })
			if got := l.Search(func(x Entry) bool { return !less(x, e) }); got != i {
				t.Fatalf("step %d: Search = %d, want %d", step, got, i)
			}
			l.Insert(i, e)
			ref = append(ref, Entry{})
			copy(ref[i+1:], ref[i:])
			ref[i] = e
		}
		if step%500 == 0 {
			check(step)
		}
	}
	check(-1)
	// Reset keeps the order and leaves room to grow
	l.Reset(ref)
	check(-2)
	if len(l.blocks) < 2 {
		t.Errorf("expected several blocks for %d entries", len(ref))
	}
}

// TestIndexInsertScales checks that inserting into a large index stays
// fast: 200,000 random inserts used to take minutes.
func TestIndexInsertScales(t *testing.T) {
	ix := &Index{KeyLen: 10}
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 200000; i++ {
		ix.Insert(fmt.Sprintf("%010d", rng.Intn(1e9)), uint32(i+1))
	}
	if ix.Len() != 200000 {
		t.Fatalf("Len = %d", ix.Len())
	}
	for i := 1; i < ix.Len(); i++ {
		if ix.At(i-1).Key > ix.At(i).Key {
			t.Fatalf("out of order at %d", i)
		}
	}
}
