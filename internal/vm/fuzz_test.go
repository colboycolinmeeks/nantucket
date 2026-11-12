// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"os"
	"strings"
	"testing"
)

// FuzzParseMem feeds damaged .mem files to RESTORE's reader.
func FuzzParseMem(f *testing.F) {
	// one character variable, CNAME = "Ann", as SAVE TO writes it
	hdr := make([]byte, 32)
	copy(hdr, "CNAME")
	hdr[11] = 'C' | 0x80
	hdr[16] = 4
	data := append(append(hdr, "Ann\x00"...), 0x1A)
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		parseMem(data)
	})
}

// TestDamagedFilesAreReported: damaged tables and .mem files give a clear
// error, not an internal one.
func TestDamagedFilesAreReported(t *testing.T) {
	a, _ := sharedSetup(t)
	// the review's example: a field widened to 250 in the header
	data, _ := os.ReadFile("t.dbf")
	data[32+16] = 250
	os.WriteFile("bad.dbf", data, 0o644)
	a.fails(`USE bad`, "field NAME goes past the end")
	// a table cut short opens with the records that are there
	a.do(`USE t
APPEND BLANK
REPLACE name WITH "BRAVO"
CLOSE ALL`)
	data, _ = os.ReadFile("t.dbf")
	os.WriteFile("short.dbf", data[:len(data)-5], 0o644)
	os.WriteFile("short.dbt", mustRead(t, "t.dbt"), 0o644)
	a.do(`USE short`)
	a.want("LastRec()", "1")
	a.do(`CLOSE ALL
PRIVATE cName := "Ann"
SAVE TO good`)
	mem := mustRead(t, "good.mem")
	os.WriteFile("bad.mem", mem[:len(mem)-3], 0o644)
	a.fails(`RESTORE FROM bad`, "Corruption detected")
	if err := a.vm.Protect(func() { a.vm.ExecStatements(`RESTORE FROM bad`) }); err != nil && strings.Contains(ErrorReport(err), "runtime error") {
		t.Errorf("internal error: %s", ErrorReport(err))
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
