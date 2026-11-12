// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestBrowseLocksSharedRecords checks that BROWSE on a shared table locks
// the record while editing it, and says so when someone else has it.
func TestBrowseLocksSharedRecords(t *testing.T) {
	e := newSim(t)
	setup := newSession(t)
	setup.do(`dbCreate("t", {{"NAME", "C", 10, 0}})
USE t
APPEND BLANK
REPLACE name WITH "ALPHA"
CLOSE ALL`)
	other := newSession(t)
	other.do(`USE t SHARED`)
	other.want("RLock()", ".T.")

	done := e.run(`USE t SHARED
BROWSE`)
	e.settle()
	e.keys("Z")
	e.settle()
	if !strings.Contains(e.screen(), "in use by another user") {
		t.Errorf("no in-use message:\n%s", e.screen())
	}
	other.do(`UNLOCK`)
	e.keys("ZETA")
	e.key(tcell.KeyEnter)
	e.settle()
	e.key(tcell.KeyEscape)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	other.do(`SKIP 0`)
	other.want("Trim(name)", "ZETAA") // typed over ALPHA
	other.want("RLock()", ".T.")      // BROWSE released its lock
}

// TestRuntimesDontShareState: each runtime has its own low-level file
// handles and menu prompts (the test runner makes one per test file).
func TestRuntimesDontShareState(t *testing.T) {
	t.Chdir(t.TempDir())
	a, b := newSession(t), newSession(t)
	a.do(`h := FCreate("x.txt")
@ 1, 1 PROMPT "left over"`)
	a.want("h", "5")
	b.want("FWrite(5, \"no\")", "0") // a's handle 5 isn't b's
	b.do(`h := FCreate("y.txt")`)
	b.want("h", "5")
	// b's MENU TO sees only b's prompt, not a's left-over one
	b.do(`@ 2, 2 PROMPT "mine"
KEYBOARD Chr(13)
nMenu := 0
MENU TO nMenu`)
	b.want("nMenu", "1")
	// Close releases a's handle: writing to it then fails
	a.vm.Close()
	a.want("FWrite(5, \"late\")", "0")
	if err := os.Remove("x.txt"); err != nil {
		t.Fatal(err)
	}
}
