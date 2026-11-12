// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package vm executes compiled xBase code.
package vm

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"nantucket/internal/compile"
	"nantucket/internal/db"
	"nantucket/internal/pp"
	"nantucket/internal/term"
	"nantucket/internal/value"
)

type Builtin func(vm *VM, args []value.Value) value.Value

type builtinDef struct {
	fn      Builtin
	rawArgs bool // receive REF args undereferenced
}

// NativeBlock is a code block implemented in Go.
type NativeBlock func(vm *VM, args []value.Value) value.Value

type Frame struct {
	fn          *compile.Func
	locals      []*value.Ref
	capt        []*value.Ref
	args        []value.Value
	privates    []string
	line        int
	prevLine    int  // the line before, for the debugger's watchpoints
	transparent bool // code blocks and macros don't own privates
	self        value.Value
}

type memvar struct {
	ref   *value.Ref
	owner *Frame // nil = PUBLIC
}

type modEntry struct {
	mod   *compile.Module
	mtime int64
	path  string
}

type VM struct {
	Con term.Console

	stack  []value.Value
	frames []*Frame

	funcs      map[string]*compile.Func
	modStatics map[*compile.Module]map[string]*compile.Func
	modules    map[string]*modEntry
	statics    []*value.Ref
	gen        int

	memvars map[string][]*memvar

	areas      [256]*db.WorkArea
	curArea    int
	aliasStack []int
	DBSet      db.Settings

	sets       map[int]value.Value
	colors     term.ColorSpec
	theme      term.Theme  // current theme
	homeTheme  term.Theme  // theme SET THEME TO (no name) returns to
	ext        map[any]any // per-session state of feature packages (see Ext)
	prevDir    string      // directory before the last CD, for CD -
	netErr     bool        // NetErr(): the last USE or APPEND BLANK hit another session
	netErrNote string      // message for the dot prompt about a USE that failed
	notes      []string    // other messages for the dot prompt (rebuilt indexes)
	macroCache map[string]*compile.Func
	fragCache  map[string]*compile.Func // compiled dot-prompt / &cmd fragments
	// per-runtime state for FOpen()/FCreate() handles and @ ... PROMPT
	memFiles    map[int]*os.File
	nextHandle  int
	lastFError  int
	textFile    *os.File            // TEXT TO FILE
	textPrinter bool                // TEXT TO PRINTER
	initProcs   []*compile.Func     // INIT PROCEDUREs still to run
	exitProcs   []*compile.Func     // EXIT PROCEDUREs to run when the runtime closes
	traceW      io.Writer           // -trace output
	tracing     bool                // anything below is on (checked on every line)
	stepping    bool                // SET STEP ON
	dbg         debugState          // breakpoints and stepping (debug.go)
	debugReq    atomic.Bool         // Alt-D was pressed
	debugOff    atomic.Bool         // AltD(0) turned Alt-D off
	trusted     bool                // SET TRUSTED ON: data file expressions aren't checked
	codePage    *db.CodePage        // SET CODEPAGE: for tables that don't name one
	srcCache    map[string][]string // program sources, for showing traced lines
	PP          *pp.PP

	errorBlock value.Value
	inErrBlock int
	interrupt  atomic.Bool
	terminate  atomic.Bool // Shutdown asked the program to stop
	busy       sync.Mutex  // held while running, released while waiting for input

	procFiles []string
	altFile   *os.File
	prnFile   *os.File
	prnCol    int
	prnRow    int
	// typeaheadLeft is the room left while KEYBOARD stuffs keys:
	// 0 = no limit, -1 = full.
	typeaheadLeft int
	onAssertFail  func(AssertFailure) // set by the test runner

	builtins map[string]builtinDef

	// keyboard handling
	setKeys map[int]value.Value
	lastKey int
	fkeys   map[int]string

	// output capture for tests (when set, console output also goes here)
	Echo io.Writer

	// Embedded holds program sources packaged into the binary (name in
	// upper case, without .prg -> source), and EmbeddedHeaders the
	// packaged #include files (file name in upper case -> source).
	Embedded        map[string]string
	EmbeddedHeaders map[string]string
	// Packaged is set when running a program packaged with "nantucket
	// build": packaged files are used before files on disk.
	Packaged bool

	quitRequested bool
	depth         int
}

