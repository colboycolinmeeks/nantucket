// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package term

import "testing"

func TestMonoAttr(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"W/N", "W/N"},    // normal
		{"W+/B", "W+/N"},  // bright text keeps its intensity
		{"GR+/B", "W+/N"}, // any bright colour becomes bright white
		{"N/W", "N/W"},    // reverse stays reverse
		{"N/BG", "N/W"},   // dark on a lighter background becomes reverse
		{"B/B", "N/N"},    // same fore and background stays hidden
		{"G/N", "W/N"},
	}
	for _, c := range cases {
		got := MonoAttr(ParseColorPair(c.in, DefaultAttr))
		if want := ParseColorPair(c.want, DefaultAttr); got != want {
			t.Errorf("MonoAttr(%s) = %d/%d, want %s", c.in, got.Fg(), got.Bg(), c.want)
		}
	}
}

func TestThemeColorsOverlay(t *testing.T) {
	th := Themes["color"]
	def := th.Colors("")
	if def.Text != th.Color || def.Std != pair("W+/B") || def.Enh != pair("W+/G") {
		t.Fatalf("default colours = %+v", def)
	}
	// Only the standard slot is given; the rest keep the theme's colours.
	cs := th.Colors("GR+/N")
	if cs.Std != pair("GR+/N") {
		t.Errorf("Std = %v", cs.Std)
	}
	if cs.Enh != def.Enh || cs.Unsel != def.Unsel {
		t.Errorf("unset slots changed: %+v", cs)
	}
}

func TestLookupTheme(t *testing.T) {
	for _, n := range []string{"COLOR", "colour", "Mono", "bw", "classic"} {
		if _, ok := LookupTheme(n); !ok {
			t.Errorf("LookupTheme(%q) failed", n)
		}
	}
	if _, ok := LookupTheme("neon"); ok {
		t.Error("LookupTheme(neon) should fail")
	}
}

func TestAutoTheme(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if AutoTheme(256).Mono || !AutoTheme(2).Mono {
		t.Error("AutoTheme should be mono only for terminals with few colours")
	}
	t.Setenv("NO_COLOR", "1")
	if !AutoTheme(256).Mono {
		t.Error("AutoTheme should honour NO_COLOR")
	}
}
