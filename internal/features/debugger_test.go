// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

const debugProg = `PROCEDURE Main
   LOCAL nTotal := 0, i
   PRIVATE cName := "Ann"
   FOR i := 1 TO 3
      nTotal += Double(i)
   NEXT
   PUBLIC nResult
   nResult := nTotal
   RETURN

FUNCTION Double(n)
   LOCAL nOut := n * 2
   RETURN nOut
`

// debugSim writes the test program and starts it with the debugger
// stopping at its first line.
func debugSim(t *testing.T) (*simEnv, chan error) {
	e := newSim(t)
	if err := os.WriteFile("calc.prg", []byte(debugProg), 0o644); err != nil {
		t.Fatal(err)
	}
	e.vm.DebugAtStart()
	done := e.run(`DO calc`)
	e.settle()
	return e, done
}

func (e *simEnv) finish(done chan error) {
	e.t.Helper()
	if err := e.wait(done); err != nil {
		e.t.Fatal(err)
	}
}

func TestDebuggerStepsAndShowsVariables(t *testing.T) {
	e, done := debugSim(t)
	scr := e.screen()
	if !strings.Contains(e.row(0), "Debugger") || !strings.Contains(e.row(0), "MAIN  calc.prg(2)") {
		t.Fatalf("debugger at the first line:\n%s", scr)
	}
	if !regexpLine(scr, `►\s+2\s+LOCAL nTotal := 0, i`) || !strings.Contains(scr, "Call stack") {
		t.Fatalf("source and panes:\n%s", scr)
	}
	// step over the LOCAL and PRIVATE lines
	e.key(tcell.KeyF10)
	e.settle()
	e.key(tcell.KeyF10)
	e.settle()
	scr = e.screen()
	if !strings.Contains(e.row(0), "calc.prg(4)") || !regexpLine(scr, `NTOTAL\s+LOCAL\s+0`) || !regexpLine(scr, `CNAME\s+PRIVATE\s+"Ann"`) {
		t.Fatalf("after two steps:\n%s", scr)
	}
	// F8 twice goes into Double()
	e.key(tcell.KeyF8)
	e.settle()
	e.key(tcell.KeyF8)
	e.settle()
	scr = e.screen()
	if !strings.Contains(e.row(0), "DOUBLE  calc.prg(12)") || !regexpLine(scr, `N\s+LOCAL\s+1`) || !strings.Contains(scr, "MAIN(5)") {
		t.Fatalf("inside Double():\n%s", scr)
	}
	// F6 steps out, back to MAIN
	e.key(tcell.KeyF6)
	e.settle()
	if !strings.Contains(e.row(0), "MAIN  calc.prg(") {
		t.Fatalf("after step out:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.finish(done)
	if got := e.eval("nResult"); got != "12" {
		t.Errorf("nResult = %s", got)
	}
}

func TestDebuggerEvaluatesAndChanges(t *testing.T) {
	e, done := debugSim(t)
	e.key(tcell.KeyF10)
	e.settle()
	// an expression using a LOCAL
	e.keys("nTotal + 40")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "nTotal + 40 = 40") {
		t.Fatalf("evaluate:\n%s", e.screen())
	}
	// an error is shown, not sent to the program's error handler
	e.keys("nTotal + \"x\"")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "Error:") {
		t.Fatalf("error:\n%s", e.screen())
	}
	// change a LOCAL, and watch an expression
	e.keys("nTotal := 100")
	e.key(tcell.KeyEnter)
	e.keys("WATCH nTotal * 2")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "nTotal * 2 = 200") {
		t.Fatalf("watch:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.finish(done)
	if got := e.eval("nResult"); got != "112" {
		t.Errorf("nResult = %s, want 112 (the changed LOCAL)", got)
	}
}

func TestDebuggerBreakpoints(t *testing.T) {
	e, done := debugSim(t)
	// breakpoint on line 8 with the cursor and F9, then run to it
	for i := 0; i < 6; i++ {
		e.key(tcell.KeyDown)
	}
	e.key(tcell.KeyF9)
	e.settle()
	if !regexpLine(e.screen(), `●\s+8\s+nResult := nTotal`) {
		t.Fatalf("breakpoint mark:\n%s", e.screen())
	}
	// and one on a function
	e.keys("BP double")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.row(0), "Breakpoint at DOUBLE()") || !strings.Contains(e.row(0), "calc.prg(12)") {
		t.Fatalf("procedure breakpoint:\n%s", e.screen())
	}
	e.keys("BP double")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.row(0), "Breakpoint") || !strings.Contains(e.row(0), "calc.prg(8)") {
		t.Fatalf("line breakpoint:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.finish(done)
	// breakpoints stay for the next run
	done = e.run(`DO calc`)
	e.settle()
	if !strings.Contains(e.row(0), "calc.prg(8)") {
		t.Fatalf("second run:\n%s", e.screen())
	}
	e.keys("BC")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.finish(done)
}

func TestDebuggerAltDAndQuit(t *testing.T) {
	e := newSim(t)
	os.WriteFile("wait.prg", []byte("PROCEDURE Main\n   LOCAL n := 7\n   Inkey(0)\n   ? \"after\"\n   RETURN\n"), 0o644)
	done := e.run(`DO wait`)
	e.settle()
	// Alt-D while the program waits for a key
	e.sim.InjectKey(tcell.KeyRune, 'd', tcell.ModAlt)
	e.settle()
	scr := e.screen()
	if !strings.Contains(e.row(0), "Alt-D") || !strings.Contains(scr, "wait.prg(3)") || !regexpLine(scr, `N\s+LOCAL\s+7`) {
		t.Fatalf("Alt-D:\n%s", scr)
	}
	e.keys("QUIT")
	e.key(tcell.KeyEnter)
	err := e.wait(done)
	if err == nil || !strings.Contains(err.Error(), "INTERRUPTED") {
		t.Fatalf("QUIT: %v", err)
	}
}

func TestAltDFunction(t *testing.T) {
	e := newSim(t)
	os.WriteFile("brk.prg", []byte("PROCEDURE Main\n   LOCAL a := {1, \"two\", {3}}\n   AltD()\n   RETURN\n"), 0o644)
	done := e.run(`DO brk`)
	e.settle()
	scr := e.screen()
	if !strings.Contains(e.row(0), "AltD()") || !strings.Contains(scr, `{1, "two", {3}}`) {
		t.Fatalf("AltD():\n%s", scr)
	}
	// open the array
	e.key(tcell.KeyTab)
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), `[2] "two"`) {
		t.Fatalf("inspect:\n%s", e.screen())
	}
	e.key(tcell.KeyEscape)
	e.key(tcell.KeyF5)
	e.finish(done)
	// AltD(0) turns Alt-D off
	if err := e.wait(e.run(`AltD(0)`)); err != nil {
		t.Fatal(err)
	}
	if e.vm.RequestDebug() {
		t.Error("Alt-D still on after AltD(0)")
	}
	e.wait(e.run(`AltD(1)`))
}

