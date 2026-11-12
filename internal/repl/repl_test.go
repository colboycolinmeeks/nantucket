// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"nantucket/internal/term"
	"nantucket/internal/vm"
)

func TestDotPrompt(t *testing.T) {
	t.Setenv("NANTUCKET_HISTORY", filepath.Join(t.TempDir(), "history"))
	sim := tcell.NewSimulationScreen("UTF-8")
	scr, err := term.NewScreenWith(sim)
	if err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 25)
	m, err := vm.New(scr)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(old)
	done := make(chan struct{})
	go func() { Run(m, scr); close(done) }()
	typ := func(s string) {
		for _, r := range s {
			sim.InjectKey(tcell.KeyRune, r, tcell.ModNone)
			time.Sleep(time.Millisecond)
		}
		sim.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
		time.Sleep(20 * time.Millisecond)
	}
	typ(`x = 6`)
	typ(`FOR i = 1 TO 2`)
	typ(`? "loop", i * x`)
	typ(`NEXT`)
	typ(`? "done"`)
	typ(`QUIT`)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("dot prompt did not quit")
	}
	cells, w, h := sim.GetContents()
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if r := cells[y*w+x].Runes; len(r) > 0 {
				sb.WriteRune(r[0])
			}
		}
		sb.WriteByte('\n')
	}
	s := sb.String()
	for _, want := range []string{". x = 6", "loop          6", "loop         12", "done", Banner, "VERSION FULL for what's new"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in screen:\n%s", want, s)
		}
	}
	if last := strings.Split(s, "\n")[24]; !strings.Contains(last, "Area 1") {
		t.Fatalf("no status bar on the bottom row:\n%s", s)
	}
}

// TestHistoryIsKept: lines typed at the dot prompt are saved, and Up
// recalls them in the next session.
func TestHistoryIsKept(t *testing.T) {
	hist := filepath.Join(t.TempDir(), "history")
	t.Setenv("NANTUCKET_HISTORY", hist)
	t.Chdir(t.TempDir())
	session := func(script func(sim tcell.SimulationScreen)) string {
		sim := tcell.NewSimulationScreen("UTF-8")
		scr, err := term.NewScreenWith(sim)
		if err != nil {
			t.Fatal(err)
		}
		sim.SetSize(80, 25)
		m, _ := vm.New(scr)
		done := make(chan struct{})
		go func() { Run(m, scr); close(done) }()
		time.Sleep(30 * time.Millisecond)
		script(sim)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("dot prompt did not quit")
		}
		sim.Show()
		cells, w, h := sim.GetContents()
		var sb strings.Builder
		for i := 0; i < w*h; i++ {
			if r := cells[i].Runes; len(r) > 0 {
				sb.WriteRune(r[0])
			}
		}
		return sb.String()
	}
	typ := func(sim tcell.SimulationScreen, s string) {
		for _, r := range s {
			sim.InjectKey(tcell.KeyRune, r, tcell.ModNone)
			time.Sleep(time.Millisecond)
		}
		sim.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
		time.Sleep(20 * time.Millisecond)
	}
	session(func(sim tcell.SimulationScreen) {
		typ(sim, `? "remember " + "me"`)
		typ(sim, `QUIT`)
	})
	data, _ := os.ReadFile(hist)
	if !strings.Contains(string(data), `? "remember " + "me"`) {
		t.Fatalf("history file = %q", data)
	}
	out := session(func(sim tcell.SimulationScreen) {
		sim.InjectKey(tcell.KeyUp, 0, tcell.ModNone) // QUIT
		time.Sleep(10 * time.Millisecond)
		sim.InjectKey(tcell.KeyUp, 0, tcell.ModNone) // the ? line
		time.Sleep(10 * time.Millisecond)
		sim.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
		time.Sleep(20 * time.Millisecond)
		typ(sim, `QUIT`)
	})
	if !strings.Contains(out, "remember me") {
		t.Errorf("recalled line did not run:\n%s", out)
	}
}

// TestWindowKeys is UI #5: windows can be handled without a mouse.
func TestWindowKeys(t *testing.T) {
	t.Setenv("NANTUCKET_HISTORY", "off")
	t.Chdir(t.TempDir())
	sim := tcell.NewSimulationScreen("UTF-8")
	scr, err := term.NewScreenWith(sim)
	if err != nil {
		t.Fatal(err)
	}
	sim.SetSize(100, 30)
	m, _ := vm.New(scr)
	done := make(chan struct{})
	go func() { Run(m, scr); close(done) }()
	screen := func() string {
		time.Sleep(40 * time.Millisecond)
		var s string
		for i := 0; i < 50 && !m.WhenIdle(func() {
			sim.Show()
			cells, w, h := sim.GetContents()
			var sb strings.Builder
			for i := 0; i < w*h; i++ {
				if r := cells[i].Runes; len(r) > 0 {
					sb.WriteRune(r[0])
				} else {
					sb.WriteByte(' ')
				}
				if (i+1)%w == 0 {
					sb.WriteByte('\n')
				}
			}
			s = sb.String()
		}); i++ {
			time.Sleep(10 * time.Millisecond)
		}
		return s
	}
	key := func(k tcell.Key) { sim.InjectKey(k, 0, tcell.ModNone); time.Sleep(20 * time.Millisecond) }
	typ := func(s string) {
		for _, r := range s {
			sim.InjectKey(tcell.KeyRune, r, tcell.ModNone)
			time.Sleep(time.Millisecond)
		}
	}
	helpCol := func() int {
		for _, line := range strings.Split(screen(), "\n") {
			if i := strings.Index(line, "1: Help"); i >= 0 {
				return i
			}
		}
		return -1
	}

	key(tcell.KeyF1)
	if helpCol() < 0 {
		t.Fatalf("F1 should open HELP:\n%s", screen())
	}
	key(tcell.KeyF1)
	if helpCol() >= 0 {
		t.Fatalf("F1 again should close HELP:\n%s", screen())
	}
	// Esc on an empty line closes it too
	key(tcell.KeyF1)
	key(tcell.KeyEscape)
	if helpCol() >= 0 {
		t.Fatalf("Esc should close HELP:\n%s", screen())
	}
	// text typed before F1 is kept
	typ(`? "kept" + "!"`)
	key(tcell.KeyF1)
	key(tcell.KeyEnter)
	if !strings.Contains(screen(), "kept!") {
		t.Fatalf("typed line was lost:\n%s", screen())
	}
	// F6: arrows move the front window
	before := helpCol()
	key(tcell.KeyF6)
	if !strings.Contains(screen(), "Window: arrows move") {
		t.Fatalf("F6 should show the window mode hint:\n%s", screen())
	}
	key(tcell.KeyLeft)
	key(tcell.KeyLeft)
	key(tcell.KeyLeft)
	key(tcell.KeyEnter)
	if after := helpCol(); after != before-3 {
		t.Errorf("window moved from column %d to %d, want %d", before, after, before-3)
	}
	// X in window mode closes it
	key(tcell.KeyF6)
	typ("x")
	time.Sleep(20 * time.Millisecond)
	if helpCol() >= 0 {
		t.Errorf("X should close the window:\n%s", screen())
	}
	typ("QUIT")
	key(tcell.KeyEnter)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("dot prompt did not quit")
	}
}
