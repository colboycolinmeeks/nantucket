// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"nantucket/internal/report"
	"nantucket/internal/vm"
)

func designSetup(t *testing.T) *simEnv {
	e := newSim(t)
	if err := e.wait(e.run(`dbCreate("cust", {{"NAME", "C", 12, 0}, {"STATE", "C", 2, 0}, {"BAL", "N", 8, 2}})
USE cust
APPEND BLANK
REPLACE name WITH "Abbott", state WITH "MA", bal WITH 100
APPEND BLANK
REPLACE name WITH "Chen", state WITH "ME", bal WITH 25`)); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestReportDesigner(t *testing.T) {
	e := designSetup(t)
	done := e.run(`CREATE REPORT bal`)
	e.settle()
	if !strings.Contains(e.row(0), "CREATE REPORT  bal.frm") || !strings.Contains(e.screen(), "Page heading line 1") {
		t.Fatalf("designer:\n%s", e.screen())
	}
	// Layout page: the first heading line
	e.keys("Balances")
	e.key(tcell.KeyEnter)
	// Columns page: NAME, then BAL; sizes come from the fields
	e.key(tcell.KeyF4)
	e.settle()
	e.keys("name")
	e.key(tcell.KeyEnter) // contents -> heading (NAME), width 12
	e.key(tcell.KeyDown)  // a new column
	e.keys("bal")
	e.key(tcell.KeyEnter)
	e.keys("Balance") // typing replaces the heading
	e.key(tcell.KeyEnter)
	e.settle()
	scr := e.screen()
	if !strings.Contains(scr, "NAME") || !strings.Contains(scr, "Balance") || !strings.Contains(scr, "Report width: 21") {
		t.Fatalf("columns:\n%s", scr)
	}
	// Groups page: group on STATE
	e.key(tcell.KeyF3)
	e.settle()
	e.keys("state")
	e.key(tcell.KeyEnter)
	e.keys("State")
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyCtrlW)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("bal.frm")
	if err != nil {
		t.Fatal(err)
	}
	r, err := report.ParseFRM(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Heading) != 1 || r.Heading[0] != "Balances" || r.GroupExpr != "state" || r.GroupHeader != "State" ||
		len(r.Columns) != 2 || r.Columns[0].Width != 12 || r.Columns[1].Header[0] != "Balance" ||
		r.Columns[1].Decimals != 2 || !r.Columns[1].Total {
		t.Fatalf("report: %+v", r)
	}
	if err := e.wait(e.run(`REPORT FORM bal TO FILE out PLAIN NOCONSOLE`)); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile("out.txt")
	if !strings.Contains(string(out), "** State MA") || !strings.Contains(string(out), "125.00") {
		t.Errorf("printed:\n%s", out)
	}

	// MODIFY REPORT opens it on the columns; ^U deletes the first
	done = e.run(`MODIFY REPORT bal`)
	e.settle()
	if !strings.Contains(e.row(0), "MODIFY REPORT") || !strings.Contains(e.screen(), "Balance") {
		t.Fatalf("modify:\n%s", e.screen())
	}
	e.key(tcell.KeyCtrlU)
	e.key(tcell.KeyCtrlW)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile("bal.frm")
	if r, err = report.ParseFRM(data); err != nil || len(r.Columns) != 1 || r.Columns[0].Expr != "bal" {
		t.Fatalf("after modify: %+v %v", r, err)
	}
}

func TestReportDesignerChecks(t *testing.T) {
	e := designSetup(t)
	done := e.run(`CREATE REPORT bad`)
	e.settle()
	// no columns yet
	e.key(tcell.KeyCtrlW)
	e.settle()
	if !strings.Contains(e.screen(), "Add at least one column") {
		t.Fatalf("no columns:\n%s", e.screen())
	}
	// a column that runs code is refused
	e.keys(`FErase("x")`)
	e.key(tcell.KeyEnter)
	e.key(tcell.KeyCtrlW)
	e.settle()
	if !strings.Contains(e.screen(), "SET TRUSTED ON") {
		t.Fatalf("unsafe column:\n%s", e.screen())
	}
	// Esc asks before throwing the changes away
	e.key(tcell.KeyEscape)
	e.settle()
	if !strings.Contains(e.screen(), "Abandon report changes?") {
		t.Fatalf("abandon:\n%s", e.screen())
	}
	e.key(tcell.KeyRight)
	e.key(tcell.KeyEnter)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("bad.frm"); err == nil {
		t.Error("an abandoned report was saved")
	}
}

func TestLabelDesigner(t *testing.T) {
	e := designSetup(t)
	done := e.run(`CREATE LABEL mail`)
	e.settle()
	if !strings.Contains(e.screen(), "Label height") {
		t.Fatalf("designer:\n%s", e.screen())
	}
	// height 3, then down to line 1
	e.key(tcell.KeyDown)
	e.keys("3")
	e.key(tcell.KeyEnter)
	for i := 0; i < 5; i++ {
		e.key(tcell.KeyDown)
	}
	e.keys(`Trim(name), "(" + state + ")"`)
	e.key(tcell.KeyEnter)
	e.keys("bal")
	e.key(tcell.KeyEnter)
	e.settle()
	if strings.Contains(e.screen(), "Line 4") {
		t.Fatalf("height 3 shows a fourth line:\n%s", e.screen())
	}
	e.key(tcell.KeyCtrlW)
	if err := e.wait(done); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("mail.lbl")
	if err != nil {
		t.Fatal(err)
	}
	l, err := report.ParseLBL(data)
	if err != nil {
		t.Fatal(err)
	}
	if l.Height != 3 || l.Width != 35 || l.Lines[0] != `Trim(name), "(" + state + ")"` || l.Lines[1] != "bal" || l.Lines[2] != "" {
		t.Fatalf("label: %+v", l)
	}
	if err := e.wait(e.run(`GO TOP
LABEL FORM mail TO FILE labels NOCONSOLE NEXT 1`)); err != nil {
		t.Fatal(err)
	}
	if out, _ := os.ReadFile("labels.txt"); !strings.HasPrefix(string(out), "Abbott (MA)\r\n100\r\n") {
		t.Errorf("labels:\n%q", out)
	}
}

func TestDesignersNeedATerminal(t *testing.T) {
	s := newSession(t)
	for _, cmd := range []string{"CREATE REPORT x", "MODIFY LABEL x"} {
		err := s.vm.Protect(func() { s.vm.ExecStatements(cmd) })
		if err == nil || !strings.Contains(vm.ErrorReport(err), "requires an interactive terminal") {
			t.Errorf("%s: %v", cmd, err)
		}
	}
}
