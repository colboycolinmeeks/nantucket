// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package value defines the dynamic value type used by the xBase runtime.
package value

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Kind uint8

const (
	NIL Kind = iota
	LOGICAL
	NUMBER
	STRING
	DATE
	ARRAY
	BLOCK
	HASH
	OBJECT
	REF
)

// Value is an xBase value. Numbers carry an optional display width and
// decimal count so output matches dBase/Clipper formatting.
type Value struct {
	K     Kind
	B     bool
	Dec   int8  // decimals for display (numbers)
	Width int16 // display width for numbers (0 = default)
	N     float64
	J     int32 // julian day for dates (0 = empty date)
	S     string
	P     any  // *Array, *Block, *Hash, Object, *Ref
	Memo  bool // a character value read from a memo field (ValType "M")
}

// MemoStr makes a character value that came from a memo field.
func MemoStr(s string) Value { return Value{K: STRING, S: s, Memo: true} }

type Array struct{ Items []Value }

// Ref is a shared variable cell. Locals, memvars and by-reference
// parameters all live in Refs.
type Ref struct{ V Value }

// Block is a compiled code block. Fn is owned by the VM.
type Block struct {
	Fn       any
	Captured []*Ref
	Src      string
}

// Object is implemented by runtime objects (Error, Get, ...).
type Object interface {
	ClassName() string
}

var Nil = Value{}
var True = Value{K: LOGICAL, B: true}
var False = Value{K: LOGICAL}

func Bool(b bool) Value  { return Value{K: LOGICAL, B: b} }
func Str(s string) Value { return Value{K: STRING, S: s} }
func Int(n int) Value    { return Value{K: NUMBER, N: float64(n)} }
func Num(n float64, dec int) Value {
	return Value{K: NUMBER, N: n, Dec: int8(dec)}
}
func NumW(n float64, width, dec int) Value {
	return Value{K: NUMBER, N: n, Dec: int8(dec), Width: int16(width)}
}
func Date(j int32) Value { return Value{K: DATE, J: j} }
func NewArray(items []Value) Value {
	return Value{K: ARRAY, P: &Array{Items: items}}
}
func ArrayOf(n int) Value { return NewArray(make([]Value, n)) }
func Obj(o Object) Value  { return Value{K: OBJECT, P: o} }
func MkRef(r *Ref) Value  { return Value{K: REF, P: r} }

func (v Value) Arr() *Array { a, _ := v.P.(*Array); return a }
func (v Value) Blk() *Block { b, _ := v.P.(*Block); return b }
func (v Value) Hsh() *Hash  { h, _ := v.P.(*Hash); return h }
func (v Value) Rf() *Ref    { r, _ := v.P.(*Ref); return r }
func (v Value) IsNil() bool { return v.K == NIL }
func (v Value) Deref() Value {
	for v.K == REF {
		v = v.P.(*Ref).V
	}
	return v
}

// TypeChar returns the ValType() letter.
func (v Value) TypeChar() string {
	switch v.K {
	case NIL:
		return "U"
	case LOGICAL:
		return "L"
	case NUMBER:
		return "N"
	case STRING:
		if v.Memo {
			return "M"
		}
		return "C"
	case DATE:
		return "D"
	case ARRAY:
		return "A"
	case BLOCK:
		return "B"
	case HASH:
		return "H"
	case OBJECT:
		return "O"
	case REF:
		return v.Deref().TypeChar()
	}
	return "U"
}

// Int returns the number truncated to int.
func (v Value) Int() int {
	if v.K != NUMBER {
		return 0
	}
	return int(v.N)
}

// Truthy is used where a logical is expected.
func (v Value) Truthy() bool { return v.K == LOGICAL && v.B }

// Round rounds half away from zero, as Clipper does.
func Round(n float64, dec int) float64 {
	p := math.Pow(10, float64(dec))
	x := n * p
	// compensate for binary representation error
	if x >= 0 {
		x = math.Floor(x + 0.5 + 1e-9)
	} else {
		x = math.Ceil(x - 0.5 - 1e-9)
	}
	return x / p
}

// FormatNum formats a number with a fixed width and decimals, like Str().
// Returns asterisks on overflow.
func FormatNum(n float64, width, dec int) string {
	if dec < 0 {
		dec = 0
	}
	n = Round(n, dec)
	s := strconv.FormatFloat(n, 'f', dec, 64)
	if s == "-0" || strings.HasPrefix(s, "-0.") && strings.Trim(s[3:], "0") == "" {
		s = s[1:]
	}
	if width <= 0 {
		return s
	}
	if len(s) > width {
		return strings.Repeat("*", width)
	}
	return strings.Repeat(" ", width-len(s)) + s
}

// DefaultWidth computes the Clipper default display width for a number.
func (v Value) DefaultWidth() int {
	if v.Width > 0 {
		return int(v.Width)
	}
	dec := int(v.Dec)
	ip := math.Abs(math.Trunc(v.N))
	digits := 1
	if ip >= 1 {
		digits = int(math.Log10(ip)) + 1
	}
	if v.N < 0 {
		digits++
	}
	w := 10
	if digits > w {
		w = digits
	}
	if dec > 0 {
		w += dec + 1
	}
	return w
}

