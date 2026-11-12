// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestSetEscapeOff(t *testing.T) {
	e := newSim(t)
	done := e.run(`SET ESCAPE OFF
cName := Space(5)
@ 5, 5 GET cName
READ`)
	e.settle()
	e.key(tcell.KeyEscape) // ignored
	e.keys("abc")
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	if got := e.str("cName"); got != "abc" {
		t.Errorf("cName = %q, want abc (Esc should be ignored)", got)
	}
}

func TestSetIntensityOff(t *testing.T) {
	e := newSim(t)
	done := e.run(`SET COLOR TO "W/N,N/W,,,N/W"
SET INTENSITY OFF
cName := "abc  "
@ 5, 5 GET cName
READ`)
	e.settle()
	_, _, st, _ := e.sim.GetContent(5, 5)
	if fg, bg, _ := st.Decompose(); fg != tcell.NewHexColor(0xAAAAAA) || bg != tcell.NewHexColor(0x000000) {
		t.Errorf("GET with INTENSITY OFF = %v on %v, want standard W/N", fg, bg)
	}
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
}

func TestScoreboard(t *testing.T) {
	e := newSim(t)
	done := e.run(`@ 0, 0 SAY "title"
nAge := 0
@ 5, 5 GET nAge PICTURE "999" RANGE 1, 120
READ`)
	e.settle()
	e.key(tcell.KeyInsert)
	e.settle()
	if !strings.Contains(e.row(0), "Ins") {
		t.Errorf("scoreboard should show Ins:\n%s", e.row(0))
	}
	e.keys("500")
	e.settle()
	if !strings.Contains(e.row(0), "Range: 1 - 120") {
		t.Errorf("scoreboard should show the range:\n%s", e.row(0))
	}
	e.key(tcell.KeyCtrlY)
	e.keys("42")
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	if r := e.row(0); !strings.HasPrefix(r, "title") || strings.Contains(r, "Ins") || strings.Contains(r, "Range") {
		t.Errorf("row 0 should be restored after READ:\n%s", r)
	}
}

func TestScoreboardOff(t *testing.T) {
	e := newSim(t)
	done := e.run(`SET SCOREBOARD OFF
cName := Space(5)
@ 5, 5 GET cName
READ`)
	e.settle()
	e.key(tcell.KeyInsert)
	e.settle()
	if strings.Contains(e.row(0), "Ins") {
		t.Errorf("no scoreboard expected:\n%s", e.row(0))
	}
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
}

func TestMessageCenter(t *testing.T) {
	e := newSim(t)
	done := e.run(`SET MESSAGE TO 23 CENTER
n := 1
@ 5, 5 PROMPT "One" MESSAGE "hello"
MENU TO n`)
	e.settle()
	if r := e.row(23); strings.Index(r, "hello") < 30 {
		t.Errorf("message should be centred:\n%q", r)
	}
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
}

func TestAChoiceSelectable(t *testing.T) {
	e := newSim(t)
	done := e.run(`n := AChoice(5, 5, 10, 20, {"Apple", "Banana", "Cherry", "Date"}, {.T., .F., "1 == 1", .T.})`)
	e.settle()
	e.key(tcell.KeyDown) // skips Banana
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	if got := e.str("n"); got != "3" {
		t.Errorf("AChoice = %s, want 3 (Banana is not selectable)", got)
	}
}

func TestAChoiceUserFunction(t *testing.T) {
	e := newSim(t)
	src := `FUNCTION Pick(nMode, nCur, nRow)
   AAdd(aModes, nMode)
   IF nMode == 3              // AC_EXCEPT
      IF LastKey() == 13
         RETURN 1             // AC_SELECT
      ELSEIF LastKey() == 27
         RETURN 0             // AC_ABORT
      ENDIF
      RETURN 3                // AC_GOTO: jump by letter
   ENDIF
RETURN 2                      // AC_CONT
`
	if _, err := e.vm.LoadSource("PICK", src); err != nil {
		t.Fatal(err)
	}
	done := e.run(`PUBLIC aModes := {}
n := AChoice(5, 5, 10, 20, {"Apple", "Banana", "Cherry"}, .T., "Pick")
cModes := ""
AEval(aModes, {|m| cModes += Str(m, 1)})`)
	e.settle()
	e.key(tcell.KeyUp) // at the top: AC_HITTOP
	e.keys("c")        // exception, user function asks for a letter jump
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	if got := e.str("n"); got != "3" {
		t.Errorf("AChoice = %s, want 3", got)
	}
	if modes := e.str("cModes"); !strings.Contains(modes, "1") || !strings.Contains(modes, "3") {
		t.Errorf("user function modes = %s, want HITTOP (1) and EXCEPT (3)", modes)
	}
}
