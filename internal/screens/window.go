// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package screens

import (
	"errors"
	"fmt"
	"strings"

	"nantucket/internal/vm"

	"nantucket/internal/term"
	"nantucket/internal/value"
)

// Windows (see term/window.go for how they are drawn).
//
// Window 0 is the full screen. WOpen() creates a numbered window and
// selects it; while a window is selected every screen command (?, @ SAY,
// @ GET, CLS, Scroll(), SaveScreen(), Row(), MaxRow()...) works inside it,
// in coordinates relative to its inside. SET WINDOW TO <n> (WSelect())
// switches window; SET WINDOW TO (or 0) goes back to the full screen.
//
// Each window keeps its own SET COLOR: selecting a window brings back the
// colours it last used.

// session is the screens' state for one runtime.
type session struct {
	statusShown bool             // status bar is on screen (see status.go)
	lastStatus  string           // status bar text last drawn
	winColors   map[int]string   // SET COLOR of windows not selected
	textWins    map[int]*textWin // HELP and VERSION FULL windows by number
	namedWins   map[string]int   // text window numbers by name
}

type sessionKey struct{}

func scr(m *vm.VM) *session {
	return m.Ext(sessionKey{}, func() any { return &session{} }).(*session)
}

var errNoWindows = errors.New("windows need the full screen console")

func windower(m *vm.VM) term.Windower {
	if w, ok := m.Con.(term.Windower); ok && m.Con.Interactive() {
		return w
	}
	return nil
}

// winErr turns a window error into a runtime error.
func winErr(op string, err error, args ...value.Value) *vm.RTError {
	return &vm.RTError{SubSystem: "TERM", GenCode: 1, SubCode: 3001, Description: err.Error(), Operation: op, Args: args, CanDefault: true}
}

// onWindow runs fn with window id selected for output, without raising it
// or changing colours, then selects the previous window again.
func onWindow(m *vm.VM, id int, fn func()) bool {
	w := windower(m)
	if w == nil {
		if id == 0 {
			fn()
			return true
		}
		return false
	}
	prev := w.WCurrent()
	if prev == id {
		fn()
		return true
	}
	if w.WSelect(id, false) != nil {
		return false
	}
	fn()
	w.WSelect(prev, false)
	return true
}

// selectWindow makes window n receive output, swapping in its colours.
func selectWindow(m *vm.VM, n int, raise bool) error {
	w := windower(m)
	if w == nil {
		if n == 0 {
			return nil
		}
		return errNoWindows
	}
	old := w.WCurrent()
	if err := w.WSelect(n, raise); err != nil {
		return err
	}
	switchColors(m, old, n)
	return nil
}

// switchColors saves window old's SET COLOR and applies window n's.
func switchColors(m *vm.VM, old, n int) {
	if old == n {
		return
	}
	if scr(m).winColors == nil {
		scr(m).winColors = map[int]string{}
	}
	scr(m).winColors[old] = m.Setting(15).S
	m.SetColorString(scr(m).winColors[n])
}

// windowClosed tidies up after window id has been closed: the colours of
// whichever window is now selected come back.
func windowClosed(m *vm.VM, id int, wasCurrent bool) {
	delete(scr(m).winColors, id)
	if tw := scr(m).textWins[id]; tw != nil {
		delete(scr(m).textWins, id)
		delete(scr(m).namedWins, tw.name)
	}
	if w := windower(m); w != nil && wasCurrent {
		m.SetColorString(scr(m).winColors[w.WCurrent()])
	}
}

// winArg reads an optional window number argument (default: the current
// window).
func winArg(m *vm.VM, a []value.Value, i int) int {
	if vm.Has(a, i) {
		return vm.ArgInt(a, i, 0)
	}
	if w := windower(m); w != nil {
		return w.WCurrent()
	}
	return 0
}

func closeWindow(m *vm.VM, id int) error {
	w := windower(m)
	if w == nil {
		return errNoWindows
	}
	cur := w.WCurrent() == id
	if err := w.WClose(id); err != nil {
		return err
	}
	windowClosed(m, id, cur)
	return nil
}

