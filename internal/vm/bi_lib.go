// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"nantucket/internal/value"
)

// ---------- strings ----------

func strOrErr(vm *VM, a []value.Value, i int, op string) (string, bool) {
	v := arg(a, i)
	if v.K != value.STRING {
		vm.Raise(errArg(op, v))
		return "", false
	}
	return v.S, true
}

func padTo(s string, n int, side byte, fill string) string {
	if n <= 0 {
		return ""
	}
	if fill == "" {
		fill = " "
	}
	f := fill[0]
	if len(s) >= n {
		if side == 'L' {
			return s[len(s)-n:]
		}
		return s[:n]
	}
	pad := n - len(s)
	switch side {
	case 'L':
		return strings.Repeat(string(f), pad) + s
	case 'C':
		l := pad / 2
		return strings.Repeat(string(f), l) + s + strings.Repeat(string(f), pad-l)
	}
	return s + strings.Repeat(string(f), pad)
}

// valToStr converts for PadX/AllTrim-like functions that accept any type.
func (vm *VM) valToStr(v value.Value) string {
	switch v.K {
	case value.STRING:
		return v.S
	case value.NUMBER:
		return strings.TrimSpace(vm.Display(v))
	case value.DATE:
		return vm.DTOC(v.J)
	case value.LOGICAL:
		if v.B {
			return "T"
		}
		return "F"
	case value.NIL:
		return ""
	}
	return v.String()
}

// Str implements Str(n, len, dec).
func (vm *VM) Str(v value.Value, a []value.Value) string {
	if has(a, 1) {
		w := argInt(a, 1, 10)
		d := 0
		if has(a, 2) {
			d = argInt(a, 2, 0)
		}
		if d > 0 && w <= d+1 {
			d = 0
		}
		return value.FormatNum(v.N, w, d)
	}
	return value.FormatNum(v.N, v.DefaultWidth(), int(v.Dec))
}

