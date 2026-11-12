// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"io"
	"os"
	"strings"
	"testing"

	"nantucket/internal/term"
)

func TestTrace(t *testing.T) {
	t.Chdir(t.TempDir())
	os.WriteFile("calc.prg", []byte(`x := Twice(21)
? x

FUNCTION Twice(n)
RETURN n * 2
`), 0o644)
	m, err := New(term.NewStream(strings.NewReader(""), io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	var trace strings.Builder
	m.SetTrace(&trace)
	m.PushBaseFrame()
	if err := m.RunFile("calc.prg", nil); err != nil {
		t.Fatal(ErrorReport(err))
	}
	got := trace.String()
	for _, want := range []string{"calc.prg(1) CALC: x := Twice(21)", "-> TWICE(1 argument(s))", "calc.prg(5) TWICE: RETURN n * 2", "calc.prg(2) CALC: ? x"} {
		if !strings.Contains(got, want) {
			t.Errorf("trace lacks %q:\n%s", want, got)
		}
	}
}

func TestEchoAndStep(t *testing.T) {
	t.Chdir(t.TempDir())
	os.WriteFile("p.prg", []byte(`SET ECHO ON
a := 1
SET ECHO OFF
b := 2
SET STEP ON
c := 3
d := 4
e := 5
? "finished", e
`), 0o644)
	// one key to step past "c := 3", then Esc stops stepping
	keys := strings.NewReader(" \x1b")
	m, err := New(term.NewStream(keys, io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	m.Echo = &out
	m.PushBaseFrame()
	if err := m.RunFile("p.prg", nil); err != nil {
		t.Fatal(ErrorReport(err))
	}
	got := out.String()
	for _, want := range []string{"p.prg(2) P: a := 1", "p.prg(6) P: c := 3", "p.prg(7) P: d := 4"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	for _, not := range []string{"b := 2", "e := 5"} {
		if strings.Contains(got, not) {
			t.Errorf("%q should not be shown:\n%s", not, got)
		}
	}
	if !strings.Contains(got, "finished") {
		t.Errorf("program did not finish:\n%s", got)
	}
}
