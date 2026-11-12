// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nantucket/internal/compile"
	"nantucket/internal/lex"
	"nantucket/internal/parse"
	"nantucket/internal/pp"
	"nantucket/internal/value"
)

// ---------- macros ----------

func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c >= 0x80
}

// MacroSubst replaces &name and &name. with the values of character
// variables. If strict is set and the whole text is a single macro of a
// variable that is missing or not character, an error is raised.
func (vm *VM) MacroSubst(text string, strict bool) string {
	if !strings.Contains(text, "&") {
		return text
	}
	var sb strings.Builder
	for i := 0; i < len(text); {
		c := text[i]
		if c == '&' && i+1 < len(text) && isIdentByte(text[i+1]) && !(text[i+1] >= '0' && text[i+1] <= '9') {
			j := i + 1
			for j < len(text) && isIdentByte(text[j]) {
				j++
			}
			name := strings.ToUpper(text[i+1 : j])
			end := j
			if end < len(text) && text[end] == '.' {
				end++
			}
			v, ok := vm.lookupMacroVar(name)
			if ok && v.K == value.STRING {
				sb.WriteString(v.S)
				i = end
				continue
			}
			if strict && i == 0 && end == len(text) {
				if !ok {
					vm.Raise(errNoVar(name))
				} else {
					vm.Raise(errArgCode(1065, "&", v))
				}
			}
			sb.WriteString(text[i:j])
			i = j
			continue
		}
		sb.WriteByte(c)
		i++
	}
	return sb.String()
}

func (vm *VM) lookupMacroVar(name string) (value.Value, bool) {
	if r := vm.findMemvar(name); r != nil {
		return r.V.Deref(), true
	}
	if wa := vm.CurWA(); wa != nil {
		if i := wa.T.FieldIndex(name); i >= 0 {
			return wa.GetField(i), true
		}
	}
	return value.Nil, false
}

func (vm *VM) compileMacro(src string, assign bool) (*compile.Func, error) {
	key := src
	if assign {
		key = "=" + src
	}
	if f, ok := vm.macroCache[key]; ok {
		return f, nil
	}
	toks, err := lex.LexLine(src)
	if err != nil {
		return nil, err
	}
	e, err := parse.ParseExpr(toks)
	if err != nil {
		return nil, err
	}
	fn, err := compile.CompileMacro(e, src, assign)
	if err != nil {
		return nil, err
	}
	fn.IsBlock = true
	if len(vm.macroCache) > 5000 {
		vm.macroCache = map[string]*compile.Func{}
	}
	vm.macroCache[key] = fn
	return fn, nil
}

// MacroEval compiles and evaluates an expression string.
func (vm *VM) MacroEval(src string) value.Value {
	if strings.TrimSpace(src) == "" {
		return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 6, SubCode: 1449, Description: "Syntax error", Operation: "&"})
	}
	fn, err := vm.compileMacro(src, false)
	if err != nil {
		return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 6, SubCode: 1449, Description: "Syntax error", Operation: "&" + src})
	}
	return vm.Call(fn, nil, nil, true)
}

func (vm *VM) MacroAssign(src string, v value.Value) {
	fn, err := vm.compileMacro(strings.TrimSpace(src), true)
	if err != nil {
		vm.Raise(&RTError{SubSystem: "BASE", GenCode: 6, SubCode: 1449, Description: "Syntax error", Operation: "&" + src})
		return
	}
	vm.Call(fn, []value.Value{v}, nil, true)
}

// CompileBlockText compiles an expression string into a code block.
func (vm *VM) CompileBlockText(src string) (value.Value, error) {
	fn, err := vm.compileMacro(src, false)
	if err != nil {
		return value.Nil, err
	}
	return value.Value{K: value.BLOCK, P: &value.Block{Fn: fn, Src: "{||" + src + "}"}}, nil
}

// ---------- statement execution (dot prompt, &cmd) ----------

