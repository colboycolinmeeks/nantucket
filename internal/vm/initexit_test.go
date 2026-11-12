// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"io"
	"os"
	"strings"
	"testing"

	"nantucket/internal/term"
)

func runProgramFile(t *testing.T, name, src string) (string, error) {
	t.Helper()
	os.WriteFile(name, []byte(src), 0o644)
	var out strings.Builder
	m, err := New(term.NewStream(strings.NewReader(""), io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	m.Echo = &out
	m.PushBaseFrame()
	err = m.RunFile(name, nil)
	m.Close()
	return out.String(), err
}

func TestInitAndExitProcedures(t *testing.T) {
	t.Chdir(t.TempDir())
	os.WriteFile("lib.prg", []byte(`INIT PROCEDURE LibStart
   ? "lib init"
RETURN
FUNCTION Lib()
RETURN "lib"
`), 0o644)
	out, err := runProgramFile(t, "main.prg", `? "main"
SET PROCEDURE TO lib
? "after", Lib()
QUIT

INIT PROCEDURE First
   ? "init"
RETURN

EXIT PROCEDURE Last
   ? "exit"
RETURN
`)
	if _, quit := err.(*QuitSignal); err != nil && !quit {
		t.Fatal(ErrorReport(err))
	}
	_ = out
	got := strings.Join(strings.Fields(strings.ReplaceAll(out, "\n", "|")), " ")
	for _, want := range []string{"|init|main", "|lib init|after lib|exit"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q lacks %q", got, want)
		}
	}
}

func TestTextToFile(t *testing.T) {
	t.Chdir(t.TempDir())
	out, err := runProgramFile(t, "t.prg", `cName := "Ann"
TEXT TO FILE letter
Dear &cName,
Hello.
ENDTEXT
? "done"
`)
	if err != nil {
		t.Fatal(ErrorReport(err))
	}
	data, err := os.ReadFile("letter.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Dear Ann,\r\nHello.\r\n" {
		t.Errorf("letter.txt = %q", data)
	}
	if !strings.Contains(out, "Dear Ann,") || !strings.Contains(out, "done") {
		t.Errorf("screen output = %q", out)
	}
}
