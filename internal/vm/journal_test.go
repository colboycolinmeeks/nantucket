// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"nantucket/internal/db"
)

func ntxHeader(t *testing.T, path string) (version uint16, recs, dirty, logCount int, size int64) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return binary.LittleEndian.Uint16(b[2:]), int(binary.LittleEndian.Uint32(b[1004:])), int(b[1008]),
		int(binary.LittleEndian.Uint32(b[1016:])), int64(len(b))
}

// TestCommitAppendsToLog is Perf #2: COMMIT writes the changes as change
// log entries instead of rewriting the whole index.
func TestCommitAppendsToLog(t *testing.T) {
	t.Chdir(t.TempDir())
	s := newSession(t)
	s.do(`dbCreate("t", {{"NAME", "C", 10, 0}})
USE t
FOR i := 1 TO 5000
   APPEND BLANK
   REPLACE name WITH StrZero(i, 10)
NEXT
INDEX ON name TO tn
CLOSE ALL
USE t INDEX tn`)
	v0, _, _, _, size0 := ntxHeader(t, "tn.ntx")
	s.do(`APPEND BLANK
REPLACE name WITH "ZZZ"
COMMIT
APPEND BLANK
REPLACE name WITH "AAA"
COMMIT`)
	v1, recs, dirty, count, size1 := ntxHeader(t, "tn.ntx")
	if v1 != v0 {
		t.Error("COMMIT rewrote the whole index")
	}
	if recs != 5002 || dirty != 0 || count != 6 { // per record: blank key added, removed, real key added
		t.Errorf("after COMMIT: recs %d dirty %d log entries %d", recs, dirty, count)
	}
	if size1-size0 > 200 {
		t.Errorf("index grew by %d bytes for two records", size1-size0)
	}
	// what a session would read now (as after a crash) includes the log
	ix, err := db.LoadNTX("tn.ntx")
	if err != nil {
		t.Fatal(err)
	}
	if ix.Len() != 5002 || ix.Find("AAA       ", 5002) < 0 || strings.TrimSpace(ix.At(5001).Key) != "ZZZ" {
		t.Errorf("replayed index: %d keys, AAA at %d, last %q", ix.Len(), ix.Find("AAA       ", 5002), ix.At(ix.Len()-1).Key)
	}
	if stale, why := ix.Stale(5002); stale {
		t.Errorf("index should be up to date: %s", why)
	}
	// closing writes a plain index again
	s.do(`CLOSE ALL`)
	if _, _, _, count, _ := ntxHeader(t, "tn.ntx"); count != 0 {
		t.Errorf("log entries after close = %d", count)
	}
	s.do(`USE t INDEX tn
SEEK "AAA"`)
	s.want("Found()", ".T.")
	s.want("ordKeyCount()", "5002")
}

// TestSharedChangesUseLog: on a shared table each change is a log entry
// that the other session picks up, not a rewrite of the whole index.
func TestSharedChangesUseLog(t *testing.T) {
	a, b := sharedSetup(t)
	a.do(`USE t SHARED INDEX tn`)
	b.do(`USE t SHARED INDEX tn`)
	v0, _, _, _, _ := ntxHeader(t, "tn.ntx")
	for _, name := range []string{"DELTA", "ECHO", "BRAVO"} {
		a.do(`APPEND BLANK
REPLACE name WITH "` + name + `"
UNLOCK`)
	}
	if v1, _, _, count, _ := ntxHeader(t, "tn.ntx"); v1 != v0 || count == 0 {
		t.Errorf("version %d -> %d, log entries %d", v0, v1, count)
	}
	b.do(`GO TOP
cAll := ""
DO WHILE !Eof()
   cAll += Trim(name) + " "
   SKIP
ENDDO`)
	b.want("cAll", "ALPHA BRAVO DELTA ECHO")
}

// TestLogIgnoredAfterOtherProgram is Arch #7: if Clipper or Harbour
// updates an index after Nantucket logged changes, the log no longer
// describes the file, so it is not replayed: the index is rebuilt.
func TestLogIgnoredAfterOtherProgram(t *testing.T) {
	for _, change := range []string{"version", "appended page"} {
		t.Run(change, func(t *testing.T) {
			t.Chdir(t.TempDir())
			s := newSession(t)
			s.do(`dbCreate("t", {{"NAME", "C", 10, 0}})
USE t
APPEND BLANK
REPLACE name WITH "ALPHA"
INDEX ON name TO tn
APPEND BLANK
REPLACE name WITH "BRAVO"
COMMIT`)
			if _, _, _, count, _ := ntxHeader(t, "tn.ntx"); count == 0 {
				t.Fatal("expected change log entries")
			}
			// copy the files as they are now (as after a crash), then
			// change the index the way another program would
			data, _ := os.ReadFile("tn.ntx")
			s.do(`CLOSE ALL`)
			switch change {
			case "version":
				binary.LittleEndian.PutUint16(data[2:], binary.LittleEndian.Uint16(data[2:])+1)
			case "appended page":
				data = append(data, make([]byte, 1024)...)
			}
			os.WriteFile("tn.ntx", data, 0o644)
			s.do(`USE t INDEX tn
SEEK "BRAVO"`)
			s.want("Found()", ".T.")
			s.want("ordKeyCount()", "2")
			if notes := strings.Join(s.vm.TakeNotes(), " "); !strings.Contains(notes, "rebuilt") {
				t.Errorf("index should have been rebuilt; notes %q", notes)
			}
		})
	}
}