// CompileStatements preprocesses and compiles source as a fragment.
// Compiled fragments are cached per runtime: a fragment is linked into the
// runtime that compiled it, so sharing them between runtimes (the test
// runner, or several sessions in one process) ran one runtime's code
// against another's state.
func (vm *VM) CompileStatements(src string) (*compile.Func, error) {
	if f, ok := vm.fragCache[src]; ok {
		return f, nil
	}
	stmts, err := vm.PP.ProcessSource(src, "")
	if err != nil {
		return nil, err
	}
	body, err := parse.ParseFragment(stmts)
	if err != nil {
		return nil, err
	}
	fn, err := compile.CompileFragment(body, "(prompt)")
	if err != nil {
		return nil, err
	}
	vm.linkModule(fn.Module)
	if len(vm.fragCache) > 2000 || vm.fragCache == nil {
		vm.fragCache = map[string]*compile.Func{}
	}
	if !strings.Contains(src, "#") {
		vm.fragCache[src] = fn
	}
	return fn, nil
}

// ExecStatements runs source text transparently in the current procedure.
func (vm *VM) ExecStatements(src string) {
	fn, err := vm.CompileStatements(src)
	if err != nil {
		vm.Raise(&RTError{SubSystem: "BASE", GenCode: 6, SubCode: 1449, Description: "Syntax error", Operation: err.Error()})
		return
	}
	vm.Call(fn, nil, nil, true)
}

// ---------- modules ----------

func (vm *VM) linkModule(mod *compile.Module) {
	if mod.Loaded {
		return
	}
	mod.Loaded = true
	mod.StaticBase = len(vm.statics)
	for i := 0; i < mod.NStatics; i++ {
		vm.statics = append(vm.statics, &value.Ref{})
	}
	for _, f := range mod.Funcs {
		if f.Static {
			m := vm.modStatics[mod]
			if m == nil {
				m = map[string]*compile.Func{}
				vm.modStatics[mod] = m
			}
			m[f.Name] = f
		} else if f.Name != "(prompt)" {
			vm.funcs[f.Name] = f
		}
	}
	for _, f := range mod.Funcs {
		switch {
		case f.Init:
			vm.initProcs = append(vm.initProcs, f)
		case f.Exit:
			vm.exitProcs = append(vm.exitProcs, f)
		}
	}
	vm.gen++
	if mod.Init != nil && len(mod.Init.Code) > 1 {
		if mod.Init.Cache == nil {
			mod.Init.Cache = make([]any, len(mod.Init.Code))
		}
		vm.Call(mod.Init, nil, nil, true)
	}
}

// CompileSource compiles program text into a module (not yet linked).
func (vm *VM) CompileSource(src, file string) (*compile.Module, error) {
	p := vm.PP.Clone()
	dir := filepath.Dir(file)
	p.Include = func(name string) (string, string, error) {
		for _, cand := range []string{filepath.Join(dir, name), name} {
			if b, err := os.ReadFile(cand); err == nil {
				return string(b), cand, nil
			}
		}
		return vm.includeFile(name)
	}
	stmts, err := p.ProcessSource(src, file)
	if err != nil {
		return nil, err
	}
	f, err := parse.ParseFile(stmts, file)
	if err != nil {
		return nil, err
	}
	return compile.CompileFile(f)
}

func (vm *VM) includeFile(name string) (string, string, error) {
	if vm.Packaged {
		if src, ok := vm.EmbeddedHeaders[strings.ToUpper(filepath.Base(name))]; ok {
			return src, name, nil
		}
	}
	if p := vm.FindFile(name, ""); p != "" {
		b, err := os.ReadFile(p)
		if err == nil {
			return string(b), p, nil
		}
	}
	if src, ok := pp.Header(name); ok {
		return src, name, nil
	}
	return "", "", fmt.Errorf("include file not found: %s", name)
}

