// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"nantucket/internal/term"
	"nantucket/internal/value"
)

var builtinTable = map[string]builtinDef{}

func reg(name string, fn Builtin)    { builtinTable[name] = builtinDef{fn: fn} }
func regRaw(name string, fn Builtin) { builtinTable[name] = builtinDef{fn: fn, rawArgs: true} }

func (vm *VM) registerBuiltins() {
	for k, v := range builtinTable {
		vm.builtins[k] = v
	}
}

// IsBuiltin reports whether a function name is built in.
func (vm *VM) IsBuiltin(name string) bool {
	_, ok := vm.builtins[strings.ToUpper(name)]
	return ok
}

// ---------- argument helpers ----------

func arg(a []value.Value, i int) value.Value {
	if i < len(a) {
		return a[i].Deref()
	}
	return value.Nil
}
func argStr(a []value.Value, i int, def string) string {
	v := arg(a, i)
	if v.K == value.STRING {
		return v.S
	}
	return def
}
func argNum(a []value.Value, i int, def float64) float64 {
	v := arg(a, i)
	if v.K == value.NUMBER {
		return v.N
	}
	return def
}
func argInt(a []value.Value, i int, def int) int {
	v := arg(a, i)
	if v.K == value.NUMBER {
		return int(v.N)
	}
	return def
}
func argBool(a []value.Value, i int, def bool) bool {
	v := arg(a, i)
	if v.K == value.LOGICAL {
		return v.B
	}
	return def
}
func has(a []value.Value, i int) bool { return i < len(a) && a[i].Deref().K != value.NIL }

// ---------- SET handling ----------

var logicalSets = map[int]bool{1: true, 2: true, 8: true, 9: true, 10: true, 11: true, 12: true, 17: true, 18: true, 21: true, 23: true,
	26: true, 27: true, 28: true, 29: true, 30: true, 31: true, 32: true, 33: true, 35: true, 37: true, 38: true,
	100: true, 101: true, 102: true, 103: true, 104: true, 106: true}

func (vm *VM) initSets() {
	s := vm.sets
	s[1] = value.False
	s[2] = value.False
	s[3] = value.Int(2)
	s[4] = value.Str("mm/dd/yy")
	s[5] = value.Int(1950)
	s[6] = value.Str("")
	s[7] = value.Str("")
	s[8] = value.True
	s[9] = value.False
	s[10] = value.False
	s[11] = value.False
	s[12] = value.True
	s[14] = value.Int(50)
	s[15] = value.Str("W/N,N/W,N,N,N/W")
	s[16] = value.Int(1)
	s[17] = value.True
	s[18] = value.False
	s[19] = value.Str("")
	s[20] = value.Str("SCREEN")
	s[23] = value.False
	s[24] = value.Str("")
	s[25] = value.Int(0)
	s[26] = value.False
	s[27] = value.False
	s[28] = value.True
	s[29] = value.False
	s[30] = value.False
	s[31] = value.True
	s[32] = value.True
	s[33] = value.False
	s[34] = value.Str("::")
	s[35] = value.False
	s[36] = value.Int(0)
	s[37] = value.False
	s[100] = value.False
	s[101] = value.True
	s[102] = value.True
	s[103] = value.False
	s[104] = value.False
	s[105] = value.Int(50)
	s[106] = value.False
}

func toLogical(v value.Value) (value.Value, bool) {
	switch v.K {
	case value.LOGICAL:
		return v, true
	case value.STRING:
		switch strings.ToUpper(strings.TrimSpace(v.S)) {
		case "ON", ".T.", "T", "Y", "YES":
			return value.True, true
		case "OFF", ".F.", "F", "N", "NO":
			return value.False, true
		}
	}
	return v, false
}

// SetValue implements Set(n, [new], [additive]).
func (vm *VM) SetValue(n int, args []value.Value) value.Value {
	old := vm.sets[n]
	if len(args) == 0 || args[0].Deref().K == value.NIL && n != 19 && n != 24 {
		return old
	}
	v := args[0].Deref()
	if logicalSets[n] {
		lv, ok := toLogical(v)
		if !ok {
			return vm.Raise(errArgCode(2020, "SET", v))
		}
		v = lv
	}
	switch n {
	case 3, 5, 14, 16, 25, 36, 105:
		if v.K != value.NUMBER {
			return vm.Raise(errArgCode(2020, "SET", v))
		}
	}
	vm.sets[n] = v
	switch n {
	case 106: // SET ECHO
		vm.updateTracing()
	case 11:
		vm.DBSet.Deleted = v.B
	case 9:
		vm.DBSet.SoftSeek = v.B
	case 15:
		vm.setColors(v.S)
	case 14:
		if t, ok := vm.Con.(interface{ SetTypeahead(n int) }); ok {
			t.SetTypeahead(v.Int())
		}
	case 16:
		vm.Con.SetCursor(v.Int())
	case 19:
		if vm.altFile != nil {
			vm.altFile.Close()
			vm.altFile = nil
		}
		if v.K == value.STRING && strings.TrimSpace(v.S) != "" {
			name := vm.DefaultPath(v.S, ".txt")
			flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
			if argBool(args, 1, false) {
				flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
			}
			f, err := os.OpenFile(name, flag, 0644)
			if err != nil {
				return vm.Raise(&RTError{SubSystem: "TERM", GenCode: 20, SubCode: 2013, Description: "Create error", Filename: name})
			}
			vm.altFile = f
		}
	case 24:
		if vm.prnFile != nil {
			vm.prnFile.Close()
			vm.prnFile = nil
		}
		if v.K == value.STRING && strings.TrimSpace(v.S) != "" {
			name := vm.DefaultPath(v.S, ".prn")
			flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
			if argBool(args, 1, false) {
				flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
			}
			f, err := os.OpenFile(name, flag, 0644)
			if err != nil {
				return vm.Raise(&RTError{SubSystem: "TERM", GenCode: 20, SubCode: 2013, Description: "Create error", Filename: name})
			}
			vm.prnFile = f
		}
	case 103:
		vm.applyCentury(v.B)
	}
	return old
}

