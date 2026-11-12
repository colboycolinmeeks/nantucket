// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package screens

import (
	"strings"

	"nantucket/internal/term"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

func init() {
	vm.Register("__HELP", func(m *vm.VM, a []value.Value) value.Value {
		showHelp(m)
		return value.Nil
	})
	vm.Register("__VERSION", func(m *vm.VM, a []value.Value) value.Value {
		showVersion(m, vm.ArgBool(a, 0, false))
		return value.Nil
	})
	// window events (closed or resized with the mouse, wheel) come from
	// the console
	vm.OnNew(func(m *vm.VM) {
		if w, ok := m.Con.(term.Windower); ok {
			w.SetOnEvent(func(e term.WinEvent) { windowEvent(m, e) })
		}
	})
}

// showVersion runs VERSION: the version line, printed. VERSION FULL also
// opens a window with what's new in this version.
func showVersion(m *vm.VM, full bool) {
	if !full {
		m.Println(vm.VersionLine)
		return
	}
	lines := []string{vm.VersionLine, vm.CopyrightLine}
	if notes := vm.WhatsNew(vm.Version); notes != "" {
		lines = append(lines, "", "What's new in "+vm.Version+":")
		for _, l := range strings.Split(notes, "\n") {
			lines = append(lines, "  "+l)
		}
	}
	showText(m, "version", "Version "+vm.Version, lines, false)
}
