// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package testrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, text string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(p), 0755)
	if err := os.WriteFile(p, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFind(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a_test.prg", "")
	write(t, dir, "b.prg", "")
	write(t, dir, "sub/c_test.prg", "")
	write(t, dir, ".hidden/d_test.prg", "")
	files, err := Find([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	if got := strings.Join(names, " "); got != "a_test.prg c_test.prg" {
		t.Errorf("Find = %s", got)
	}
}

func TestAssertions(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "calc_test.prg", `PUBLIC nBase := 10

PROCEDURE TestPass
   AssertEquals(12, nBase + 2)
   ASSERT nBase == 10
   AssertTrue("b" $ "abc")
   AssertFalse(Empty("x"))
   AssertEquals({1, {"a" => 2}}, {1, {"a" => 2}})
   AssertError({|| 1 / "x"})
RETURN

PROCEDURE TestFail
   AssertEquals("abc", "abd", "names")
   ASSERT 1 > 2
   AssertError({|| 1})
RETURN

PROCEDURE TestCrash
   x := NoSuchFunction()
RETURN
`)
	r := RunFile(f, Options{})
	if len(r.Tests) != 3 {
		t.Fatalf("tests = %+v", r.Tests)
	}
	if pass := r.Tests[0]; pass.Name != "TestPass" || len(pass.Failures) != 0 {
		t.Errorf("TestPass: %+v", pass)
	}
	fail := strings.Join(r.Tests[1].Failures, "\n")
	for _, want := range []string{
		`calc_test.prg:13: names: expected "abc", got "abd"`,
		`calc_test.prg:14: 1 > 2`,
		`calc_test.prg:15: expected an error, but none was raised`,
	} {
		if !strings.Contains(fail, want) {
			t.Errorf("TestFail failures missing %q:\n%s", want, fail)
		}
	}
	if crash := strings.Join(r.Tests[2].Failures, "\n"); !strings.Contains(crash, "Undefined function: NOSUCHFUNCTION") {
		t.Errorf("TestCrash: %s", crash)
	}
}

func TestExpectedOutput(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "out_test.prg", `ACCEPT "Name? " TO cName
? "Hello", cName
FOR i := 1 TO 3
   ? "line", Str(i, 1)
NEXT
`)
	write(t, dir, "out_test.in", "Ann\n")
	write(t, dir, "out_test.out", "Name?\nHello Ann\nline 1\nline 2\nline 3\n")
	if r := RunFile(f, Options{}); !r.Passed() {
		t.Fatalf("expected pass; output:\n%s\ndiff: %v", r.Output, r.OutDiff)
	}

	write(t, dir, "out_test.out", "Name?\nHello Bob\nline 1\nline 2\nline 3\n")
	r := RunFile(f, Options{})
	if r.Passed() {
		t.Fatal("expected an output mismatch")
	}
	got := strings.Join(r.OutDiff, "\n")
	if !strings.Contains(got, "- Hello Bob") || !strings.Contains(got, "+ Hello Ann") {
		t.Errorf("diff:\n%s", got)
	}

	// -update rewrites the expected output
	if r := RunFile(f, Options{Update: true}); !r.Updated {
		t.Fatal("expected .out to be written")
	}
	if r := RunFile(f, Options{}); !r.Passed() {
		t.Errorf("after update: %v", r.OutDiff)
	}
}

func TestRunAllReport(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ok_test.prg", "ASSERT .T.\n")
	write(t, dir, "bad_test.prg", "PROCEDURE TestOne\n   ASSERT .F. MESSAGE \"broken\"\nRETURN\n")
	files, _ := Find([]string{dir})
	var out bytes.Buffer
	if RunAll(files, Options{Out: &out}) {
		t.Fatal("expected failure")
	}
	report := out.String()
	for _, want := range []string{"FAIL  ", "TestOne", "bad_test.prg:2: broken", "ok    ", "2 files, 2 tests, 1 failed"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
}

func TestDiff(t *testing.T) {
	got := diff([]string{"a", "b", "c", "d", "e", "f", "g", "h"}, []string{"a", "b", "c", "X", "e", "f", "g", "h"})
	want := "  b\n  c\n- d\n+ X\n  e\n  f"
	if strings.Join(got, "\n") != want {
		t.Errorf("diff =\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
}

func TestTestPrgHint(t *testing.T) {
	t.Chdir(t.TempDir())
	if h := testPrgHint(nil); h != "" {
		t.Errorf("no test.prg: hint %q", h)
	}
	os.WriteFile("test.prg", []byte(`? 1`), 0o644)
	if h := testPrgHint(nil); !strings.Contains(h, "nantucket test.prg") {
		t.Errorf("hint = %q", h)
	}
	if h := testPrgHint([]string{"dir"}); h != "" {
		t.Errorf("with paths given: hint %q", h)
	}
}