func init() {
	reg("LEN", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		switch v.K {
		case value.STRING:
			return value.Int(len(v.S))
		case value.ARRAY:
			return value.Int(len(v.Arr().Items))
		case value.HASH:
			return value.Int(v.Hsh().Len())
		case value.NIL:
			return value.Int(0)
		}
		return vm.Raise(errArgCode(1111, "LEN", v))
	})
	reg("UPPER", func(vm *VM, a []value.Value) value.Value {
		s, ok := strOrErr(vm, a, 0, "UPPER")
		if !ok {
			return value.Str("")
		}
		return value.Str(strings.ToUpper(s))
	})
	reg("LOWER", func(vm *VM, a []value.Value) value.Value {
		s, ok := strOrErr(vm, a, 0, "LOWER")
		if !ok {
			return value.Str("")
		}
		return value.Str(strings.ToLower(s))
	})
	trimR := func(vm *VM, a []value.Value) value.Value {
		s, ok := strOrErr(vm, a, 0, "TRIM")
		if !ok {
			return value.Str("")
		}
		return value.Str(strings.TrimRight(s, " "))
	}
	reg("TRIM", trimR)
	reg("RTRIM", trimR)
	reg("LTRIM", func(vm *VM, a []value.Value) value.Value {
		s, ok := strOrErr(vm, a, 0, "LTRIM")
		if !ok {
			return value.Str("")
		}
		return value.Str(strings.TrimLeft(s, " \t\r\n"))
	})
	reg("ALLTRIM", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		if v.K != value.STRING {
			return value.Str(strings.TrimSpace(vm.valToStr(v)))
		}
		return value.Str(strings.Trim(v.S, " \t\r\n"))
	})
	reg("SUBSTR", func(vm *VM, a []value.Value) value.Value {
		s, ok := strOrErr(vm, a, 0, "SUBSTR")
		if !ok {
			return value.Str("")
		}
		start := argInt(a, 1, 1)
		if start < 0 {
			start = len(s) + start + 1
			if start < 1 {
				start = 1
			}
		}
		if start == 0 {
			start = 1
		}
		if start > len(s) {
			return value.Str("")
		}
		end := len(s)
		if has(a, 2) {
			n := argInt(a, 2, 0)
			if n <= 0 {
				return value.Str("")
			}
			if start-1+n < end {
				end = start - 1 + n
			}
		}
		return value.Str(s[start-1 : end])
	})
	reg("LEFT", func(vm *VM, a []value.Value) value.Value {
		s, ok := strOrErr(vm, a, 0, "LEFT")
		if !ok {
			return value.Str("")
		}
		n := argInt(a, 1, 0)
		if n < 0 {
			n = 0
		}
		if n > len(s) {
			n = len(s)
		}
		return value.Str(s[:n])
	})
	reg("RIGHT", func(vm *VM, a []value.Value) value.Value {
		s, ok := strOrErr(vm, a, 0, "RIGHT")
		if !ok {
			return value.Str("")
		}
		n := argInt(a, 1, 0)
		if n < 0 {
			n = 0
		}
		if n > len(s) {
			n = len(s)
		}
		return value.Str(s[len(s)-n:])
	})
	reg("AT", func(vm *VM, a []value.Value) value.Value {
		sub, s := argStr(a, 0, ""), argStr(a, 1, "")
		from := argInt(a, 2, 1)
		if sub == "" || from < 1 || from > len(s) {
			return value.Int(0)
		}
		i := strings.Index(s[from-1:], sub)
		if i < 0 {
			return value.Int(0)
		}
		return value.Int(i + from)
	})
	reg("HB_AT", builtinTable["AT"].fn)
	reg("RAT", func(vm *VM, a []value.Value) value.Value {
		sub, s := argStr(a, 0, ""), argStr(a, 1, "")
		if sub == "" {
			return value.Int(0)
		}
		return value.Int(strings.LastIndex(s, sub) + 1)
	})
	reg("SPACE", func(vm *VM, a []value.Value) value.Value {
		n := argInt(a, 0, 0)
		if n < 0 {
			n = 0
		}
		return value.Str(strings.Repeat(" ", n))
	})
	reg("REPLICATE", func(vm *VM, a []value.Value) value.Value {
		n := argInt(a, 1, 0)
		if n < 0 {
			n = 0
		}
		return value.Str(strings.Repeat(argStr(a, 0, ""), n))
	})
	reg("STR", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		if v.K != value.NUMBER {
			return vm.Raise(errArgCode(1099, "STR", v))
		}
		return value.Str(vm.Str(v, a))
	})
	reg("STRZERO", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		if v.K != value.NUMBER {
			return vm.Raise(errArgCode(1099, "STRZERO", v))
		}
		s := vm.Str(v, a)
		neg := strings.Contains(s, "-")
		s = strings.ReplaceAll(s, " ", "0")
		if neg {
			s = "-" + strings.Replace(s, "-", "0", 1)[1:]
		}
		return value.Str(s)
	})
	reg("HB_NTOS", func(vm *VM, a []value.Value) value.Value {
		return value.Str(strings.TrimSpace(vm.Str(arg(a, 0), nil)))
	})
	reg("VAL", func(vm *VM, a []value.Value) value.Value {
		s := strings.TrimSpace(argStr(a, 0, ""))
		end := 0
		dot := -1
		for end < len(s) {
			c := s[end]
			if c >= '0' && c <= '9' || (end == 0 && (c == '-' || c == '+')) {
				end++
				continue
			}
			if c == '.' && dot < 0 {
				dot = end
				end++
				continue
			}
			break
		}
		n, _ := strconv.ParseFloat(s[:end], 64)
		dec := 0
		if dot >= 0 {
			dec = end - dot - 1
		}
		return value.Num(n, dec)
	})
	reg("STRTRAN", func(vm *VM, a []value.Value) value.Value {
		s, find, repl := argStr(a, 0, ""), argStr(a, 1, ""), argStr(a, 2, "")
		if find == "" {
			return value.Str(s)
		}
		start := argInt(a, 3, 1)
		count := argInt(a, 4, -1)
		var sb strings.Builder
		occ := 0
		done := 0
		for {
			i := strings.Index(s, find)
			if i < 0 {
				break
			}
			occ++
			if occ >= start && (count < 0 || done < count) {
				sb.WriteString(s[:i])
				sb.WriteString(repl)
				done++
			} else {
				sb.WriteString(s[:i+len(find)])
			}
			s = s[i+len(find):]
		}
		sb.WriteString(s)
		return value.Str(sb.String())
	})
	reg("STUFF", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		start := argInt(a, 1, 1)
		del := argInt(a, 2, 0)
		ins := argStr(a, 3, "")
		if start < 1 {
			start = 1
		}
		if start > len(s)+1 {
			start = len(s) + 1
		}
		end := start - 1 + del
		if end > len(s) {
			end = len(s)
		}
		if end < start-1 {
			end = start - 1
		}
		return value.Str(s[:start-1] + ins + s[end:])
	})
	for _, p := range []struct {
		n string
		s byte
	}{{"PADR", 'R'}, {"PAD", 'R'}, {"PADL", 'L'}, {"PADC", 'C'}} {
		side := p.s
		reg(p.n, func(vm *VM, a []value.Value) value.Value {
			return value.Str(padTo(vm.valToStr(arg(a, 0)), argInt(a, 1, 0), side, argStr(a, 2, " ")))
		})
	}
	reg("CHR", func(vm *VM, a []value.Value) value.Value {
		n := argInt(a, 0, 0)
		return value.Str(string([]byte{byte(n)}))
	})
	reg("ASC", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		if s == "" {
			return value.Int(0)
		}
		return value.Int(int(s[0]))
	})
	reg("ISALPHA", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		return value.Bool(s != "" && (s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z'))
	})
	reg("ISDIGIT", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		return value.Bool(s != "" && s[0] >= '0' && s[0] <= '9')
	})
	reg("ISUPPER", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		return value.Bool(s != "" && s[0] >= 'A' && s[0] <= 'Z')
	})
	reg("ISLOWER", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		return value.Bool(s != "" && s[0] >= 'a' && s[0] <= 'z')
	})
	reg("SOUNDEX", func(vm *VM, a []value.Value) value.Value { return value.Str(soundex(argStr(a, 0, ""))) })
	reg("DIFFERENCE", func(vm *VM, a []value.Value) value.Value {
		x, y := soundex(argStr(a, 0, "")), soundex(argStr(a, 1, ""))
		n := 0
		for i := 0; i < 4; i++ {
			if x[i] == y[i] {
				n++
			}
		}
		return value.Int(n)
	})
	reg("HARDCR", func(vm *VM, a []value.Value) value.Value {
		return value.Str(strings.ReplaceAll(argStr(a, 0, ""), "\x8D\n", "\r\n"))
	})
	reg("MEMOTRAN", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		hard, soft := argStr(a, 1, ";"), argStr(a, 2, " ")
		s = strings.ReplaceAll(s, "\r\n", hard)
		s = strings.ReplaceAll(s, "\x8D\n", soft)
		s = strings.ReplaceAll(s, "\n", hard)
		return value.Str(s)
	})
	reg("MLCOUNT", func(vm *VM, a []value.Value) value.Value {
		return value.Int(len(memoLines(argStr(a, 0, ""), argInt(a, 1, 79))))
	})
	reg("MEMOLINE", func(vm *VM, a []value.Value) value.Value {
		w := argInt(a, 1, 79)
		ls := memoLines(argStr(a, 0, ""), w)
		n := argInt(a, 2, 1)
		if n < 1 || n > len(ls) {
			return value.Str(strings.Repeat(" ", w))
		}
		return value.Str(padTo(ls[n-1], w, 'R', " "))
	})
	reg("MLPOS", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		ls := memoLines(s, argInt(a, 1, 79))
		n := argInt(a, 2, 1)
		pos := 1
		for i := 0; i < n-1 && i < len(ls); i++ {
			pos += len(ls[i])
		}
		return value.Int(pos)
	})
	reg("DESCEND", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		switch v.K {
		case value.STRING:
			b := []byte(v.S)
			for i := range b {
				b[i] = 255 - b[i]
			}
			return value.Str(string(b))
		case value.NUMBER:
			return value.Num(-v.N, int(v.Dec))
		case value.DATE:
			return value.Int(5231808 - int(v.J))
		case value.LOGICAL:
			return value.Bool(!v.B)
		}
		return v
	})
	reg("I2BIN", func(vm *VM, a []value.Value) value.Value {
		n := argInt(a, 0, 0)
		return value.Str(string([]byte{byte(n), byte(n >> 8)}))
	})
	reg("BIN2I", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "") + "\x00\x00"
		return value.Int(int(int16(uint16(s[0]) | uint16(s[1])<<8)))
	})
	reg("BIN2W", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "") + "\x00\x00"
		return value.Int(int(uint16(s[0]) | uint16(s[1])<<8))
	})
	reg("L2BIN", func(vm *VM, a []value.Value) value.Value {
		n := argInt(a, 0, 0)
		return value.Str(string([]byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}))
	})
	reg("BIN2L", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "") + "\x00\x00\x00\x00"
		return value.Int(int(int32(uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24)))
	})
	reg("HB_VALTOSTR", func(vm *VM, a []value.Value) value.Value {
		return value.Str(strings.TrimSpace(vm.Display(arg(a, 0))))
	})
	reg("HB_ATOKENS", func(vm *VM, a []value.Value) value.Value {
		s := argStr(a, 0, "")
		d := argStr(a, 1, " ")
		var parts []string
		if d == " " {
			parts = strings.Fields(s)
		} else {
			parts = strings.Split(s, d)
		}
		items := make([]value.Value, len(parts))
		for i, p := range parts {
			items[i] = value.Str(p)
		}
		return value.NewArray(items)
	})
	reg("HB_TOKENCOUNT", func(vm *VM, a []value.Value) value.Value {
		s, d := argStr(a, 0, ""), argStr(a, 1, " ")
		if d == " " {
			return value.Int(len(strings.Fields(s)))
		}
		return value.Int(len(strings.Split(s, d)))
	})
	reg("HB_TOKENGET", func(vm *VM, a []value.Value) value.Value {
		s, n, d := argStr(a, 0, ""), argInt(a, 1, 1), argStr(a, 2, " ")
		var parts []string
		if d == " " {
			parts = strings.Fields(s)
		} else {
			parts = strings.Split(s, d)
		}
		if n >= 1 && n <= len(parts) {
			return value.Str(parts[n-1])
		}
		return value.Str("")
	})
	reg("HB_STRREPLACE", builtinTable["STRTRAN"].fn)
	reg("HB_LEFTEQ", func(vm *VM, a []value.Value) value.Value {
		return value.Bool(strings.HasPrefix(argStr(a, 0, ""), argStr(a, 1, "")))
	})
	reg("HB_DEFAULT", func(vm *VM, a []value.Value) value.Value { return value.Nil })

	// ---------- math ----------
	num := func(name string, f func(float64) float64, dec int) {
		reg(name, func(vm *VM, a []value.Value) value.Value {
			v := arg(a, 0)
			if v.K != value.NUMBER {
				return vm.Raise(errArg(name, v))
			}
			d := dec
			if d < 0 {
				d = vm.setDecimals()
			}
			return value.Num(f(v.N), d)
		})
	}
	num("SQRT", math.Sqrt, -1)
	num("EXP", math.Exp, -1)
	num("LOG", math.Log, -1)
	num("SIN", math.Sin, -1)
	num("COS", math.Cos, -1)
	num("TAN", math.Tan, -1)
	num("ASIN", math.Asin, -1)
	num("ACOS", math.Acos, -1)
	num("ATAN", math.Atan, -1)
	num("LOG10", math.Log10, -1)
	num("CEILING", math.Ceil, 0)
	num("FLOOR", math.Floor, 0)
	reg("ABS", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		if v.K != value.NUMBER {
			return vm.Raise(errArg("ABS", v))
		}
		v.N = math.Abs(v.N)
		v.Width = 0
		return v
	})
	reg("INT", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		if v.K != value.NUMBER {
			return vm.Raise(errArg("INT", v))
		}
		return value.Num(math.Trunc(v.N), 0)
	})
	reg("ROUND", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		if v.K != value.NUMBER {
			return vm.Raise(errArg("ROUND", v))
		}
		d := argInt(a, 1, 0)
		r := value.Round(v.N, d)
		if d < 0 {
			d = 0
		}
		return value.Num(r, d)
	})
	reg("MOD", func(vm *VM, a []value.Value) value.Value {
		x, y := argNum(a, 0, 0), argNum(a, 1, 0)
		if y == 0 {
			return value.Num(x, 0)
		}
		r := math.Mod(x, y)
		if r != 0 && (r < 0) != (y < 0) {
			r += y
		}
		return value.Num(r, maxDec(arg(a, 0), arg(a, 1)))
	})
	reg("MAX", func(vm *VM, a []value.Value) value.Value {
		x, y := arg(a, 0), arg(a, 1)
		if vm.compare(opGT, x, y).B || vm.compare(opEQ, x, y).B {
			return x
		}
		return y
	})
	reg("MIN", func(vm *VM, a []value.Value) value.Value {
		x, y := arg(a, 0), arg(a, 1)
		if vm.compare(opLT, x, y).B || vm.compare(opEQ, x, y).B {
			return x
		}
		return y
	})
	reg("HB_RANDOM", func(vm *VM, a []value.Value) value.Value {
		if has(a, 1) {
			lo, hi := argNum(a, 0, 0), argNum(a, 1, 1)
			return value.Num(lo+rand.Float64()*(hi-lo), vm.setDecimals())
		}
		if has(a, 0) {
			return value.Num(rand.Float64()*argNum(a, 0, 1), vm.setDecimals())
		}
		return value.Num(rand.Float64(), vm.setDecimals())
	})
	reg("HB_RANDOMINT", func(vm *VM, a []value.Value) value.Value {
		lo, hi := 1, argInt(a, 0, 1)
		if has(a, 1) {
			lo, hi = argInt(a, 0, 1), argInt(a, 1, 1)
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		return value.Int(lo + rand.Intn(hi-lo+1))
	})
	reg("RAND", builtinTable["HB_RANDOM"].fn)
	reg("PI", func(vm *VM, a []value.Value) value.Value { return value.Num(math.Pi, vm.setDecimals()) })

	// ---------- dates / time ----------
	reg("DATE", func(vm *VM, a []value.Value) value.Value {
		t := time.Now()
		return value.Date(value.ToJulian(t.Year(), int(t.Month()), t.Day()))
	})
	reg("TIME", func(vm *VM, a []value.Value) value.Value { return value.Str(time.Now().Format("15:04:05")) })
	reg("SECONDS", func(vm *VM, a []value.Value) value.Value {
		t := time.Now()
		s := float64(t.Hour()*3600+t.Minute()*60+t.Second()) + float64(t.Nanosecond()/10000000)/100
		return value.Num(s, 2)
	})
	reg("HB_MILLISECONDS", func(vm *VM, a []value.Value) value.Value { return value.Int(int(time.Now().UnixMilli())) })
	reg("DAY", func(vm *VM, a []value.Value) value.Value {
		_, _, d := value.FromJulian(arg(a, 0).J)
		return value.Int(d)
	})
	reg("MONTH", func(vm *VM, a []value.Value) value.Value {
		_, m, _ := value.FromJulian(arg(a, 0).J)
		return value.Int(m)
	})
	reg("YEAR", func(vm *VM, a []value.Value) value.Value {
		y, _, _ := value.FromJulian(arg(a, 0).J)
		return value.Int(y)
	})
	reg("DOW", func(vm *VM, a []value.Value) value.Value { return value.Int(value.DOW(arg(a, 0).J)) })
	reg("CDOW", func(vm *VM, a []value.Value) value.Value {
		d := value.DOW(arg(a, 0).J)
		if d == 0 {
			return value.Str("")
		}
		return value.Str(time.Weekday(d - 1).String())
	})
	reg("CMONTH", func(vm *VM, a []value.Value) value.Value {
		_, m, _ := value.FromJulian(arg(a, 0).J)
		if m == 0 {
			return value.Str("")
		}
		return value.Str(time.Month(m).String())
	})
	reg("CTOD", func(vm *VM, a []value.Value) value.Value { return value.Date(vm.CTOD(argStr(a, 0, ""))) })
	reg("DTOC", func(vm *VM, a []value.Value) value.Value { return value.Str(vm.DTOC(arg(a, 0).J)) })
	reg("DTOS", func(vm *VM, a []value.Value) value.Value { return value.Str(value.DTOS(arg(a, 0).J)) })
	reg("STOD", func(vm *VM, a []value.Value) value.Value { return value.Date(value.STOD(argStr(a, 0, ""))) })
	reg("HB_STOD", builtinTable["STOD"].fn)
	reg("HB_DATE", func(vm *VM, a []value.Value) value.Value {
		y, m, d := argInt(a, 0, 0), argInt(a, 1, 0), argInt(a, 2, 0)
		if !value.ValidDate(y, m, d) {
			return value.Date(0)
		}
		return value.Date(value.ToJulian(y, m, d))
	})
	reg("ADDMONTH", func(vm *VM, a []value.Value) value.Value {
		y, m, d := value.FromJulian(arg(a, 0).J)
		n := argInt(a, 1, 1)
		m += n
		for m > 12 {
			m -= 12
			y++
		}
		for m < 1 {
			m += 12
			y--
		}
		if dm := value.DaysInMonth(y, m); d > dm {
			d = dm
		}
		return value.Date(value.ToJulian(y, m, d))
	})
	reg("BOM", func(vm *VM, a []value.Value) value.Value {
		y, m, _ := value.FromJulian(arg(a, 0).J)
		return value.Date(value.ToJulian(y, m, 1))
	})
	reg("EOM", func(vm *VM, a []value.Value) value.Value {
		y, m, _ := value.FromJulian(arg(a, 0).J)
		return value.Date(value.ToJulian(y, m, value.DaysInMonth(y, m)))
	})
	reg("ELAPTIME", func(vm *VM, a []value.Value) value.Value {
		p := func(s string) int {
			var h, m, x int
			parts := strings.Split(s, ":")
			if len(parts) == 3 {
				h, _ = strconv.Atoi(parts[0])
				m, _ = strconv.Atoi(parts[1])
				x, _ = strconv.Atoi(parts[2])
			}
			return h*3600 + m*60 + x
		}
		d := p(argStr(a, 1, "")) - p(argStr(a, 0, ""))
		if d < 0 {
			d += 86400
		}
		return value.Str(strings.Join([]string{itoaPad(d/3600, 2), itoaPad(d/60%60, 2), itoaPad(d%60, 2)}, ":"))
	})

	// ---------- arrays ----------
	regRaw("ARRAY", func(vm *VM, a []value.Value) value.Value {
		var mk func(dims []int) value.Value
		mk = func(dims []int) value.Value {
			n := dims[0]
			if n < 0 {
				n = 0
			}
			items := make([]value.Value, n)
			if len(dims) > 1 {
				for i := range items {
					items[i] = mk(dims[1:])
				}
			}
			return value.NewArray(items)
		}
		dims := make([]int, len(a))
		for i := range a {
			dims[i] = argInt(a, i, 0)
		}
		if len(dims) == 0 {
			return value.ArrayOf(0)
		}
		return mk(dims)
	})
	reg("AADD", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY {
			return vm.Raise(errArg("AADD", arr))
		}
		arr.Arr().Items = append(arr.Arr().Items, arg(a, 1))
		return arg(a, 1)
	})
	reg("ASIZE", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY {
			return vm.Raise(errArg("ASIZE", arr))
		}
		n := argInt(a, 1, 0)
		if n < 0 {
			n = 0
		}
		it := arr.Arr().Items
		if n < len(it) {
			arr.Arr().Items = it[:n]
		} else {
			arr.Arr().Items = append(it, make([]value.Value, n-len(it))...)
		}
		return arr
	})
	reg("ADEL", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY {
			return arr
		}
		it := arr.Arr().Items
		i := argInt(a, 1, 0)
		if i >= 1 && i <= len(it) {
			copy(it[i-1:], it[i:])
			it[len(it)-1] = value.Nil
			if argBool(a, 2, false) {
				arr.Arr().Items = it[:len(it)-1]
			}
		}
		return arr
	})
	reg("AINS", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY {
			return arr
		}
		it := arr.Arr().Items
		i := argInt(a, 1, 0)
		if argBool(a, 3, false) {
			it = append(it, value.Nil)
			arr.Arr().Items = it
		}
		if i >= 1 && i <= len(it) {
			copy(it[i:], it[i-1:len(it)-1])
			it[i-1] = arg(a, 2)
		}
		return arr
	})
	reg("AFILL", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY {
			return arr
		}
		it := arr.Arr().Items
		start := argInt(a, 2, 1)
		cnt := argInt(a, 3, len(it)-start+1)
		for i := start; i < start+cnt && i <= len(it); i++ {
			if i >= 1 {
				it[i-1] = arg(a, 1)
			}
		}
		return arr
	})
	reg("ASCAN", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY {
			return value.Int(0)
		}
		it := arr.Arr().Items
		x := arg(a, 1)
		start := argInt(a, 2, 1)
		cnt := argInt(a, 3, len(it)-start+1)
		for i := start; i < start+cnt && i <= len(it); i++ {
			if i < 1 {
				continue
			}
			if x.K == value.BLOCK {
				if vm.EvalBlock(x, it[i-1], value.Int(i)).Truthy() {
					return value.Int(i)
				}
			} else if it[i-1].K == x.K && vm.compare(opEQ, it[i-1], x).Truthy() {
				return value.Int(i)
			}
		}
		return value.Int(0)
	})
	reg("HB_ASCAN", builtinTable["ASCAN"].fn)
	reg("AEVAL", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY {
			return arr
		}
		it := arr.Arr().Items
		b := arg(a, 1)
		start := argInt(a, 2, 1)
		cnt := argInt(a, 3, len(it)-start+1)
		for i := start; i < start+cnt && i <= len(arr.Arr().Items); i++ {
			if i >= 1 {
				vm.EvalBlock(b, arr.Arr().Items[i-1], value.Int(i))
			}
		}
		return arr
	})
	reg("ACOPY", func(vm *VM, a []value.Value) value.Value {
		src, dst := arg(a, 0), arg(a, 1)
		if src.K != value.ARRAY || dst.K != value.ARRAY {
			return dst
		}
		s, d := src.Arr().Items, dst.Arr().Items
		start := argInt(a, 2, 1)
		cnt := argInt(a, 3, len(s)-start+1)
		tpos := argInt(a, 4, 1)
		for i := 0; i < cnt; i++ {
			si, di := start+i, tpos+i
			if si < 1 || si > len(s) || di < 1 || di > len(d) {
				break
			}
			d[di-1] = s[si-1]
		}
		return dst
	})
	reg("ACLONE", func(vm *VM, a []value.Value) value.Value { return cloneVal(arg(a, 0)) })
	reg("ATAIL", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY || len(arr.Arr().Items) == 0 {
			return value.Nil
		}
		it := arr.Arr().Items
		return it[len(it)-1]
	})
	reg("ASORT", func(vm *VM, a []value.Value) value.Value {
		arr := arg(a, 0)
		if arr.K != value.ARRAY {
			return arr
		}
		it := arr.Arr().Items
		start := argInt(a, 1, 1)
		cnt := argInt(a, 2, len(it)-start+1)
		if start < 1 {
			start = 1
		}
		end := start - 1 + cnt
		if end > len(it) {
			end = len(it)
		}
		if start > end {
			return arr
		}
		seg := it[start-1 : end]
		b := arg(a, 3)
		sort.SliceStable(seg, func(i, j int) bool {
			if b.K == value.BLOCK {
				return vm.EvalBlock(b, seg[i], seg[j]).Truthy()
			}
			x, y := seg[i], seg[j]
			if x.K != y.K {
				return x.K < y.K
			}
			if x.K == value.STRING {
				return x.S < y.S
			}
			r := vm.compare(opLT, x, y)
			return r.Truthy()
		})
		return arr
	})

	// ---------- hashes (xHarbour) ----------
	reg("HASH", func(vm *VM, a []value.Value) value.Value {
		h := value.NewHash()
		for i := 0; i+1 < len(a); i += 2 {
			h.Set(arg(a, i), arg(a, i+1))
		}
		return value.HashVal(h)
	})
	reg("HB_HASH", builtinTable["HASH"].fn)
	hget := func(vm *VM, a []value.Value) value.Value {
		h := arg(a, 0)
		if h.K != value.HASH {
			return vm.Raise(errArg("HGET", h))
		}
		v, ok := h.Hsh().Get(arg(a, 1))
		if !ok {
			if len(a) > 2 {
				return arg(a, 2)
			}
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 2, SubCode: 1132, Description: "Bound error: hash key not found", Args: []value.Value{arg(a, 1)}})
		}
		return v
	}
	reg("HGET", hget)
	reg("HB_HGET", hget)
	reg("HB_HGETDEF", func(vm *VM, a []value.Value) value.Value {
		h := arg(a, 0)
		if h.K == value.HASH {
			if v, ok := h.Hsh().Get(arg(a, 1)); ok {
				return v
			}
		}
		return arg(a, 2)
	})
	hset := func(vm *VM, a []value.Value) value.Value {
		h := arg(a, 0)
		if h.K == value.HASH {
			h.Hsh().Set(arg(a, 1), arg(a, 2))
		}
		return h
	}
	reg("HSET", hset)
	reg("HB_HSET", hset)
	hhas := func(vm *VM, a []value.Value) value.Value {
		h := arg(a, 0)
		if h.K != value.HASH {
			return value.False
		}
		_, ok := h.Hsh().Get(arg(a, 1))
		return value.Bool(ok)
	}
	reg("HHASKEY", hhas)
	reg("HB_HHASKEY", hhas)
	hdel := func(vm *VM, a []value.Value) value.Value {
		h := arg(a, 0)
		if h.K == value.HASH {
			h.Hsh().Del(arg(a, 1))
		}
		return h
	}
	reg("HDEL", hdel)
	reg("HB_HDEL", hdel)
	hkeys := func(vm *VM, a []value.Value) value.Value {
		h := arg(a, 0)
		if h.K != value.HASH {
			return value.ArrayOf(0)
		}
		return value.NewArray(append([]value.Value(nil), h.Hsh().Keys...))
	}
	reg("HGETKEYS", hkeys)
	reg("HB_HKEYS", hkeys)
	hvals := func(vm *VM, a []value.Value) value.Value {
		h := arg(a, 0)
		if h.K != value.HASH {
			return value.ArrayOf(0)
		}
		return value.NewArray(append([]value.Value(nil), h.Hsh().Vals...))
	}
	reg("HGETVALUES", hvals)
	reg("HB_HVALUES", hvals)
	reg("HB_HCLONE", func(vm *VM, a []value.Value) value.Value { return cloneVal(arg(a, 0)) })
	reg("HB_ISHASH", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.HASH) })
	reg("HB_ISARRAY", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.ARRAY) })
	reg("HB_ISSTRING", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.STRING) })
	reg("HB_ISCHAR", builtinTable["HB_ISSTRING"].fn)
	reg("HB_ISNUMERIC", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.NUMBER) })
	reg("HB_ISLOGICAL", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.LOGICAL) })
	reg("HB_ISDATE", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.DATE) })
	reg("HB_ISBLOCK", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.BLOCK) })
	reg("HB_ISNIL", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.NIL) })
	reg("HB_ISOBJECT", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).K == value.OBJECT) })
}