// LoadFile compiles and links a .prg file, reusing it if unchanged.
func (vm *VM) LoadFile(path string) (*compile.Module, error) {
	abs, _ := filepath.Abs(path)
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if me, ok := vm.modules[abs]; ok && me.mtime == st.ModTime().UnixNano() {
		return me.mod, nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mod, err := vm.CompileSource(string(src), path)
	if err != nil {
		return nil, err
	}
	vm.linkModule(mod)
	vm.modules[abs] = &modEntry{mod: mod, mtime: st.ModTime().UnixNano(), path: abs}
	return mod, nil
}

// LoadSource compiles and links source held in memory (packaged programs).
func (vm *VM) LoadSource(name, src string) (*compile.Module, error) {
	key := "embedded:" + strings.ToUpper(name)
	if me, ok := vm.modules[key]; ok {
		return me.mod, nil
	}
	mod, err := vm.CompileSource(src, name)
	if err != nil {
		return nil, err
	}
	vm.linkModule(mod)
	vm.modules[key] = &modEntry{mod: mod, path: key}
	return mod, nil
}

// FindFile searches the current directory, SET DEFAULT and SET PATH.
// If ext is given and name has no extension, it is added.
func (vm *VM) FindFile(name, ext string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if ext != "" && filepath.Ext(name) == "" {
		name += ext
	}
	var dirs []string
	if filepath.IsAbs(name) {
		dirs = []string{""}
	} else {
		dirs = []string{""}
		if d := vm.sets[7].S; d != "" {
			dirs = append([]string{d}, dirs...)
		}
		for _, p := range strings.FieldsFunc(vm.sets[6].S, func(r rune) bool { return r == ';' || r == ',' }) {
			dirs = append(dirs, strings.TrimSpace(p))
		}
	}
	for _, d := range dirs {
		base := filepath.Join(d, name)
		for _, cand := range caseVariants(base) {
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return cand
			}
		}
	}
	return ""
}

func caseVariants(p string) []string {
	dir, file := filepath.Split(p)
	return []string{p, filepath.Join(dir, strings.ToLower(file)), filepath.Join(dir, strings.ToUpper(file))}
}

// DefaultPath applies SET DEFAULT to a file being created.
func (vm *VM) DefaultPath(name, ext string) string {
	name = strings.TrimSpace(name)
	if ext != "" && filepath.Ext(name) == "" {
		name += ext
	}
	if !filepath.IsAbs(name) {
		if d := vm.sets[7].S; d != "" {
			name = filepath.Join(d, name)
		}
	}
	return name
}

// Do implements DO <proc> [WITH args].
func (vm *VM) Do(name string, args []value.Value) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if strings.HasSuffix(name, ".PRG") {
		name = strings.TrimSuffix(name, ".PRG")
	}
	var mod *compile.Module
	if fr := vm.curFrame(); fr != nil {
		mod = fr.fn.Module
	}
	if vm.Packaged {
		if src, ok := vm.Embedded[name]; ok {
			m, err := vm.LoadSource(name, src)
			if err != nil {
				vm.compileErr(err)
				return
			}
			vm.callMain(m, name, args)
			return
		}
	}
	if f, _ := vm.lookupFunc(mod, name); f != nil {
		// reload from disk if the source file changed
		if f.File != "" && strings.EqualFold(strings.TrimSuffix(filepath.Base(f.File), filepath.Ext(f.File)), name) {
			if p := vm.FindFile(f.File, ""); p != "" {
				if m, err := vm.LoadFile(p); err == nil {
					vm.callMain(m, name, args)
					return
				}
			}
		}
		vm.Call(f, args, nil, false)
		return
	}
	if p := vm.FindFile(name, ".prg"); p != "" {
		m, err := vm.LoadFile(p)
		if err != nil {
			vm.compileErr(err)
			return
		}
		vm.callMain(m, name, args)
		return
	}
	if src, ok := vm.Embedded[name]; ok {
		m, err := vm.LoadSource(name, src)
		if err != nil {
			vm.compileErr(err)
			return
		}
		vm.callMain(m, name, args)
		return
	}
	if _, b := vm.lookupFunc(nil, name); b != nil {
		vm.callBuiltin(b, args)
		return
	}
	vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2004, Description: "File does not exist", Filename: strings.ToLower(name) + ".prg"})
}

// runInitProcs runs the INIT PROCEDUREs of modules loaded since it last
// ran, as Clipper does before the main program starts.
func (vm *VM) runInitProcs() {
	for len(vm.initProcs) > 0 {
		f := vm.initProcs[0]
		vm.initProcs = vm.initProcs[1:]
		vm.Call(f, nil, nil, false)
	}
}

// RunExitProcs runs the EXIT PROCEDUREs once, when the program ends
// (including QUIT). An error in one is reported and the rest still run.
func (vm *VM) RunExitProcs() {
	for len(vm.exitProcs) > 0 {
		f := vm.exitProcs[0]
		vm.exitProcs = vm.exitProcs[1:]
		if err := vm.Protect(func() { vm.Call(f, nil, nil, false) }); err != nil {
			if _, quit := err.(*QuitSignal); !quit {
				vm.Println(ErrorReport(err))
			}
		}
	}
}