// regexpLine reports whether a line of the screen matches a pattern.
func regexpLine(scr, pattern string) bool {
	re := regexp.MustCompile(pattern)
	for _, l := range strings.Split(scr, "\n") {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

func TestDebuggerConditionalBreakpoints(t *testing.T) {
	e, done := debugSim(t)
	// a line inside the loop, only on the third time round
	e.keys("BP 5 IF i == 3")
	e.key(tcell.KeyEnter)
	e.settle()
	scr := e.screen()
	if !strings.Contains(scr, "Breakpoint set at calc.prg(5) IF i == 3") || !regexpLine(scr, `◆\s+5\s+nTotal \+= Double\(i\)`) {
		t.Fatalf("set:\n%s", scr)
	}
	// a condition that doesn't compile is refused
	e.keys("BP 8 IF nTotal +")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "not a valid condition") || e.vm.HasBreak("calc.prg", 8) {
		t.Fatalf("bad condition:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.settle()
	scr = e.screen()
	if !strings.Contains(e.row(0), "Breakpoint IF i == 3") || !strings.Contains(e.row(0), "calc.prg(5)") || !regexpLine(scr, `I\s+LOCAL\s+3`) {
		t.Fatalf("stopped:\n%s", scr)
	}
	// clear it; a procedure breakpoint on its parameter instead
	e.keys("BP 5")
	e.key(tcell.KeyEnter)
	e.keys("BP double IF n == 3")
	e.key(tcell.KeyEnter)
	// and a line breakpoint whose condition is never true
	e.keys("BP 8 IF nTotal > 1000")
	e.key(tcell.KeyEnter)
	e.keys("BL")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "Breakpoints: DOUBLE() IF n == 3, CALC.PRG(8) IF nTotal > 1000") {
		t.Fatalf("list:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.settle()
	scr = e.screen()
	if !strings.Contains(e.row(0), "Breakpoint at DOUBLE() IF n == 3") || !regexpLine(scr, `N\s+LOCAL\s+3`) {
		t.Fatalf("procedure condition:\n%s", scr)
	}
	// nothing else stops the program
	e.key(tcell.KeyF5)
	e.finish(done)
	if got := e.eval("nResult"); got != "12" {
		t.Errorf("nResult = %s", got)
	}
}

func TestDebuggerConditionError(t *testing.T) {
	e, done := debugSim(t)
	// a condition that fails when run stops the program and says why
	e.keys(`BP 8 IF nTotal + "x"`)
	e.key(tcell.KeyEnter)
	e.keys(`BP 9 IF "not logical"`)
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.row(0), "condition error") || !strings.Contains(e.row(0), "calc.prg(8)") {
		t.Fatalf("condition error:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.row(0), "condition isn't logical") {
		t.Fatalf("not logical:\n%s", e.screen())
	}
	e.keys("BC")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.finish(done)
}

func TestDebuggerWatchpoints(t *testing.T) {
	e, done := debugSim(t)
	// a LOCAL of MAIN: NIL until line 2 runs
	e.keys("WP nTotal")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "The program will stop when nTotal changes (until MAIN() returns)") ||
		!regexpLine(e.screen(), `◉ nTotal in MAIN\(\) = NIL`) {
		t.Fatalf("set:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "Watchpoint: nTotal changed from NIL to 0 at calc.prg(2)") {
		t.Fatalf("first change:\n%s", e.screen())
	}
	// the next change is made by line 5, through a call to Double()
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "nTotal changed from 0 to 2 at calc.prg(5)") || !strings.Contains(e.row(0), "MAIN  calc.prg(5)") {
		t.Fatalf("second change:\n%s", e.screen())
	}
	// a change made in the debugger doesn't stop the program
	e.keys("nTotal := 10")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "nTotal changed from 10 to 14") {
		t.Fatalf("after changing it here:\n%s", e.screen())
	}
	// a PUBLIC that doesn't exist yet: F9 in the variables pane toggles
	e.keys("WP nTotal")
	e.key(tcell.KeyEnter)
	e.keys("WP nResult")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "Watchpoint: nResult changed from (undefined) to .F. at calc.prg(7)") {
		t.Fatalf("PUBLIC created:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "nResult changed from .F. to 20 at calc.prg(8)") {
		t.Fatalf("PUBLIC assigned:\n%s", e.screen())
	}
	e.keys("BC")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.finish(done)
}

func TestDebuggerWatchpointScope(t *testing.T) {
	e := newSim(t)
	os.WriteFile("arr.prg", []byte(`PROCEDURE Main
   LOCAL a := {1, 2}
   Fill(a)
   a[2] := 5
   RETURN

PROCEDURE Fill(x)
   LOCAL n := 0
   AltD()
   n := 1
   x[1] := 9
   RETURN
`), 0o644)
	done := e.run(`DO arr`)
	e.settle()
	// F9 on the selected variable: X, the first row
	e.key(tcell.KeyTab)
	e.key(tcell.KeyF9)
	e.keys("WP n")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "Watchpoint: n changed from 0 to 1 at arr.prg(10)") {
		t.Fatalf("n:\n%s", e.screen())
	}
	// a change inside an array counts
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "Watchpoint: X changed from {1, 2} to {9, 2} at arr.prg(11)") {
		t.Fatalf("array element:\n%s", e.screen())
	}
	// Fill() returns: its watchpoints go, and the program doesn't stop again
	e.key(tcell.KeyF5)
	e.finish(done)
	if notes := e.vm.TakeNotes(); strings.Join(notes, "|") != "Watchpoint on X removed: FILL() returned|Watchpoint on n removed: FILL() returned" {
		t.Errorf("notes: %q", notes)
	}
}