// windowEvent handles window changes made with the mouse or by resizing.
func windowEvent(m *vm.VM, e term.WinEvent) {
	switch e.Kind {
	case term.WinClosed:
		windowClosed(m, e.ID, e.WasCurrent)
	case term.WinResized:
		drawText(m, e.ID)
	case term.WinScroll:
		if tw := scr(m).textWins[e.ID]; tw != nil {
			tw.top += e.Delta
			drawText(m, e.ID)
		}
	}
}

// ---------- text windows (HELP, VERSION FULL) ----------

// textWin is a read-only window of text that stays on screen while the
// user carries on at the dot prompt. It scrolls with the mouse wheel and
// shows more when made bigger.
type textWin struct {
	name, title string
	lines       []string
	top         int // first line shown
}

// showText shows lines in the text window called name, opening it (at the
// top left or right of the screen) or bringing it back if it is already
// open. Without a full screen the lines are printed instead.
func showText(m *vm.VM, name, title string, lines []string, right bool) {
	w := windower(m)
	if w == nil || !textWindow(m, w, name, title, lines, right) {
		for _, l := range lines {
			m.Println(l)
		}
	}
}

func textWindow(m *vm.VM, w term.Windower, name, title string, lines []string, right bool) bool {
	prev := w.WCurrent()
	if id := scr(m).namedWins[name]; id != 0 {
		if _, ok := w.WInfo(id); ok {
			w.WSetState(id, term.WinNormal)
			w.WSelect(id, true)
			w.WSelect(prev, false)
			return true
		}
		delete(scr(m).namedWins, name)
		delete(scr(m).textWins, id)
	}
	width := 0
	for _, l := range lines {
		width = max(width, term.DisplayLen(l))
	}
	var rows, cols int
	onWindow(m, 0, func() { rows, cols = m.Con.Size() })
	if scr(m).statusShown {
		rows--
	}
	// leave at least 6 lines of the dot prompt in view
	h, wd := min(len(lines)+2, rows-6), min(width+4, cols)
	if h < 5 || wd < 20 {
		return false
	}
	left := 0
	if right {
		left = cols - wd
	}
	th := m.Theme()
	id, err := w.WOpen(0, left, h-1, left+wd-1, term.WinOpts{
		Title:    title,
		Fill:     th.Help,
		Frame:    term.MkAttr(th.WinFrame.Fg(), th.Help.Bg()),
		Controls: term.MkAttr(th.WinControls.Fg(), th.Help.Bg()),
		Sizable:  true,
	})
	if err != nil {
		return false
	}
	w.WSelect(prev, false)
	if scr(m).textWins == nil {
		scr(m).textWins, scr(m).namedWins = map[int]*textWin{}, map[string]int{}
	}
	scr(m).textWins[id] = &textWin{name: name, title: title, lines: lines}
	scr(m).namedWins[name] = id
	drawText(m, id)
	if prev == 0 {
		// keep the dot prompt below the window
		r, _ := m.Con.Pos()
		for i := 0; r < h-1 && i < rows; i++ {
			m.Con.NewLine()
			r, _ = m.Con.Pos()
		}
	}
	return true
}

// drawText fills a text window from its top line, and shows in the title
// which lines are in view when they do not all fit.
func drawText(m *vm.VM, id int) {
	w, tw := windower(m), scr(m).textWins[id]
	if w == nil || tw == nil {
		return
	}
	onWindow(m, id, func() {
		rows, cols := m.Con.Size()
		tw.top = max(0, min(tw.top, len(tw.lines)-rows))
		a := m.Theme().Help
		m.Con.Scroll(0, 0, rows-1, cols-1, 0, a)
		for i := 0; i < rows && tw.top+i < len(tw.lines); i++ {
			m.Con.Put(i, 1, tw.lines[tw.top+i], a)
		}
		title := tw.title
		if rows < len(tw.lines) {
			title = fmt.Sprintf("%s %d-%d of %d, wheel scrolls", tw.title, tw.top+1, min(tw.top+rows, len(tw.lines)), len(tw.lines))
		}
		w.WSetTitle(id, title)
	})
}

// showHelp opens the command summary (HELP) in a text window at the top
// right of the screen.
func showHelp(m *vm.VM) {
	showText(m, "help", "Help", strings.Split(helpText, "\n"), true)
}

// ---------- functions ----------