func New(con term.Console) (*VM, error) {
	p, err := pp.NewStd(nil)
	if err != nil {
		return nil, err
	}
	vm := &VM{
		Con:        con,
		funcs:      map[string]*compile.Func{},
		modStatics: map[*compile.Module]map[string]*compile.Func{},
		modules:    map[string]*modEntry{},
		memvars:    map[string][]*memvar{},
		curArea:    1,
		sets:       map[int]value.Value{},
		macroCache: map[string]*compile.Func{},
		PP:         p,
		setKeys:    map[int]value.Value{},
		fkeys:      map[int]string{},
		builtins:   map[string]builtinDef{},
		memFiles:   map[int]*os.File{},
		nextHandle: 5,
	}
	vm.PP.Include = vm.includeFile
	// the runtime is busy except while the console waits for input; see
	// Shutdown
	vm.busy.Lock()
	if w, ok := con.(term.WaitNotifier); ok {
		w.SetWaitHook(func(waiting bool) {
			if waiting {
				vm.busy.Unlock()
			} else {
				vm.busy.Lock()
			}
		})
	}
	vm.initSets()
	vm.registerBuiltins()
	vm.homeTheme = term.Themes["color"]
	vm.theme = vm.homeTheme
	vm.colors = vm.theme.Colors("")
	vm.sets[15] = value.Str(vm.colors.Text)
	vm.errorBlock = value.Value{K: value.BLOCK, P: &value.Block{Fn: NativeBlock(defaultErrorBlock), Src: "{|e| DefError(e)}"}}
	// GetList is a public array used by @...GET / READ
	vm.publicVar("GETLIST", value.ArrayOf(0))
	for _, f := range newHooks {
		f(vm)
	}
	return vm, nil
}

func defaultErrorBlock(vm *VM, args []value.Value) value.Value {
	if len(args) > 0 {
		if e, ok := args[0].P.(*RTError); ok {
			panic(e)
		}
	}
	panic(errMsg("unknown error"))
}

// ---------- stack ----------

func (vm *VM) push(v value.Value) { vm.stack = append(vm.stack, v) }
func (vm *VM) pop() value.Value {
	n := len(vm.stack) - 1
	v := vm.stack[n]
	vm.stack = vm.stack[:n]
	return v
}
func (vm *VM) top() *value.Value { return &vm.stack[len(vm.stack)-1] }

// ---------- errors ----------

// Raise reports a runtime error through the error block. If the error
// block returns, its value is returned as a substitute result.
func (vm *VM) Raise(e *RTError) value.Value {
	if fr := vm.curFrame(); fr != nil && e.Proc == "" {
		e.Proc, e.Line = vm.procName(fr), fr.line
		e.Trace = vm.Trace()
	}
	if vm.inErrBlock > 3 {
		panic(e)
	}
	vm.inErrBlock++
	defer func() { vm.inErrBlock-- }()
	return vm.EvalBlock(vm.errorBlock, value.Obj(e))
}

func (vm *VM) curFrame() *Frame {
	if len(vm.frames) == 0 {
		return nil
	}
	return vm.frames[len(vm.frames)-1]
}

func (vm *VM) procName(fr *Frame) string {
	if fr.fn.IsBlock {
		// name a block after the function that defined it
		return "(b)" + vm.ownerName(fr)
	}
	return fr.fn.Name
}

func (vm *VM) ownerName(fr *Frame) string {
	for i := len(vm.frames) - 1; i >= 0; i-- {
		if vm.frames[i] == fr {
			for j := i - 1; j >= 0; j-- {
				if !vm.frames[j].fn.IsBlock {
					return vm.frames[j].fn.Name
				}
			}
		}
	}
	return ""
}

// Trace returns the call stack, innermost first.
func (vm *VM) Trace() []string {
	var out []string
	for i := len(vm.frames) - 1; i >= 0; i-- {
		fr := vm.frames[i]
		out = append(out, fmt.Sprintf("%s(%d)", vm.procName(fr), fr.line))
	}
	return out
}

// ---------- memvars ----------

func (vm *VM) findMemvar(name string) *value.Ref {
	if st := vm.memvars[name]; len(st) > 0 {
		return st[len(st)-1].ref
	}
	return nil
}

// ownerFrame is the procedure that owns new private variables.
func (vm *VM) ownerFrame() *Frame {
	for i := len(vm.frames) - 1; i >= 0; i-- {
		if !vm.frames[i].transparent {
			return vm.frames[i]
		}
	}
	if len(vm.frames) > 0 {
		return vm.frames[0]
	}
	return nil
}

func (vm *VM) privateVar(name string, v value.Value) *value.Ref {
	fr := vm.ownerFrame()
	if fr != nil {
		// re-declaring a private at the same level reuses it
		if st := vm.memvars[name]; len(st) > 0 && st[len(st)-1].owner == fr {
			st[len(st)-1].ref.V = v
			return st[len(st)-1].ref
		}
	}
	ref := &value.Ref{V: v}
	vm.memvars[name] = append(vm.memvars[name], &memvar{ref: ref, owner: fr})
	if fr != nil {
		fr.privates = append(fr.privates, name)
	} else {
		vm.memvars[name][len(vm.memvars[name])-1].owner = nil
	}
	return ref
}