func (vm *VM) applyCentury(on bool) {
	f := vm.sets[4].S
	lf := strings.ToLower(f)
	if on {
		if !strings.Contains(lf, "yyyy") {
			i := strings.Index(lf, "yy")
			if i >= 0 {
				f = f[:i] + "yyyy" + f[i+2:]
			}
		}
	} else if i := strings.Index(lf, "yyyy"); i >= 0 {
		f = f[:i] + "yy" + f[i+4:]
	}
	vm.sets[4] = value.Str(f)
}

// printer output
func (vm *VM) printerOn() bool { return vm.sets[23].Truthy() }

func (vm *VM) writePrinter(s string) {
	f := vm.prnFile
	if f == nil {
		name := vm.DefaultPath("nantucket", ".prn")
		var err error
		f, err = os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		vm.prnFile = f
	}
	// Track the print head and indent each line by SET MARGIN.
	margin := strings.Repeat(" ", max(0, vm.sets[25].Int()))
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\n':
			vm.prnRow++
			vm.prnCol = 0
			sb.WriteByte(c)
		case '\r':
			vm.prnCol = 0
			sb.WriteByte(c)
		case '\f':
			vm.prnRow, vm.prnCol = 0, 0
			sb.WriteByte(c)
		default:
			if vm.prnCol == 0 {
				sb.WriteString(margin)
			}
			vm.prnCol++
			sb.WriteByte(c)
		}
	}
	f.WriteString(sb.String())
}

// Out writes console output honoring CONSOLE/ALTERNATE/PRINTER.
func (vm *VM) Out(s string, newline bool) {
	if vm.sets[17].Truthy() {
		if newline {
			vm.Con.NewLine()
		}
		vm.Con.Out(s, vm.colors.Std)
	}
	if vm.Echo != nil {
		if newline {
			fmt.Fprint(vm.Echo, "\n")
		}
		fmt.Fprint(vm.Echo, s)
	}
	if vm.altFile != nil && vm.sets[18].Truthy() {
		if newline {
			vm.altFile.WriteString("\r\n")
		}
		vm.altFile.WriteString(s)
	}
	if vm.printerOn() {
		if newline {
			vm.writePrinter("\r\n")
		}
		vm.writePrinter(s)
	}
}

// endText stops TEXT TO FILE / PRINTER output.
func (vm *VM) endText() {
	if vm.textFile != nil {
		vm.textFile.Close()
		vm.textFile = nil
	}
	vm.textPrinter = false
}

// Println is a convenience for system messages.
func (vm *VM) Println(s string) { vm.Out(s, true) }

func (vm *VM) joinDisplay(args []value.Value) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if v := a.Deref(); v.Memo {
			// memo text wraps at SET MEMOWIDTH
			parts[i] = strings.Join(memoLines(v.S, vm.memoWidth()), "\n")
			continue
		}
		parts[i] = vm.Display(a)
	}
	return strings.Join(parts, " ")
}

// memoWidth is the SET MEMOWIDTH value used to wrap memo text.
func (vm *VM) memoWidth() int { return max(4, vm.sets[105].Int()) }

func (vm *VM) colorArg(args []value.Value, i int, def term.Attr) term.Attr {
	if s := argStr(args, i, ""); s != "" {
		return term.ParseColorPair(strings.Split(s, ",")[0], def)
	}
	return def
}

func (vm *VM) deviceScreen() bool {
	return !strings.EqualFold(vm.sets[20].S, "PRINTER")
}

// ---------- keyboard ----------

