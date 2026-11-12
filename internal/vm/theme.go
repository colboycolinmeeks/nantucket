// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"strings"

	"nantucket/internal/term"
	"nantucket/internal/value"
)

// SetStartTheme sets the theme chosen at startup. SET THEME TO with no name
// returns to it.
func (vm *VM) SetStartTheme(t term.Theme) {
	vm.homeTheme = t
	vm.SetTheme(t)
	if vm.Con.Interactive() {
		vm.Con.Scroll(0, 0, 999, 999, 0, vm.colors.Std)
	}
}

// SetTheme switches theme and resets the colours to the theme's defaults.
func (vm *VM) SetTheme(t term.Theme) {
	vm.theme = t
	vm.setColors("")
}

// Theme returns the current theme.
func (vm *VM) Theme() term.Theme { return vm.theme }

// StdAttr is the current standard colour (SET COLOR slot 1).
func (vm *VM) StdAttr() term.Attr { return vm.colors.Std }

// setColors applies a SET COLOR string on top of the theme's defaults.
func (vm *VM) setColors(s string) {
	// SET COLOR TO "W/N" passes the quotes along; drop them
	if t := strings.TrimSpace(s); len(t) >= 2 && (t[0] == '"' || t[0] == '\'') && t[len(t)-1] == t[0] {
		s = t[1 : len(t)-1]
	}
	vm.colors = vm.theme.Colors(s)
	vm.sets[15] = value.Str(vm.colors.Text)
	if th, ok := vm.Con.(term.Themer); ok {
		th.ApplyTheme(vm.theme, vm.colors.Std)
	}
}

func init() {
	// SetTheme([cName]) -> cOldName. "" returns to the startup theme.
	// Switching theme resets the colours and clears the screen.
	reg("SETTHEME", func(vm *VM, a []value.Value) value.Value {
		old := vm.theme.Name
		if len(a) == 0 || arg(a, 0).K != value.STRING {
			return value.Str(old)
		}
		name := strings.TrimSpace(arg(a, 0).S)
		t := vm.homeTheme
		if name != "" {
			var ok bool
			if t, ok = term.LookupTheme(name); !ok {
				return vm.Raise(errArgCode(2020, "SETTHEME", arg(a, 0)))
			}
		}
		vm.SetTheme(t)
		if vm.Con.Interactive() {
			vm.Con.Scroll(0, 0, 999, 999, 0, vm.colors.Std)
			vm.Con.SetPos(0, 0)
		}
		return value.Str(old)
	})
}