func TestDebuggerHitCounts(t *testing.T) {
	e, done := debugSim(t)
	// stop on the second time round the loop only
	e.keys("BP 5 HIT 2")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "Breakpoint set at calc.prg(5) HIT 2") || !regexpLine(e.screen(), `◆\s+5\s+nTotal`) {
		t.Fatalf("set:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.row(0), "Breakpoint, hit 2 (HIT 2)") || !regexpLine(e.screen(), `I\s+LOCAL\s+2`) {
		t.Fatalf("second hit:\n%s", e.screen())
	}
	// the count shows with the cursor on the line
	if !strings.Contains(e.screen(), "Breakpoint at line 5 HIT 2 (2 hits so far)") {
		t.Fatalf("count on the cursor line:\n%s", e.screen())
	}
	// Double(), counting only calls with an odd argument (1 was before)
	e.keys("BP 5")
	e.key(tcell.KeyEnter)
	e.keys("BP double IF n % 2 == 1 HIT >= 1")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "Breakpoint at DOUBLE() IF n % 2 == 1, hit 1 (HIT >= 1)") || !regexpLine(e.screen(), `N\s+LOCAL\s+3`) {
		t.Fatalf("procedure with condition and count:\n%s", e.screen())
	}
	e.keys("BC")
	e.key(tcell.KeyEnter)
	// every second time line 12 runs
	e.keys("BP 12 HIT % 2")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.finish(done)
	// set while stopped at line 12 in the last call: no more hits
	if bl := e.vm.Breaks(); len(bl) != 1 || bl[0].Hits != 0 {
		t.Fatalf("breaks after the run: %+v", bl)
	}

	// a new run starts the counts again: two calls to Double() reach line
	// 12 twice, and the second stops
	e.vm.ResetDebugStep() // as the dot prompt does before each command
	done = e.run(`DO calc`)
	e.settle()
	if !strings.Contains(e.row(0), "Breakpoint, hit 2 (HIT % 2)") || !regexpLine(e.screen(), `N\s+LOCAL\s+2`) {
		t.Fatalf("new run:\n%s", e.screen())
	}
	e.keys("BL")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "Breakpoints: CALC.PRG(12) HIT % 2 [2 hits]") {
		t.Fatalf("list:\n%s", e.screen())
	}
	e.keys("BP 12 HIT x")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "Error: a hit count is a number") {
		t.Fatalf("bad count:\n%s", e.screen())
	}
	e.keys("BC")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.finish(done)
}