func cloneVal(v value.Value) value.Value {
	switch v.K {
	case value.ARRAY:
		it := v.Arr().Items
		out := make([]value.Value, len(it))
		for i, x := range it {
			out[i] = cloneVal(x)
		}
		return value.NewArray(out)
	case value.HASH:
		h := value.NewHash()
		for i, k := range v.Hsh().Keys {
			h.Set(k, cloneVal(v.Hsh().Vals[i]))
		}
		return value.HashVal(h)
	}
	return v
}

func soundex(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	codes := map[byte]byte{'B': '1', 'F': '1', 'P': '1', 'V': '1', 'C': '2', 'G': '2', 'J': '2', 'K': '2', 'Q': '2', 'S': '2', 'X': '2', 'Z': '2',
		'D': '3', 'T': '3', 'L': '4', 'M': '5', 'N': '5', 'R': '6'}
	out := []byte{}
	var last byte
	for i := 0; i < len(s) && len(out) < 4; i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			continue
		}
		code := codes[c]
		if len(out) == 0 {
			out = append(out, c)
			last = code
			continue
		}
		if code != 0 && code != last {
			out = append(out, code)
		}
		if c != 'H' && c != 'W' {
			last = code
		}
	}
	for len(out) < 4 {
		out = append(out, '0')
	}
	return string(out)
}

// memoLines word-wraps text into lines of at most width characters.
func memoLines(s string, width int) []string {
	if width < 4 {
		width = 4
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\x8D\n", " ")
	var out []string
	for _, para := range strings.Split(s, "\n") {
		para = strings.ReplaceAll(para, "\t", "    ")
		for len(para) > width {
			cut := strings.LastIndexByte(para[:width+1], ' ')
			if cut <= 0 {
				cut = width
				out = append(out, para[:cut])
				para = para[cut:]
				continue
			}
			out = append(out, para[:cut])
			para = para[cut+1:]
		}
		out = append(out, para)
	}
	if len(out) > 0 && out[len(out)-1] == "" && strings.HasSuffix(s, "\n") {
		out = out[:len(out)-1]
	}
	if s == "" {
		return nil
	}
	return out
}
