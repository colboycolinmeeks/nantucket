// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package term provides the console used by the runtime: a full screen
// tcell implementation and a plain stream implementation for scripts.
package term

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Attr is a Clipper style color attribute: low nibble foreground,
// high nibble background (0..15 each).
type Attr uint16

const (
	AttrUnderline Attr = 1 << 8
	AttrBlink     Attr = 1 << 9
	AttrReverse   Attr = 1 << 10
)

func MkAttr(fg, bg int) Attr { return Attr(fg&15) | Attr(bg&15)<<4 }
func (a Attr) Fg() int       { return int(a & 15) }
func (a Attr) Bg() int       { return int(a>>4) & 15 }

var DefaultAttr = MkAttr(7, 0)

type Cell struct {
	R rune
	A Attr
}

// Console is the abstract screen/keyboard.
type Console interface {
	// Out writes text at the cursor (scrolling at the bottom), like ?/??.
	Out(s string, a Attr)
	NewLine()
	SetPos(r, c int)
	Pos() (int, int)
	Size() (rows, cols int)
	// Put writes text at a position without scrolling (clipped).
	Put(r, c int, s string, a Attr)
	// Scroll scrolls a region by n lines (0 = clear).
	Scroll(top, left, bottom, right, n int, a Attr)
	// ReadKey waits for a key. timeout < 0 waits forever; 0 polls.
	ReadKey(timeout time.Duration) (int, bool)
	// PushKeys adds keys to the typeahead buffer.
	PushKeys(keys []int)
	// UnreadKey puts a key back at the front of the buffer, so it is the
	// next one read (used by NextKey()).
	UnreadKey(k int)
	ClearKeys()
	Flush()
	SetCursor(shape int)
	Save(top, left, bottom, right int) []Cell
	Restore(top, left, bottom, right int, cells []Cell)
	Interactive() bool
	Bell()
	Close()
	// Suspend gives the terminal back (for RUN); Resume reclaims it.
	Suspend()
	Resume()
}

// WaitNotifier is implemented by consoles that report when they are
// blocked waiting for input: the hook is called with true before waiting
// and false after. The runtime uses it to know when it is idle and can be
// shut down safely (see vm.Shutdown).
type WaitNotifier interface {
	SetWaitHook(func(waiting bool))
}

// ExtendedKeys is implemented by consoles that can report the shifted
// editing keys below. They are off by default, so programs reading
// Inkey() see Shift+Left as Left, as in Clipper; the program editor turns
// them on to select text.
type ExtendedKeys interface {
	SetExtendedKeys(on bool)
}

// Shifted editing keys, reported only when extended keys are on.
const (
	KShUp    = 1101
	KShDown  = 1102
	KShLeft  = 1103
	KShRight = 1104
	KShHome  = 1105
	KShEnd   = 1106
	KShPgUp  = 1107
	KShPgDn  = 1108
	KCtrlIns = 1109
	KShIns   = 1110
	KShDel   = 1111
	KCtrlZ   = 1112 // Ctrl-Z itself (in Clipper's key table it is Ctrl-Left)
)

// Key codes (Clipper inkey.ch values).
const (
	KUp        = 5
	KDown      = 24
	KLeft      = 19
	KRight     = 4
	KHome      = 1
	KEnd       = 6
	KPgUp      = 18
	KPgDn      = 3
	KCtrlLeft  = 26
	KCtrlRight = 2
	KCtrlHome  = 29
	KCtrlEnd   = 23
	KCtrlPgUp  = 31
	KCtrlPgDn  = 30
	KEnter     = 13
	KEsc       = 27
	KIns       = 22
	KDel       = 7
	KBS        = 8
	KTab       = 9
	KShTab     = 271
	KF1        = 28
	KF2        = -1
	KF3        = -2
	KF4        = -3
	KF5        = -4
	KF6        = -5
	KF7        = -6
	KF8        = -7
	KF9        = -8
	KF10       = -9
	KF11       = -40
	KF12       = -41
	KCtrlW     = 23
	KCtrlY     = 25
	KCtrlN     = 14
	KCtrlQ     = 17
	KCtrlT     = 20
	KCtrlU     = 21
	KCtrlF     = 6
	KCtrlL     = 12
	KCtrlK     = 11
	KCtrlS     = 19
	KCtrlC     = 3
	KAltC      = 302
	KAltD      = 288
	// KDebug is sent by Alt-D to wake a program waiting for a key, so
	// that it opens the debugger. Readers other than Inkey() ignore it.
	KDebug = -999
)

