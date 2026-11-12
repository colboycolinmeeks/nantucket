// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"math"
	"strings"

	"nantucket/internal/compile"
	"nantucket/internal/value"
)

func maxDec(a, b value.Value) int {
	if a.Dec > b.Dec {
		return int(a.Dec)
	}
	return int(b.Dec)
}

func (vm *VM) setDecimals() int {
	return vm.sets[3].Int() // _SET_DECIMALS
}

// arithNames and arithCodes give the operator and error code for
// argument errors in arithmetic. They are built once: building them on
// every operation took most of the time of a busy loop (Perf #3).
var (
	arithNames = map[compile.Op]string{compile.ADD: "+", compile.SUB: "-", compile.MUL: "*", compile.DIV: "/", compile.MOD: "%", compile.POW: "**"}
	arithCodes = map[compile.Op]int{compile.ADD: 1081, compile.SUB: 1082, compile.MUL: 1083, compile.DIV: 1084, compile.MOD: 1085, compile.POW: 1088}
)

func (vm *VM) arith(op compile.Op, a, b value.Value) value.Value {
	opName, codes := arithNames[op], arithCodes
	if a.K == value.NUMBER && b.K == value.NUMBER {
		switch op {
		case compile.ADD:
			return value.Num(a.N+b.N, maxDec(a, b))
		case compile.SUB:
			return value.Num(a.N-b.N, maxDec(a, b))
		case compile.MUL:
			d := int(a.Dec) + int(b.Dec)
			return value.Num(a.N*b.N, d)
		case compile.DIV:
			if b.N == 0 {
				return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 5, SubCode: 1340, Description: "Zero divisor", Operation: "/", Args: []value.Value{a, b}, CanSubst: true})
			}
			r := a.N / b.N
			d := vm.setDecimals()
			if r == math.Trunc(r) && a.Dec == 0 && b.Dec == 0 && d == 0 {
				return value.Num(r, 0)
			}
			return value.Num(r, d)
		case compile.MOD:
			if b.N == 0 {
				return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 5, SubCode: 1341, Description: "Zero divisor", Operation: "%", Args: []value.Value{a, b}, CanSubst: true})
			}
			return value.Num(math.Mod(a.N, b.N), maxDec(a, b))
		case compile.POW:
			r := math.Pow(a.N, b.N)
			if r == math.Trunc(r) && b.Dec == 0 && b.N >= 0 && a.Dec == 0 {
				return value.Num(r, 0)
			}
			return value.Num(r, vm.setDecimals())
		}
	}
	switch op {
	case compile.ADD:
		switch {
		case a.K == value.STRING && b.K == value.STRING:
			return value.Str(a.S + b.S)
		case a.K == value.DATE && b.K == value.NUMBER:
			return value.Date(a.J + int32(b.N))
		case a.K == value.NUMBER && b.K == value.DATE:
			return value.Date(b.J + int32(a.N))
		case a.K == value.HASH && b.K == value.HASH:
			h := value.NewHash()
			for i, k := range a.Hsh().Keys {
				h.Set(k, a.Hsh().Vals[i])
			}
			for i, k := range b.Hsh().Keys {
				h.Set(k, b.Hsh().Vals[i])
			}
			return value.HashVal(h)
		}
	case compile.SUB:
		switch {
		case a.K == value.STRING && b.K == value.STRING:
			t := strings.TrimRight(a.S, " ")
			return value.Str(t + b.S + strings.Repeat(" ", len(a.S)-len(t)))
		case a.K == value.DATE && b.K == value.DATE:
			return value.Int(int(a.J - b.J))
		case a.K == value.DATE && b.K == value.NUMBER:
			return value.Date(a.J - int32(b.N))
		}
	}
	return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 1, SubCode: codes[op], Description: "Argument error", Operation: opName, Args: []value.Value{a, b}, CanSubst: true})
}

func (vm *VM) exact() bool { return vm.sets[1].Truthy() }

// strEq implements '=' for strings with SET EXACT.
func (vm *VM) strEq(a, b string) bool {
	if vm.exact() {
		return strings.TrimRight(a, " ") == strings.TrimRight(b, " ")
	}
	if len(b) == 0 {
		return true
	}
	if len(a) < len(b) {
		return false
	}
	return a[:len(b)] == b
}

func (vm *VM) equalExact(a, b value.Value) bool {
	if a.K != b.K {
		return false
	}
	switch a.K {
	case value.NIL:
		return true
	case value.STRING:
		return a.S == b.S
	case value.NUMBER:
		return a.N == b.N
	case value.DATE:
		return a.J == b.J
	case value.LOGICAL:
		return a.B == b.B
	}
	return a.P == b.P
}