func TestDebuggerLogpoints(t *testing.T) {
	e, done := debugSim(t)
	e.keys(`LP 5 "i={i} total={nTotal}"`)
	e.key(tcell.KeyEnter)
	e.keys(`LP double "n is {n}, name {cName}" IF n > 1 HIT >= 2`)
	e.key(tcell.KeyEnter)
	e.keys(`LP 8 "{nTotal + 'x'}"`)
	e.key(tcell.KeyEnter)
	e.keys(`LOGFILE dbg`)
	e.key(tcell.KeyEnter)
	e.settle()
	if !regexpLine(e.screen(), `◇\s+5\s+nTotal \+= Double\(i\)`) || !strings.Contains(e.screen(), "The log is also written to") {
		t.Fatalf("set:\n%s", e.screen())
	}
	// logpoints don't stop the program
	e.key(tcell.KeyF5)
	e.finish(done)
	lines, _ := e.vm.DebugLog()
	want := []string{
		"calc.prg(5) MAIN: i=1 total=0",
		"calc.prg(5) MAIN: i=2 total=2",
		"calc.prg(5) MAIN: i=3 total=6",
		"calc.prg(12) DOUBLE: n is 3, name Ann",
	}
	if len(lines) != 5 || strings.Join(lines[:4], "|") != strings.Join(want, "|") ||
		!strings.HasPrefix(lines[4], "calc.prg(8) MAIN: {nTotal + 'x': ") {
		t.Fatalf("log:\n%s", strings.Join(lines, "\n"))
	}
	if b, err := os.ReadFile("dbg.log"); err != nil || strings.Count(string(b), "\n") != 5 {
		t.Fatalf("log file: %q %v", b, err)
	}

	// the next stop says what was logged; F3 shows the log
	e.vm.ResetDebugStep()
	e.vm.ToggleBreak("calc.prg", 9)
	done = e.run(`DO calc`)
	e.settle()
	if !strings.Contains(e.screen(), "5 new log lines (F3), last: calc.prg(8) MAIN: {nTotal") {
		t.Fatalf("new lines:\n%s", e.screen())
	}
	e.key(tcell.KeyF3)
	e.settle()
	scr := e.screen()
	if !strings.Contains(scr, "Log  (10 lines, 5 new, also in dbg.log)") || !strings.Contains(scr, "calc.prg(12) DOUBLE: n is 3, name Ann") {
		t.Fatalf("log view:\n%s", scr)
	}
	e.key(tcell.KeyEscape)
	// BL lists logpoints with their messages; LP <line> clears one
	e.keys("LP 5")
	e.key(tcell.KeyEnter)
	e.keys("BL")
	e.key(tcell.KeyEnter)
	e.settle()
	if strings.Contains(e.screen(), "CALC.PRG(5)") || !strings.Contains(e.screen(), `DOUBLE() LOG "n is {n}, name {cName}" IF n > 1 HIT >= 2`) {
		t.Fatalf("list:\n%s", e.screen())
	}
	// a bad expression in the message is refused
	e.keys(`LP 6 "{nTotal +}"`)
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "Error: not a valid expression in the message: {nTotal +}") {
		t.Fatalf("bad message:\n%s", e.screen())
	}
	e.keys("BC")
	e.key(tcell.KeyEnter)
	e.keys("LOGFILE")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.finish(done)
}