// cp437 maps bytes 128..255 to Unicode for display of legacy data.
var cp437 = []rune("ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒáíóúñÑªº¿⌐¬½¼¡«»░▒▓│┤╡╢╖╕╣║╗╝╜╛┐└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀αßΓπΣσµτΦΘΩδ∞φε∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ ")

// Runes decodes an xBase byte string for display: valid UTF-8 is used as
// is; other high bytes are shown using code page 437.
func Runes(s string) []rune {
	out := make([]rune, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			if c < 32 && c != 0 {
				out = append(out, ctrlGlyph(c))
			} else if c == 0 {
				out = append(out, ' ')
			} else {
				out = append(out, rune(c))
			}
			i++
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		if r != utf8.RuneError || n > 1 {
			out = append(out, r)
			i += n
			continue
		}
		out = append(out, cp437[c-128])
		i++
	}
	return out
}

func ctrlGlyph(c byte) rune {
	switch c {
	case '\t':
		return ' '
	}
	return []rune(" ☺☻♥♦♣♠•◘○◙♂♀♪♫☼►◄↕‼¶§▬↨↑↓→←∟↔▲▼")[c]
}

// DisplayLen returns the number of screen columns for s.
func DisplayLen(s string) int { return len(Runes(s)) }

// ---------- color strings ----------

var colorLetters = map[string]int{"N": 0, "B": 1, "G": 2, "BG": 3, "GB": 3, "R": 4, "RB": 5, "BR": 5, "GR": 6, "RG": 6, "W": 7, "X": 0, "U": 7, "I": 0}

// ParseColorPair parses "W+/B" style specs.
func ParseColorPair(spec string, def Attr) Attr {
	spec = strings.TrimSpace(strings.ToUpper(spec))
	if spec == "" {
		return def
	}
	fgs, bgs := spec, ""
	if i := strings.IndexByte(spec, '/'); i >= 0 {
		fgs, bgs = spec[:i], spec[i+1:]
	}
	fg, bg := def.Fg(), def.Bg()
	var extra Attr
	parse := func(s string) (int, bool, bool) {
		bright := strings.Contains(s, "+")
		blink := strings.Contains(s, "*")
		s = strings.NewReplacer("+", "", "*", "").Replace(s)
		s = strings.TrimSpace(s)
		if s == "" {
			return -1, bright, blink
		}
		if n, ok := colorLetters[s]; ok {
			return n, bright, blink
		}
		// numeric colors
		n := 0
		for _, ch := range s {
			if ch < '0' || ch > '9' {
				return -1, bright, blink
			}
			n = n*10 + int(ch-'0')
		}
		return n & 15, bright, blink
	}
	if f, br, bl := parse(fgs); f >= 0 || br {
		if f >= 0 {
			fg = f
		}
		if br {
			fg |= 8
		}
		if bl {
			extra |= AttrBlink
		}
		if strings.Contains(fgs, "U") {
			extra |= AttrUnderline
		}
		if strings.Contains(fgs, "I") {
			extra |= AttrReverse
		}
	}
	if b, br, _ := parse(bgs); b >= 0 || br {
		if b >= 0 {
			bg = b
		}
		if br {
			bg |= 8
		}
	}
	return MkAttr(fg, bg) | extra
}

// ColorSpec is a parsed SET COLOR string.
type ColorSpec struct {
	Std, Enh, Border, Bg, Unsel Attr
	Text                        string
}

func ParseColor(s string) ColorSpec {
	cs := ColorSpec{Std: MkAttr(7, 0), Enh: MkAttr(0, 7), Border: MkAttr(0, 0), Bg: MkAttr(7, 0), Unsel: MkAttr(0, 7)}
	if strings.TrimSpace(s) == "" {
		cs.Text = "W/N,N/W,N,N,N/W"
		return cs
	}
	return ParseColorOver(s, cs)
}

// ParseColorOver parses a SET COLOR string; slots it leaves out keep the
// values from base.
func ParseColorOver(s string, cs ColorSpec) ColorSpec {
	parts := strings.Split(s, ",")
	dst := []*Attr{&cs.Std, &cs.Enh, &cs.Border, &cs.Bg, &cs.Unsel}
	for i, p := range parts {
		if i >= len(dst) {
			break
		}
		if strings.TrimSpace(p) == "" {
			continue
		}
		*dst[i] = ParseColorPair(p, *dst[i])
		if i == 1 && len(parts) < 5 {
			cs.Unsel = cs.Enh
		}
	}
	cs.Text = s
	return cs
}
