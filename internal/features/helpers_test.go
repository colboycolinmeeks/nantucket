// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

// Helpers for tests of the runtime with all its features: a runtime on a
// simulated full screen (simEnv) and plain sessions (session).

import (
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	_ "nantucket/internal/features"
	"nantucket/internal/term"
	"nantucket/internal/vm"
)

type simEnv struct {
	t   *testing.T
	sim tcell.SimulationScreen
	vm  *vm.VM
	dir string
}

func newSim(t *testing.T) *simEnv {
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
	scr.OnInterrupt = m.Interrupt
	scr.OnDebug = m.RequestDebug
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
	// GetContents returns tcell's live cell buffer, which the program
	// goroutine keeps drawing into; hold the screen's lock while reading it.
	if l, ok := e.sim.(sync.Locker); ok {
		l.Lock()
		defer l.Unlock()
	}
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

// settle waits until the screen has stopped changing: unchanged for
// 100ms, after at least 60ms, giving up after 5s. A fixed sleep was too
// short on slow machines, such as CI under the race detector.
func (e *simEnv) settle() {
	const step = 20 * time.Millisecond
	time.Sleep(60 * time.Millisecond)
	prev, still := e.screen(), 0
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && still < 5; {
		time.Sleep(step)
		if cur := e.screen(); cur == prev {
			still++
		} else {
			prev, still = cur, 0
		}
	}
}

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

// edit opens MODIFY COMMAND on name, runs keys (a function that sends
// them), saves with Ctrl-W and returns the saved file.
func (e *simEnv) edit(t *testing.T, name, initial string, keys func()) string {
	t.Helper()
	if initial != "" {
		os.WriteFile(name, []byte(initial), 0o644)
	}
	done := e.run(`MODIFY COMMAND ` + name)
	e.settle()
	keys()
	e.key(tcell.KeyCtrlW)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(name)
	return string(b)
}

// session is one Nantucket user on a plain console.
type session struct {
	t  *testing.T
	vm *vm.VM
}

func newSession(t *testing.T) *session {
	m, err := vm.New(term.NewStream(strings.NewReader(""), io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	m.PushBaseFrame()
	t.Cleanup(m.CloseAll)
	return &session{t, m}
}

func (s *session) do(src string) {
	s.t.Helper()
	if err := s.vm.Protect(func() { s.vm.ExecStatements(src) }); err != nil {
		s.t.Fatalf("%s: %s", src, vm.ErrorReport(err))
	}
}

func (s *session) get(expr string) string {
	s.t.Helper()
	s.do("zzGot := " + expr)
	v, _ := s.vm.GetMemvar("ZZGOT")
	return strings.TrimSpace(s.vm.Display(v))
}

func (s *session) want(expr, want string) {
	s.t.Helper()
	if got := s.get(expr); got != want {
		s.t.Errorf("%s = %s, want %s", expr, got, want)
	}
}
