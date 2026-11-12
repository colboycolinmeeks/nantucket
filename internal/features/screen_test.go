// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestModiComm(t *testing.T) {
	e := newSim(t)
	done := e.run(`MODIFY COMMAND hello`)
	e.settle()
	e.keys(`? "Hello from MODI COMM"`)
	e.key(tcell.KeyEnter)
	e.keys(`x := 6 * 7`)
	e.key(tcell.KeyEnter)
	e.keys(`? x`)
	e.settle()
	scr := e.screen()
	if !strings.Contains(scr, "MODIFY COMMAND  hello.prg") || !strings.Contains(scr, `x := 6 * 7`) {
		t.Fatalf("editor screen wrong:\n%s", scr)
	}
	e.key(tcell.KeyCtrlW)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(e.dir, "hello.prg"))
	if err != nil {
		t.Fatal(err)
	}
	want := "? \"Hello from MODI COMM\"\nx := 6 * 7\n? x\n"
	if string(data) != want {
		t.Fatalf("saved %q want %q", data, want)
	}
	done = e.run(`CLEAR` + "\n" + `DO hello`)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	scr = e.screen()
	if !strings.Contains(scr, "Hello from MODI COMM") || !strings.Contains(scr, "42") {
		t.Fatalf("program output wrong:\n%s", scr)
	}
}

func TestSayGetRead(t *testing.T) {
	e := newSim(t)
	done := e.run(`CLEAR
cName := Space(10)
nAge := 0
dWhen := CToD("")
@ 2, 5 SAY "Name:" GET cName PICTURE "@!"
@ 3, 5 SAY "Age: " GET nAge PICTURE "999" VALID nAge < 150
@ 4, 5 SAY "When:" GET dWhen
READ
@ 10, 0 SAY "[" + cName + "]" + Str(nAge) + DToC(dWhen)`)
	e.settle()
	e.keys("smith")
	e.key(tcell.KeyEnter)
	e.keys("200")
	e.settle()
	// VALID fails: still on age; fix it
	e.key(tcell.KeyHome)
	e.keys("042")
	e.settle()
	e.keys("123124")
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	scr := e.screen()
	if !strings.Contains(scr, "[SMITH     ]        4212/31/24") {
		t.Fatalf("READ results wrong:\n%s", scr)
	}
}

func TestBrowseAndMenu(t *testing.T) {
	e := newSim(t)
	done := e.run(`dbCreate("t", {{"NAME","C",10,0},{"QTY","N",5,0}})
USE t
APPEND BLANK
REPLACE name WITH "apple", qty WITH 3
APPEND BLANK
REPLACE name WITH "pear", qty WITH 7
GO TOP
BROWSE`)
	e.settle()
	scr := e.screen()
	if !strings.Contains(scr, "apple") || !strings.Contains(scr, "pear") || !strings.Contains(scr, "NAME") {
		t.Fatalf("browse screen:\n%s", scr)
	}
	// edit qty of first record
	e.key(tcell.KeyRight)
	e.keys("12")
	e.key(tcell.KeyEnter)
	e.settle()
	e.key(tcell.KeyEscape)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	done = e.run(`GO TOP
@ 20,0 SAY "qty=" + Str(qty,5)
CLEAR
@ 5,5 PROMPT "One"
@ 6,5 PROMPT "Two"
@ 7,5 PROMPT "Three"
nChoice := 1
MENU TO nChoice
@ 12,0 SAY "choice=" + Str(nChoice,1)`)
	e.settle()
	e.key(tcell.KeyDown)
	e.key(tcell.KeyDown)
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	scr = e.screen()
	if !strings.Contains(scr, "choice=3") {
		t.Fatalf("menu:\n%s", scr)
	}
	v, _ := e.vm.GetMemvar("nChoice")
	_ = v
	done = e.run(`GO TOP
x := qty`)
	e.wait(done)
	if q, _ := e.vm.GetMemvar("x"); q.N != 12 {
		t.Fatalf("browse edit not saved, qty=%v", q.N)
	}
}
