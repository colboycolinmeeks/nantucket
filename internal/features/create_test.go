// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCreateAndAppend(t *testing.T) {
	e := newSim(t)
	done := e.run(`CREATE people`)
	e.settle()
	// field 1: NAME C 12
	e.keys("name")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyEnter) // accept type C -> width
	e.keys("12")
	e.key(tcell.KeyEnter)
	// field 2: AGE N 3 0
	e.keys("age")
	e.key(tcell.KeyEnter)
	e.keys("N")
	e.key(tcell.KeyEnter)
	e.keys("3")
	e.key(tcell.KeyEnter)
	e.keys("0")
	e.key(tcell.KeyEnter)
	e.settle()
	scr := e.screen()
	if !strings.Contains(scr, "NAME") || !strings.Contains(scr, "Numeric") {
		t.Fatalf("structure editor:\n%s", scr)
	}
	e.key(tcell.KeyCtrlW)
	e.settle()
	// "Input data records now?" -> Yes
	e.key(tcell.KeyEnter)
	e.settle()
	e.keys("Alice")
	e.key(tcell.KeyEnter)
	e.keys("30")
	e.key(tcell.KeyEnter)
	e.settle()
	e.keys("Bob")
	e.key(tcell.KeyEnter)
	e.keys("41")
	e.key(tcell.KeyEnter)
	e.settle()
	e.key(tcell.KeyEscape)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	done = e.run(`nCount := RecCount()
GO BOTTOM
cLast := Trim(name) + Str(age, 3)`)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	n, _ := e.vm.GetMemvar("nCount")
	last, _ := e.vm.GetMemvar("cLast")
	if n.N != 2 || last.S != "Bob 41" {
		t.Fatalf("got %v records, last %q\n%s", n.N, last.S, e.screen())
	}
}
