// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"bytes"
	"os"
	"testing"
)

// recordBytes returns the first record of a DBF file, without the
// deletion flag.
func recordBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hdrLen := int(b[8]) | int(b[9])<<8
	recLen := int(b[10]) | int(b[11])<<8
	return b[hdrLen+1 : hdrLen+recLen]
}

// TestCodePageWrite is Arch #6: text is stored in the table's code page,
// so DOS programs read it.
func TestCodePageWrite(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NANTUCKET_CODEPAGE", "")
	s := newSession(t)
	s.do(`dbCreate("t", {{"NAME", "C", 6, 0}, {"NOTES", "M", 10, 0}})
USE t
APPEND BLANK
REPLACE name WITH "café", notes WITH "naïve"`)
	s.want(`name`, "café")
	s.want(`notes`, "naïve")
	s.want(`dbCodePage()`, "437")
	s.do(`CLOSE ALL`)
	if rec := recordBytes(t, "t.dbf"); !bytes.HasPrefix(rec, []byte("caf\x82  ")) {
		t.Errorf("stored % x, want CP437 é (82)", rec[:6])
	}
	if b, _ := os.ReadFile("t.dbf"); b[29] != 0x01 {
		t.Errorf("language driver ID %#x, want 0x01", b[29])
	}
	memo, _ := os.ReadFile("t.dbt")
	if !bytes.Contains(memo, []byte("na\x8bve")) {
		t.Error("memo not stored in CP437")
	}
	s.do(`USE t`)
	s.want(`notes`, "naïve")
}

// TestCodePageRead: legacy bytes are read in the right code page; text
// written by earlier versions (UTF-8) still reads in AUTO.
func TestCodePageRead(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NANTUCKET_CODEPAGE", "")
	s := newSession(t)
	s.do(`SET CODEPAGE TO UTF8
dbCreate("old", {{"NAME", "C", 8, 0}})
USE old
APPEND BLANK
REPLACE name WITH "café"
CLOSE ALL
SET CODEPAGE TO`)
	if b, _ := os.ReadFile("old.dbf"); b[29] != 0 {
		t.Errorf("UTF8 table has driver ID %#x", b[29])
	}
	s.want(`dbCodePage()`, "AUTO")
	s.do(`USE old`)
	s.want(`name`, "café")
	s.do(`APPEND BLANK
REPLACE name WITH "Müller"
CLOSE ALL`)

	// a DOS table: no driver ID, CP850 bytes
	b, _ := os.ReadFile("old.dbf")
	hdrLen := int(b[8]) | int(b[9])<<8
	copy(b[hdrLen+1:], "Ma\xa4ana  ") // ñ in 437 and 850
	os.WriteFile("dos.dbf", b, 0644)
	s.do(`USE dos`)
	s.want(`name`, "Mañana")
	s.do(`GO 2`)
	s.want(`name`, "Müller")

	// a Windows table: driver ID 0x03 means 1252, whatever SET CODEPAGE says
	b[29] = 0x03
	copy(b[hdrLen+1:], "Ma\xf1ana  ")
	os.WriteFile("win.dbf", b, 0644)
	s.do(`SET CODEPAGE TO 850
USE win`)
	s.want(`name`, "Mañana")
	s.want(`dbCodePage()`, "1252")
	s.fails(`SET CODEPAGE TO klingon`, "Unknown code page")
}

// TestCodePageIndex: keys are kept in the code page, so SEEK finds
// accented text and the order matches DOS programs (é after z in 437).
func TestCodePageIndex(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NANTUCKET_CODEPAGE", "")
	s := newSession(t)
	s.do(`dbCreate("t", {{"NAME", "C", 8, 0}})
USE t
APPEND BLANK
REPLACE name WITH "zebra"
APPEND BLANK
REPLACE name WITH "éclair"
APPEND BLANK
REPLACE name WITH "apple"
INDEX ON name TO tn
GO TOP`)
	s.want(`name`, "apple")
	s.do(`GO BOTTOM`)
	s.want(`name`, "éclair")
	s.do(`SEEK "écl"`)
	s.want(`Found()`, ".T.")
	s.want(`RecNo()`, "2")
	s.do(`CLOSE ALL
USE t INDEX tn
SEEK "éclair"`)
	s.want(`Found()`, ".T.")
}
