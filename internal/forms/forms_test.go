// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package forms

import (
	"os"
	"strings"
	"testing"

	"nantucket/internal/report"
	"nantucket/internal/vmtest"
)

func formSetup(t *testing.T) *vmtest.Session {
	t.Chdir(t.TempDir())
	s := vmtest.New(t)
	s.Do(`dbCreate("cust", {{"NAME", "C", 12, 0}, {"STATE", "C", 2, 0}, {"BAL", "N", 8, 2}, {"STREET", "C", 20, 0}})
USE cust
APPEND BLANK
REPLACE name WITH "Abbott", state WITH "MA", bal WITH 100, street WITH "1 High St"
APPEND BLANK
REPLACE name WITH "Baker", state WITH "MA", bal WITH 50.5, street WITH ""
APPEND BLANK
REPLACE name WITH "Chen", state WITH "ME", bal WITH 25, street WITH "9 Elm Rd"
INDEX ON state + name TO custst`)
	return s
}

func writeForm(t *testing.T, name string, enc func() ([]byte, error)) {
	t.Helper()
	data, err := enc()
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(name, data, 0o644)
}

func readOut(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func TestReportForm(t *testing.T) {
	s := formSetup(t)
	r := &report.Report{
		Heading: []string{"Balances"}, GroupExpr: "state", GroupHeader: "State",
		PageWidth: 40, Lines: 58, EjectBefore: false,
		Columns: []report.Column{
			{Header: []string{"Name"}, Expr: "name", Width: 8},
			{Header: []string{"Balance"}, Expr: "bal", Width: 10, Decimals: 2, Total: true},
		},
	}
	writeForm(t, "bal.frm", r.Encode)
	s.Do(`REPORT FORM bal TO FILE out PLAIN`)
	got := readOut(t, "out.txt")
	want := `                Balances

Name        Balance

** State MA
Abbott       100.00
Baker         50.50
** Subtotal **
             150.50

** State ME
Chen          25.00
** Subtotal **
              25.00

*** Total ***
             175.50
`
	if got != want {
		t.Errorf("report:\n%s\nwant:\n%s", got, want)
	}
	// SUMMARY: totals only; FOR filters
	s.Do(`REPORT FORM bal TO FILE out2 PLAIN SUMMARY FOR state = "MA"`)
	got = readOut(t, "out2.txt")
	if strings.Contains(got, "Abbott") || !strings.Contains(got, "150.50") || strings.Contains(got, "ME") {
		t.Errorf("summary:\n%s", got)
	}
	// pages: page number and date, and a form feed when a page is full
	r.Lines = 8
	r.GroupExpr = ""
	writeForm(t, "bal.frm", r.Encode)
	s.Do(`REPORT FORM bal TO FILE out3 HEADING "Q1"`)
	got = readOut(t, "out3.txt")
	if !strings.HasPrefix(got, "Page No.     1\n") || !strings.Contains(got, "\fPage No.     2") || !strings.Contains(got, "Q1") {
		t.Errorf("paged report:\n%q", got)
	}
	// a long text value wraps within its column
	r.Columns[0].Expr = `name + " of " + street`
	writeForm(t, "bal.frm", r.Encode)
	s.Do(`GO TOP
REPORT FORM bal TO FILE out4 PLAIN NEXT 1`)
	if got := readOut(t, "out4.txt"); !strings.Contains(got, "Abbott       100.00\n    of 1\nHigh St\n") {
		t.Errorf("wrapping:\n%s", got)
	}
}

func TestLabelForm(t *testing.T) {
	s := formSetup(t)
	l := &report.Label{Height: 3, Width: 14, LeftMargin: 1, LinesBetween: 1, SpacesBetween: 2, Across: 2,
		Lines: []string{`Trim(name), "(" + state + ")"`, "street", `"USA"`}}
	writeForm(t, "mail.lbl", l.Encode)
	s.Do(`LABEL FORM mail TO FILE labels`)
	got := readOut(t, "labels.txt")
	want := ` Abbott (MA)     Baker (MA)
 1 High St       USA
 USA

 Chen (ME)
 9 Elm Rd
 USA

`
	if got != want {
		t.Errorf("labels:\n%q\nwant:\n%q", got, want)
	}
	s.Do(`GO BOTTOM
SKIP
LABEL FORM mail TO FILE stars SAMPLE REST`)
	if got := readOut(t, "stars.txt"); !strings.HasPrefix(got, " **************  **************\n") {
		t.Errorf("sample:\n%s", got)
	}
	s.Fails(`REPORT FORM missing`, "Open error")
	os.WriteFile("junk.frm", []byte("not a report"), 0o644)
	s.Fails(`REPORT FORM junk`, "not a valid report")
}

// TestFormCantRunCode is Sec #4 for report forms: a column expression
// that runs code is refused unless SET TRUSTED is ON.
func TestFormCantRunCode(t *testing.T) {
	s := formSetup(t)
	r := &report.Report{PageWidth: 40, Lines: 58, Columns: []report.Column{
		{Header: []string{"Name"}, Expr: `name + iif(FCreate("PWNED.txt") > 0, "", "")`, Width: 20}}}
	writeForm(t, "bad.frm", r.Encode)
	s.Fails(`REPORT FORM bad TO FILE out PLAIN`, "calls FCREATE()")
	if _, err := os.Stat("PWNED.txt"); err == nil {
		t.Fatal("the report form ran its code")
	}
	s.Do(`SET TRUSTED ON
REPORT FORM bad TO FILE out PLAIN`)
	if _, err := os.Stat("PWNED.txt"); err != nil {
		t.Error("with SET TRUSTED ON the form should run")
	}
}