// Inkey waits for a key (secs < 0 forever, 0 poll) and handles SET KEY.
func (vm *VM) Inkey(secs float64, handleSetKey bool) int {
	for {
		var d time.Duration
		switch {
		case secs < 0:
			d = -1
		case secs == 0:
			d = 0
		default:
			d = time.Duration(secs * float64(time.Second))
		}
		k, ok := vm.Con.ReadKey(d)
		if !ok {
			if secs < 0 && !vm.Con.Interactive() {
				// end of input in stream mode
				panic(&QuitSignal{})
			}
			return 0
		}
		if k == term.KDebug {
			vm.handleDebugKey()
			continue
		}
		vm.lastKey = k
		if handleSetKey {
			if b, ok := vm.setKeys[k]; ok && b.K == value.BLOCK {
				fr := vm.ownerFrame()
				pn, ln := "", 0
				if fr != nil {
					pn, ln = fr.fn.Name, fr.line
				}
				vm.EvalBlock(b, value.Str(pn), value.Int(ln), value.Str(""))
				continue
			}
			if s, ok := vm.fkeys[k]; ok {
				keys := make([]int, 0, len(s))
				for i := 0; i < len(s); i++ {
					c := int(s[i])
					if s[i] == ';' {
						c = term.KEnter
					}
					keys = append(keys, c)
				}
				vm.Con.PushKeys(keys)
				continue
			}
		}
		return k
	}
}

// ---------- core builtins ----------

