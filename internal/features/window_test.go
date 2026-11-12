// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"nantucket/internal/term"
	"nantucket/internal/vm"
)

func TestWindowOutput(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`CLS
? "base"
n := WOpen(2, 10, 8, 40, "Cust")
? "inside"
@ 2, 3 SAY "said"
nRows := MaxRow()
nCols := MaxCol()
nCur := WSelect()
SET WINDOW TO 0
? "base again"
nTop := WRow(n)
nLast := WLastCol(n)`)); err != nil {
		t.Fatal(err)
	}
	for v, want := range map[string]string{"n": "1", "nRows": "4", "nCols": "28", "nCur": "1", "nTop": "2", "nLast": "40"} {
		if got := e.str(v); got != want {
			t.Errorf("%s = %s, want %s", v, got, want)
		}
	}
	if !strings.Contains(e.row(2), "1: Cust") || !strings.Contains(e.row(2), "[_][↑][x]") {
		t.Errorf("title bar = %q", e.row(2))
	}
	if got := e.row(4); !strings.Contains(got, "║inside") {
		t.Errorf("window text row = %q", got)
	}
	if !strings.Contains(e.row(5), "said") {
		t.Errorf("@ SAY in window = %q", e.row(5))
	}
	if err := e.wait(e.run(`WClose(1)`)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.screen(), "Cust") || !strings.Contains(e.screen(), "base again") {
		t.Errorf("after close:\n%s", e.screen())
	}
}

func TestWindowChecks(t *testing.T) {
	e := newSim(t)
	for _, src := range []string{
		`WOpen(0, 0, 1, 10)`,   // too small
		`WOpen(20, 0, 30, 10)`, // off the screen
		`WOpen(-1, 0, 5, 10)`,  // negative
		`WOpen("a", 0, 5, 10)`, // not a number
		`WSelect(7)`,           // no such window
		`WClose(0)`,            // the full screen
		`WMove(1, 1, 3)`,       // no such window
		`WTitle("x", 0)`,       // window 0 has no title
	} {
		if err := e.wait(e.run(src)); err == nil {
			t.Errorf("%s: expected an error", src)
		}
	}
	if err := e.wait(e.run(`n := WOpen(1, 1, 5, 20)
WMove(30, 0)`)); err == nil {
		t.Error("moving off the screen should fail")
	}
	if err := e.wait(e.run(`n := WOpen(1, 1, 5, 20, "Fixed", , .F.)
WMaximize()`)); err == nil {
		t.Error("maximising a window that is not sizable should fail")
	}
}

func TestWindowColours(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`SET COLOR TO GR+/N
n := WOpen(2, 2, 8, 30, "c", "N/W")
c1 := SetColor()
SET WINDOW TO 0
c0 := SetColor()
SET WINDOW TO n
c2 := SetColor()`)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(e.str("c1"), "N/W") || !strings.HasPrefix(e.str("c2"), "N/W") {
		t.Errorf("window colours = %q / %q", e.str("c1"), e.str("c2"))
	}
	if !strings.HasPrefix(e.str("c0"), "GR+/N") {
		t.Errorf("window 0 colours = %q", e.str("c0"))
	}
}

func TestWindowStates(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`n := WOpen(3, 3, 9, 33, "Box")
WMaximize()
s1 := WState()
r1 := MaxRow()
WMinimize()
s2 := WState()
WRestore()
s3 := WState()
r3 := MaxRow()`)); err != nil {
		t.Fatal(err)
	}
	if e.str("s1") != "2" || e.str("r1") != "22" || e.str("s2") != "1" || e.str("s3") != "2" || e.str("r3") != "22" {
		t.Errorf("states: max=%s rows=%s min=%s restored=%s rows=%s", e.str("s1"), e.str("r1"), e.str("s2"), e.str("s3"), e.str("r3"))
	}
	if err := e.wait(e.run(`WRestore()
r4 := MaxRow()
WMinimize()`)); err != nil {
		t.Fatal(err)
	}
	if e.str("r4") != "4" {
		t.Errorf("restored rows = %s, want 4", e.str("r4"))
	}
	if !strings.Contains(e.row(24), "[1 Box ↑]") {
		t.Errorf("minimised tab missing: %q", e.row(24))
	}
}

func TestWindowMouse(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`n := WOpen(2, 10, 8, 40, "Drag")
? "x"
SET WINDOW TO 0`)); err != nil {
		t.Fatal(err)
	}
	// drag the title bar down 5 rows and right 4 columns
	e.mouse(15, 2, tcell.Button1)
	e.mouse(19, 7, tcell.Button1)
	e.mouse(19, 7, tcell.ButtonNone)
	e.idle()
	if e.eval("WRow(1)") != "7" || e.eval("WCol(1)") != "14" {
		t.Fatalf("after drag: top %s left %s", e.eval("WRow(1)"), e.eval("WCol(1)"))
	}
	// drag the resize corner (bottom right is 13, 44)
	e.mouse(44, 13, tcell.Button1)
	e.mouse(54, 16, tcell.Button1)
	e.mouse(54, 16, tcell.ButtonNone)
	e.idle()
	if e.eval("WLastRow(1)") != "16" || e.eval("WLastCol(1)") != "54" {
		t.Fatalf("after resize: bottom %s right %s", e.eval("WLastRow(1)"), e.eval("WLastCol(1)"))
	}
	// [x] is the last control: right edge - 2
	e.mouse(52, 7, tcell.Button1)
	e.mouse(52, 7, tcell.ButtonNone)
	e.idle()
	if e.eval("WNum()") != "0" {
		t.Fatalf("close button did not close the window:\n%s", e.screen())
	}
}

func TestWindowNeedsScreen(t *testing.T) {
	m, err := vm.New(term.NewStdStream())
	if err != nil {
		t.Fatal(err)
	}
	m.PushBaseFrame()
	if err := m.Protect(func() { m.ExecStatements(`WOpen(1, 1, 5, 20)`) }); err == nil {
		t.Error("WOpen on a stream console should fail")
	}
	if err := m.Protect(func() { m.ExecStatements(`SET WINDOW TO 0`) }); err != nil {
		t.Errorf("SET WINDOW TO 0 on a stream console: %v", err)
	}
}

func TestHelpWindow(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`CLS
HELP`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.screen(), "1: Help") || !strings.Contains(e.screen(), "USE <file>") {
		t.Fatalf("no help window:\n%s", e.screen())
	}
	// output keeps going to the full screen
	if e.eval("WSelect()") != "0" {
		t.Error("HELP should not select its window")
	}
	// the wheel scrolls it
	e.mouse(60, 5, tcell.WheelDown)
	e.mouse(60, 5, tcell.ButtonNone)
	e.idle()
	if !strings.Contains(e.row(0), "Help 4-") {
		t.Errorf("help did not scroll: %q", e.row(0))
	}
	// HELP again brings it back after minimising rather than opening another
	if err := e.wait(e.run(`WMinimize(1)
HELP
nWins := Len(WList())
nState := WState(1)`)); err != nil {
		t.Fatal(err)
	}
	if e.str("nWins") != "1" || e.str("nState") != "0" {
		t.Errorf("HELP again: %s windows, state %s", e.str("nWins"), e.str("nState"))
	}
}
