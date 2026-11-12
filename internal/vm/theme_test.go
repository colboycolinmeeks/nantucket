// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"nantucket/internal/term"
)

func TestSetThemeAndColor(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`SET THEME TO mono
lColor := IsColor()
cTheme := SetTheme()
SET COLOR TO GR+/B
cSet := SetColor()
SET COLOR TO
cReset := SetColor()
SET THEME TO classic
cClassic := SetColor()`)); err != nil {
		t.Fatal(err)
	}
	get := func(n string) string {
		v, _ := e.vm.GetMemvar(n)
		return e.vm.Display(v)
	}
	if get("lColor") != ".F." || get("cTheme") != "mono" {
		t.Errorf("mono theme: IsColor()=%s SetTheme()=%s", get("lColor"), get("cTheme"))
	}
	if get("cSet") != "GR+/B" {
		t.Errorf("SET COLOR override = %q", get("cSet"))
	}
	if get("cReset") != term.Themes["mono"].Color {
		t.Errorf("SET COLOR TO should reset to the theme, got %q", get("cReset"))
	}
	if get("cClassic") != term.Themes["classic"].Color {
		t.Errorf("SET THEME should reset colours, got %q", get("cClassic"))
	}
}

func TestSetThemeUnknown(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`SET THEME TO neon`)); err == nil {
		t.Fatal("expected an error for an unknown theme")
	}
}

func TestMonoRendering(t *testing.T) {
	e := newSim(t)
	if err := e.wait(e.run(`SET THEME TO mono
@ 1, 0 SAY "hi" COLOR "GR+/B"
@ 2, 0 SAY "lo" COLOR "N/BG"`)); err != nil {
		t.Fatal(err)
	}
	e.sim.Show()
	_, _, st, _ := e.sim.GetContent(0, 1)
	fg, bg, at := st.Decompose()
	if fg != tcell.ColorWhite || bg != tcell.ColorBlack || at&tcell.AttrBold == 0 {
		t.Errorf("GR+/B in mono = %v on %v (attrs %v), want bold white on black", fg, bg, at)
	}
	_, _, st, _ = e.sim.GetContent(0, 2)
	if fg, bg, _ := st.Decompose(); fg != tcell.ColorBlack || bg != tcell.ColorSilver {
		t.Errorf("N/BG in mono = %v on %v, want reverse", fg, bg)
	}
}