func cmp3(x, y float64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

func (vm *VM) compare(op compile.Op, a, b value.Value) value.Value {
	if op == compile.EXACTEQ {
		if a.K == value.STRING && b.K == value.STRING {
			return value.Bool(a.S == b.S)
		}
		if a.K == b.K || a.K == value.NIL || b.K == value.NIL {
			if a.K == value.NUMBER {
				return value.Bool(a.N == b.N)
			}
			return value.Bool(vm.equalExact(a, b))
		}
		return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 1, SubCode: 1070, Description: "Argument error", Operation: "==", Args: []value.Value{a, b}, CanSubst: true})
	}
	if a.K == value.NIL || b.K == value.NIL {
		switch op {
		case compile.EQ:
			return value.Bool(a.K == b.K)
		case compile.NE:
			return value.Bool(a.K != b.K)
		}
		return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 1, SubCode: 1073, Description: "Argument error", Operation: opSym(op), Args: []value.Value{a, b}, CanSubst: true})
	}
	if a.K != b.K {
		return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 1, SubCode: 1071, Description: "Argument error", Operation: opSym(op), Args: []value.Value{a, b}, CanSubst: true})
	}
	var c int
	switch a.K {
	case value.STRING:
		if op == compile.EQ {
			return value.Bool(vm.strEq(a.S, b.S))
		}
		if op == compile.NE {
			return value.Bool(!vm.strEq(a.S, b.S))
		}
		x, y := a.S, b.S
		if vm.exact() {
			x, y = strings.TrimRight(x, " "), strings.TrimRight(y, " ")
		} else if len(x) > len(y) {
			x = x[:len(y)]
		}
		c = strings.Compare(x, y)
	case value.NUMBER:
		c = cmp3(a.N, b.N)
	case value.DATE:
		c = cmp3(float64(a.J), float64(b.J))
	case value.LOGICAL:
		ai, bi := 0, 0
		if a.B {
			ai = 1
		}
		if b.B {
			bi = 1
		}
		c = ai - bi
	default:
		if op == compile.EQ {
			return value.Bool(a.P == b.P)
		}
		if op == compile.NE {
			return value.Bool(a.P != b.P)
		}
		return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 1, SubCode: 1072, Description: "Argument error", Operation: opSym(op), Args: []value.Value{a, b}})
	}
	switch op {
	case compile.EQ:
		return value.Bool(c == 0)
	case compile.NE:
		return value.Bool(c != 0)
	case compile.LT:
		return value.Bool(c < 0)
	case compile.LE:
		return value.Bool(c <= 0)
	case compile.GT:
		return value.Bool(c > 0)
	case compile.GE:
		return value.Bool(c >= 0)
	}
	return value.False
}

func opSym(op compile.Op) string {
	switch op {
	case compile.EQ:
		return "="
	case compile.NE:
		return "<>"
	case compile.LT:
		return "<"
	case compile.LE:
		return "<="
	case compile.GT:
		return ">"
	case compile.GE:
		return ">="
	}
	return op.String()
}

// ---------- display formatting ----------

// Display formats a value the way ? shows it.
func (vm *VM) Display(v value.Value) string {
	v = v.Deref()
	switch v.K {
	case value.NUMBER:
		if vm.sets[2].Truthy() && v.Width == 0 { // SET FIXED
			d := vm.setDecimals()
			nv := v
			nv.Dec = int8(d)
			return value.FormatNum(v.N, nv.DefaultWidth(), d)
		}
		return value.FormatNum(v.N, v.DefaultWidth(), int(v.Dec))
	case value.DATE:
		return vm.DTOC(v.J)
	case value.OBJECT:
		if e, ok := v.P.(*RTError); ok {
			return "Error " + e.Description
		}
	}
	return v.String()
}

func (vm *VM) dateFormat() string {
	f := vm.sets[4].S
	if f == "" {
		f = "mm/dd/yy"
	}
	return f
}

// DTOC formats a date with the current SET DATE format.
func (vm *VM) DTOC(j int32) string {
	f := strings.ToLower(vm.dateFormat())
	if j <= 0 {
		var sb strings.Builder
		for _, c := range f {
			if c == 'y' || c == 'm' || c == 'd' {
				sb.WriteByte(' ')
			} else {
				sb.WriteRune(c)
			}
		}
		return sb.String()
	}
	y, m, d := value.FromJulian(j)
	return formatDate(f, y, m, d)
}

func formatDate(f string, y, m, d int) string {
	var sb strings.Builder
	for i := 0; i < len(f); {
		c := f[i]
		n := 1
		for i+n < len(f) && f[i+n] == c {
			n++
		}
		switch c {
		case 'y':
			s := itoaPad(y, 4)
			if n < 4 {
				s = s[len(s)-n:]
			}
			sb.WriteString(s)
		case 'm':
			sb.WriteString(itoaPad(m, n))
		case 'd':
			sb.WriteString(itoaPad(d, n))
		default:
			sb.WriteString(f[i : i+n])
		}
		i += n
	}
	return sb.String()
}

func itoaPad(n, w int) string {
	s := ""
	if n == 0 {
		s = "0"
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	for len(s) < w {
		s = "0" + s
	}
	return s
}

// CTOD parses a date string using the current format.
func (vm *VM) CTOD(s string) int32 {
	f := strings.ToLower(vm.dateFormat())
	var nums []int
	cur, in := 0, false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			cur = cur*10 + int(c-'0')
			in = true
		} else if in {
			nums = append(nums, cur)
			cur, in = 0, false
		}
	}
	if in {
		nums = append(nums, cur)
	}
	if len(nums) < 3 {
		return 0
	}
	var order []byte
	for i := 0; i < len(f); i++ {
		c := f[i]
		if (c == 'y' || c == 'm' || c == 'd') && (len(order) == 0 || order[len(order)-1] != c) {
			order = append(order, c)
		}
	}
	var y, m, d int
	for i, c := range order {
		if i >= 3 {
			break
		}
		switch c {
		case 'y':
			y = nums[i]
		case 'm':
			m = nums[i]
		case 'd':
			d = nums[i]
		}
	}
	if y < 100 {
		epoch := vm.sets[5].Int()
		if epoch == 0 {
			epoch = 1950
		}
		century := epoch / 100 * 100
		if y < epoch%100 {
			century += 100
		}
		y += century
	}
	if !value.ValidDate(y, m, d) {
		return 0
	}
	return value.ToJulian(y, m, d)
}

const (
	opEQ = compile.EQ
	opLT = compile.LT
	opGT = compile.GT
)

const (
	opADD = compile.ADD
	opDIV = compile.DIV
)
