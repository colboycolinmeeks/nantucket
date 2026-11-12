// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import "sort"

// blockMax is the most entries a block holds before it is split.
const blockMax = 1024

// entryList is an index's keys in order, held as a list of sorted blocks.
// Inserting or removing a key only shifts entries within one block, so
// adding records to an indexed table stays fast however large the index
// grows (one long slice made every insert move the whole tail).
//
// Positions are global (0 .. Len()-1), as with a slice.
type entryList struct {
	blocks [][]Entry
	starts []int // starts[b] is the position of blocks[b][0]
	n      int
}

func (l *entryList) Len() int { return l.n }

// locate finds the block holding position i and the offset within it.
func (l *entryList) locate(i int) (b, off int) {
	b = sort.Search(len(l.starts), func(k int) bool { return l.starts[k] > i }) - 1
	return b, i - l.starts[b]
}

// At returns the entry at position i.
func (l *entryList) At(i int) Entry {
	b, off := l.locate(i)
	return l.blocks[b][off]
}

// Search returns the first position for which f is true, or Len() if
// there is none. f must be false and then true along the list, as for
// sort.Search.
func (l *entryList) Search(f func(Entry) bool) int {
	b := sort.Search(len(l.blocks), func(k int) bool {
		blk := l.blocks[k]
		return f(blk[len(blk)-1])
	})
	if b == len(l.blocks) {
		return l.n
	}
	blk := l.blocks[b]
	return l.starts[b] + sort.Search(len(blk), func(k int) bool { return f(blk[k]) })
}

// Insert puts e at position i (0 <= i <= Len()).
func (l *entryList) Insert(i int, e Entry) {
	if len(l.blocks) == 0 {
		l.blocks = [][]Entry{{e}}
		l.starts = []int{0}
		l.n = 1
		return
	}
	var b, off int
	if i >= l.n {
		b = len(l.blocks) - 1
		off = len(l.blocks[b])
	} else {
		b, off = l.locate(i)
	}
	blk := append(l.blocks[b], Entry{})
	copy(blk[off+1:], blk[off:])
	blk[off] = e
	l.blocks[b] = blk
	l.n++
	if len(blk) > blockMax {
		half := len(blk) / 2
		right := append([]Entry(nil), blk[half:]...)
		l.blocks[b] = blk[:half:half]
		l.blocks = append(l.blocks, nil)
		copy(l.blocks[b+2:], l.blocks[b+1:])
		l.blocks[b+1] = right
		l.starts = append(l.starts, 0)
		l.reindexFrom(b + 1)
		return
	}
	for k := b + 1; k < len(l.starts); k++ {
		l.starts[k]++
	}
}

// Delete removes the entry at position i.
func (l *entryList) Delete(i int) {
	b, off := l.locate(i)
	blk := l.blocks[b]
	copy(blk[off:], blk[off+1:])
	blk = blk[:len(blk)-1]
	l.n--
	if len(blk) == 0 {
		l.blocks = append(l.blocks[:b], l.blocks[b+1:]...)
		l.starts = l.starts[:len(l.blocks)]
		l.reindexFrom(b)
		return
	}
	l.blocks[b] = blk
	for k := b + 1; k < len(l.starts); k++ {
		l.starts[k]--
	}
}

// reindexFrom recomputes block start positions from block b onwards.
func (l *entryList) reindexFrom(b int) {
	for k := b; k < len(l.blocks); k++ {
		if k == 0 {
			l.starts[k] = 0
		} else {
			l.starts[k] = l.starts[k-1] + len(l.blocks[k-1])
		}
	}
}

// Reset replaces the contents with all, which must already be in order.
// Blocks start three quarters full so that inserts have room.
func (l *entryList) Reset(all []Entry) {
	l.blocks, l.starts, l.n = nil, nil, len(all)
	size := blockMax * 3 / 4
	for p := 0; p < len(all); p += size {
		end := min(p+size, len(all))
		l.blocks = append(l.blocks, append([]Entry(nil), all[p:end]...))
		l.starts = append(l.starts, p)
	}
}

// All returns every entry in order, as one new slice.
func (l *entryList) All() []Entry {
	out := make([]Entry, 0, l.n)
	for _, blk := range l.blocks {
		out = append(out, blk...)
	}
	return out
}