func (vm *VM) publicVar(name string, v value.Value) *value.Ref {
	if st := vm.memvars[name]; len(st) > 0 {
		for _, m := range st {
			if m.owner == nil {
				return m.ref
			}
		}
		ref := &value.Ref{V: v}
		vm.memvars[name] = append([]*memvar{{ref: ref}}, st...)
		return ref
	}
	ref := &value.Ref{V: v}
	vm.memvars[name] = []*memvar{{ref: ref}}
	return ref
}

// SetMemvar assigns a memvar, creating a private if it doesn't exist.
func (vm *VM) SetMemvar(name string, v value.Value) {
	name = strings.ToUpper(name)
	if r := vm.findMemvar(name); r != nil {
		r.V = v
		return
	}
	vm.privateVar(name, v)
}

func (vm *VM) GetMemvar(name string) (value.Value, bool) {
	if r := vm.findMemvar(strings.ToUpper(name)); r != nil {
		return r.V, true
	}
	return value.Nil, false
}

func (vm *VM) releaseFramePrivates(fr *Frame) {
	for i := len(fr.privates) - 1; i >= 0; i-- {
		name := fr.privates[i]
		st := vm.memvars[name]
		for j := len(st) - 1; j >= 0; j-- {
			if st[j].owner == fr {
				st = append(st[:j], st[j+1:]...)
				break
			}
		}
		if len(st) == 0 {
			delete(vm.memvars, name)
		} else {
			vm.memvars[name] = st
		}
	}
	fr.privates = nil
}

// ---------- variables: fields / ambiguous ----------

func (vm *VM) CurWA() *db.WorkArea { return vm.areas[vm.curArea] }

func (vm *VM) fieldIn(wa *db.WorkArea, name string) int {
	if wa == nil {
		return -1
	}
	return wa.T.FieldIndex(name)
}

func (vm *VM) getField(wa *db.WorkArea, name string) value.Value {
	if wa == nil {
		return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 13, SubCode: 1002, Description: "Alias does not exist", Operation: name})
	}
	i := wa.T.FieldIndex(name)
	if i < 0 {
		return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 3, SubCode: 1003, Description: "Variable does not exist", Operation: name})
	}
	return wa.GetField(i)
}

func (vm *VM) putField(wa *db.WorkArea, name string, v value.Value) {
	if wa == nil {
		vm.Raise(&RTError{SubSystem: "BASE", GenCode: 13, SubCode: 1002, Description: "Alias does not exist", Operation: name})
		return
	}
	i := wa.T.FieldIndex(name)
	if i < 0 {
		vm.Raise(&RTError{SubSystem: "BASE", GenCode: 3, SubCode: 1003, Description: "Variable does not exist", Operation: name})
		return
	}
	if err := wa.PutField(i, v.Deref()); err != nil {
		if _, ok := err.(*db.FieldTypeError); ok {
			vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 1, SubCode: 1020, Description: "Data type error", Operation: name, Args: []value.Value{v}})
			return
		}
		vm.Raise(dbError(err, name))
	}
}

func (vm *VM) pushVarAmbig(name string) value.Value {
	if wa := vm.CurWA(); wa != nil {
		if i := wa.T.FieldIndex(name); i >= 0 {
			return wa.GetField(i)
		}
	}
	if r := vm.findMemvar(name); r != nil {
		return r.V
	}
	return vm.Raise(errNoVar(name))
}

func (vm *VM) popVarAmbig(name string, v value.Value) {
	if wa := vm.CurWA(); wa != nil {
		if i := wa.T.FieldIndex(name); i >= 0 {
			vm.putField(wa, name, v)
			return
		}
	}
	if r := vm.findMemvar(name); r != nil {
		r.V = v
		return
	}
	vm.privateVar(name, v)
}