func TestDebuggerDataBreakpoints(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`dbCreate("cust", {{"NAME", "C", 8, 0}, {"BAL", "N", 8, 2}})
USE cust
APPEND BLANK
REPLACE name WITH "Abbott", bal WITH 100
APPEND BLANK
REPLACE name WITH "Baker", bal WITH 50
APPEND BLANK
REPLACE name WITH "Chen", bal WITH 25
CLOSE ALL`)); err != nil {
		t.Fatal(err)
	}
	os.WriteFile("upd.prg", []byte(`PROCEDURE Main
   USE cust
   REPLACE ALL bal WITH bal * 2
   GO 2
   cust->name := "Zed"
   REPLACE bal WITH bal
   RETURN
`), 0o644)
	e.vm.DebugAtStart()
	done := e.run(`DO upd`)
	e.settle()
	e.key(tcell.KeyF10) // the table is open after line 2
	e.settle()
	e.keys("DB nosuch->bal")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "Error: no table open with the alias NOSUCH") {
		t.Fatalf("bad alias:\n%s", e.screen())
	}
	// every change from the second on, inside one REPLACE ALL
	e.keys("DB cust->bal HIT >= 2")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "Data breakpoint: CUST->BAL, record 2, 50.00 to 100.00, hit 2 (HIT >= 2)") ||
		!strings.Contains(e.row(0), "MAIN  upd.prg(3)") {
		t.Fatalf("second change:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), "CUST->BAL, record 3, 25.00 to 50.00") {
		t.Fatalf("third change:\n%s", e.screen())
	}
	// any field of the table, with a condition on the changed record
	e.keys("DB cust->bal")
	e.key(tcell.KeyEnter)
	e.keys(`DB cust->* IF Trim(name) == "Zed"`)
	e.key(tcell.KeyEnter)
	e.keys("BL")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), `Breakpoints: DB CUST->* IF Trim(name) == "Zed"`) {
		t.Fatalf("list:\n%s", e.screen())
	}
	e.key(tcell.KeyF5)
	e.settle()
	if !strings.Contains(e.screen(), `CUST->NAME, record 2, "Baker   " to "Zed`) || !strings.Contains(e.row(0), "upd.prg(5)") {
		t.Fatalf("assignment:\n%s", e.screen())
	}
	// "db := 1" is an expression, not the DB command
	e.keys("db := 1")
	e.key(tcell.KeyEnter)
	e.settle()
	if !strings.Contains(e.screen(), "db := 1 = 1") {
		t.Fatalf("expression:\n%s", e.screen())
	}
	// writing the same value isn't a change: the program runs to the end
	e.key(tcell.KeyF5)
	e.finish(done)
	// a REPLACE typed at the dot prompt doesn't stop
	if err := e.wait(e.run(`REPLACE name WITH "Typed"`)); err != nil {
		t.Fatal(err)
	}
	if n := len(e.vm.DataBreaks()); n != 1 {
		t.Fatalf("%d data breakpoints", n)
	}
	e.vm.ClearDataBreaks()
}
