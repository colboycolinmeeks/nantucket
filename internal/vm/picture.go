// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"math"
	"strconv"
	"strings"

	"nantucket/internal/value"
)

// ---------- PICTURE / Transform ----------

// Picture is a parsed PICTURE clause.
type Picture struct {
	Fn     string // function letters (upper)
	Tmpl   string
	Scroll int // @S<n>
}

// ParsePicture parses a PICTURE clause.
func ParsePicture(p string) Picture {
	var pc Picture
	if strings.HasPrefix(p, "@") {
		rest := p[1:]
		sp := strings.IndexByte(rest, ' ')
		f := rest
		if sp >= 0 {
			f, pc.Tmpl = rest[:sp], rest[sp+1:]
		}
		uf := strings.ToUpper(f)
		if i := strings.IndexByte(uf, 'S'); i >= 0 {
			j := i + 1
			for j < len(uf) && uf[j] >= '0' && uf[j] <= '9' {
				j++
			}
			pc.Scroll, _ = strconv.Atoi(uf[i+1 : j])
			uf = uf[:i] + uf[j:]
		}
		pc.Fn = uf
	} else {
		pc.Tmpl = p
	}
	return pc
}

func (p Picture) Has(c byte) bool { return strings.IndexByte(p.Fn, c) >= 0 }

func IsTmplChar(c byte, typ byte) bool {
	switch c {
	case '9', '#':
		return true
	case 'A', 'N', 'X', 'L', 'Y', '!', 'a', 'n', 'x', 'l', 'y':
		return typ == 'C' || typ == 'L' || c == 'L' || c == 'Y'
	case '.', ',', '$', '*':
		return false
	}
	return false
}

// Transform formats a value with a PICTURE.
func (vm *VM) Transform(v value.Value, pic string) string {
	v = v.Deref()
	pc := ParsePicture(pic)
	switch v.K {
	case value.STRING:
		s := v.S
		if pc.Has('!') {
			s = strings.ToUpper(s)
		}
		if pc.Tmpl == "" {
			return s
		}
		return ApplyCharTemplate(s, pc.Tmpl, pc.Has('R'))
	case value.NUMBER:
		if pc.Has('Z') && v.N == 0 {
			w := len(pc.Tmpl)
			if w == 0 {
				w = v.DefaultWidth()
			}
			return strings.Repeat(" ", w)
		}
		var s string
		if pc.Tmpl == "" {
			s = vm.Display(v)
		} else {
			s = formatNumTemplate(math.Abs(v.N), pc.Tmpl, pc.Has('E'))
			if v.N < 0 && !pc.Has('(') && !pc.Has('X') {
				s = placeMinus(s)
			}
		}
		if v.N < 0 && pc.Has('(') {
			t := strings.TrimLeft(strings.Replace(s, "-", " ", 1), " ")
			s = padTo("("+t+")", len(s)+1, 'L', " ")
		}
		if pc.Has('B') {
			t := strings.TrimLeft(s, " ")
			s = t + strings.Repeat(" ", len(s)-len(t))
		}
		if pc.Has('C') && v.N > 0 {
			s += " CR"
		}
		if pc.Has('X') && v.N < 0 {
			s += " DB"
		}
		return s
	case value.DATE:
		if pc.Has('E') {
			if v.J == 0 {
				return "  /  /  "
			}
			y, m, d := value.FromJulian(v.J)
			return formatDate("dd/mm/yy", y, m, d)
		}
		return vm.DTOC(v.J)
	case value.LOGICAL:
		t := strings.ToUpper(pc.Tmpl)
		if strings.Contains(t, "Y") || pc.Has('Y') {
			if v.B {
				return "Y"
			}
			return "N"
		}
		if v.B {
			return "T"
		}
		return "F"
	}
	return vm.Display(v)
}

func placeMinus(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] != ' ' {
			if i > 0 {
				b[i-1] = '-'
				return string(b)
			}
			return strings.Repeat("*", len(s))
		}
	}
	return s
}

func ApplyCharTemplate(s, tmpl string, r bool) string {
	var out strings.Builder
	si := 0
	for i := 0; i < len(tmpl); i++ {
		c := tmpl[i]
		if IsTmplChar(c, 'C') {
			ch := byte(' ')
			if si < len(s) {
				ch = s[si]
				si++
			}
			if c == '!' {
				ch = byte(strings.ToUpper(string(ch))[0])
			}
			out.WriteByte(ch)
		} else {
			out.WriteByte(c)
			if !r {
				si++
			}
		}
	}
	return out.String()
}

func formatNumTemplate(n float64, tmpl string, euro bool) string {
	dot := strings.IndexByte(tmpl, '.')
	decs := 0
	if dot >= 0 {
		for i := dot + 1; i < len(tmpl); i++ {
			if tmpl[i] == '9' || tmpl[i] == '#' {
				decs++
			}
		}
	}
	digits := value.FormatNum(n, 0, decs)
	intPart, frac := digits, ""
	if i := strings.IndexByte(digits, '.'); i >= 0 {
		intPart, frac = digits[:i], digits[i+1:]
	}
	if intPart == "0" {
		intPart = ""
	}
	out := []byte(tmpl)
	// fill integer positions right to left
	end := len(tmpl)
	if dot >= 0 {
		end = dot
	}
	k := len(intPart) - 1
	lastDigitPos := end
	for i := end - 1; i >= 0; i-- {
		c := tmpl[i]
		switch c {
		case '9', '#':
			if k >= 0 {
				out[i] = intPart[k]
				k--
				lastDigitPos = i
			} else {
				out[i] = ' '
			}
		case ',':
			if k >= 0 {
				out[i] = ','
			} else {
				out[i] = ' '
			}
		case '$', '*':
			if k >= 0 {
				out[i] = intPart[k]
				k--
				lastDigitPos = i
			} else {
				out[i] = c
			}
		}
	}
	if k >= 0 {
		return strings.Repeat("*", len(tmpl))
	}
	if intPart == "" && dot >= 0 && dot > 0 {
		// show a leading zero before the decimal point
		if tmpl[dot-1] == '9' || tmpl[dot-1] == '#' {
			out[dot-1] = '0'
		}
	} else if intPart == "" && dot < 0 && len(tmpl) > 0 {
		out[len(tmpl)-1] = '0'
	}
	_ = lastDigitPos
	if dot >= 0 {
		fi := 0
		for i := dot + 1; i < len(tmpl); i++ {
			if tmpl[i] == '9' || tmpl[i] == '#' {
				if fi < len(frac) {
					out[i] = frac[fi]
				} else {
					out[i] = '0'
				}
				fi++
			}
		}
	}
	s := string(out)
	if euro {
		s = strings.Map(func(r rune) rune {
			switch r {
			case '.':
				return ','
			case ',':
				return '.'
			}
			return r
		}, s)
	}
	return s
}

func init() {
	reg("TRANSFORM", func(vm *VM, a []value.Value) value.Value {
		return value.Str(vm.Transform(arg(a, 0), argStr(a, 1, "")))
	})
}