// areaByAlias resolves an alias name or work area designator.
func (vm *VM) areaByAlias(a string) int {
	a = strings.ToUpper(strings.TrimSpace(a))
	for i := 1; i < len(vm.areas); i++ {
		if wa := vm.areas[i]; wa != nil && wa.Alias == a {
			return i
		}
	}
	if len(a) == 1 && a[0] >= 'A' && a[0] <= 'J' {
		return int(a[0]-'A') + 1
	}
	n := 0
	for _, c := range a {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	if n >= 1 && n < len(vm.areas) {
		return n
	}
	return 0
}

func (vm *VM) areaFromValue(v value.Value) int {
	v = v.Deref()
	switch v.K {
	case value.NUMBER:
		return v.Int()
	case value.STRING:
		return vm.areaByAlias(v.S)
	}
	return 0
}

func (vm *VM) aliasErr(a string) {
	vm.Raise(&RTError{SubSystem: "BASE", GenCode: 13, SubCode: 1002, Description: "Alias does not exist", Operation: a})
}

// ---------- function calls ----------

type callTarget struct {
	gen  int
	user *compile.Func
	bi   *builtinDef
}

func (vm *VM) lookupFunc(mod *compile.Module, name string) (*compile.Func, *builtinDef) {
	if mod != nil {
		if m := vm.modStatics[mod]; m != nil {
			if f := m[name]; f != nil {
				return f, nil
			}
		}
	}
	if f := vm.funcs[name]; f != nil {
		return f, nil
	}
	if b, ok := vm.builtins[name]; ok {
		return nil, &b
	}
	// Clipper significant length is 10 characters
	if len(name) > 10 {
		if f := vm.funcs[name[:10]]; f != nil {
			return f, nil
		}
	}
	return nil, nil
}

// CallFunc calls a function by name with arguments.
func (vm *VM) CallFunc(name string, args []value.Value) value.Value {
	name = strings.ToUpper(name)
	var mod *compile.Module
	if fr := vm.curFrame(); fr != nil {
		mod = fr.fn.Module
	}
	f, b := vm.lookupFunc(mod, name)
	if f != nil {
		return vm.Call(f, args, nil, false)
	}
	if b != nil {
		return vm.callBuiltin(b, args)
	}
	return vm.Raise(errNoFunc(name))
}

func (vm *VM) callBuiltin(b *builtinDef, args []value.Value) value.Value {
	if !b.rawArgs {
		for i := range args {
			if args[i].K == value.REF {
				args[i] = args[i].Deref()
			}
		}
	}
	return b.fn(vm, args)
}

// Call runs a compiled function.
func (vm *VM) Call(fn *compile.Func, args []value.Value, capt []*value.Ref, transparent bool) value.Value {
	fr := &Frame{fn: fn, capt: capt, args: args, transparent: transparent || fn.IsBlock}
	fr.locals = make([]*value.Ref, fn.NLocals)
	// one allocation for all the locals rather than one each (Perf #3)
	cells := make([]value.Ref, fn.NLocals)
	for i := 0; i < fn.NLocals; i++ {
		if i < fn.NParams && i < len(args) && args[i].K == value.REF {
			fr.locals[i] = args[i].P.(*value.Ref) // passed by reference
			continue
		}
		if i < fn.NParams && i < len(args) {
			cells[i].V = args[i]
		}
		fr.locals[i] = &cells[i]
	}
	vm.depth++
	if vm.tracing {
		vm.traceCall(fn, len(args))
		vm.debugCall(fn)
	}
	if vm.depth > 3000 {
		vm.depth = 0
		panic(errMsg("Stack overflow (recursion too deep)"))
	}
	vm.frames = append(vm.frames, fr)
	sp := len(vm.stack)
	defer func() {
		vm.depth--
		vm.releaseFramePrivates(fr)
		// pop this frame (and anything above it left by a panic)
		for i := len(vm.frames) - 1; i >= 0; i-- {
			if vm.frames[i] == fr {
				vm.frames = vm.frames[:i]
				break
			}
		}
		if len(vm.stack) > sp {
			vm.stack = vm.stack[:sp]
		}
	}()
	pc := 0
	for {
		r, st, npc := vm.run(fr, pc)
		if st == stReturn {
			return r
		}
		pc = npc
	}
}

// EvalBlock evaluates a code block value.
func (vm *VM) EvalBlock(b value.Value, args ...value.Value) value.Value {
	b = b.Deref()
	if b.K != value.BLOCK {
		return vm.Raise(errArgCode(1004, "EVAL", b))
	}
	blk := b.Blk()
	switch fn := blk.Fn.(type) {
	case *compile.Func:
		return vm.Call(fn, args, blk.Captured, true)
	case NativeBlock:
		return fn(vm, args)
	}
	return value.Nil
}

// ---------- execution ----------

const (
	stReturn = iota
	stSeqEnd
	stJump
)

func (vm *VM) run(fr *Frame, pc int) (value.Value, int, int) {
	fn := fr.fn
	code := fn.Code
	for pc < len(code) {
		in := code[pc]
		pc++
		switch in.Op {
		case compile.NOP:
		case compile.LINE:
			fr.prevLine, fr.line = fr.line, int(in.A)
			if vm.tracing {
				vm.traceLine(fr)
			}
			if vm.terminate.Load() {
				panic(&QuitSignal{})
			}
			if vm.interrupt.Load() {
				vm.interrupt.Store(false)
				panic(&CancelSignal{Interrupted: true})
			}
			if vm.debugReq.Load() {
				vm.checkDebugRequest(fr)
			}
		case compile.PUSHNIL:
			vm.push(value.Nil)
		case compile.PUSHTRUE:
			vm.push(value.True)
		case compile.PUSHFALSE:
			vm.push(value.False)
		case compile.PUSHCONST:
			vm.push(fn.Consts[in.A])
		case compile.PUSHLOCAL:
			vm.push(fr.locals[in.A].V)
		case compile.POPLOCAL:
			fr.locals[in.A].V = vm.pop().Deref()
		case compile.PUSHLOCALREF:
			vm.push(value.MkRef(fr.locals[in.A]))
		case compile.PUSHCAPT:
			vm.push(fr.capt[in.A].V)
		case compile.POPCAPT:
			fr.capt[in.A].V = vm.pop().Deref()
		case compile.PUSHCAPTREF:
			vm.push(value.MkRef(fr.capt[in.A]))
		case compile.PUSHSTATIC:
			vm.push(vm.statics[fn.Module.StaticBase+int(in.A)].V)
		case compile.POPSTATIC:
			vm.statics[fn.Module.StaticBase+int(in.A)].V = vm.pop().Deref()
		case compile.PUSHSTATICREF:
			vm.push(value.MkRef(vm.statics[fn.Module.StaticBase+int(in.A)]))
		case compile.PUSHMEMVAR:
			name := fn.Names[in.A]
			if r := vm.findMemvar(name); r != nil {
				vm.push(r.V)
			} else {
				vm.push(vm.Raise(errNoVar(name)))
			}
		case compile.POPMEMVAR:
			name := fn.Names[in.A]
			v := vm.pop().Deref()
			if r := vm.findMemvar(name); r != nil {
				r.V = v
			} else {
				vm.privateVar(name, v)
			}
		case compile.PUSHMEMVARREF:
			name := fn.Names[in.A]
			r := vm.findMemvar(name)
			if r == nil {
				r = vm.privateVar(name, value.Nil)
			}
			vm.push(value.MkRef(r))
		case compile.PUSHVAR:
			vm.push(vm.pushVarAmbig(fn.Names[in.A]))
		case compile.POPVAR:
			vm.popVarAmbig(fn.Names[in.A], vm.pop().Deref())
		case compile.PUSHVARREF:
			name := fn.Names[in.A]
			if wa := vm.CurWA(); wa != nil && wa.T.FieldIndex(name) >= 0 {
				vm.push(wa.GetField(wa.T.FieldIndex(name)))
			} else if r := vm.findMemvar(name); r != nil {
				vm.push(value.MkRef(r))
			} else {
				vm.push(vm.Raise(errNoVar(name)))
			}
		case compile.PUSHFIELD:
			wa := vm.CurWA()
			if in.B > 0 {
				a := fn.Names[in.B-1]
				n := vm.areaByAlias(a)
				if n == 0 {
					vm.aliasErr(a)
					vm.push(value.Nil)
					continue
				}
				wa = vm.areas[n]
			}
			vm.push(vm.getField(wa, fn.Names[in.A]))
		case compile.POPFIELD:
			wa := vm.CurWA()
			if in.B > 0 {
				a := fn.Names[in.B-1]
				n := vm.areaByAlias(a)
				if n == 0 {
					vm.pop()
					vm.aliasErr(a)
					continue
				}
				wa = vm.areas[n]
			}
			vm.putField(wa, fn.Names[in.A], vm.pop())
		case compile.PUSHFIELDX:
			a := vm.pop()
			n := vm.areaFromValue(a)
			var wa *db.WorkArea
			if n > 0 {
				wa = vm.areas[n]
			}
			vm.push(vm.getField(wa, fn.Names[in.A]))
		case compile.POPFIELDX:
			a := vm.pop()
			v := vm.pop()
			n := vm.areaFromValue(a)
			var wa *db.WorkArea
			if n > 0 {
				wa = vm.areas[n]
			}
			vm.putField(wa, fn.Names[in.A], v)
		case compile.PUSHALIAS:
			a := fn.Names[in.A]
			n := vm.areaByAlias(a)
			if n == 0 {
				vm.aliasErr(a)
				n = vm.curArea
			}
			vm.aliasStack = append(vm.aliasStack, vm.curArea)
			vm.curArea = n
		case compile.PUSHALIASX:
			a := vm.pop()
			n := vm.areaFromValue(a)
			if n == 0 {
				vm.aliasErr(a.String())
				n = vm.curArea
			}
			vm.aliasStack = append(vm.aliasStack, vm.curArea)
			vm.curArea = n
		case compile.POPALIAS:
			if k := len(vm.aliasStack); k > 0 {
				vm.curArea = vm.aliasStack[k-1]
				vm.aliasStack = vm.aliasStack[:k-1]
			}
		case compile.MACROPUSH:
			vm.push(vm.MacroEval(vm.MacroSubst(fn.Consts[in.A].S, true)))
		case compile.MACROPUSHX:
			v := vm.pop().Deref()
			if v.K != value.STRING {
				vm.push(vm.Raise(errArgCode(1065, "&", v)))
				continue
			}
			vm.push(vm.MacroEval(v.S))
		case compile.MACROPOP:
			v := vm.pop()
			vm.MacroAssign(vm.MacroSubst(fn.Consts[in.A].S, true), v)
		case compile.MACROPOPX:
			name := vm.pop().Deref()
			v := vm.pop()
			vm.MacroAssign(name.S, v)
		case compile.MACROTEXT:
			vm.push(value.Str(vm.MacroSubst(fn.Consts[in.A].S, false)))
		case compile.ARRAYGET:
			idx := vm.pop().Deref()
			arr := vm.pop().Deref()
			vm.push(vm.index(arr, idx))
		case compile.ARRAYSET:
			v := vm.pop().Deref()
			idx := vm.pop().Deref()
			arr := vm.pop().Deref()
			vm.setIndex(arr, idx, v)
			vm.push(v)
		case compile.MKARRAY:
			n := int(in.A)
			items := make([]value.Value, n)
			base := len(vm.stack) - n
			for i := 0; i < n; i++ {
				items[i] = vm.stack[base+i]
				if items[i].K == value.REF {
					items[i] = items[i].Deref()
				}
			}
			vm.stack = vm.stack[:base]
			vm.push(value.NewArray(items))
		case compile.MKHASH:
			n := int(in.A)
			h := value.NewHash()
			base := len(vm.stack) - 2*n
			for i := 0; i < n; i++ {
				h.Set(vm.stack[base+2*i].Deref(), vm.stack[base+2*i+1].Deref())
			}
			vm.stack = vm.stack[:base]
			vm.push(value.HashVal(h))
		case compile.MKBLOCK:
			bf := fn.Blocks[in.A]
			capt := make([]*value.Ref, len(bf.Captures))
			for i, c := range bf.Captures {
				if c.FromLocal {
					capt[i] = fr.locals[c.Index]
				} else {
					capt[i] = fr.capt[c.Index]
				}
			}
			vm.push(value.Value{K: value.BLOCK, P: &value.Block{Fn: bf, Captured: capt, Src: bf.Src}})
		case compile.CALL:
			argc := int(in.B)
			base := len(vm.stack) - argc
			args := make([]value.Value, argc)
			copy(args, vm.stack[base:])
			vm.stack = vm.stack[:base]
			var tgt *callTarget
			if c, ok := fn.Cache[pc-1].(*callTarget); ok && c.gen == vm.gen {
				tgt = c
			} else {
				u, b := vm.lookupFunc(fn.Module, fn.Names[in.A])
				if u == nil && b == nil {
					vm.push(vm.Raise(errNoFunc(fn.Names[in.A])))
					continue
				}
				tgt = &callTarget{gen: vm.gen, user: u, bi: b}
				fn.Cache[pc-1] = tgt
			}
			if tgt.user != nil {
				vm.push(vm.Call(tgt.user, args, nil, false))
			} else {
				vm.push(vm.callBuiltin(tgt.bi, args))
			}
		case compile.CALLX:
			argc := int(in.B)
			base := len(vm.stack) - argc
			args := make([]value.Value, argc)
			copy(args, vm.stack[base:])
			vm.stack = vm.stack[:base]
			callee := vm.pop().Deref()
			if callee.K == value.BLOCK {
				vm.push(vm.EvalBlock(callee, args...))
			} else {
				vm.push(vm.CallFunc(strings.TrimSpace(callee.S), args))
			}
		case compile.SEND:
			argc := int(in.B)
			base := len(vm.stack) - argc
			args := make([]value.Value, argc)
			copy(args, vm.stack[base:])
			vm.stack = vm.stack[:base]
			obj := vm.pop().Deref()
			vm.push(vm.send(obj, fn.Names[in.A], args))
		case compile.SENDSET:
			v := vm.pop().Deref()
			obj := vm.pop().Deref()
			vm.send(obj, "_"+fn.Names[in.A], []value.Value{v})
			vm.push(v)
		case compile.DO, compile.DOX:
			argc := int(in.B)
			base := len(vm.stack) - argc
			args := make([]value.Value, argc)
			copy(args, vm.stack[base:])
			vm.stack = vm.stack[:base]
			var name string
			if in.Op == compile.DO {
				name = fn.Names[in.A]
			} else {
				name = strings.ToUpper(strings.TrimSpace(vm.pop().Deref().S))
			}
			vm.Do(name, args)
		case compile.POP:
			vm.stack = vm.stack[:len(vm.stack)-1]
		case compile.DUP:
			vm.push(*vm.top())
		case compile.DUP2:
			n := len(vm.stack)
			vm.push(vm.stack[n-2])
			vm.push(vm.stack[n-1])
		case compile.ADD, compile.SUB, compile.MUL, compile.DIV, compile.MOD, compile.POW:
			b := vm.pop().Deref()
			a := vm.pop().Deref()
			vm.push(vm.arith(in.Op, a, b))
		case compile.NEG:
			a := vm.pop().Deref()
			if a.K != value.NUMBER {
				vm.push(vm.Raise(errArgCode(1080, "-", a)))
				continue
			}
			a.N = -a.N
			vm.push(a)
		case compile.NOT:
			a := vm.pop().Deref()
			if a.K != value.LOGICAL {
				vm.push(vm.Raise(errArgCode(1077, ".NOT.", a)))
				continue
			}
			vm.push(value.Bool(!a.B))
		case compile.EQ, compile.EXACTEQ, compile.NE, compile.LT, compile.LE, compile.GT, compile.GE:
			b := vm.pop().Deref()
			a := vm.pop().Deref()
			vm.push(vm.compare(in.Op, a, b))
		case compile.INSTR:
			b := vm.pop().Deref()
			a := vm.pop().Deref()
			if a.K == value.STRING && b.K == value.STRING {
				vm.push(value.Bool(strings.Contains(b.S, a.S)))
			} else if b.K == value.ARRAY {
				found := false
				for _, it := range b.Arr().Items {
					if vm.equalExact(a, it) {
						found = true
						break
					}
				}
				vm.push(value.Bool(found))
			} else if b.K == value.HASH {
				_, ok := b.Hsh().Get(a)
				vm.push(value.Bool(ok))
			} else {
				vm.push(vm.Raise(errArgCode(1109, "$", a, b)))
			}
		case compile.TOLOGIC:
			v := vm.top().Deref()
			if v.K != value.LOGICAL {
				*vm.top() = vm.Raise(errArgCode(1078, ".AND./.OR.", v))
			}
		case compile.JMP:
			pc = int(in.A)
		case compile.JF:
			v := vm.pop().Deref()
			if v.K != value.LOGICAL {
				v = vm.Raise(&RTError{SubSystem: "BASE", GenCode: 1, SubCode: 1066, Description: "Argument error: conditional", Args: []value.Value{v}})
			}
			if !v.B {
				pc = int(in.A)
			}
		case compile.JT:
			v := vm.pop().Deref()
			if v.K == value.LOGICAL && v.B {
				pc = int(in.A)
			}
		case compile.JFK:
			if v := vm.top(); v.K == value.LOGICAL && !v.B {
				pc = int(in.A)
			} else {
				vm.pop()
			}
		case compile.JTK:
			if v := vm.top(); v.K == value.LOGICAL && v.B {
				pc = int(in.A)
			} else {
				vm.pop()
			}
		case compile.RET:
			return vm.pop().Deref(), stReturn, 0
		case compile.RETNIL:
			return value.Nil, stReturn, 0
		case compile.SEQBEGIN:
			var with *value.Value
			if in.B == 2 {
				b := vm.pop().Deref()
				with = &b
			}
			r, st, npc := vm.runSeq(fr, pc, int(in.A), in.B == 1, with)
			if st == stReturn {
				return r, stReturn, 0
			}
			pc = npc
		case compile.SEQEND:
			return value.Nil, stSeqEnd, pc
		case compile.PRIVATE:
			vm.privateVar(fn.Names[in.A], value.Nil)
		case compile.PRIVATEX:
			name := strings.ToUpper(strings.TrimSpace(vm.pop().Deref().S))
			vm.privateVar(name, value.Nil)
		case compile.PUBLIC:
			name := fn.Names[in.A]
			vm.publicVar(name, value.False)
		case compile.PARAM:
			name := fn.Names[in.A]
			i := int(in.B)
			owner := vm.ownerFrame()
			var arg value.Value
			if owner != nil && i < len(owner.args) {
				arg = owner.args[i]
			}
			if arg.K == value.REF {
				ref := arg.P.(*value.Ref)
				vm.memvars[name] = append(vm.memvars[name], &memvar{ref: ref, owner: owner})
				if owner != nil {
					owner.privates = append(owner.privates, name)
				}
			} else {
				vm.privateVar(name, arg)
			}
		case compile.FORTEST:
			step := vm.pop().Deref()
			to := vm.pop().Deref()
			v := vm.pop().Deref()
			if step.K == value.NUMBER && step.N < 0 {
				vm.push(vm.compare(compile.GE, v, to))
			} else {
				vm.push(vm.compare(compile.LE, v, to))
			}
		case compile.FOREACHGET:
			idx := vm.pop().Deref()
			coll := vm.pop().Deref()
			i := idx.Int()
			switch coll.K {
			case value.ARRAY:
				items := coll.Arr().Items
				if i >= 1 && i <= len(items) {
					vm.push(items[i-1])
				} else {
					vm.push(value.Nil)
				}
			case value.HASH:
				h := coll.Hsh()
				if i >= 1 && i <= h.Len() {
					vm.push(h.Vals[i-1])
				} else {
					vm.push(value.Nil)
				}
			case value.STRING:
				if i >= 1 && i <= len(coll.S) {
					vm.push(value.Str(coll.S[i-1 : i]))
				} else {
					vm.push(value.Str(""))
				}
			default:
				vm.push(value.Nil)
			}
		default:
			panic(fmt.Sprintf("unknown opcode %v", in.Op))
		}
	}
	return value.Nil, stReturn, 0
}

// runSeq runs a BEGIN SEQUENCE / TRY body. with, if set, is the error
// block used while the body runs (BEGIN SEQUENCE WITH <block>).
func (vm *VM) runSeq(fr *Frame, pc, recoverPC int, try bool, with *value.Value) (r value.Value, st int, npc int) {
	if with != nil {
		old := vm.errorBlock
		vm.errorBlock = *with
		defer func() { vm.errorBlock = old }()
	}
	sp := len(vm.stack)
	nframes := len(vm.frames)
	aliasDepth := len(vm.aliasStack)
	var caught *value.Value
	func() {
		defer func() {
			if x := recover(); x != nil {
				switch e := x.(type) {
				case *BreakSignal:
					v := e.V
					caught = &v
				case *RTError:
					if try {
						v := value.Obj(e)
						caught = &v
						return
					}
					panic(x)
				default:
					panic(x)
				}
			}
		}()
		r, st, npc = vm.run(fr, pc)
	}()
	if caught != nil {
		if len(vm.stack) > sp {
			vm.stack = vm.stack[:sp]
		}
		if len(vm.frames) > nframes {
			vm.frames = vm.frames[:nframes]
		}
		if len(vm.aliasStack) > aliasDepth {
			vm.curArea = vm.aliasStack[aliasDepth]
			vm.aliasStack = vm.aliasStack[:aliasDepth]
		}
		vm.push(*caught)
		return value.Nil, stJump, recoverPC
	}
	if st == stSeqEnd {
		return value.Nil, stJump, npc
	}
	return r, st, npc
}

// ---------- arrays / objects ----------

func (vm *VM) index(arr, idx value.Value) value.Value {
	switch arr.K {
	case value.ARRAY:
		items := arr.Arr().Items
		if idx.K != value.NUMBER {
			return vm.Raise(errArgCode(1068, "array access", arr, idx))
		}
		i := idx.Int()
		if i < 1 || i > len(items) {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 2, SubCode: 1132, Description: "Bound error: array access", Args: []value.Value{idx}})
		}
		return items[i-1]
	case value.HASH:
		v, ok := arr.Hsh().Get(idx)
		if !ok {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 2, SubCode: 1132, Description: "Bound error: hash key not found", Args: []value.Value{idx}})
		}
		return v
	case value.STRING:
		// xHarbour string indexing
		i := idx.Int()
		if i >= 1 && i <= len(arr.S) {
			return value.Str(arr.S[i-1 : i])
		}
		return value.Str("")
	}
	return vm.Raise(errArgCode(1068, "array access", arr, idx))
}