func init() {
	// WOpen(nTop, nLeft, nBottom, nRight [, cTitle] [, cColor] [, lSizable])
	//   -> nWindow. Opens a framed window (the frame is on the given
	//   coordinates) and selects it. Sizable defaults to .T.
	vm.Register("WOPEN", func(m *vm.VM, a []value.Value) value.Value {
		for i := 0; i < 4; i++ {
			if vm.Arg(a, i).K != value.NUMBER {
				return m.Raise(vm.ErrArg(1001, "WOPEN", a...))
			}
		}
		w := windower(m)
		if w == nil {
			return m.Raise(winErr("WOPEN", errNoWindows))
		}
		color := vm.ArgStr(a, 5, m.Setting(15).S)
		cs := m.Theme().Colors(strings.Trim(strings.TrimSpace(color), `"'`))
		th := m.Theme()
		old := w.WCurrent()
		id, err := w.WOpen(vm.ArgInt(a, 0, 0), vm.ArgInt(a, 1, 0), vm.ArgInt(a, 2, 0), vm.ArgInt(a, 3, 0), term.WinOpts{
			Title:    vm.ArgStr(a, 4, ""),
			Fill:     cs.Std,
			Frame:    term.MkAttr(th.WinFrame.Fg(), cs.Std.Bg()),
			Controls: term.MkAttr(th.WinControls.Fg(), cs.Std.Bg()),
			Sizable:  vm.ArgBool(a, 6, true),
		})
		if err != nil {
			return m.Raise(winErr("WOPEN", err, a...))
		}
		switchColors(m, old, id)
		m.SetColorString(cs.Text)
		return value.Int(id)
	})
	// WSelect([nWindow]) -> nOld. Selects (and raises) a window; 0 is the
	// full screen.
	vm.Register("WSELECT", func(m *vm.VM, a []value.Value) value.Value {
		old := winArg(m, nil, 0)
		if !vm.Has(a, 0) {
			return value.Int(old)
		}
		if vm.Arg(a, 0).K != value.NUMBER {
			return m.Raise(vm.ErrArg(1001, "WSELECT", a...))
		}
		if err := selectWindow(m, vm.ArgInt(a, 0, 0), true); err != nil {
			return m.Raise(winErr("WSELECT", err, a...))
		}
		return value.Int(old)
	})
	// WClose([nWindow]) closes a window (default: the current one).
	vm.Register("WCLOSE", func(m *vm.VM, a []value.Value) value.Value {
		if err := closeWindow(m, winArg(m, a, 0)); err != nil {
			return m.Raise(winErr("WCLOSE", err, a...))
		}
		return value.Nil
	})
	// WAClose() closes every window and selects the full screen.
	vm.Register("WACLOSE", func(m *vm.VM, a []value.Value) value.Value {
		if w := windower(m); w != nil {
			for _, id := range w.WList() {
				closeWindow(m, id)
			}
		}
		return value.Nil
	})
	// WMove(nTop, nLeft [, nWindow]) moves a window's frame.
	vm.Register("WMOVE", func(m *vm.VM, a []value.Value) value.Value {
		w := windower(m)
		if w == nil {
			return m.Raise(winErr("WMOVE", errNoWindows))
		}
		if vm.Arg(a, 0).K != value.NUMBER || vm.Arg(a, 1).K != value.NUMBER {
			return m.Raise(vm.ErrArg(1001, "WMOVE", a...))
		}
		if err := w.WMove(winArg(m, a, 2), vm.ArgInt(a, 0, 0), vm.ArgInt(a, 1, 0)); err != nil {
			return m.Raise(winErr("WMOVE", err, a...))
		}
		return value.Nil
	})
	// WSize(nRows, nCols [, nWindow]) sets a window's frame size.
	vm.Register("WSIZE", func(m *vm.VM, a []value.Value) value.Value {
		w := windower(m)
		if w == nil {
			return m.Raise(winErr("WSIZE", errNoWindows))
		}
		if vm.Arg(a, 0).K != value.NUMBER || vm.Arg(a, 1).K != value.NUMBER {
			return m.Raise(vm.ErrArg(1001, "WSIZE", a...))
		}
		if err := w.WResize(winArg(m, a, 2), vm.ArgInt(a, 0, 0), vm.ArgInt(a, 1, 0)); err != nil {
			return m.Raise(winErr("WSIZE", err, a...))
		}
		return value.Nil
	})
	state := func(name string, s term.WinState) {
		vm.Register(name, func(m *vm.VM, a []value.Value) value.Value {
			w := windower(m)
			if w == nil {
				return m.Raise(winErr(name, errNoWindows))
			}
			if err := w.WSetState(winArg(m, a, 0), s); err != nil {
				return m.Raise(winErr(name, err, a...))
			}
			return value.Nil
		})
	}
	// WMinimize([n]), WMaximize([n]), WRestore([n])
	state("WMINIMIZE", term.WinMinimized)
	state("WMAXIMIZE", term.WinMaximized)
	state("WRESTORE", term.WinNormal)

	info := func(m *vm.VM, a []value.Value, i int) (term.WinInfo, bool) {
		w := windower(m)
		if w == nil {
			return term.WinInfo{}, false
		}
		return w.WInfo(winArg(m, a, i))
	}
	num := func(name string, f func(term.WinInfo) int) {
		vm.Register(name, func(m *vm.VM, a []value.Value) value.Value {
			in, ok := info(m, a, 0)
			if !ok {
				if vm.Has(a, 0) && vm.ArgInt(a, 0, 0) != 0 {
					return m.Raise(winErr(name, term.ErrNoWindow, a...))
				}
				r, c := m.Con.Size()
				in = term.WinInfo{Bottom: r - 1, Right: c - 1, Rows: r, Cols: c}
			}
			return value.Int(f(in))
		})
	}
	// Frame position of a window (default: current): WRow/WCol are the
	// top left, WLastRow/WLastCol the bottom right. WState: 0 normal,
	// 1 minimised, 2 maximised.
	num("WROW", func(i term.WinInfo) int { return i.Top })
	num("WCOL", func(i term.WinInfo) int { return i.Left })
	num("WLASTROW", func(i term.WinInfo) int { return i.Bottom })
	num("WLASTCOL", func(i term.WinInfo) int { return i.Right })
	num("WSTATE", func(i term.WinInfo) int { return int(i.State) })
	// WNum() -> highest open window number (0 when none).
	vm.Register("WNUM", func(m *vm.VM, a []value.Value) value.Value {
		n := 0
		if w := windower(m); w != nil {
			if ids := w.WList(); len(ids) > 0 {
				n = ids[len(ids)-1]
			}
		}
		return value.Int(n)
	})
	// WList() -> array of open window numbers.
	vm.Register("WLIST", func(m *vm.VM, a []value.Value) value.Value {
		var out []value.Value
		if w := windower(m); w != nil {
			for _, id := range w.WList() {
				out = append(out, value.Int(id))
			}
		}
		return value.NewArray(out)
	})
	// WTitle([cTitle] [, nWindow]) -> cOld
	vm.Register("WTITLE", func(m *vm.VM, a []value.Value) value.Value {
		in, ok := info(m, a, 1)
		if !ok || in.ID == 0 {
			if vm.Has(a, 0) {
				return m.Raise(winErr("WTITLE", term.ErrWindowZero, a...))
			}
			return value.Str("")
		}
		if vm.Has(a, 0) {
			if vm.Arg(a, 0).K != value.STRING {
				return m.Raise(vm.ErrArg(1001, "WTITLE", a...))
			}
			windower(m).WSetTitle(in.ID, vm.Arg(a, 0).S)
		}
		return value.Str(in.Title)
	})
	// WSizable([lSizable] [, nWindow]) -> lOld. A sizable window can be
	// resized and maximised by the user.
	vm.Register("WSIZABLE", func(m *vm.VM, a []value.Value) value.Value {
		in, ok := info(m, a, 1)
		if !ok || in.ID == 0 {
			if vm.Has(a, 0) {
				return m.Raise(winErr("WSIZABLE", term.ErrWindowZero, a...))
			}
			return value.False
		}
		if vm.Has(a, 0) {
			if vm.Arg(a, 0).K != value.LOGICAL {
				return m.Raise(vm.ErrArg(1001, "WSIZABLE", a...))
			}
			windower(m).WSetSizable(in.ID, vm.Arg(a, 0).B)
		}
		return value.Bool(in.Sizable)
	})
}
