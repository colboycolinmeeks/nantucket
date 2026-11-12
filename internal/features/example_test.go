// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"nantucket/internal/db"
	"nantucket/internal/vm"
)

func TestContactsExample(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "contacts.prg"))
	if err != nil {
		t.Fatal(err)
	}
	e := newSim(t)
	os.WriteFile(filepath.Join(e.dir, "contacts.prg"), src, 0644)
	done := e.run(`DO contacts`)
	e.settle()
	if s := e.screen(); !strings.Contains(s, "Add a contact") {
		t.Fatalf("menu not shown:\n%s", s)
	}
	e.key(tcell.KeyEnter) // Add
	e.settle()
	e.keys("alice")
	e.key(tcell.KeyEnter)
	e.keys("5551234567")
	e.key(tcell.KeyEnter) // SET CONFIRM ON: full fields need Enter
	e.settle()
	e.keys("Paris")
	e.key(tcell.KeyEnter)
	e.keys("01021990")
	e.key(tcell.KeyEnter)
	e.settle()
	if s := e.screen(); !strings.Contains(s, "Saved ALICE") {
		t.Fatalf("no confirmation:\n%s", s)
	}
	e.key(tcell.KeyEnter) // close alert
	e.settle()
	for i := 0; i < 4; i++ {
		e.key(tcell.KeyDown)
	}
	e.key(tcell.KeyEnter) // Quit
	err = e.wait(done)
	if _, ok := err.(*vm.QuitSignal); !ok && err != nil {
		t.Fatal(err)
	}
	tb, err := db.OpenTable(filepath.Join(e.dir, "contacts.dbf"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Close()
	buf := make([]byte, tb.RecLen)
	tb.ReadRecord(1, buf)
	got := strings.TrimSpace(tb.GetField(buf, 0).S) + "|" + strings.TrimSpace(tb.GetField(buf, 1).S) + "|" + strings.TrimSpace(tb.GetField(buf, 2).S)
	if tb.RecCount() != 1 || got != "ALICE|5551234567|Paris" {
		t.Fatalf("records=%d got %q", tb.RecCount(), got)
	}
}
