// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestEditorStopsOnCompileError is UI #2: saving a program that doesn't
// compile keeps the editor open on the faulty line.
func TestEditorStopsOnCompileError(t *testing.T) {
	e := newSim(t)
	done := e.run(`MODIFY COMMAND bad.prg`)
	e.settle()
	e.keys(`? "ok"`)
	e.key(tcell.KeyEnter)
	e.keys(`? 1 +`)
	e.key(tcell.KeyEnter)
	e.keys(`? "end"`)
	e.key(tcell.KeyCtrlW)
	e.settle()
	scr := e.screen()
	if !strings.Contains(scr, "Line 2:") || !strings.Contains(scr, "MODIFY COMMAND") {
		t.Fatalf("editor should stay open showing the error:\n%s", scr)
	}
	// the work was saved anyway
	if b, _ := os.ReadFile("bad.prg"); !strings.Contains(string(b), `? 1 +`) {
		t.Errorf("file not saved: %q", b)
	}
	// the cursor is on line 2: fix it there
	e.key(tcell.KeyEnd)
	e.keys(` 1`)
	e.key(tcell.KeyCtrlW)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile("bad.prg"); !strings.Contains(string(b), "? 1 + 1\n") {
		t.Errorf("fixed file = %q", b)
	}
	// saving again without changes leaves anyway
	done = e.run(`MODIFY COMMAND bad2.prg`)
	e.settle()
	e.keys(`? (`)
	e.key(tcell.KeyCtrlW)
	e.settle()
	e.key(tcell.KeyCtrlW)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
}

func TestEditorUndo(t *testing.T) {
	e := newSim(t)
	got := e.edit(t, "u.txt", "", func() {
		e.keys("abc")
		e.key(tcell.KeyEnter)
		e.keys("def")
		e.key(tcell.KeyCtrlZ) // undo "def" (Ctrl-Z)
		e.key(tcell.KeyCtrlU) // undo Enter (Ctrl-U works too)
		e.keys("X")
	})
	if got != "abcX\n" {
		t.Errorf("after undo: %q", got)
	}
}

func TestEditorSelectCopyPaste(t *testing.T) {
	e := newSim(t)
	got := e.edit(t, "s.txt", "one two\n", func() {
		for i := 0; i < 3; i++ {
			e.mod(tcell.KeyRight, tcell.ModShift) // select "one"
		}
		e.mod(tcell.KeyInsert, tcell.ModCtrl) // copy
		e.key(tcell.KeyEnd)
		e.mod(tcell.KeyInsert, tcell.ModShift) // paste
		e.key(tcell.KeyHome)
		e.key(tcell.KeyHome)
		for i := 0; i < 4; i++ {
			e.mod(tcell.KeyRight, tcell.ModShift) // "one "
		}
		e.keys("1 ") // typing replaces the selection
	})
	if got != "1 twoone\n" {
		t.Errorf("got %q", got)
	}
	got = e.edit(t, "s2.txt", "keep\ncut me\nkeep\n", func() {
		e.key(tcell.KeyDown)
		e.mod(tcell.KeyDown, tcell.ModShift) // the whole second line
		e.mod(tcell.KeyDelete, tcell.ModShift)
		e.key(tcell.KeyDown)
		e.key(tcell.KeyEnd)
		e.key(tcell.KeyEnter)
		e.mod(tcell.KeyInsert, tcell.ModShift)
	})
	if got != "keep\nkeep\ncut me\n\n" {
		t.Errorf("cut and paste: %q", got)
	}
}

func TestEditorCtrlKBlocks(t *testing.T) {
	e := newSim(t)
	ctrlK := func(c rune) {
		e.key(tcell.KeyCtrlK)
		e.keys(string(c))
	}
	got := e.edit(t, "b.txt", "A\nB\nC\n", func() {
		e.key(tcell.KeyDown)
		ctrlK('b') // block starts at B
		e.key(tcell.KeyDown)
		ctrlK('k') // ... and ends at the start of C
		e.key(tcell.KeyDown)
		e.key(tcell.KeyEnd) // end of C
		e.key(tcell.KeyEnter)
		ctrlK('c') // copy "B\n" here
	})
	if got != "A\nB\nC\nB\n\n" {
		t.Errorf("copy: %q", got)
	}
	got = e.edit(t, "v.txt", "A\nB\nC\n", func() {
		ctrlK('b')
		e.key(tcell.KeyDown)
		ctrlK('k') // "A\n"
		e.key(tcell.KeyDown)
		e.key(tcell.KeyDown)
		e.key(tcell.KeyEnd)
		e.key(tcell.KeyEnter)
		ctrlK('v') // move it after C
	})
	if got != "B\nC\nA\n\n" {
		t.Errorf("move: %q", got)
	}
	got = e.edit(t, "y.txt", "A\nB\nC\n", func() {
		e.key(tcell.KeyDown)
		ctrlK('b')
		e.key(tcell.KeyDown)
		ctrlK('k')
		ctrlK('y') // delete "B\n"
	})
	if got != "A\nC\n" {
		t.Errorf("delete: %q", got)
	}
	got = e.edit(t, "z.txt", "A\nB\nC\n", func() {
		e.key(tcell.KeyDown)
		ctrlK('b')
		e.key(tcell.KeyDown)
		ctrlK('k')
		ctrlK('y')
		e.key(tcell.KeyCtrlU) // undo brings it back
		e.key(tcell.KeyCtrlU) // nothing more to undo
	})
	if got != "A\nB\nC\n" {
		t.Errorf("undo delete: %q", got)
	}
}