func init() {
	reg("QOUT", func(vm *VM, a []value.Value) value.Value {
		vm.Out(vm.joinDisplay(a), true)
		return value.Nil
	})
	reg("QQOUT", func(vm *VM, a []value.Value) value.Value {
		vm.Out(vm.joinDisplay(a), false)
		return value.Nil
	})
	reg("OUTSTD", func(vm *VM, a []value.Value) value.Value {
		os.Stdout.WriteString(vm.joinDisplay(a))
		return value.Nil
	})
	reg("OUTERR", func(vm *VM, a []value.Value) value.Value {
		os.Stderr.WriteString(vm.joinDisplay(a))
		return value.Nil
	})
	reg("__TEXTLINE", func(vm *VM, a []value.Value) value.Value {
		s := vm.MacroSubst(argStr(a, 0, ""), false)
		vm.Out(s, true)
		if vm.textFile != nil {
			vm.textFile.WriteString(s + "\r\n")
		}
		if vm.textPrinter && !vm.printerOn() {
			vm.writePrinter(s + "\r\n")
		}
		return value.Nil
	})
	// __TextTo("FILE", cFile) / __TextTo("PRINTER") start sending TEXT
	// lines to a file (created afresh, .txt by default) or the printer as
	// well as the screen; __TextTo() stops.
	reg("__TEXTTO", func(vm *VM, a []value.Value) value.Value {
		vm.endText()
		switch strings.ToUpper(argStr(a, 0, "")) {
		case "PRINTER":
			vm.textPrinter = true
		case "FILE":
			path := vm.DefaultPath(argStr(a, 1, ""), ".txt")
			f, err := os.Create(path)
			if err != nil {
				return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 20, SubCode: 2011, Description: "Create error: " + err.Error(), Filename: path})
			}
			vm.textFile = f
		}
		return value.Nil
	})
	reg("__MACROSTR", func(vm *VM, a []value.Value) value.Value {
		return value.Str(vm.MacroSubst(argStr(a, 0, ""), false))
	})
	reg("__EXECSTMT", func(vm *VM, a []value.Value) value.Value {
		vm.ExecStatements(argStr(a, 0, ""))
		return value.Nil
	})
	reg("__EJECT", func(vm *VM, a []value.Value) value.Value {
		if vm.printerOn() || !vm.deviceScreen() {
			vm.writePrinter("\f")
		}
		return value.Nil
	})
	reg("DEVPOS", func(vm *VM, a []value.Value) value.Value {
		r, c := argInt(a, 0, 0), argInt(a, 1, 0)
		if vm.deviceScreen() {
			vm.Con.SetPos(r, c)
		} else {
			if r < vm.prnRow {
				vm.writePrinter("\f") // earlier row: start a new page
			}
			if r > vm.prnRow {
				vm.writePrinter(strings.Repeat("\r\n", r-vm.prnRow))
			}
			if c < vm.prnCol {
				vm.writePrinter("\r\n")
			}
			if c > vm.prnCol {
				vm.writePrinter(strings.Repeat(" ", c-vm.prnCol))
			}
		}
		return value.Nil
	})
	reg("DEVOUT", func(vm *VM, a []value.Value) value.Value {
		vm.devOut(vm.Display(arg(a, 0)), vm.colorArg(a, 1, vm.colors.Std))
		return value.Nil
	})
	reg("DEVOUTPICT", func(vm *VM, a []value.Value) value.Value {
		s := vm.Display(arg(a, 0))
		if has(a, 1) {
			s = vm.Transform(arg(a, 0), argStr(a, 1, ""))
		}
		vm.devOut(s, vm.colorArg(a, 2, vm.colors.Std))
		return value.Nil
	})
	reg("DISPOUT", func(vm *VM, a []value.Value) value.Value {
		vm.screenOut(vm.Display(arg(a, 0)), vm.colorArg(a, 1, vm.colors.Std))
		return value.Nil
	})
	reg("DISPOUTAT", func(vm *VM, a []value.Value) value.Value {
		vm.Con.Put(argInt(a, 0, 0), argInt(a, 1, 0), vm.Display(arg(a, 2)), vm.colorArg(a, 3, vm.colors.Std))
		return value.Nil
	})
	reg("SETPOS", func(vm *VM, a []value.Value) value.Value {
		vm.Con.SetPos(argInt(a, 0, 0), argInt(a, 1, 0))
		return value.Nil
	})
	reg("ROW", func(vm *VM, a []value.Value) value.Value { r, _ := vm.Con.Pos(); return value.Int(r) })
	reg("COL", func(vm *VM, a []value.Value) value.Value { _, c := vm.Con.Pos(); return value.Int(c) })
	reg("PROW", func(vm *VM, a []value.Value) value.Value { return value.Int(vm.prnRow) })
	reg("PCOL", func(vm *VM, a []value.Value) value.Value { return value.Int(vm.prnCol) })
	reg("SETPRC", func(vm *VM, a []value.Value) value.Value {
		vm.prnRow, vm.prnCol = argInt(a, 0, 0), argInt(a, 1, 0)
		return value.Nil
	})
	reg("MAXROW", func(vm *VM, a []value.Value) value.Value { r, _ := vm.Con.Size(); return value.Int(r - 1) })
	reg("MAXCOL", func(vm *VM, a []value.Value) value.Value { _, c := vm.Con.Size(); return value.Int(c - 1) })
	reg("SCROLL", func(vm *VM, a []value.Value) value.Value {
		rows, cols := vm.Con.Size()
		vm.Con.Scroll(argInt(a, 0, 0), argInt(a, 1, 0), argInt(a, 2, rows-1), argInt(a, 3, cols-1), argInt(a, 4, 0), vm.colors.Std)
		return value.Nil
	})
	reg("DISPBOX", func(vm *VM, a []value.Value) value.Value {
		vm.dispBox(argInt(a, 0, 0), argInt(a, 1, 0), argInt(a, 2, 0), argInt(a, 3, 0), arg(a, 4), vm.colorArg(a, 5, vm.colors.Std))
		return value.Nil
	})
	reg("DISPBEGIN", func(vm *VM, a []value.Value) value.Value { return value.Nil })
	reg("DISPEND", func(vm *VM, a []value.Value) value.Value { vm.Con.Flush(); return value.Nil })
	reg("DISPCOUNT", func(vm *VM, a []value.Value) value.Value { return value.Int(0) })
	reg("SAVESCREEN", func(vm *VM, a []value.Value) value.Value {
		rows, cols := vm.Con.Size()
		t, l, b, r := argInt(a, 0, 0), argInt(a, 1, 0), argInt(a, 2, rows-1), argInt(a, 3, cols-1)
		return value.Str(encodeCells(vm.Con.Save(t, l, b, r)))
	})
	reg("RESTSCREEN", func(vm *VM, a []value.Value) value.Value {
		rows, cols := vm.Con.Size()
		t, l, b, r := argInt(a, 0, 0), argInt(a, 1, 0), argInt(a, 2, rows-1), argInt(a, 3, cols-1)
		vm.Con.Restore(t, l, b, r, decodeCells(argStr(a, 4, "")))
		return value.Nil
	})
	reg("SETCOLOR", func(vm *VM, a []value.Value) value.Value {
		old := vm.colors.Text
		if len(a) > 0 && arg(a, 0).K == value.STRING {
			vm.setColors(arg(a, 0).S)
		}
		return value.Str(old)
	})
	reg("SETCURSOR", func(vm *VM, a []value.Value) value.Value {
		old := vm.sets[16]
		if has(a, 0) {
			vm.sets[16] = value.Int(argInt(a, 0, 1))
			vm.Con.SetCursor(argInt(a, 0, 1))
		}
		return old
	})
	reg("SETBLINK", func(vm *VM, a []value.Value) value.Value { return value.False })
	reg("SETMODE", func(vm *VM, a []value.Value) value.Value { return value.False })
	reg("ISCOLOR", func(vm *VM, a []value.Value) value.Value { return value.Bool(!vm.theme.Mono) })
	reg("ISCOLOUR", func(vm *VM, a []value.Value) value.Value { return value.Bool(!vm.theme.Mono) })
	reg("NOSNOW", func(vm *VM, a []value.Value) value.Value { return value.Nil })
	reg("TONE", func(vm *VM, a []value.Value) value.Value { vm.Con.Bell(); return value.Nil })
	reg("COLORSELECT", func(vm *VM, a []value.Value) value.Value { return value.Nil })

	reg("INKEY", func(vm *VM, a []value.Value) value.Value {
		secs := 0.0
		if has(a, 0) {
			secs = argNum(a, 0, 0)
			if secs == 0 {
				secs = -1
			}
		}
		return value.Int(vm.Inkey(secs, true))
	})
	reg("LASTKEY", func(vm *VM, a []value.Value) value.Value { return value.Int(vm.lastKey) })
	reg("NEXTKEY", func(vm *VM, a []value.Value) value.Value {
		k, ok := vm.Con.ReadKey(0)
		if !ok {
			return value.Int(0)
		}
		vm.Con.UnreadKey(k)
		return value.Int(k)
	})
	reg("__KEYBOARD", func(vm *VM, a []value.Value) value.Value {
		vm.Con.ClearKeys()
		vm.typeaheadLeft = max(1, vm.sets[14].Int()) // KEYBOARD stuffs at most SET TYPEAHEAD keys
		vm.pushKeys(arg(a, 0))
		vm.typeaheadLeft = 0
		return value.Nil
	})
	reg("HB_KEYPUT", func(vm *VM, a []value.Value) value.Value {
		vm.pushKeys(arg(a, 0))
		return value.Nil
	})
	reg("SETKEY", func(vm *VM, a []value.Value) value.Value {
		k := argInt(a, 0, 0)
		old := vm.setKeys[k]
		if len(a) > 1 {
			b := arg(a, 1)
			if b.K == value.BLOCK {
				vm.setKeys[k] = b
			} else {
				delete(vm.setKeys, k)
			}
		}
		return old
	})
	reg("__SETFUNCTION", func(vm *VM, a []value.Value) value.Value {
		n := argInt(a, 0, 0)
		codes := map[int]int{1: 28, 2: -1, 3: -2, 4: -3, 5: -4, 6: -5, 7: -6, 8: -7, 9: -8, 10: -9}
		if k, ok := codes[n]; ok {
			vm.fkeys[k] = argStr(a, 1, "")
		}
		return value.Nil
	})

	reg("__WAIT", func(vm *VM, a []value.Value) value.Value {
		p := "Press any key to continue..."
		if has(a, 0) {
			p = vm.Display(arg(a, 0))
		}
		vm.Out(p, true)
		vm.Con.Flush()
		k := vm.Inkey(-1, true)
		if k >= 32 && k < 256 {
			vm.Out(string(rune(k)), false)
			return value.Str(string(rune(k)))
		}
		return value.Str("")
	})
	reg("__ACCEPT", func(vm *VM, a []value.Value) value.Value {
		if has(a, 0) {
			vm.Out(vm.Display(arg(a, 0)), true)
		} else {
			vm.Out("", true)
		}
		s, _ := term.ReadLine(vm.Con, vm.colors.Std, "", 255, nil)
		vm.echoInput(s)
		return value.Str(s)
	})
	reg("__INPUT", func(vm *VM, a []value.Value) value.Value {
		for {
			if has(a, 0) {
				vm.Out(vm.Display(arg(a, 0)), true)
			} else {
				vm.Out("", true)
			}
			s, ok := term.ReadLine(vm.Con, vm.colors.Std, "", 255, nil)
			vm.echoInput(s)
			if !ok && !vm.Con.Interactive() {
				return value.Nil
			}
			if strings.TrimSpace(s) == "" {
				continue
			}
			return vm.MacroEval(s)
		}
	})
	reg("__QUIT", func(vm *VM, a []value.Value) value.Value { panic(&QuitSignal{}) })
	reg("__CANCEL", func(vm *VM, a []value.Value) value.Value { panic(&CancelSignal{}) })
	reg("__RUN", func(vm *VM, a []value.Value) value.Value {
		cmd := argStr(a, 0, "")
		// a shell's cd would only change the shell's own directory, which
		// ends straight away, so !cd and RUN cd change ours instead
		if dir, ok := shellCD(cmd); ok {
			vm.changeDir("RUN", dir)
			return value.Nil
		}
		vm.runShell(cmd)
		return value.Nil
	})
	// __ChDir(cDir): the CD / CHDIR command. No directory: show the
	// current one.
	reg("__CHDIR", func(vm *VM, a []value.Value) value.Value {
		vm.changeDir("CD", argStr(a, 0, ""))
		return value.Nil
	})

	reg("SET", func(vm *VM, a []value.Value) value.Value {
		return vm.SetValue(argInt(a, 0, 0), a[min(1, len(a)):])
	})
	reg("__SETCENTURY", func(vm *VM, a []value.Value) value.Value {
		return vm.SetValue(103, a)
	})
	reg("_DFSET", func(vm *VM, a []value.Value) value.Value {
		if vm.sets[103].Truthy() {
			vm.sets[4] = value.Str(argStr(a, 0, "mm/dd/yyyy"))
		} else {
			vm.sets[4] = value.Str(argStr(a, 1, "mm/dd/yy"))
		}
		return value.Nil
	})
	reg("__SETPROC", func(vm *VM, a []value.Value) value.Value {
		name := argStr(a, 0, "")
		if strings.TrimSpace(name) == "" {
			vm.procFiles = nil
			return value.Nil
		}
		if vm.Packaged {
			key := strings.ToUpper(strings.TrimSuffix(filepath.Base(strings.TrimSpace(name)), filepath.Ext(name)))
			if src, ok := vm.Embedded[key]; ok {
				if _, err := vm.LoadSource(key, src); err != nil {
					vm.compileErr(err)
				}
				vm.runInitProcs()
				return value.Nil
			}
		}
		p := vm.FindFile(name, ".prg")
		if p == "" {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2004, Description: "File does not exist", Filename: name})
		}
		if _, err := vm.LoadFile(p); err != nil {
			vm.compileErr(err)
		}
		vm.procFiles = append(vm.procFiles, p)
		vm.runInitProcs()
		return value.Nil
	})
	reg("__COMPILE", func(vm *VM, a []value.Value) value.Value {
		p := vm.FindFile(argStr(a, 0, ""), ".prg")
		if p == "" {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2004, Description: "File does not exist", Filename: argStr(a, 0, "")})
		}
		if _, err := vm.LoadFile(p); err != nil {
			vm.Println(err.Error())
			return value.False
		}
		vm.Println("Compiled " + p)
		return value.True
	})

	// evaluation / types
	reg("EVAL", func(vm *VM, a []value.Value) value.Value {
		if len(a) == 0 {
			return vm.Raise(errArgCode(1004, "EVAL"))
		}
		return vm.EvalBlock(a[0], a[1:]...)
	})
	reg("VALTYPE", func(vm *VM, a []value.Value) value.Value { return value.Str(arg(a, 0).TypeChar()) })
	reg("TYPE", func(vm *VM, a []value.Value) value.Value { return value.Str(vm.typeOf(argStr(a, 0, ""))) })
	reg("EMPTY", func(vm *VM, a []value.Value) value.Value { return value.Bool(arg(a, 0).Empty()) })
	reg("PCOUNT", func(vm *VM, a []value.Value) value.Value {
		if fr := vm.ownerFrame(); fr != nil {
			return value.Int(len(fr.args))
		}
		return value.Int(0)
	})
	reg("PVALUE", func(vm *VM, a []value.Value) value.Value {
		if fr := vm.ownerFrame(); fr != nil {
			i := argInt(a, 0, 0)
			if i >= 1 && i <= len(fr.args) {
				return fr.args[i-1].Deref()
			}
		}
		return value.Nil
	})
	reg("HB_APARAMS", func(vm *VM, a []value.Value) value.Value {
		if fr := vm.ownerFrame(); fr != nil {
			items := make([]value.Value, len(fr.args))
			for i, x := range fr.args {
				items[i] = x.Deref()
			}
			return value.NewArray(items)
		}
		return value.ArrayOf(0)
	})
	reg("PROCNAME", func(vm *VM, a []value.Value) value.Value {
		lvl := argInt(a, 0, 0)
		i := len(vm.frames) - 1 - lvl
		if i >= 0 {
			return value.Str(vm.procName(vm.frames[i]))
		}
		return value.Str("")
	})
	reg("PROCLINE", func(vm *VM, a []value.Value) value.Value {
		lvl := argInt(a, 0, 0)
		i := len(vm.frames) - 1 - lvl
		if i >= 0 {
			return value.Int(vm.frames[i].line)
		}
		return value.Int(0)
	})
	reg("PROCFILE", func(vm *VM, a []value.Value) value.Value {
		if fr := vm.ownerFrame(); fr != nil {
			return value.Str(fr.fn.File)
		}
		return value.Str("")
	})
	reg("BREAK", func(vm *VM, a []value.Value) value.Value { panic(&BreakSignal{V: arg(a, 0)}) })
	reg("ERRORBLOCK", func(vm *VM, a []value.Value) value.Value {
		old := vm.errorBlock
		if has(a, 0) && arg(a, 0).K == value.BLOCK {
			vm.errorBlock = arg(a, 0)
		}
		return old
	})
	reg("ERRORNEW", func(vm *VM, a []value.Value) value.Value {
		return value.Obj(&RTError{SubSystem: "BASE", Severity: 2})
	})
	reg("ERRORLEVEL", func(vm *VM, a []value.Value) value.Value { return value.Int(0) })
	reg("__ERRRT_BASE", func(vm *VM, a []value.Value) value.Value { return value.Nil })
	reg("DEFERROR", func(vm *VM, a []value.Value) value.Value { return defaultErrorBlock(vm, a) })
	reg("FUNCNAME", func(vm *VM, a []value.Value) value.Value { return value.Nil })
	reg("__DYNSN2SYM", func(vm *VM, a []value.Value) value.Value { return value.Nil })
	reg("ISFUNCTION", func(vm *VM, a []value.Value) value.Value {
		f, b := vm.lookupFunc(nil, strings.ToUpper(argStr(a, 0, "")))
		return value.Bool(f != nil || b != nil)
	})
	reg("HB_ISFUNCTION", builtinTable["ISFUNCTION"].fn)
	reg("__FIELDGETNAME", func(vm *VM, a []value.Value) value.Value {
		return vm.getField(vm.CurWA(), argStr(a, 0, ""))
	})
	reg("__FIELDPUTNAME", func(vm *VM, a []value.Value) value.Value {
		vm.putField(vm.CurWA(), argStr(a, 0, ""), arg(a, 1))
		return arg(a, 1)
	})
	reg("__SELF", func(vm *VM, a []value.Value) value.Value { return value.Nil })

	// environment
	reg("GETENV", func(vm *VM, a []value.Value) value.Value { return value.Str(os.Getenv(argStr(a, 0, ""))) })
	reg("GETE", builtinTable["GETENV"].fn)
	reg("HB_GETENV", builtinTable["GETENV"].fn)
	reg("OS", func(vm *VM, a []value.Value) value.Value { return value.Str(runtime.GOOS) })
	reg("VERSION", func(vm *VM, a []value.Value) value.Value { return value.Str("Nantucket " + Version) })
	reg("HB_COMPILER", func(vm *VM, a []value.Value) value.Value { return value.Str("Go " + runtime.Version()) })
	reg("MEMORY", func(vm *VM, a []value.Value) value.Value { return value.Int(65535) })
	reg("DISKSPACE", func(vm *VM, a []value.Value) value.Value { return value.Int(1 << 30) })
	reg("NETNAME", func(vm *VM, a []value.Value) value.Value { h, _ := os.Hostname(); return value.Str(h) })
	reg("CURDIR", func(vm *VM, a []value.Value) value.Value { d, _ := os.Getwd(); return value.Str(d) })
	reg("DIRCHANGE", func(vm *VM, a []value.Value) value.Value {
		if err := os.Chdir(argStr(a, 0, "")); err != nil {
			return value.Int(-1)
		}
		return value.Int(0)
	})
	reg("MAKEDIR", func(vm *VM, a []value.Value) value.Value {
		if err := os.Mkdir(argStr(a, 0, ""), 0755); err != nil {
			return value.Int(-1)
		}
		return value.Int(0)
	})
	reg("HB_SHELL", func(vm *VM, a []value.Value) value.Value { vm.runShell(argStr(a, 0, "")); return value.Nil })
	reg("__QUITPROG", builtinTable["__QUIT"].fn)
}

