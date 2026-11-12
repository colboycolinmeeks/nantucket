// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package vmtest has helpers for testing xBase code from Go tests in any
// package.
package vmtest

import (
	"io"
	"strings"
	"testing"

	"nantucket/internal/term"
	"nantucket/internal/vm"
)

// Session is a runtime using a plain (stream) console.
type Session struct {
	T  *testing.T
	VM *vm.VM
}

// New starts a session, closed when the test ends.
func New(t *testing.T) *Session {
	t.Helper()
	m, err := vm.New(term.NewStream(strings.NewReader(""), io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	m.PushBaseFrame()
	t.Cleanup(m.CloseAll)
	return &Session{t, m}
}

// Do runs statements and fails the test on an error.
func (s *Session) Do(src string) {
	s.T.Helper()
	if err := s.VM.Protect(func() { s.VM.ExecStatements(src) }); err != nil {
		s.T.Fatalf("%s: %s", src, vm.ErrorReport(err))
	}
}

// Fails runs statements that should raise an error containing want.
func (s *Session) Fails(src, want string) {
	s.T.Helper()
	err := s.VM.Protect(func() { s.VM.ExecStatements(src) })
	if err == nil || !strings.Contains(vm.ErrorReport(err), want) {
		s.T.Fatalf("%s: got %v, want an error containing %q", src, err, want)
	}
}

// Get evaluates an expression and returns it as displayed, trimmed.
func (s *Session) Get(expr string) string {
	s.T.Helper()
	s.Do("zzGot := " + expr)
	v, _ := s.VM.GetMemvar("ZZGOT")
	return strings.TrimSpace(s.VM.Display(v))
}

// Want checks that an expression has the given value.
func (s *Session) Want(expr, want string) {
	s.T.Helper()
	if got := s.Get(expr); got != want {
		s.T.Errorf("%s = %s, want %s", expr, got, want)
	}
}
