// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"io"
	"strings"
	"testing"

	"nantucket/internal/term"
)

// session is one Nantucket user. Two sessions in one test behave like two
// users: Linux locks belong to each open file, not to the process.
type session struct {
	t  *testing.T
	vm *VM
}

func newSession(t *testing.T) *session {
	m, err := New(term.NewStream(strings.NewReader(""), io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	m.PushBaseFrame()
	t.Cleanup(m.CloseAll)
	return &session{t, m}
}

// do runs statements and fails the test on an error.
func (s *session) do(src string) {
	s.t.Helper()
	if err := s.vm.Protect(func() { s.vm.ExecStatements(src) }); err != nil {
		s.t.Fatalf("%s: %s", src, ErrorReport(err))
	}
}

// fails runs statements that should raise an error containing want.
func (s *session) fails(src, want string) {
	s.t.Helper()
	err := s.vm.Protect(func() { s.vm.ExecStatements(src) })
	if err == nil || !strings.Contains(ErrorReport(err), want) {
		s.t.Fatalf("%s: got %v, want an error containing %q", src, err, want)
	}
}

// get evaluates an expression.
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

func sharedSetup(t *testing.T) (*session, *session) {
	t.Chdir(t.TempDir())
	a, b := newSession(t), newSession(t)
	a.do(`dbCreate("t", {{"NAME", "C", 10, 0}, {"NOTES", "M", 10, 0}})
USE t
APPEND BLANK
REPLACE name WITH "ALPHA"
INDEX ON name TO tn
CLOSE ALL`)
	return a, b
}

func TestExclusiveOpenConflicts(t *testing.T) {
	a, b := sharedSetup(t)
	a.do(`USE t`) // SET EXCLUSIVE is ON by default
	b.do(`USE t`)
	b.want("NetErr()", ".T.")
	b.want("Used()", ".F.")
	b.do(`USE t SHARED`)
	b.want("NetErr()", ".T.")
	a.do(`CLOSE ALL
USE t SHARED`)
	b.do(`USE t EXCLUSIVE`)
	b.want("NetErr()", ".T.")
	b.do(`USE t SHARED`)
	b.want("NetErr()", ".F.")
	b.want("Used()", ".T.")
}

// TestSharedAppendsKeepBothRecords is the Arch #2 report: two sessions
// appending to one table used to overwrite each other's record.
func TestSharedAppendsKeepBothRecords(t *testing.T) {
	a, b := sharedSetup(t)
	a.do(`USE t SHARED INDEX tn`)
	b.do(`USE t SHARED INDEX tn`)
	a.do(`APPEND BLANK
REPLACE name WITH "BRAVO", notes WITH "from a"`)
	b.do(`APPEND BLANK
REPLACE name WITH "CHARLIE", notes WITH "from b"`)
	a.do(`UNLOCK`)
	b.do(`UNLOCK`)
	a.want("LastRec()", "3")
	b.want("LastRec()", "3")
	// each sees the other's record through the shared index
	a.do(`SEEK "CHARLIE"`)
	a.want("Found()", ".T.")
	a.want("Trim(notes)", "from b")
	b.do(`SEEK "BRAVO"`)
	b.want("Found()", ".T.")
	b.want("Trim(notes)", "from a")
	a.do(`CLOSE ALL`)
	b.do(`CLOSE ALL`)
	// nothing lost, and the index on disk has all three in order
	a.do(`USE t INDEX tn
cAll := ""
DO WHILE !Eof()
   cAll += Trim(name) + " "
   SKIP
ENDDO`)
	a.want("cAll", "ALPHA BRAVO CHARLIE")
}

func TestRecordAndFileLocks(t *testing.T) {
	a, b := sharedSetup(t)
	a.do(`USE t SHARED`)
	b.do(`USE t SHARED`)
	a.want("RLock()", ".T.")
	a.want("IsLocked()", ".T.")
	b.want("RLock()", ".F.")
	b.want("FLock()", ".F.")
	b.want("Len(dbRLockList())", "0")
	a.want("dbRLockList()[1]", "1")
	a.do(`UNLOCK`)
	b.want("RLock()", ".T.")
	a.want("FLock()", ".F.")
	b.do(`UNLOCK`)
	a.want("FLock()", ".T.")
	b.want("RLock()", ".F.")
	a.do(`REPLACE name WITH "ALL MINE"`) // FLock covers every record
	a.do(`UNLOCK ALL`)
	b.want("RLock()", ".T.")
	b.do(`SKIP 0`)
	b.want("Trim(name)", "ALL MINE") // re-read from disk
}

func TestSharedWritesNeedLocks(t *testing.T) {
	a, _ := sharedSetup(t)
	a.do(`USE t SHARED`)
	a.fails(`REPLACE name WITH "X"`, "Lock required")
	a.fails(`DELETE`, "Lock required")
	a.fails(`PACK`, "Exclusive required")
	a.fails(`ZAP`, "Exclusive required")
	a.do(`RLock()
REPLACE name WITH "X"
UNLOCK`)
	a.want("Trim(name)", "X")
	// an exclusive table needs no locks
	a.do(`USE t EXCLUSIVE
REPLACE name WITH "Y"`)
	a.want("RLock()", ".T.")
}

func TestUseInUseNoteAtPrompt(t *testing.T) {
	a, b := sharedSetup(t)
	a.do(`USE t`)
	b.do(`USE t`)
	if note := b.vm.TakeNetErrNote(); !strings.Contains(note, "in use") {
		t.Errorf("note = %q", note)
	}
	b.do(`USE t
lErr := NetErr()`)
	if note := b.vm.TakeNetErrNote(); note != "" {
		t.Errorf("a program that checks NetErr() should get no note, got %q", note)
	}
}

// TestConcurrentSharedAppends has two sessions appending at the same time,
// as two users on a shared drive would. Every record and index key must
// survive.
func TestConcurrentSharedAppends(t *testing.T) {
	a, b := sharedSetup(t)
	const n = 300
	run := func(s *session, who string, done chan<- error) {
		done <- s.vm.Protect(func() {
			s.vm.ExecStatements(`USE t SHARED INDEX tn
FOR i := 1 TO ` + "300" + `
   APPEND BLANK
   REPLACE name WITH "` + who + `" + StrZero(i, 4), notes WITH "` + who + `"
   UNLOCK
NEXT
CLOSE ALL`)
		})
	}
	done := make(chan error, 2)
	go run(a, "A", done)
	go run(b, "B", done)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(ErrorReport(err))
		}
	}
	a.do(`USE t INDEX tn
nA := 0
nB := 0
nBad := 0
cBad := ""
cLast := " "
DO WHILE !Eof()
   IF !(cLast == " ") .AND. Trim(name) <= cLast
      nBad++
      cBad += "order:" + cLast + ">" + Trim(name) + " "
   ENDIF
   cLast := Trim(name)
   IF Left(name, 1) == "A" .AND. name != "ALPHA"
      nA++
      IF !(Trim(notes) == "A")
         nBad++
         cBad += "noteA:" + Trim(name) + "=" + Trim(notes) + " "
      ENDIF
   ELSEIF Left(name, 1) == "B"
      nB++
      IF !(Trim(notes) == "B")
         nBad++
         cBad += "noteB:" + Trim(name) + "=" + Trim(notes) + " "
      ENDIF
   ENDIF
   SKIP
ENDDO`)
	a.want("LastRec()", "601")
	a.want("nA", "300")
	a.want("nB", "300")
	a.want("nBad", "0")
	a.want("cBad", "")
	_ = n
}
