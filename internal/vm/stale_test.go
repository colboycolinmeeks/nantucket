// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"os"
	"strings"
	"testing"
)

// TestIndexMissedRecordsIsRebuilt: records added while an index wasn't
// open used to be invisible in index order. Now the index notices.
func TestIndexMissedRecordsIsRebuilt(t *testing.T) {
	a, _ := sharedSetup(t)
	a.do(`USE t
APPEND BLANK
REPLACE name WITH "BRAVO"
CLOSE ALL
USE t INDEX tn
SEEK "BRAVO"`)
	a.want("Found()", ".T.")
	a.want("ordKeyCount()", "2")
	notes := strings.Join(a.vm.TakeNotes(), "\n")
	if !strings.Contains(notes, "tn.ntx was out of date") || !strings.Contains(notes, "saved with 1 records but the table has 2") {
		t.Errorf("notes = %q", notes)
	}
	// once rebuilt it is up to date: no note the next time
	a.do(`CLOSE ALL
USE t INDEX tn`)
	if n := a.vm.TakeNotes(); len(n) != 0 {
		t.Errorf("notes after rebuild = %q", n)
	}
}

// TestUnsavedFlagMeansRebuild: an index whose unsaved-changes flag is set
// on disk was not saved after a change, and is rebuilt when opened.
func TestUnsavedFlagMeansRebuild(t *testing.T) {
	a, _ := sharedSetup(t)
	setFlag(t, "tn.ntx", 1)
	a.do(`USE t INDEX tn`)
	if notes := strings.Join(a.vm.TakeNotes(), " "); !strings.Contains(notes, "not saved after its last change") {
		t.Errorf("notes = %q", notes)
	}
	a.do(`CLOSE ALL`)
	if ntxFlag(t, "tn.ntx") != 0 {
		t.Error("flag should be cleared after the rebuild")
	}
}

// TestSharedIndexLeftByStoppedSession: another session set the flag and
// was stopped before finishing. The next session to change the index
// rebuilds it rather than saving over it and hiding the problem.
func TestSharedIndexLeftByStoppedSession(t *testing.T) {
	a, b := sharedSetup(t)
	a.do(`USE t SHARED INDEX tn`)
	b.do(`USE t SHARED INDEX tn`)
	// a session that died after adding CHARLIE to the table but before
	// adding it to the index
	c := newSession(t)
	c.do(`USE t SHARED`)
	c.do(`APPEND BLANK
REPLACE name WITH "CHARLIE"
UNLOCK
CLOSE ALL`)
	setFlag(t, "tn.ntx", 1)
	b.do(`APPEND BLANK
REPLACE name WITH "BRAVO"
UNLOCK`)
	if ntxFlag(t, "tn.ntx") != 0 {
		t.Error("flag should be cleared")
	}
	for _, s := range []*session{a, b} {
		s.do(`SEEK "CHARLIE"`)
		s.want("Found()", ".T.")
		s.do(`SEEK "BRAVO"`)
		s.want("Found()", ".T.")
		s.want("ordKeyCount()", "3")
	}
}

func setFlag(t *testing.T, path string, v byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt([]byte{v}, 1008); err != nil {
		t.Fatal(err)
	}
}

func ntxFlag(t *testing.T, path string) byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b[1008]
}