func (vm *VM) echoInput(s string) {
	if vm.Echo != nil {
		fmt.Fprint(vm.Echo, s)
	}
}

// limitKeys trims keys to the room left by KEYBOARD (0 = no limit).
func (vm *VM) limitKeys(keys []int) []int {
	if vm.typeaheadLeft <= 0 {
		return keys
	}
	if len(keys) > vm.typeaheadLeft {
		keys = keys[:vm.typeaheadLeft]
	}
	vm.typeaheadLeft -= len(keys)
	if vm.typeaheadLeft == 0 {
		vm.typeaheadLeft = -1 // full: drop the rest
	}
	return keys
}

func (vm *VM) pushKeys(v value.Value) {
	switch v.K {
	case value.STRING:
		keys := make([]int, 0, len(v.S))
		for i := 0; i < len(v.S); i++ {
			c := int(v.S[i])
			if c == ';' {
				c = term.KEnter
			}
			keys = append(keys, c)
		}
		vm.Con.PushKeys(vm.limitKeys(keys))
	case value.NUMBER:
		vm.Con.PushKeys(vm.limitKeys([]int{v.Int()}))
	case value.ARRAY:
		for _, it := range v.Arr().Items {
			vm.pushKeys(it)
		}
	}
}

// devOut writes @...SAY output to the screen or printer.
func (vm *VM) devOut(s string, a term.Attr) {
	if !vm.deviceScreen() {
		vm.writePrinter(s)
		return
	}
	vm.screenOut(s, a)
}

