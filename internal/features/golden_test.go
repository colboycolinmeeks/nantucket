// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nantucket/internal/term"
	"nantucket/internal/vm"
)

var update = flag.Bool("update", false, "rewrite golden .out files")

// runProgram runs a .prg in a temp dir with a stream console and returns its output.
func runProgram(t *testing.T, prg string, input string) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("testdata", prg))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// copy all testdata programs so DO / SET PROCEDURE can find them
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(src), "*.prg"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0644)
	}
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)
	var out bytes.Buffer
	con := term.NewStream(strings.NewReader(input), &out)
	m, err := vm.New(con)
	if err != nil {
		t.Fatal(err)
	}
	m.PushBaseFrame()
	err = m.RunFile(filepath.Join(dir, prg), nil)
	m.CloseAll()
	if err != nil {
		if _, ok := err.(*vm.QuitSignal); !ok {
			out.WriteString("\nERROR: " + vm.ErrorReport(err))
		}
	}
	return out.String()
}

func TestGolden(t *testing.T) {
	progs, _ := filepath.Glob("testdata/*.prg")
	for _, p := range progs {
		name := filepath.Base(p)
		if name == "lib.prg" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			got := runProgram(t, name, "")
			golden := strings.TrimSuffix(p, ".prg") + ".out"
			if *update {
				os.WriteFile(golden, []byte(got), 0644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run go test -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}
		})
	}
}
