// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nantucket/internal/vm"

	"github.com/gdamore/tcell/v2"
)

// TestDemoRuns plays through examples/demo.prg on a simulated screen,
// pressing Enter at every prompt and Esc to leave BROWSE and EDIT, and
// checks it finishes without an error and cleans up its files.
func TestDemoRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	src, err := filepath.Abs("../../examples/demo.prg")
	if err != nil {
		t.Fatal(err)
	}
	e := newSim(t)
	done := make(chan error, 1)
	go func() { done <- e.vm.RunFile(src, nil) }()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case err := <-done:
			// the tour returns to where it was started; it must not QUIT
			if _, quit := err.(*vm.QuitSignal); quit {
				t.Fatal("the demo should return, not QUIT")
			}
			if err != nil {
				t.Fatalf("demo failed: %s\n%s", vm.ErrorReport(err), e.screen())
			}
			// and it puts back the settings it changed
			for expr, want := range map[string]string{"Set(_SET_DATEFORMAT)": "mm/dd/yy", "Set(_SET_CENTURY)": ".F.",
				"Set(_SET_CONFIRM)": ".F.", "Set(_SET_SCOREBOARD)": ".T.", "Set(_SET_CURSOR)": "1"} {
				if got := e.eval(expr); got != want {
					t.Errorf("after the demo %s = %s, want %s", expr, got, want)
				}
			}
			if !strings.Contains(e.screen(), "Thanks for taking the Nantucket tour") {
				t.Fatalf("demo did not reach the end:\n%s", e.screen())
			}
			if left, _ := os.ReadDir(e.dir); len(left) != 0 {
				t.Errorf("demo left files behind: %v", left)
			}
			return
		case <-deadline:
			t.Fatalf("demo did not finish:\n%s", e.screen())
		case <-time.After(30 * time.Millisecond):
		}
		// read the screen only while the program waits for a key, so the
		// test never reads it while the program is drawing
		var scr string
		if !e.vm.WhenIdle(func() { scr = e.screen() }) {
			continue
		}
		if strings.Contains(scr, "BROWSE - Esc when done") || strings.Contains(scr, "Esc exit") {
			e.key(tcell.KeyEscape)
		} else {
			e.key(tcell.KeyEnter)
		}
	}
}