func (vm *VM) setIndex(arr, idx, v value.Value) {
	switch arr.K {
	case value.ARRAY:
		items := arr.Arr().Items
		if idx.K != value.NUMBER {
			vm.Raise(errArgCode(1069, "array assign", arr, idx))
			return
		}
		i := idx.Int()
		if i < 1 || i > len(items) {
			vm.Raise(&RTError{SubSystem: "BASE", GenCode: 2, SubCode: 1133, Description: "Bound error: array assign", Args: []value.Value{idx}})
			return
		}
		items[i-1] = v
	case value.HASH:
		arr.Hsh().Set(idx, v)
	default:
		vm.Raise(errArgCode(1069, "array assign", arr, idx))
	}
}

// VMObject is implemented by objects that answer messages.
type VMObject interface {
	value.Object
	Send(vm *VM, msg string, args []value.Value) value.Value
}

func (vm *VM) send(obj value.Value, msg string, args []value.Value) value.Value {
	for i := range args {
		args[i] = args[i].Deref()
	}
	switch obj.K {
	case value.OBJECT:
		if o, ok := obj.P.(VMObject); ok {
			return o.Send(vm, msg, args)
		}
	case value.BLOCK:
		if msg == "EVAL" {
			return vm.EvalBlock(obj, args...)
		}
	case value.HASH:
		h := obj.Hsh()
		if strings.HasPrefix(msg, "_") && len(args) > 0 {
			h.Set(value.Str(msg[1:]), args[0])
			return args[0]
		}
		for i, k := range h.Keys {
			if k.K == value.STRING && strings.EqualFold(k.S, msg) {
				return h.Vals[i]
			}
		}
	}
	return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 5, SubCode: 1004, Description: "No exported method", Operation: msg, Args: []value.Value{obj}})
}