func (vm *VM) screenOut(s string, a term.Attr) {
	r, c := vm.Con.Pos()
	vm.Con.Put(r, c, s, a)
	vm.Con.SetPos(r, c+term.DisplayLen(s))
	if vm.Echo != nil {
		fmt.Fprint(vm.Echo, s)
	}
}

var boxSingle = []rune("┌─┐│┘─└│")
var boxDouble = []rune("╔═╗║╝═╚║")

func (vm *VM) dispBox(t, l, b, r int, spec value.Value, at term.Attr) {
	var chars []rune
	fill := rune(0)
	switch spec.K {
	case value.NUMBER:
		if spec.Int() == 2 {
			chars = boxDouble
		} else {
			chars = boxSingle
		}
	case value.STRING:
		rs := term.Runes(spec.S)
		if len(rs) >= 8 {
			chars = rs[:8]
			if len(rs) >= 9 {
				fill = rs[8]
			}
		} else {
			chars = boxSingle
		}
	default:
		chars = boxSingle
	}
	if b < t || r < l {
		return
	}
	put := func(row, col int, ch rune) { vm.Con.Put(row, col, string(ch), at) }
	if t == b {
		for c := l; c <= r; c++ {
			put(t, c, chars[1])
		}
		return
	}
	if l == r {
		for row := t; row <= b; row++ {
			put(row, l, chars[3])
		}
		return
	}
	put(t, l, chars[0])
	put(t, r, chars[2])
	put(b, r, chars[4])
	put(b, l, chars[6])
	for c := l + 1; c < r; c++ {
		put(t, c, chars[1])
		put(b, c, chars[5])
	}
	for row := t + 1; row < b; row++ {
		put(row, l, chars[7])
		put(row, r, chars[3])
		if fill != 0 {
			for c := l + 1; c < r; c++ {
				put(row, c, fill)
			}
		}
	}
	vm.Con.SetPos(t+1, l+1)
}

