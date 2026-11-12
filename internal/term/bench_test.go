// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package term

// Benchmarks for screen output, with and without windows open (Perf #5):
//
//	go test ./internal/term -run x -bench Output

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func benchOutput(b *testing.B, windows bool) {
	sim := tcell.NewSimulationScreen("UTF-8")
	sc, _ := NewScreenWith(sim)
	sim.SetSize(80, 25)
	if windows {
		id, _ := sc.WOpen(2, 2, 10, 40, WinOpts{Title: "a", Sizable: true})
		sc.WSetState(id, WinMinimized)
		sc.WOpen(5, 30, 15, 70, WinOpts{Title: "b", Sizable: true})
		sc.WSelect(0, false)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for l := 0; l < 100; l++ {
			sc.Out("line of output text that is fairly long", DefaultAttr)
			sc.NewLine()
		}
	}
}

func BenchmarkOutputNoWindows(b *testing.B)   { benchOutput(b, false) }
func BenchmarkOutputWithWindows(b *testing.B) { benchOutput(b, true) }
