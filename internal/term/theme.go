// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package term

import (
	"os"
	"sort"
	"strings"
)

// Theme is a named set of default colours. Color is the default SET COLOR
// string; programs override it with SET COLOR TO, and SET COLOR TO with no
// argument goes back to it. The other fields colour the built-in screens
// (MODIFY COMMAND, BROWSE, ALERT), which SET COLOR does not change.
type Theme struct {
	Name string
	// Mono renders everything in black and white: colours are mapped to
	// normal, bright and reverse video (see MonoAttr).
	Mono       bool
	Color      string
	Header     Attr // BROWSE / EDIT column headers
	Alert      Attr // ALERT() box
	AlertHi    Attr // selected ALERT() button
	Edit       Attr // editor text
	EditStatus Attr // editor status line
	EditHelp   Attr // editor help popup
	Status     Attr // dot prompt status bar (SET STATUS ON)
	Banner     Attr // dot prompt start-up banner
	// Windows: the frame and title colour (its background is the window's
	// own), the title bar controls, and the HELP window.
	WinFrame    Attr
	WinControls Attr
	Help        Attr
}

func pair(s string) Attr { return ParseColorPair(s, DefaultAttr) }

// Themes lists the built-in themes by name.
var Themes = map[string]Theme{
	"classic": {
		Name:        "classic",
		Color:       "W/N,N/W,N,N,N/W",
		Header:      pair("W+/B"),
		Alert:       pair("W+/R"),
		AlertHi:     pair("N/W"),
		Edit:        pair("W/B"),
		EditStatus:  pair("N/BG"),
		EditHelp:    pair("W+/BG"),
		Status:      pair("N/W"),
		Banner:      pair("W+/N"),
		WinFrame:    pair("W+/N"),
		WinControls: pair("GR+/N"),
		Help:        pair("N/W"),
	},
	// color is the default: white on blue, with GETs in white on green.
	"color": {
		Name:        "color",
		Color:       "W+/B,W+/G,B,B,W+/G",
		Header:      pair("GR+/B"),
		Alert:       pair("W+/R"),
		AlertHi:     pair("N/W"),
		Edit:        pair("W+/B"),
		EditStatus:  pair("N/BG"),
		EditHelp:    pair("W+/BG"),
		Status:      pair("N/BG"),
		Banner:      pair("GR+/B"),
		WinFrame:    pair("W+/B"),
		WinControls: pair("GR+/B"),
		Help:        pair("N/BG"),
	},
	"mono": {
		Name:        "mono",
		Mono:        true,
		Color:       "W/N,N/W,N,N,N/W",
		Header:      pair("N/W"),
		Alert:       pair("W/N"),
		AlertHi:     pair("N/W"),
		Edit:        pair("W/N"),
		EditStatus:  pair("N/W"),
		EditHelp:    pair("N/W"),
		Status:      pair("N/W"),
		Banner:      pair("W+/N"),
		WinFrame:    pair("W+/N"),
		WinControls: pair("W+/N"),
		Help:        pair("N/W"),
	},
}

// ThemeNames returns the built-in theme names, sorted.
func ThemeNames() []string {
	var names []string
	for n := range Themes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// LookupTheme finds a theme by name (case-insensitive). "colour", "bw" and
// "monochrome" are accepted as aliases.
func LookupTheme(name string) (Theme, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "colour":
		n = "color"
	case "bw", "monochrome", "mono":
		n = "mono"
	}
	t, ok := Themes[n]
	return t, ok
}

// AutoTheme picks a theme for a terminal that supports the given number of
// colours: mono when NO_COLOR is set or there are fewer than 8 colours.
func AutoTheme(colors int) Theme {
	if os.Getenv("NO_COLOR") != "" || colors < 8 {
		return Themes["mono"]
	}
	return Themes["color"]
}

// Colors parses a SET COLOR string on top of this theme's defaults, so
// slots left out of s keep the theme's colours.
func (t Theme) Colors(s string) ColorSpec {
	base := ParseColor(t.Color)
	if strings.TrimSpace(s) == "" {
		return base
	}
	return ParseColorOver(s, base)
}

// grey approximates how light each palette colour looks.
var grey = [16]int{0, 1, 3, 4, 2, 2, 4, 6, 4, 3, 7, 8, 4, 5, 8, 9}

// MonoAttr maps a colour attribute to black and white: hidden when fore and
// background match, reverse video when the background is the lighter of the
// two, otherwise white on black (bright if the foreground is bright).
func MonoAttr(a Attr) Attr {
	fg, bg := a.Fg(), a.Bg()
	extra := a &^ 0xff
	switch {
	case fg == bg:
		return MkAttr(0, 0) | extra
	case grey[bg] > grey[fg]:
		return MkAttr(0, 7) | extra
	case fg >= 8:
		return MkAttr(15, 0) | extra
	}
	return MkAttr(7, 0) | extra
}

// Themer is implemented by consoles that render themes (the full screen).
type Themer interface {
	ApplyTheme(t Theme, std Attr)
}
