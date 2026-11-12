// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"strings"
	"testing"

	"nantucket/internal/screens"
	"nantucket/internal/vm"

	"github.com/gdamore/tcell/v2"
)

func TestStatusBar(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`dbCreate("cust", {{"NAME", "C", 10, 0}})
USE cust ALIAS c
APPEND BLANK
APPEND BLANK
APPEND BLANK
GO 2
DELETE
SET STATUS ON`)); err != nil {
		t.Fatal(err)
	}
	screens.RefreshStatus(e.vm)
	got := e.row(24)
	for _, want := range []string{"Area 1", "CUST.DBF (C)", "Rec 2/3", "Del"} {
		if !strings.Contains(got, want) {
			t.Errorf("status bar %q lacks %q", got, want)
		}
	}
	// scrolling output stays above the status bar
	if err := e.wait(e.run(`FOR i := 1 TO 40
? "line", i
NEXT`)); err != nil {
		t.Fatal(err)
	}
	if got := e.row(24); !strings.Contains(got, "Area 1") {
		t.Errorf("status bar scrolled away: %q", got)
	}
	if got := e.row(23); !strings.Contains(got, "line") {
		t.Errorf("row above status bar = %q", got)
	}
	if err := e.wait(e.run(`SET STATUS OFF`)); err != nil {
		t.Fatal(err)
	}
	screens.RefreshStatus(e.vm)
	if got := strings.TrimSpace(e.row(24)); got != "" {
		t.Errorf("status bar not removed: %q", got)
	}
}

func TestStatusBarNoTable(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`SET STATUS ON`)); err != nil {
		t.Fatal(err)
	}
	screens.RefreshStatus(e.vm)
	if got := e.row(24); !strings.Contains(got, "No table in use") {
		t.Errorf("status bar = %q", got)
	}
}

func TestDefaultColours(t *testing.T) {
	e := newSim(t)
	done := e.run(`CLS
cName := "abc  "
@ 5, 5 SAY "Name" GET cName
READ`)
	e.settle()
	white, blue, green := tcell.NewHexColor(0xFFFFFF), tcell.NewHexColor(0x0000AA), tcell.NewHexColor(0x00AA00)
	_, _, st, _ := e.sim.GetContent(5, 5)
	if fg, bg, _ := st.Decompose(); fg != white || bg != blue {
		t.Errorf("SAY = %v on %v, want white on blue", fg, bg)
	}
	_, _, st, _ = e.sim.GetContent(10, 5)
	if fg, bg, _ := st.Decompose(); fg != white || bg != green {
		t.Errorf("GET = %v on %v, want white on green", fg, bg)
	}
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
}

func TestVersionCommand(t *testing.T) {
	if vm.WhatsNew(vm.Version) == "" {
		t.Fatalf("WHATSNEW.md has no section for %s", vm.Version)
	}
	e := newSim(t)
	if err := e.wait(e.run(`CLS
VERSION`)); err != nil {
		t.Fatal(err)
	}
	scr := e.screen()
	if !strings.Contains(scr, "Nantucket "+vm.Version) || strings.Contains(scr, "What's new") {
		t.Errorf("VERSION should print just the version:\n%s", scr)
	}
	if e.eval("WNum()") != "0" {
		t.Error("VERSION should not open a window")
	}
}

func TestVersionFull(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`CLS
VERSION FULL
VERSION FULL
nWins := Len(WList())
nSel := WSelect()`)); err != nil {
		t.Fatal(err)
	}
	scr := e.screen()
	if !strings.Contains(scr, "1: Version "+vm.Version) || !strings.Contains(scr, "What's new in "+vm.Version) || !strings.Contains(scr, "Apache License 2.0") {
		t.Errorf("VERSION FULL window missing:\n%s", scr)
	}
	if e.str("nWins") != "1" || e.str("nSel") != "0" {
		t.Errorf("VERSION FULL twice: %s windows, selected %s", e.str("nWins"), e.str("nSel"))
	}
	// it sits with HELP; closing one leaves the other
	if err := e.wait(e.run(`HELP
WClose(1)
nLeft := Len(WList())`)); err != nil {
		t.Fatal(err)
	}
	if e.str("nLeft") != "1" || !strings.Contains(e.screen(), "Help") {
		t.Errorf("after closing VERSION FULL:\n%s", e.screen())
	}
}