func encodeCells(cells []term.Cell) string {
	var sb strings.Builder
	for _, c := range cells {
		sb.WriteString(string(c.R))
		sb.WriteByte(0)
		sb.WriteByte(byte(c.A))
		sb.WriteByte(byte(c.A >> 8))
		sb.WriteByte(0xFF)
	}
	return sb.String()
}

func decodeCells(s string) []term.Cell {
	var cells []term.Cell
	for len(s) > 0 {
		i := strings.IndexByte(s, 0)
		if i < 0 || i+4 > len(s) {
			break
		}
		r := []rune(s[:i])
		ch := ' '
		if len(r) > 0 {
			ch = r[0]
		}
		a := term.Attr(s[i+1]) | term.Attr(s[i+2])<<8
		cells = append(cells, term.Cell{R: ch, A: a})
		s = s[i+4:]
	}
	return cells
}

// shellCD recognises a plain "cd <dir>" (or chdir) shell command, one
// with nothing else for the shell to do, and returns the directory.
func shellCD(cmd string) (string, bool) {
	cmd = strings.TrimSpace(cmd)
	word, rest, _ := strings.Cut(cmd, " ")
	if !strings.EqualFold(word, "cd") && !strings.EqualFold(word, "chdir") {
		return "", false
	}
	if strings.ContainsAny(rest, ";&|<>`()") {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// changeDir changes the working directory, as CD does. "" shows the
// current directory, "-" goes back to the previous one, and ~ and $VAR
// are expanded.
func (vm *VM) changeDir(op, dir string) {
	dir = strings.TrimSpace(dir)
	if len(dir) >= 2 && (dir[0] == '"' || dir[0] == '\'') && dir[len(dir)-1] == dir[0] {
		dir = dir[1 : len(dir)-1]
	}
	cur, _ := os.Getwd()
	switch {
	case dir == "":
		vm.Println(cur)
		return
	case dir == "-":
		if vm.prevDir == "" {
			vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2006, Description: "No previous directory", Operation: op})
			return
		}
		dir = vm.prevDir
	}
	dir = os.ExpandEnv(dir)
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = home + dir[1:]
		}
	}
	if err := os.Chdir(dir); err != nil {
		desc := "Cannot change directory"
		if errors.Is(err, os.ErrNotExist) {
			desc = "Directory does not exist"
		} else if errors.Is(err, os.ErrPermission) {
			desc = "Permission denied"
		} else if fi, serr := os.Stat(dir); serr == nil && !fi.IsDir() {
			desc = "Not a directory"
		}
		vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2006, Description: desc, Operation: op, Filename: dir})
		return
	}
	vm.prevDir = cur
}

