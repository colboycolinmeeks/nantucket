// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

// A runtime on a simulated full screen, for the core's own screen tests.
// (Tests of BROWSE, GET, windows and the editor are in internal/features,
// which links those features in.)

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"nantucket/internal/term"
)

type simEnv struct {
	t   *testing.T
	sim tcell.SimulationScreen
	vm  *VM
	dir string
}

func newSim(t *testing.T) *simEnv {
	sim := tcell.NewSimulationScreen("UTF-8")
	scr, err := term.NewScreenWith(sim)
	if err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 25)
	m, err := New(scr)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(old) })
	m.PushBaseFrame()
	return &simEnv{t: t, sim: sim, vm: m, dir: dir}
}

// run executes src in the background and returns a channel closed when done.
func (e *simEnv) run(src string) chan error {
	done := make(chan error, 1)
	go func() {
		done <- e.vm.Protect(func() { e.vm.ExecStatements(src) })
	}()
	return done
}

func (e *simEnv) keys(s string) {
	for _, r := range s {
		e.sim.InjectKey(tcell.KeyRune, r, tcell.ModNone)
		time.Sleep(2 * time.Millisecond)
	}
}

func (e *simEnv) key(k tcell.Key) {
	e.sim.InjectKey(k, 0, tcell.ModNone)
	time.Sleep(5 * time.Millisecond)
}

func (e *simEnv) mod(k tcell.Key, m tcell.ModMask) {
	e.sim.InjectKey(k, 0, m)
	e.settle()
}

func (e *simEnv) mouse(x, y int, b tcell.ButtonMask) {
	e.sim.InjectMouse(x, y, b, tcell.ModNone)
	time.Sleep(10 * time.Millisecond)
}

func (e *simEnv) screen() string {
	e.sim.Show()
	cells, w, h := e.sim.GetContents()
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				sb.WriteRune(c.Runes[0])
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (e *simEnv) row(n int) string {
	return strings.Split(e.screen(), "\n")[n]
}

func (e *simEnv) wait(done chan error) error {
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		e.t.Fatalf("timeout; screen:\n%s", e.screen())
	}
	return nil
}

func (e *simEnv) settle() { time.Sleep(60 * time.Millisecond) }

// str shows a memory variable's value.
func (e *simEnv) str(name string) string {
	v, _ := e.vm.GetMemvar(name)
	return strings.TrimSpace(e.vm.Display(v))
}

// eval evaluates an expression on the runtime.
func (e *simEnv) eval(expr string) string {
	if err := e.wait(e.run("zzEval := " + expr)); err != nil {
		e.t.Fatalf("%s: %v", expr, err)
	}
	return e.str("zzEval")
}

// idle runs the key loop briefly so the runtime handles mouse events.
func (e *simEnv) idle() {
	e.wait(e.run(`Inkey(0.1)`))
}