func (v Value) String() string {
	switch v.K {
	case NIL:
		return "NIL"
	case LOGICAL:
		if v.B {
			return ".T."
		}
		return ".F."
	case NUMBER:
		return FormatNum(v.N, 0, int(v.Dec))
	case STRING:
		return v.S
	case DATE:
		return DTOS(v.J)
	case ARRAY:
		return "{...}"
	case BLOCK:
		return "{||...}"
	case HASH:
		return "{=>}"
	case OBJECT:
		return "Object " + v.P.(Object).ClassName()
	case REF:
		return v.Deref().String()
	}
	return fmt.Sprint(v.P)
}

// Empty implements Empty().
func (v Value) Empty() bool {
	switch v.K {
	case NIL:
		return true
	case LOGICAL:
		return !v.B
	case NUMBER:
		return v.N == 0
	case STRING:
		return strings.TrimLeft(v.S, " \t\r\n") == ""
	case DATE:
		return v.J == 0
	case ARRAY:
		return len(v.Arr().Items) == 0
	case HASH:
		return v.Hsh().Len() == 0
	case REF:
		return v.Deref().Empty()
	}
	return false
}

// ---------- dates ----------

// ToJulian converts a calendar date to a julian day number.
func ToJulian(y, m, d int) int32 {
	if y == 0 && m == 0 && d == 0 {
		return 0
	}
	a := (14 - m) / 12
	yy := y + 4800 - a
	mm := m + 12*a - 3
	return int32(d + (153*mm+2)/5 + 365*yy + yy/4 - yy/100 + yy/400 - 32045)
}

// FromJulian converts a julian day to y, m, d.
func FromJulian(j int32) (int, int, int) {
	if j <= 0 {
		return 0, 0, 0
	}
	a := int(j) + 32044
	b := (4*a + 3) / 146097
	c := a - 146097*b/4
	d := (4*c + 3) / 1461
	e := c - 1461*d/4
	m := (5*e + 2) / 153
	day := e - (153*m+2)/5 + 1
	month := m + 3 - 12*(m/10)
	year := 100*b + d - 4800 + m/10
	return year, month, day
}

// ValidDate reports whether y/m/d is a real calendar date.
func ValidDate(y, m, d int) bool {
	if m < 1 || m > 12 || d < 1 || y < 1 || y > 9999 {
		return false
	}
	return d <= DaysInMonth(y, m)
}

func DaysInMonth(y, m int) int {
	switch m {
	case 4, 6, 9, 11:
		return 30
	case 2:
		if (y%4 == 0 && y%100 != 0) || y%400 == 0 {
			return 29
		}
		return 28
	}
	return 31
}

// DTOS returns YYYYMMDD or 8 spaces.
func DTOS(j int32) string {
	if j <= 0 {
		return "        "
	}
	y, m, d := FromJulian(j)
	return fmt.Sprintf("%04d%02d%02d", y, m, d)
}

// STOD parses YYYYMMDD.
func STOD(s string) int32 {
	s = strings.TrimSpace(s)
	if len(s) != 8 {
		return 0
	}
	y, e1 := strconv.Atoi(s[0:4])
	m, e2 := strconv.Atoi(s[4:6])
	d, e3 := strconv.Atoi(s[6:8])
	if e1 != nil || e2 != nil || e3 != nil || !ValidDate(y, m, d) {
		return 0
	}
	return ToJulian(y, m, d)
}

// DOW returns 1=Sunday..7=Saturday.
func DOW(j int32) int {
	if j <= 0 {
		return 0
	}
	return int((j+1)%7) + 1
}

// ---------- Hash (xHarbour) ----------

type Hash struct {
	Keys  []Value
	Vals  []Value
	index map[string]int
}

func NewHash() *Hash { return &Hash{index: map[string]int{}} }

func hkey(k Value) string {
	k = k.Deref()
	switch k.K {
	case STRING:
		return "C" + k.S
	case NUMBER:
		return "N" + strconv.FormatFloat(k.N, 'g', -1, 64)
	case DATE:
		return "D" + strconv.Itoa(int(k.J))
	case LOGICAL:
		if k.B {
			return "LT"
		}
		return "LF"
	}
	return fmt.Sprintf("P%p", k.P)
}

func (h *Hash) Len() int { return len(h.Keys) }
func (h *Hash) Get(k Value) (Value, bool) {
	if i, ok := h.index[hkey(k)]; ok {
		return h.Vals[i], true
	}
	return Nil, false
}
func (h *Hash) Set(k, v Value) {
	hk := hkey(k)
	if i, ok := h.index[hk]; ok {
		h.Vals[i] = v
		return
	}
	h.index[hk] = len(h.Keys)
	h.Keys = append(h.Keys, k.Deref())
	h.Vals = append(h.Vals, v)
}
func (h *Hash) Del(k Value) {
	hk := hkey(k)
	i, ok := h.index[hk]
	if !ok {
		return
	}
	h.Keys = append(h.Keys[:i], h.Keys[i+1:]...)
	h.Vals = append(h.Vals[:i], h.Vals[i+1:]...)
	h.index = map[string]int{}
	for j, kk := range h.Keys {
		h.index[hkey(kk)] = j
	}
}
func HashVal(h *Hash) Value { return Value{K: HASH, P: h} }