func (vm *VM) runShell(cmd string) {
	cmd = strings.TrimSpace(cmd)
	shell, flag := "/bin/sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/c"
	}
	vm.Con.Suspend()
	c := exec.Command(shell, flag, cmd)
	if cmd == "" {
		c = exec.Command(shell)
	}
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	c.Run()
	if vm.Con.Interactive() {
		fmt.Print("\nPress Enter to return...")
		var b [1]byte
		os.Stdin.Read(b[:])
	}
	vm.Con.Resume()
}

// typeOf implements Type(): evaluates an expression string safely.
func (vm *VM) typeOf(expr string) (t string) {
	if strings.TrimSpace(expr) == "" {
		return "U"
	}
	fn, err := vm.compileMacro(expr, false)
	if err != nil {
		return "UE"
	}
	saveEB := vm.errorBlock
	vm.errorBlock = value.Value{K: value.BLOCK, P: &value.Block{Fn: NativeBlock(func(vm *VM, a []value.Value) value.Value {
		panic(&BreakSignal{V: value.Str("\x00typeerr")})
	})}}
	defer func() {
		vm.errorBlock = saveEB
		if r := recover(); r != nil {
			if b, ok := r.(*BreakSignal); ok && b.V.K == value.STRING && b.V.S == "\x00typeerr" {
				t = "U"
				return
			}
			if _, ok := r.(*RTError); ok {
				t = "U"
				return
			}
			panic(r)
		}
	}()
	sp, nf := len(vm.stack), len(vm.frames)
	v := func() (v value.Value) {
		defer func() {
			if r := recover(); r != nil {
				vm.stack = vm.stack[:sp]
				vm.frames = vm.frames[:nf]
				panic(r)
			}
		}()
		return vm.Call(fn, nil, nil, true)
	}()
	return v.TypeChar()
}

const Version = "0.2"
