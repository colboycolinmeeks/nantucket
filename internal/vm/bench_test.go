// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nantucket/internal/term"
)

// BenchmarkLoop measures the interpreter on a loop that calls a function,
// does arithmetic and string work. Profile it with:
//
//	go test ./internal/vm -run x -bench Loop -cpuprofile cpu.prof
func BenchmarkLoop(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "loop.prg")
	os.WriteFile(path, []byte(`LOCAL i, n := 0, s := ""
FOR i := 1 TO 200000
   n := n + Add(i, 2)
   IF i % 1000 == 0
      s := s + Str(i)
   ENDIF
NEXT
RETURN

FUNCTION Add(a, b)
RETURN a + b
`), 0o644)
	m, err := New(term.NewStream(strings.NewReader(""), io.Discard))
	if err != nil {
		b.Fatal(err)
	}
	m.PushBaseFrame()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := m.RunFile(path, nil); err != nil {
			b.Fatal(ErrorReport(err))
		}
	}
}
