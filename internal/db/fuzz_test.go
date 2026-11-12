// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"nantucket/internal/value"
)

// The fuzz tests feed damaged files to the readers. Opening or reading a
// damaged file may fail, but must never panic or run out of memory.
// Run one for longer with, for example:
//
//	go test ./internal/db -run x -fuzz FuzzOpenTable -fuzztime 1m

func sampleTable(t testing.TB, dir string) string {
	path := filepath.Join(dir, "s.dbf")
	fields := []FieldDef{{Name: "NAME", Type: 'C', Len: 10}, {Name: "N", Type: 'N', Len: 8, Dec: 2},
		{Name: "D", Type: 'D', Len: 8}, {Name: "L", Type: 'L', Len: 1}, {Name: "M", Type: 'M', Len: 10}}
	if err := CreateTable(path, fields); err != nil {
		t.Fatal(err)
	}
	tb, err := OpenTable(path, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		buf := tb.BlankRecord()
		tb.PutField(buf, 0, value.Str("ROW"))
		tb.PutField(buf, 1, value.Num(float64(i)*1.5, 2))
		tb.PutField(buf, 2, value.Date(value.ToJulian(2026, 11, i+1)))
		tb.PutField(buf, 3, value.Bool(i%2 == 0))
		tb.PutField(buf, 4, value.Str("memo text"))
		tb.AppendRecord(buf)
	}
	tb.Close()
	return path
}

func FuzzOpenTable(f *testing.F) {
	dir := f.TempDir()
	data, _ := os.ReadFile(sampleTable(f, dir))
	f.Add(data)
	f.Add(data[:40])
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "f.dbf")
		os.WriteFile(path, data, 0o644)
		// a memo file to go with it, from the sample
		tb, err := OpenTableMode(path, true, OpenNoLock)
		if err != nil {
			return
		}
		defer tb.Close()
		buf := make([]byte, tb.RecLen)
		for r := 1; r <= tb.RecCount() && r <= 20; r++ {
			if tb.ReadRecord(r, buf) != nil {
				return
			}
			for i := range tb.Fields {
				tb.GetField(buf, i)
			}
		}
	})
}

func FuzzParseNTX(f *testing.F) {
	ix := &Index{KeyLen: 10, KeyExpr: "NAME", TableRecs: 3}
	ix.SetEntries([]Entry{{"ALPHA     ", 1}, {"BRAVO     ", 2}, {"CHARLIE   ", 3}})
	f.Add(ix.encode())
	f.Fuzz(func(t *testing.T, data []byte) {
		ix, err := parseNTX("f.ntx", data)
		if err != nil {
			return
		}
		for i := 0; i < ix.Len() && i < 100; i++ {
			ix.At(i)
		}
		ix.Stale(3)
	})
}

func FuzzMemo(f *testing.F) {
	dir := f.TempDir()
	sampleTable(f, dir)
	dbt, _ := os.ReadFile(filepath.Join(dir, "s.dbt"))
	for _, typ := range []MemoType{MemoDBT3, MemoDBT4, MemoFPT} {
		f.Add(dbt, uint8(typ))
	}
	f.Fuzz(func(t *testing.T, data []byte, typ uint8) {
		path := filepath.Join(t.TempDir(), "f.dbt")
		os.WriteFile(path, data, 0o644)
		m, err := OpenMemo(path, MemoType(typ%3), true)
		if err != nil {
			return
		}
		defer m.Close()
		for blk := 0; blk < 8; blk++ {
			m.Read(blk)
		}
	})
}

// TestMemoLengthIsChecked: a memo claiming to be 4 GB long is reported as
// damaged rather than allocated.
func TestMemoLengthIsChecked(t *testing.T) {
	for _, typ := range []MemoType{MemoFPT, MemoDBT4} {
		path := filepath.Join(t.TempDir(), "m.fpt")
		if err := CreateMemo(path, typ); err != nil {
			t.Fatal(err)
		}
		m, err := OpenMemo(path, typ, false)
		if err != nil {
			t.Fatal(err)
		}
		blk, err := m.Write(0, "hello")
		if err != nil {
			t.Fatal(err)
		}
		if s, err := m.Read(blk); err != nil || s != "hello" {
			t.Fatalf("read back %q, %v", s, err)
		}
		// claim a length of 0xFFFFFFF0
		off := int64(blk) * int64(m.blockSize)
		if typ == MemoFPT {
			m.f.WriteAt([]byte{0xFF, 0xFF, 0xFF, 0xF0}, off+4)
		} else {
			m.f.WriteAt([]byte{0xF0, 0xFF, 0xFF, 0xFF}, off+4)
		}
		if _, err := m.Read(blk); !errors.Is(err, ErrBadMemo) {
			t.Errorf("type %d: got %v, want ErrBadMemo", typ, err)
		}
		m.Close()
	}
}