func (vm *VM) callMain(m *compile.Module, name string, args []value.Value) {
	vm.runInitProcs()
	if f := vm.funcs[name]; f != nil && f.Module == m {
		vm.Call(f, args, nil, false)
		return
	}
	if len(m.Funcs) > 0 {
		vm.Call(m.Funcs[0], args, nil, false)
	}
}

// CompileError wraps a compile-time error raised at run time.
type CompileError struct{ Err error }

func (c *CompileError) Error() string { return c.Err.Error() }

func (vm *VM) compileErr(err error) {
	vm.Raise(&RTError{SubSystem: "COMPILER", GenCode: 6, SubCode: 1, Description: "Compile error", Operation: err.Error()})
}

// RunFile runs a program file as the main program.
// RunEmbedded runs a packaged program (see "nantucket build").
func (vm *VM) RunEmbedded(name string, args []value.Value) error {
	name = strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	src, ok := vm.Embedded[name]
	if !ok {
		return fmt.Errorf("packaged program %s not found", name)
	}
	m, err := vm.LoadSource(name, src)
	if err != nil {
		return err
	}
	return vm.Protect(func() { vm.callMain(m, name, args) })
}

func (vm *VM) RunFile(path string, args []value.Value) error {
	m, err := vm.LoadFile(path)
	if err != nil {
		return err
	}
	return vm.Protect(func() {
		base := strings.ToUpper(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		vm.callMain(m, base, args)
	})
}

// Protect runs f, converting runtime panics into errors.
func (vm *VM) Protect(f func()) (err error) {
	saveFrames := len(vm.frames)
	saveStack := len(vm.stack)
	defer func() {
		if r := recover(); r != nil {
			vm.frames = vm.frames[:saveFrames]
			if len(vm.stack) > saveStack {
				vm.stack = vm.stack[:saveStack]
			}
			vm.aliasStack = nil
			vm.depth = len(vm.frames)
			switch e := r.(type) {
			case *RTError:
				err = e
			case *BreakSignal:
				err = nil
			case *QuitSignal:
				err = e
			case *CancelSignal:
				err = e
			default:
				err = fmt.Errorf("internal error: %v", r)
				if os.Getenv("NANTUCKET_DEBUG") != "" {
					panic(r)
				}
			}
		}
	}()
	f()
	return nil
}

func (q *QuitSignal) Error() string { return "QUIT" }
func (c *CancelSignal) Error() string {
	if c.Interrupted {
		return "*** INTERRUPTED ***"
	}
	return "CANCEL"
}

// Interrupt requests the running program to stop (Ctrl-C).
func (vm *VM) Interrupt() { vm.interrupt.Store(true) }

// Shutdown stops the program safely when Nantucket is told to end (SIGTERM,
// or SIGHUP when the terminal closes). It asks the program to stop at its
// next line, waits up to timeout for it to stop or be waiting for input,
// then writes all changed records and indexes and closes the tables. It
// returns false if the program didn't reach a safe point in time; tables
// are then left as they are (indexes that missed changes are rebuilt the
// next time they are opened). After Shutdown returns true the runtime
// must not be used again; the caller should exit.
func (vm *VM) Shutdown(timeout time.Duration) bool {
	vm.terminate.Store(true)
	deadline := time.Now().Add(timeout)
	for !vm.busy.TryLock() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	vm.closeFiles() // no EXIT PROCEDUREs: they might wait for input
	return true
}

// PushBaseFrame creates the persistent dot-prompt procedure level.
func (vm *VM) PushBaseFrame() {
	fn := &compile.Func{Name: "(prompt)", Module: &compile.Module{Name: "(prompt)"}}
	vm.frames = append(vm.frames, &Frame{fn: fn})
}

// ErrorReport formats an error with its call trace.
func ErrorReport(err error) string {
	if e, ok := err.(*RTError); ok {
		var sb strings.Builder
		sb.WriteString(e.Error())
		for _, t := range e.Trace {
			if strings.HasPrefix(t, "(prompt)") {
				continue
			}
			sb.WriteString("\nCalled from " + t)
		}
		return sb.String()
	}
	return err.Error()
}
