// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package debugger is Nantucket's full screen debugger. It opens over a
// running program when the user presses Alt-D, when the program calls
// AltD(), at a breakpoint, after a step, or at the first line with
// nantucket -debug. It shows the program's source at the current line, the
// call stack and the variables, and lets the user step through the
// program, set breakpoints, watch expressions and evaluate or change
// values.
package debugger

import (
	"fmt"
	"path/filepath"
	"strings"

	"nantucket/internal/term"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

func init() { vm.SetDebugHook(run) }

// session is what the debugger remembers between pauses.
type session struct {
	watches []string
	history []string
	focus   int // pane with the keyboard: paneSource, paneVars or paneStack
}

const (
	paneSource = iota
	paneVars
	paneStack
)

type sessionKey struct{}

func sess(m *vm.VM) *session {
	return m.Ext(sessionKey{}, func() any { return &session{} }).(*session)
}

// dbg is the debugger on screen during one pause.
type dbg struct {
	m      *vm.VM
	s      *session
	why    string
	frames []vm.DebugFrame
	sel    int // selected frame
	// source pane
	file     string
	lines    []string
	cursor   int // line with the cursor (1-based)
	srcTop   int // first line shown (1-based)
	varCur   int // selected row in the variables pane
	varTop   int
	stackTop int
	msg      string
	rows     []varRow
	rowsH    int // height of the lower panes
	srcH     int
	rowsN    int
	cols     int
	split    int // column of the line between variables and stack
	lowerTop int // first row of the lower panes
	cmdRow   int
	attrs    attrs
}

type attrs struct {
	text, bar, exec, sel term.Attr
}

// varRow is one line of the variables pane.
type varRow struct {
	name, scope string
	val         value.Value
	err         string
	watch       int    // watch number + 1, 0 for a variable
	wp          string // the expression of a watchpoint row
	shown       string // a watchpoint's value as last seen
	ref         *value.Ref
}

// run is the debug hook: it shows the debugger until the user lets the
// program go on.
func run(m *vm.VM, why string) {
	d := &dbg{m: m, s: sess(m), why: why}
	th := m.Theme()
	d.attrs = attrs{text: th.Edit, bar: th.EditStatus, exec: th.AlertHi, sel: th.Header}

	// draw on a window of our own when the program has windows open, so
	// the debugger isn't hidden under them
	w, _ := m.Con.(term.Windower)
	prevWin, ownWin := 0, 0
	if w != nil && len(w.WList()) > 0 {
		prevWin = w.WCurrent()
		w.WSelect(0, false)
		r, c := m.Con.Size()
		if id, err := w.WOpen(0, 0, r-1, c-1, term.WinOpts{Title: "Debugger", Fill: th.Edit, Frame: th.EditStatus, Controls: th.EditStatus}); err == nil {
			ownWin = id
		}
	}
	rows, cols := m.Con.Size()
	saveRow, saveCol := m.Con.Pos()
	saved := m.Con.Save(0, 0, rows-1, cols-1)
	defer func() {
		m.Con.Restore(0, 0, rows-1, cols-1, saved)
		m.Con.SetPos(saveRow, saveCol)
		if ownWin != 0 {
			w.WClose(ownWin)
		}
		if w != nil && ownWin != 0 {
			w.WSelect(prevWin, false)
		}
		m.Con.Flush()
	}()

	// only calls in program files: not the dot prompt or macros
	for _, f := range m.DebugFrames() {
		if f.Program() {
			d.frames = append(d.frames, f)
		}
	}
	// start at the innermost call on a line: not a code block run by a
	// command (REPLACE runs one), which has no line of its own
	for i, f := range d.frames {
		if f.Line > 0 {
			d.sel = i
			break
		}
	}
	d.showFrame()
	// a reason too long for the title bar is shown in full below
	if len([]rune(why))+40 > cols {
		d.msg = why
	}
	if notes := m.TakeDebugNotes(); len(notes) > 0 {
		d.msg = strings.Join(notes, "; ")
	}
	if n, last := m.UnseenLog(); n > 0 && d.msg == "" {
		d.msg = fmt.Sprintf("%d new log %s (F3), last: %s", n, plural(n, "line"), last)
	}
	for {
		d.layout()
		d.draw()
		k, ok := m.Con.ReadKey(-1)
		if !ok {
			return
		}
		if d.key(k, saved, rows, cols) {
			return
		}
	}
}

// showFrame loads the selected frame's source and puts the cursor on its
// current line.
func (d *dbg) showFrame() {
	if d.sel < 0 || d.sel >= len(d.frames) {
		return
	}
	f := d.frames[d.sel]
	if f.Program() {
		d.file = f.File
		d.lines = d.m.SourceLines(f.File)
		d.cursor = max(f.Line, 1)
		d.srcTop = 0
	}
	d.varCur, d.varTop = 0, 0
}

func (d *dbg) layout() {
	rows, cols := d.m.Con.Size()
	d.rowsN, d.cols = rows, cols
	d.cmdRow = rows - 2
	avail := rows - 4 // title, divider, command line, keys
	d.srcH = max(avail*11/20, 3)
	d.rowsH = max(avail-d.srcH, 2)
	d.lowerTop = 1 + d.srcH + 1
	d.split = cols * 2 / 3
}

// ---------- drawing ----------

func (d *dbg) put(r, c int, s string, w int, a term.Attr) {
	if w <= 0 {
		return
	}
	d.m.Con.Put(r, c, vm.PadTo(clip(s, w), w, 'R', " "), a)
}

func clip(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s
}

func (d *dbg) draw() {
	a := d.attrs
	cols := d.cols
	where := ""
	if d.sel < len(d.frames) {
		f := d.frames[d.sel]
		where = fmt.Sprintf("%s  %s(%d)", f.Proc, filepath.Base(f.File), f.Line)
	}
	d.put(0, 0, " Debugger   "+where+"   "+d.why, cols, a.bar)
	d.drawSource()
	// divider with pane titles
	div := []rune(strings.Repeat("─", cols))
	d.m.Con.Put(1+d.srcH, 0, string(div), a.text)
	d.title(1+d.srcH, 1, " Variables ", d.s.focus == paneVars)
	d.m.Con.Put(1+d.srcH, d.split, "┬", a.text)
	d.title(1+d.srcH, d.split+2, " Call stack ", d.s.focus == paneStack)
	d.drawVars()
	d.drawStack()
	msg := d.msg
	if b, ok := d.m.BreakAt(d.file, d.cursor); msg == "" && ok && (b.Rule() != "" || b.Hits > 0) && d.s.focus == paneSource {
		kind := "Breakpoint"
		if b.Log != "" {
			kind = "Logpoint"
		}
		msg = fmt.Sprintf("%s at line %d %s", kind, d.cursor, b.Rule())
		if b.Hit.Op != 0 || b.Hits > 0 {
			word := "hits"
			if b.Hits == 1 {
				word = "hit"
			}
			msg += fmt.Sprintf(" (%d %s so far)", b.Hits, word)
		}
	}
	if msg == "" {
		msg = "Type an expression to see it, x := 5 to change x, WATCH <expr>, BP <line>"
	}
	d.put(d.cmdRow, 0, "> "+msg, cols, a.text)
	d.put(d.rowsN-1, 0, " F5 Go  F8 Step  F10 Over  F6 Out  F9 Break  F3 Log  F4 Screen  Tab  F1 Help", cols, a.bar)
	// cursor
	switch d.s.focus {
	case paneSource:
		d.m.Con.SetPos(1+d.cursor-d.srcTop, 0)
	case paneVars:
		d.m.Con.SetPos(d.lowerTop+d.varCur-d.varTop, 0)
	case paneStack:
		d.m.Con.SetPos(d.lowerTop+d.sel-d.stackTop, d.split+1)
	}
}

func (d *dbg) title(r, c int, s string, focused bool) {
	a := d.attrs.text
	if focused {
		a = d.attrs.sel
	}
	d.m.Con.Put(r, c, s, a)
}

func (d *dbg) drawSource() {
	a := d.attrs
	h := d.srcH
	n := len(d.lines)
	if d.srcTop == 0 { // centre the current line the first time
		d.srcTop = max(d.cursor-h/2, 1)
	}
	if d.cursor < d.srcTop {
		d.srcTop = d.cursor
	}
	if d.cursor >= d.srcTop+h {
		d.srcTop = d.cursor - h + 1
	}
	execLine := 0
	if d.sel < len(d.frames) && d.frames[d.sel].File == d.file {
		execLine = d.frames[d.sel].Line
	}
	for i := 0; i < h; i++ {
		ln := d.srcTop + i
		row := 1 + i
		if ln > n || d.file == "" {
			d.put(row, 0, "", d.cols, a.text)
			if i == 0 && d.file == "" {
				d.put(row, 0, "  (no source for this call)", d.cols, a.text)
			}
			continue
		}
		mark := " "
		if b, ok := d.m.BreakAt(d.file, ln); ok {
			switch {
			case b.Log != "":
				mark = "◇"
			case b.Rule() != "":
				mark = "◆"
			default:
				mark = "●"
			}
		}
		if ln == execLine {
			mark += "►"
		} else {
			mark += " "
		}
		text := strings.ReplaceAll(d.lines[ln-1], "\t", "    ")
		at := a.text
		if ln == execLine {
			at = a.exec
		} else if ln == d.cursor && d.s.focus == paneSource {
			at = a.sel
		}
		d.put(row, 0, fmt.Sprintf("%s%4d  %s", mark, ln, text), d.cols, at)
	}
}

// buildRows lists the watches, then the selected frame's locals and
// statics, then the visible PRIVATE and PUBLIC variables.
func (d *dbg) buildRows() {
	d.rows = d.rows[:0]
	var f vm.DebugFrame
	if d.sel < len(d.frames) {
		f = d.frames[d.sel]
	}
	for i, w := range d.s.watches {
		r := varRow{name: w, scope: "WATCH", watch: i + 1}
		v, err := d.m.DebugEval(f, w)
		if err != nil {
			r.err = err.Error()
		} else {
			r.val = v
		}
		d.rows = append(d.rows, r)
	}
	for _, w := range d.m.Watchpoints() {
		d.rows = append(d.rows, varRow{name: w.String(), scope: "WP", wp: w.Expr, shown: w.Value})
	}
	for _, v := range d.m.DebugLocals(f) {
		d.rows = append(d.rows, varRow{name: v.Name, scope: v.Scope, val: v.Ref.V, ref: v.Ref})
	}
	for _, v := range d.m.DebugMemvars() {
		d.rows = append(d.rows, varRow{name: v.Name, scope: v.Scope, val: v.Ref.V, ref: v.Ref})
	}
}

func (d *dbg) drawVars() {
	a := d.attrs
	d.buildRows()
	h := d.rowsH
	w := d.split
	d.varCur = min(max(d.varCur, 0), max(len(d.rows)-1, 0))
	if d.varCur < d.varTop {
		d.varTop = d.varCur
	}
	if d.varCur >= d.varTop+h {
		d.varTop = d.varCur - h + 1
	}
	nameW := 12
	for _, r := range d.rows {
		if r.watch == 0 && r.wp == "" {
			nameW = max(nameW, len(r.name))
		}
	}
	nameW = min(nameW, w/3)
	for i := 0; i < h; i++ {
		row := d.lowerTop + i
		k := d.varTop + i
		if k >= len(d.rows) {
			d.put(row, 0, "", w, a.text)
			if k == 0 {
				d.put(row, 0, "  (no variables)", w, a.text)
			}
			continue
		}
		r := d.rows[k]
		val := ""
		if r.err != "" {
			val = "Error: " + r.err
		} else {
			val = Show(d.m, r.val, w)
		}
		line := fmt.Sprintf(" %-*s %-7s %s", nameW, clip(r.name, nameW), r.scope, val)
		switch {
		case r.watch > 0:
			line = fmt.Sprintf(" %s = %s", r.name, val)
		case r.wp != "":
			line = fmt.Sprintf(" ◉ %s = %s", r.name, r.shown)
		}
		at := a.text
		if k == d.varCur && d.s.focus == paneVars {
			at = a.sel
		}
		d.put(row, 0, line, w, at)
	}
}

func (d *dbg) drawStack() {
	a := d.attrs
	h := d.rowsH
	x := d.split + 1
	w := d.cols - x
	if d.sel < d.stackTop {
		d.stackTop = d.sel
	}
	if d.sel >= d.stackTop+h {
		d.stackTop = d.sel - h + 1
	}
	for i := 0; i < h; i++ {
		row := d.lowerTop + i
		d.m.Con.Put(row, d.split, "│", a.text)
		k := d.stackTop + i
		if k >= len(d.frames) {
			d.put(row, x, "", w, a.text)
			continue
		}
		f := d.frames[k]
		s := fmt.Sprintf(" %s(%d)", f.Proc, f.Line)
		if f.Program() {
			s += "  " + filepath.Base(f.File)
		}
		at := a.text
		if k == d.sel {
			at = a.exec
			if d.s.focus == paneStack {
				at = a.sel
			}
		}
		d.put(row, x, s, w, at)
	}
}

// Show formats a value for the debugger, within about w characters.
func Show(m *vm.VM, v value.Value, w int) string {
	return clip(show(m, v, 2), max(w, 10))
}

func show(m *vm.VM, v value.Value, depth int) string {
	v = v.Deref()
	switch v.K {
	case value.NIL:
		return "NIL"
	case value.STRING:
		q := `"`
		if strings.Contains(v.S, `"`) {
			q = "'"
		}
		return q + v.S + q
	case value.NUMBER:
		return strings.TrimSpace(m.Display(v))
	case value.LOGICAL:
		if v.B {
			return ".T."
		}
		return ".F."
	case value.DATE:
		return m.DTOC(v.J)
	case value.ARRAY:
		arr := v.Arr()
		if arr == nil {
			return "{}"
		}
		if depth <= 0 {
			return fmt.Sprintf("{...} (%d)", len(arr.Items))
		}
		var parts []string
		for i, it := range arr.Items {
			if i == 20 {
				parts = append(parts, "...")
				break
			}
			parts = append(parts, show(m, it, depth-1))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case value.HASH:
		h := v.Hsh()
		if h == nil {
			return "{=>}"
		}
		if depth <= 0 {
			return fmt.Sprintf("{=>...} (%d)", len(h.Keys))
		}
		var parts []string
		for i := range h.Keys {
			if i == 20 {
				parts = append(parts, "...")
				break
			}
			parts = append(parts, show(m, h.Keys[i], depth-1)+" => "+show(m, h.Vals[i], depth-1))
		}
		if len(parts) == 0 {
			return "{=>}"
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case value.BLOCK:
		if b := v.Blk(); b != nil && b.Src != "" {
			return b.Src
		}
		return "{|| ... }"
	}
	return m.Display(v)
}

// ---------- keys ----------

// key handles a key; it reports true when the program should go on.
func (d *dbg) key(k int, saved []term.Cell, rows, cols int) bool {
	m := d.m
	d.msg = ""
	switch k {
	case term.KF5:
		return true
	case term.KF8:
		m.DebugStep(vm.StepInto)
		return true
	case term.KF10:
		m.DebugStep(vm.StepOver)
		return true
	case term.KF6:
		m.DebugStep(vm.StepOut)
		return true
	case term.KF7:
		if d.file == "" {
			d.msg = "No source here"
			return false
		}
		m.DebugRunTo(d.file, d.cursor)
		return true
	case term.KF9:
		if d.s.focus == paneVars {
			d.toggleWatchpoint()
			return false
		}
		if d.file != "" {
			if m.ToggleBreak(d.file, d.cursor) {
				d.msg = fmt.Sprintf("Breakpoint set at %s(%d); BP IF <condition> makes it conditional", filepath.Base(d.file), d.cursor)
			} else {
				d.msg = fmt.Sprintf("Breakpoint cleared at %s(%d)", filepath.Base(d.file), d.cursor)
			}
		}
		return false
	case term.KF3:
		d.showLog()
		return false
	case term.KF4:
		m.Con.Restore(0, 0, rows-1, cols-1, saved)
		m.Con.Flush()
		m.Con.ReadKey(-1)
		return false
	case term.KF1:
		d.help()
		return false
	case term.KTab:
		d.s.focus = (d.s.focus + 1) % 3
		return false
	case term.KShTab:
		d.s.focus = (d.s.focus + 2) % 3
		return false
	case term.KDebug:
		return false
	}
	switch d.s.focus {
	case paneSource:
		if d.sourceKey(k) {
			return false
		}
	case paneVars:
		if d.varsKey(k) {
			return false
		}
	case paneStack:
		if d.stackKey(k) {
			return false
		}
	}
	if k >= 32 && k < 256 {
		m.Con.PushKeys([]int{k})
		return d.command("")
	}
	if k == term.KEnter {
		return d.command("")
	}
	return false
}

func (d *dbg) sourceKey(k int) bool {
	n := len(d.lines)
	switch k {
	case term.KUp:
		d.cursor = max(d.cursor-1, 1)
	case term.KDown:
		d.cursor = min(d.cursor+1, max(n, 1))
	case term.KPgUp:
		d.cursor = max(d.cursor-d.srcH, 1)
	case term.KPgDn:
		d.cursor = min(d.cursor+d.srcH, max(n, 1))
	case term.KCtrlPgUp, term.KCtrlHome:
		d.cursor = 1
	case term.KCtrlPgDn, term.KCtrlEnd:
		d.cursor = max(n, 1)
	case term.KHome:
		// back to the line the program is on
		if d.sel < len(d.frames) {
			d.cursor = max(d.frames[d.sel].Line, 1)
		}
	default:
		return false
	}
	return true
}

func (d *dbg) varsKey(k int) bool {
	switch k {
	case term.KUp:
		d.varCur--
	case term.KDown:
		d.varCur++
	case term.KPgUp:
		d.varCur -= d.rowsH
	case term.KPgDn:
		d.varCur += d.rowsH
	case term.KHome, term.KCtrlPgUp:
		d.varCur = 0
	case term.KEnd, term.KCtrlPgDn:
		d.varCur = len(d.rows) - 1
	case term.KDel:
		if d.varCur < len(d.rows) && d.rows[d.varCur].watch > 0 {
			i := d.rows[d.varCur].watch - 1
			d.s.watches = append(d.s.watches[:i], d.s.watches[i+1:]...)
		} else if d.varCur < len(d.rows) && d.rows[d.varCur].wp != "" {
			d.m.ClearWatchpoint(d.rows[d.varCur].wp)
			d.msg = "Watchpoint on " + d.rows[d.varCur].wp + " removed"
		}
	case term.KEnter:
		if d.varCur >= len(d.rows) {
			return true
		}
		r := d.rows[d.varCur]
		v := r.val.Deref()
		if r.err == "" && (v.K == value.ARRAY || v.K == value.HASH) {
			d.inspect(r.name, v)
			return true
		}
		if r.wp != "" {
			d.command(r.wp)
			return true
		}
		if r.watch > 0 {
			d.command(r.name)
			return true
		}
		d.command(r.name + " := " + literal(d.m, v))
	default:
		return false
	}
	return true
}

func (d *dbg) stackKey(k int) bool {
	old := d.sel
	switch k {
	case term.KUp:
		d.sel = max(d.sel-1, 0)
	case term.KDown:
		d.sel = min(d.sel+1, len(d.frames)-1)
	case term.KHome:
		d.sel = 0
	case term.KEnd:
		d.sel = len(d.frames) - 1
	case term.KEnter:
		d.s.focus = paneSource
	default:
		return false
	}
	if d.sel != old {
		d.showFrame()
	}
	return true
}

// literal writes a value the way it would be typed, for editing it.
func literal(m *vm.VM, v value.Value) string {
	switch v.K {
	case value.DATE:
		return "SToD(\"" + value.DTOS(v.J) + "\")"
	case value.BLOCK, value.OBJECT:
		return ""
	}
	return show(m, v, 0)
}

// ---------- the command line ----------

// command reads a command on the command line, starting with initial,
// and runs it. It reports true when the program should go on.
func (d *dbg) command(initial string) bool {
	m := d.m
	d.put(d.cmdRow, 0, "> ", d.cols, d.attrs.text)
	m.Con.SetPos(d.cmdRow, 2)
	s, ok := term.ReadLine(m.Con, d.attrs.sel, initial, 0, d.s.history)
	s = strings.TrimSpace(s)
	if !ok || s == "" {
		return false
	}
	if n := len(d.s.history); n == 0 || d.s.history[n-1] != s {
		d.s.history = append(d.s.history, s)
		if len(d.s.history) > 100 {
			d.s.history = d.s.history[1:]
		}
	}
	word, rest := s, ""
	if i := strings.IndexAny(s, " \t"); i > 0 {
		word, rest = s[:i], strings.TrimSpace(s[i+1:])
	}
	if looksLikeExpr(rest) {
		word = "" // "w := 5" changes w; it isn't WATCH
	}
	switch strings.ToUpper(word) {
	case "GO", "G":
		if rest == "" {
			return true
		}
	case "STEP":
		if rest == "" {
			m.DebugStep(vm.StepInto)
			return true
		}
	case "QUIT", "CANCEL":
		if rest == "" {
			m.CancelProgram()
		}
	case "WATCH", "W":
		if rest != "" {
			if _, err := m.CompileBlockText(rest); err != nil {
				d.msg = "Not an expression: " + rest
				return false
			}
			d.s.watches = append(d.s.watches, rest)
			d.msg = "Watching " + rest
			return false
		}
	case "UNWATCH":
		d.s.watches = nil
		d.msg = "Watches cleared"
		return false
	case "LP":
		d.logpoint(rest)
		return false
	case "LOG":
		switch strings.ToUpper(rest) {
		case "":
			d.showLog()
			return false
		case "CLEAR":
			m.ClearDebugLog()
			d.msg = "Log cleared"
			return false
		}
	case "LOGFILE":
		path, err := m.SetDebugLogFile(rest)
		switch {
		case err != nil:
			d.msg = "Error: " + err.Error()
		case path == "":
			d.msg = "The log is no longer written to a file"
		default:
			d.msg = "The log is also written to " + path
		}
		return false
	case "BP":
		d.breakpoint(rest)
		return false
	case "WP":
		d.watchpoint(rest)
		return false
	case "DB":
		d.dataBreak(rest)
		return false
	case "BC":
		if rest == "" {
			m.ClearBreaks()
			m.ClearWatchpoints()
			m.ClearDataBreaks()
			d.msg = "Breakpoints and watchpoints cleared"
			return false
		}
	case "BL":
		if rest == "" {
			var list []string
			for _, b := range m.Breaks() {
				list = append(list, b.String())
			}
			for _, w := range m.Watchpoints() {
				list = append(list, "WP "+w.String())
			}
			for _, b := range m.DataBreaks() {
				list = append(list, "DB "+b.String())
			}
			if len(list) == 0 {
				d.msg = "No breakpoints"
			} else {
				d.msg = "Breakpoints: " + strings.Join(list, ", ")
			}
			return false
		}
	case "?", "??":
		s = rest
	}
	if strings.HasPrefix(s, "?") {
		s = strings.TrimSpace(strings.TrimLeft(s, "?"))
	}
	var f vm.DebugFrame
	if d.sel < len(d.frames) {
		f = d.frames[d.sel]
	}
	v, err := m.DebugEval(f, s)
	if err != nil {
		d.msg = "Error: " + err.Error()
		return false
	}
	d.msg = s + " = " + Show(m, v, d.cols)
	return false
}

// watchpoint handles WP <expr>: stop when its value changes, or stop
// watching it if it is watched already. With no expression it uses the
// variable selected in the variables pane.
func (d *dbg) watchpoint(expr string) {
	if expr == "" {
		d.toggleWatchpoint()
		return
	}
	if d.m.ClearWatchpoint(expr) {
		d.msg = "Watchpoint on " + expr + " removed"
		return
	}
	var f vm.DebugFrame
	if d.sel < len(d.frames) {
		f = d.frames[d.sel]
	}
	if err := d.m.SetWatchpoint(f, expr); err != nil {
		d.msg = "Error: " + err.Error()
		return
	}
	d.msg = "The program will stop when " + expr + " changes"
	for _, w := range d.m.Watchpoints() {
		if strings.EqualFold(w.Expr, expr) && w.Proc != "" {
			d.msg += " (until " + w.Proc + "() returns)"
		}
	}
}

// toggleWatchpoint sets or clears a watchpoint on the selected row of the
// variables pane (F9 there).
func (d *dbg) toggleWatchpoint() {
	if d.varCur >= len(d.rows) {
		d.msg = "Select a variable to watch for changes"
		return
	}
	r := d.rows[d.varCur]
	if r.wp != "" {
		d.watchpoint(r.wp)
		return
	}
	d.watchpoint(r.name)
}

// wordAt reports whether s has the keyword word (in any case) at i, with
// space or the end of s on either side.
func wordAt(s string, i int, word string) bool {
	if i+len(word) > len(s) || !strings.EqualFold(s[i:i+len(word)], word) {
		return false
	}
	before := i == 0 || s[i-1] == ' ' || s[i-1] == '\t'
	after := i+len(word) == len(s) || s[i+len(word)] == ' ' || s[i+len(word)] == '\t'
	return before && after
}

// splitIf splits "<target> IF <condition>" at the first IF.
func splitIf(s string) (target, cond string, hasIf bool) {
	for i := 0; i < len(s); i++ {
		if wordAt(s, i, "IF") {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+2:]), true
		}
	}
	return strings.TrimSpace(s), "", false
}

// splitHit splits "<rest> HIT <count>" at the last HIT that is followed
// by a hit count, so that a condition can still contain the word.
func splitHit(s string) (rest string, hit vm.HitCount, hasHit bool, err error) {
	for i := len(s) - 3; i >= 0; i-- {
		if !wordAt(s, i, "HIT") {
			continue
		}
		h, e := vm.ParseHitCount(s[i+3:])
		if e != nil {
			if err == nil {
				err = e
			}
			continue
		}
		return strings.TrimSpace(s[:i]), h, true, nil
	}
	return strings.TrimSpace(s), vm.HitCount{}, false, err
}

// parseBP reads BP's argument: [<line>|<name>] [IF <condition>] [HIT
// <count>], with IF and HIT in either order.
func parseBP(arg string) (target, cond string, hit vm.HitCount, rule bool, err error) {
	rest, hit, hasHit, herr := splitHit(arg)
	target, cond, hasIf := splitIf(rest)
	if !hasHit {
		// HIT before IF: "BP 5 HIT 3 IF x > 1"
		if target, hit, hasHit, herr = splitHit(target); hasHit {
			herr = nil
		}
	}
	if !hasHit && herr != nil {
		// a HIT that isn't followed by a count is a mistake, unless it is
		// part of the condition (a variable called hit)
		for i := 0; i < len(target); i++ {
			if wordAt(target, i, "HIT") {
				return "", "", hit, false, herr
			}
		}
	}
	if hasIf && cond == "" {
		return "", "", hit, false, fmt.Errorf("BP ... IF needs a condition, such as BP 12 IF nTotal > 100")
	}
	return target, cond, hit, hasIf || hasHit, nil
}

// breakpoint handles BP [<line>|<name>] [IF <condition>] [HIT <count>]: a
// line of the file shown (the cursor's when no line is given) or a
// procedure. Without IF or HIT it sets or clears the breakpoint; with
// them it sets one, replacing any already there.
func (d *dbg) breakpoint(arg string) {
	m := d.m
	target, cond, hit, rule, err := parseBP(arg)
	if err != nil {
		d.msg = "Error: " + err.Error()
		return
	}
	if target == "" {
		target = fmt.Sprint(d.cursor)
	}
	desc := vm.Breakpoint{Cond: cond, Hit: hit}.Rule()
	if desc != "" {
		desc = " " + desc
	}
	var n int
	if _, err := fmt.Sscan(target, &n); err == nil && fmt.Sprint(n) == target {
		if d.file == "" {
			d.msg = "No source here"
			return
		}
		where := fmt.Sprintf("%s(%d)", filepath.Base(d.file), n)
		switch {
		case rule:
			if err := m.SetBreak(d.file, n, cond, hit); err != nil {
				d.msg = "Error: " + err.Error()
				return
			}
			d.msg = "Breakpoint set at " + where + desc
		case m.ToggleBreak(d.file, n):
			d.msg = "Breakpoint set at " + where
		default:
			d.msg = "Breakpoint cleared at " + where
		}
		return
	}
	name := strings.ToUpper(strings.TrimSuffix(target, "()"))
	switch {
	case rule:
		if err := m.SetProcBreak(name, cond, hit); err != nil {
			d.msg = "Error: " + err.Error()
			return
		}
		d.msg = "Breakpoint set at " + name + "()" + desc
	case m.ToggleProcBreak(name):
		d.msg = "Breakpoint set at " + name + "()"
	default:
		d.msg = "Breakpoint cleared at " + name + "()"
	}
}

// looksLikeExpr reports whether what follows a command word makes the
// whole line an expression instead: an operator, as in "w := 5" or
// "db + 1".
func looksLikeExpr(rest string) bool {
	if rest == "" {
		return false
	}
	for _, op := range []string{":=", "+=", "-=", "*=", "/=", "==", "!=", "<>", "<=", ">=", "->", "++", "--"} {
		if strings.HasPrefix(rest, op) {
			return true
		}
	}
	return strings.ContainsRune("=+*/<>^$%[", rune(rest[0]))
}

// dataBreak handles DB <alias->field | field | alias->*> [IF c] [HIT n]:
// stop when the field changes, in any record. Without IF or HIT it sets
// or clears the data breakpoint.
func (d *dbg) dataBreak(arg string) {
	m := d.m
	spec, cond, hit, rule, err := parseBP(arg)
	if err != nil {
		d.msg = "Error: " + err.Error()
		return
	}
	if spec == "" {
		d.msg = "DB needs a field: DB customer->balance, DB balance or DB customer->*"
		return
	}
	if !rule && m.ClearDataBreak(spec) {
		d.msg = "Data breakpoint on " + strings.ToUpper(spec) + " cleared"
		return
	}
	name, err := m.SetDataBreak(spec, cond, hit)
	if err != nil {
		d.msg = "Error: " + err.Error()
		return
	}
	d.msg = "The program will stop when " + name + " changes"
	if r := (vm.Breakpoint{Cond: cond, Hit: hit}).Rule(); r != "" {
		d.msg += " " + r
	}
}

// ---------- logpoints ----------

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func isQuote(c byte) bool { return c == '"' || c == '\'' }

// parseLP reads LP's argument: [<line>|<name>] ["<message>" [IF <cond>]
// [HIT <count>]]. The message needs quotes when IF or HIT follow it;
// without them it runs to the end, so it can say "if". No message means
// clear the logpoint.
func parseLP(arg string) (target, msg, cond string, hit vm.HitCount, err error) {
	rest := strings.TrimSpace(arg)
	if rest != "" && !isQuote(rest[0]) {
		w, after := rest, ""
		if i := strings.IndexAny(rest, " \t"); i > 0 {
			w, after = rest[:i], strings.TrimSpace(rest[i+1:])
		}
		digits := strings.Trim(w, "0123456789") == ""
		if digits || strings.HasSuffix(w, "()") || after == "" || isQuote(after[0]) {
			target, rest = w, after
		}
	}
	if rest == "" {
		return target, "", "", hit, nil
	}
	if !isQuote(rest[0]) {
		return target, rest, "", hit, nil
	}
	end := strings.IndexByte(rest[1:], rest[0])
	if end < 0 {
		return "", "", "", hit, fmt.Errorf("the message has no closing %c", rest[0])
	}
	msg = rest[1 : 1+end]
	extra, cond, hit, _, err := parseBP(rest[2+end:])
	if err != nil {
		return "", "", "", hit, err
	}
	if extra != "" {
		return "", "", "", hit, fmt.Errorf("after the message, only IF <condition> and HIT <count> can follow")
	}
	return target, msg, cond, hit, nil
}

// logpoint handles LP: set a logpoint, or clear one when there is no
// message.
func (d *dbg) logpoint(arg string) {
	m := d.m
	target, msg, cond, hit, err := parseLP(arg)
	if err != nil {
		d.msg = "Error: " + err.Error()
		return
	}
	if target == "" {
		target = fmt.Sprint(d.cursor)
	}
	desc := vm.Breakpoint{Log: msg, Cond: cond, Hit: hit}.Rule()
	if strings.Trim(target, "0123456789") == "" {
		var line int
		fmt.Sscan(target, &line)
		if d.file == "" {
			d.msg = "No source here"
			return
		}
		where := fmt.Sprintf("%s(%d)", filepath.Base(d.file), line)
		if msg == "" {
			if m.ClearBreak(d.file, line) {
				d.msg = "Cleared " + where
			} else {
				d.msg = "Nothing to clear at " + where
			}
			return
		}
		if err := m.SetLogpoint(d.file, line, msg, cond, hit); err != nil {
			d.msg = "Error: " + err.Error()
			return
		}
		d.msg = "Logpoint set at " + where + " " + desc
		return
	}
	name := strings.ToUpper(strings.TrimSuffix(target, "()"))
	if msg == "" {
		if m.ClearProcBreak(name) {
			d.msg = "Cleared " + name + "()"
		} else {
			d.msg = "Nothing to clear at " + name + "()"
		}
		return
	}
	if err := m.SetProcLogpoint(name, msg, cond, hit); err != nil {
		d.msg = "Error: " + err.Error()
		return
	}
	d.msg = "Logpoint set at " + name + "() " + desc
}

// showLog shows the log, at its end.
func (d *dbg) showLog() {
	lines, unseen := d.m.DebugLog()
	title := fmt.Sprintf("Log  (%d %s", len(lines), plural(len(lines), "line"))
	if unseen > 0 {
		title += fmt.Sprintf(", %d new", unseen)
	}
	if f := d.m.DebugLogFile(); f != "" {
		title += ", also in " + filepath.Base(f)
	}
	title += ")  Esc close"
	if len(lines) == 0 {
		lines = []string{"Nothing logged yet. LP sets a logpoint: LP 12 \"total is {nTotal}\""}
	}
	d.boxAt(title, lines, true, len(lines)-1)
}

// ---------- pop-ups ----------

// box draws a framed box in the middle of the screen with lines in it,
// and waits for a key; it returns the key and the selected line.
func (d *dbg) box(title string, lines []string, choose bool) (int, int) {
	return d.boxAt(title, lines, choose, 0)
}

// boxAt is box with the selection (or the view) starting at line start.
func (d *dbg) boxAt(title string, lines []string, choose bool, start int) (int, int) {
	m := d.m
	w := len([]rune(title)) + 4
	for _, l := range lines {
		w = max(w, len([]rune(l))+4)
	}
	w = min(w, d.cols-2)
	h := min(len(lines)+2, d.rowsN-2)
	top, left := (d.rowsN-h)/2, (d.cols-w)/2
	saved := m.Con.Save(top, left, top+h-1, left+w-1)
	defer m.Con.Restore(top, left, top+h-1, left+w-1, saved)
	a := d.attrs
	cur, first := min(max(start, 0), max(len(lines)-1, 0)), 0
	inner := h - 2
	for {
		m.Con.Put(top, left, "┌"+strings.Repeat("─", w-2)+"┐", a.bar)
		m.Con.Put(top, left+2, clip(" "+title+" ", w-4), a.bar)
		if cur < first {
			first = cur
		}
		if cur >= first+inner {
			first = cur - inner + 1
		}
		for i := 0; i < inner; i++ {
			k := first + i
			s := ""
			if k < len(lines) {
				s = lines[k]
			}
			at := a.text
			if choose && k == cur {
				at = a.sel
			}
			m.Con.Put(top+1+i, left, "│", a.bar)
			d.put(top+1+i, left+1, " "+s, w-2, at)
			m.Con.Put(top+1+i, left+w-1, "│", a.bar)
		}
		m.Con.Put(top+h-1, left, "└"+strings.Repeat("─", w-2)+"┘", a.bar)
		m.Con.SetPos(top+1+cur-first, left+1)
		k, ok := m.Con.ReadKey(-1)
		if !ok {
			return term.KEsc, cur
		}
		switch k {
		case term.KUp:
			cur = max(cur-1, 0)
		case term.KDown:
			cur = min(cur+1, max(len(lines)-1, 0))
		case term.KPgUp:
			cur = max(cur-inner, 0)
		case term.KPgDn:
			cur = min(cur+inner, max(len(lines)-1, 0))
		case term.KDebug:
		default:
			return k, cur
		}
	}
}

// inspect shows the elements of an array or hash; Enter opens an element
// that is itself an array or hash.
func (d *dbg) inspect(name string, v value.Value) {
	for {
		var lines []string
		var elems []value.Value
		var names []string
		switch v.K {
		case value.ARRAY:
			for i, it := range v.Arr().Items {
				n := fmt.Sprintf("%s[%d]", name, i+1)
				names = append(names, n)
				elems = append(elems, it.Deref())
				lines = append(lines, fmt.Sprintf("[%d] %s", i+1, Show(d.m, it, d.cols-12)))
			}
		case value.HASH:
			h := v.Hsh()
			for i := range h.Keys {
				k := show(d.m, h.Keys[i], 0)
				names = append(names, name+"["+k+"]")
				elems = append(elems, h.Vals[i].Deref())
				lines = append(lines, k+" => "+Show(d.m, h.Vals[i], d.cols-12))
			}
		}
		if len(lines) == 0 {
			lines = []string{"(empty)"}
		}
		k, i := d.box(fmt.Sprintf("%s  (%d)  Enter open  Esc close", name, len(elems)), lines, true)
		if k != term.KEnter || i >= len(elems) {
			return
		}
		if e := elems[i]; e.K == value.ARRAY || e.K == value.HASH {
			d.inspect(names[i], e)
		}
	}
}

const helpText = `F5          Go: run until a breakpoint (or the end)
F8          Step: run one line, going into procedures it calls
F10         Step over: run one line, without stopping in procedures
F6          Step out: run until this procedure returns
F7          Run to the line with the cursor
F9          Set or clear a breakpoint on the line with the cursor (●)
F3          Show the log written by logpoints
F4          Show the program's screen (any key comes back)
Tab         Move between the source, variables and call stack
            Source: arrows and PgUp/PgDn move, Home: current line
            Variables: Enter changes a value or opens an array,
                       Del removes a watch
            Call stack: arrows show another call's source and locals
Type        to evaluate an expression in the selected call:
  <expr>    show its value; x := 5 changes x (LOCALs too)
  WATCH <x> keep showing an expression's value; UNWATCH clears them
  WP <x>    stop when x changes (◉); F9 on a variable does the same
  DB a->f   stop when field f of table a changes in any record (a->* any
            field); IF and HIT work here too; DB a->f again clears it
  BP [n]    set or clear a breakpoint at line n (default: cursor)
  BP <name> stop when a procedure or function is called
  BP .. IF c stop there only when c is true (◆); c can use LOCALs
  BP .. HIT n  stop on the nth hit; HIT >= n from the nth, HIT % n every nth
  LP [n|name] "text {expr}" [IF c] [HIT n]
            log the text (◇) instead of stopping; LP n clears it
  LOG       show the log (LOG CLEAR empties it); LOGFILE <f> copies it
  BL, BC    list or clear all breakpoints, watchpoints and data breakpoints
  QUIT      stop the program
Alt-D opens the debugger while a program runs; so does AltD().`

func (d *dbg) help() {
	d.box("Debugger keys  (↑↓ scroll, other keys close)", strings.Split(helpText, "\n"), false)
}
