// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package term

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gdamore/tcell/v2"
)

// Windows
//
// The full screen console is a stack of windows. Window 0 is the whole
// screen and is always at the bottom. Numbered windows (1..MaxWindows) sit
// on top of it, each with its own cells, cursor and a frame whose top line
// is a title bar with minimise, maximise and close controls.
//
// All console output (Out, Put, Scroll, Save, Restore, SetPos, Size...) goes
// to the selected window, in coordinates relative to its inside, so a
// program written for the full screen runs unchanged inside a window.
//
// The screen shown on the terminal is the windows composited in stacking
// order. Every write updates the terminal cells it touches, so the terminal
// always shows the current picture.
//
// With the mouse a window can be raised (click), moved (drag the title
// bar), resized (drag the bottom right corner, sizable windows only) and
// minimised, maximised, restored or closed with the title bar controls.
// Mouse events are handled while the program waits for a key.

// MaxWindows is the highest window number.
const MaxWindows = 255

// WinState is a window's display state.
type WinState int

const (
	WinNormal WinState = iota
	WinMinimized
	WinMaximized
)

// WinOpts are the settings for a new window.
type WinOpts struct {
	Title    string
	Fill     Attr // inside colour
	Frame    Attr // frame and title colour
	Controls Attr // title bar controls and resize corner
	Sizable  bool // the user can resize and maximise it
}

// WinInfo describes a window. Top/Left/Bottom/Right are the frame's screen
// coordinates (inclusive).
type WinInfo struct {
	ID                       int
	Top, Left, Bottom, Right int
	Rows, Cols               int // inside size
	State                    WinState
	Title                    string
	Sizable                  bool
}

// Windower is implemented by consoles that support windows.
type Windower interface {
	WOpen(top, left, bottom, right int, o WinOpts) (int, error)
	// WSelect makes a window receive console output. raise also brings it
	// to the top of the stack (window 0 always stays at the bottom).
	WSelect(id int, raise bool) error
	WCurrent() int
	WClose(id int) error
	WCloseAll()
	WMove(id, top, left int) error
	WResize(id, rows, cols int) error // frame size
	WSetState(id int, s WinState) error
	WSetTitle(id int, title string) error
	WSetSizable(id int, on bool) error
	WInfo(id int) (WinInfo, bool)
	WList() []int // open numbered windows, lowest first
	WTop() int    // the front window that isn't minimised (0: none)
	// SetOnEvent sets a function told about window changes: closed with
	// the mouse, resized, or scrolled with the mouse wheel.
	SetOnEvent(func(e WinEvent))
	SetReserve(n int)
}

// WinEventKind says what happened to a window.
type WinEventKind int

const (
	WinClosed  WinEventKind = iota // closed with the mouse
	WinResized                     // inside size changed
	WinScroll                      // mouse wheel: Delta lines (negative = up)
)

// WinEvent reports a window change to the program side.
type WinEvent struct {
	ID         int
	Kind       WinEventKind
	Delta      int
	WasCurrent bool // WinClosed: it was the selected window
}

// Window errors.
var (
	ErrNoWindow     = errors.New("window does not exist")
	ErrWindowZero   = errors.New("window 0 is the full screen and cannot be changed")
	ErrTooMany      = fmt.Errorf("too many windows (the limit is %d)", MaxWindows)
	ErrWindowBounds = errors.New("window does not fit on the screen")
	ErrWindowSmall  = errors.New("window is too small (3 rows and 3 columns at least)")
)

type window struct {
	id                  int
	top, left, h, w     int // frame, screen coordinates and size
	border              bool
	title               string
	fill, frame, ctrls  Attr
	sizable             bool
	state, prev         WinState // prev: state to go back to from minimised
	nTop, nLeft, nH, nW int      // geometry before maximising
	cells               []Cell
	ih, iw              int // inside size
	row, col            int // cursor, inside coordinates
}

func (w *window) inset() int {
	if w.border {
		return 1
	}
	return 0
}

// resize changes the frame size, keeping the inside's top left content.
func (w *window) resize(h, wd int) {
	in := w.inset()
	ih, iw := max(h-2*in, 0), max(wd-2*in, 0)
	cells := make([]Cell, ih*iw)
	for r := 0; r < ih; r++ {
		for c := 0; c < iw; c++ {
			if r < w.ih && c < w.iw {
				cells[r*iw+c] = w.cells[r*w.iw+c]
			} else {
				cells[r*iw+c] = Cell{' ', w.fill}
			}
		}
	}
	w.h, w.w, w.ih, w.iw, w.cells = h, wd, ih, iw, cells
	w.row = min(w.row, max(ih-1, 0))
	w.col = min(w.col, iw)
}

func newWindow(id, top, left, h, wd int, border bool, fill Attr) *window {
	w := &window{id: id, top: top, left: left, border: border, fill: fill}
	w.resize(h, wd)
	return w
}

type rect struct{ y0, x0, y1, x1 int }

type dragState struct {
	w      *window
	dy, dx int
	resize bool
}

// ---------- compositing ----------

// stubs lays out minimised windows as short tabs along the bottom of the
// scrolling area. The layout is worked out once and kept until a window
// opens, closes, changes state or title, or the screen changes size (see
// invalidate): painting asks for it for every cell.
// tab is a minimised window's tab: where it is and what it shows.
type tab struct {
	w    *window
	r    rect
	text []rune
}

func (sc *Screen) stubs() []tab {
	if sc.stubCache == nil {
		sc.stubCache = sc.layoutStubs()
	}
	return sc.stubCache
}

// invalidate drops the cached window layout (tabs and front window).
func (sc *Screen) invalidate() {
	sc.stubCache, sc.activeCache = nil, nil
}

func (sc *Screen) layoutStubs() []tab {
	rows, cols := sc.base.h, sc.base.w
	y := max(rows-1-sc.reserve, 0)
	x := 0
	out := []tab{} // not nil: an empty layout is still a cached layout
	for _, id := range sc.WList() {
		w := sc.wins[id]
		if w.state != WinMinimized {
			continue
		}
		text := []rune(stubText(w))
		n := len(text)
		if x+n > cols {
			break
		}
		out = append(out, tab{w, rect{y, x, y, x + n - 1}, text})
		x += n + 1
	}
	return out
}

func stubText(w *window) string {
	t := []rune(w.title)
	if len(t) > 12 {
		t = append(t[:11], '…')
	}
	s := "[" + strconv.Itoa(w.id)
	if len(t) > 0 {
		s += " " + string(t)
	}
	return s + " ↑]"
}

// active is the topmost window that is not minimised (cached; see
// invalidate).
func (sc *Screen) active() *window {
	if sc.activeCache == nil {
		sc.activeCache = sc.base
		for i := len(sc.z) - 1; i >= 0; i-- {
			if sc.z[i].state != WinMinimized {
				sc.activeCache = sc.z[i]
				break
			}
		}
	}
	return sc.activeCache
}

// hit finds the window shown at a screen position. stub is true when it is
// a minimised window's tab.
func (sc *Screen) hit(y, x int) (w *window, stub bool) {
	return sc.hitWith(y, x, sc.stubs())
}

func (sc *Screen) hitWith(y, x int, stubs []tab) (w *window, stub bool) {
	w, _ = sc.hitTab(y, x, stubs)
	if w != nil {
		return w, true
	}
	for i := len(sc.z) - 1; i >= 0; i-- {
		w := sc.z[i]
		if w.state == WinMinimized {
			continue
		}
		if y >= w.top && y < w.top+w.h && x >= w.left && x < w.left+w.w {
			return w, false
		}
	}
	return nil, false
}

// hitTab finds the minimised window tab at a position, if any.
func (sc *Screen) hitTab(y, x int, stubs []tab) (*window, *tab) {
	// all tabs are on one row
	if len(stubs) == 0 || y != stubs[0].r.y0 {
		return nil, nil
	}
	for i := range stubs {
		if t := &stubs[i]; x >= t.r.x0 && x <= t.r.x1 {
			return t.w, t
		}
	}
	return nil, nil
}

var (
	singleBox = []rune("┌┐└┘─│")
	doubleBox = []rune("╔╗╚╝═║")
)

// controls is the title bar control strip; each control is 3 cells wide.
func controls(w *window) []rune {
	switch {
	case !w.sizable:
		return []rune("[_][x]")
	case w.state == WinMaximized:
		return []rune("[_][↕][x]")
	}
	return []rune("[_][↑][x]")
}

// titleText is what the title bar shows: the window number and title.
func titleText(w *window) []rune {
	s := " " + strconv.Itoa(w.id)
	if w.title != "" {
		s += ": " + w.title
	}
	return []rune(s + " ")
}

// ctrlStart is where the controls start on the top line, or -1 when the
// window is too narrow to show them.
func ctrlStart(w *window) int {
	n := len(controls(w))
	if w.w < n+2 {
		return -1
	}
	return w.w - 1 - n
}

// frameCell returns the frame character at a position on the border.
func (sc *Screen) frameCell(w *window, ry, rx int) (rune, Attr) {
	box := singleBox
	if w == sc.active() {
		box = doubleBox
	}
	last, right := w.h-1, w.w-1
	switch {
	case ry == 0:
		if rx == 0 {
			return box[0], w.frame
		}
		if rx == right {
			return box[1], w.frame
		}
		cs := ctrlStart(w)
		if cs >= 0 && rx >= cs {
			return controls(w)[rx-cs], w.ctrls
		}
		end := right
		if cs >= 0 {
			end = cs
		}
		if t := titleText(w); rx >= 2 && rx-2 < len(t) && rx < end-1 {
			return t[rx-2], w.frame
		}
		return box[4], w.frame
	case ry == last:
		if rx == 0 {
			return box[2], w.frame
		}
		if rx == right {
			if w.sizable && w.state == WinNormal {
				return '◢', w.ctrls
			}
			return box[3], w.frame
		}
		return box[4], w.frame
	}
	return box[5], w.frame
}

// cellAt is the composited cell at a screen position. stubs is the
// layout of minimised windows (see stubs).
func (sc *Screen) cellAt(y, x int, stubs []tab) (rune, Attr) {
	if len(sc.wins) == 0 {
		if y < sc.base.ih && x < sc.base.iw {
			c := sc.base.cells[y*sc.base.iw+x]
			return c.R, c.A
		}
		return ' ', sc.fill
	}
	if w, t := sc.hitTab(y, x, stubs); t != nil {
		return t.text[x-t.r.x0], w.ctrls
	}
	w, _ := sc.hitWith(y, x, nil)
	if w == nil {
		return ' ', sc.fill
	}
	ry, rx := y-w.top, x-w.left
	in := w.inset()
	if in > 0 && (ry == 0 || rx == 0 || ry == w.h-1 || rx == w.w-1) {
		return sc.frameCell(w, ry, rx)
	}
	c := w.cells[(ry-in)*w.iw+rx-in]
	return c.R, c.A
}

// paint copies a screen rectangle of the composited picture to the
// terminal.
func (sc *Screen) paint(r rect) {
	tw, th := sc.s.Size()
	var stubs []tab
	if len(sc.wins) > 0 {
		stubs = sc.stubs()
	}
	for y := max(r.y0, 0); y <= min(r.y1, th-1); y++ {
		for x := max(r.x0, 0); x <= min(r.x1, tw-1); x++ {
			ch, a := sc.cellAt(y, x, stubs)
			if ch == 0 {
				ch = ' '
			}
			sc.s.SetContent(x, y, ch, nil, sc.style(a))
		}
	}
}

func (sc *Screen) paintAll() { sc.paint(rect{0, 0, 1 << 20, 1 << 20}) }

// paintInside paints part of a window's inside (inside coordinates).
func (sc *Screen) paintInside(w *window, r0, c0, r1, c1 int) {
	if w.state == WinMinimized {
		return
	}
	in := w.inset()
	oy, ox := w.top+in, w.left+in
	sc.paint(rect{oy + r0, ox + c0, oy + r1, ox + c1})
}

func (sc *Screen) outer(w *window) rect {
	return rect{w.top, w.left, w.top + w.h - 1, w.left + w.w - 1}
}

// repaintWindow repaints a window's area plus an older area it left, and
// the minimised tabs (whose layout may have changed).
func (sc *Screen) repaintAround(w *window, old rect) {
	sc.paint(old)
	sc.paint(sc.outer(w))
	sc.paintStubRow()
}

func (sc *Screen) paintStubRow() {
	y := max(sc.base.h-1-sc.reserve, 0)
	sc.paint(rect{y, 0, y, 1 << 20})
}

// ---------- window management (Windower) ----------

func (sc *Screen) get(id int) (*window, error) {
	if id == 0 {
		return sc.base, nil
	}
	w, ok := sc.wins[id]
	if !ok {
		return nil, fmt.Errorf("window %d does not exist", id)
	}
	return w, nil
}

func (sc *Screen) getNumbered(id int) (*window, error) {
	if id == 0 {
		return nil, ErrWindowZero
	}
	return sc.get(id)
}

func (sc *Screen) checkRect(top, left, h, wd int) error {
	if h < 3 || wd < 3 {
		return ErrWindowSmall
	}
	if top < 0 || left < 0 || top+h > sc.base.h || left+wd > sc.base.w {
		return fmt.Errorf("%w (%d rows by %d columns)", ErrWindowBounds, sc.base.h, sc.base.w)
	}
	return nil
}

func (sc *Screen) WOpen(top, left, bottom, right int, o WinOpts) (int, error) {
	sc.invalidate()
	defer sc.invalidate()
	sc.checkSize()
	h, wd := bottom-top+1, right-left+1
	if err := sc.checkRect(top, left, h, wd); err != nil {
		return 0, err
	}
	id := 0
	for i := 1; i <= MaxWindows; i++ {
		if _, used := sc.wins[i]; !used {
			id = i
			break
		}
	}
	if id == 0 {
		return 0, ErrTooMany
	}
	w := newWindow(id, top, left, h, wd, true, o.Fill)
	w.title, w.frame, w.ctrls, w.sizable = o.Title, o.Frame, o.Controls, o.Sizable
	sc.wins[id] = w
	sc.z = append(sc.z, w)
	sc.cur = w
	sc.updateMouse()
	sc.paintAll() // the previous top window's frame changes too
	sc.showCursor()
	return id, nil
}

func (sc *Screen) WSelect(id int, raise bool) error {
	w, err := sc.get(id)
	if err != nil {
		return err
	}
	sc.cur = w
	if raise && id != 0 {
		sc.raise(w)
	}
	sc.showCursor()
	return nil
}

func (sc *Screen) WCurrent() int { return sc.cur.id }

func (sc *Screen) raise(w *window) {
	sc.invalidate()
	defer sc.invalidate()
	if w.id == 0 || sc.z[len(sc.z)-1] == w {
		return
	}
	for i, zw := range sc.z {
		if zw == w {
			sc.z = append(append(sc.z[:i:i], sc.z[i+1:]...), w)
			break
		}
	}
	sc.paintAll()
}

func (sc *Screen) WClose(id int) error {
	sc.invalidate()
	defer sc.invalidate()
	w, err := sc.getNumbered(id)
	if err != nil {
		return err
	}
	delete(sc.wins, id)
	for i, zw := range sc.z {
		if zw == w {
			sc.z = append(sc.z[:i:i], sc.z[i+1:]...)
			break
		}
	}
	if sc.drag != nil && sc.drag.w == w {
		sc.drag = nil
	}
	if sc.cur == w {
		sc.cur = sc.active()
	}
	sc.updateMouse()
	sc.paintAll()
	sc.showCursor()
	return nil
}

func (sc *Screen) WCloseAll() {
	for _, id := range sc.WList() {
		sc.WClose(id)
	}
}

func (sc *Screen) WMove(id, top, left int) error {
	w, err := sc.getNumbered(id)
	if err != nil {
		return err
	}
	if w.state == WinMaximized {
		return fmt.Errorf("window %d is maximised", id)
	}
	if err := sc.checkRect(top, left, w.h, w.w); err != nil {
		return err
	}
	old := sc.outer(w)
	w.top, w.left = top, left
	sc.repaintAround(w, old)
	sc.showCursor()
	return nil
}

func (sc *Screen) WResize(id, rows, cols int) error {
	w, err := sc.getNumbered(id)
	if err != nil {
		return err
	}
	if w.state == WinMaximized {
		return fmt.Errorf("window %d is maximised", id)
	}
	if err := sc.checkRect(w.top, w.left, rows, cols); err != nil {
		return err
	}
	old := sc.outer(w)
	sc.resizeWin(w, rows, cols)
	sc.repaintAround(w, old)
	sc.showCursor()
	return nil
}

// maxRect is the area a maximised window fills: the screen above any
// reserved status line.
func (sc *Screen) maxRect() (top, left, h, wd int) {
	return 0, 0, max(sc.base.h-sc.reserve, 3), sc.base.w
}

func (sc *Screen) WSetState(id int, s WinState) error {
	sc.invalidate()
	defer sc.invalidate()
	w, err := sc.getNumbered(id)
	if err != nil {
		return err
	}
	if s == w.state {
		return nil
	}
	switch s {
	case WinMinimized:
		// output still goes to a minimised window; it is just not shown
		w.prev = w.state
		w.state = WinMinimized
	case WinMaximized:
		if !w.sizable {
			return fmt.Errorf("window %d is not sizable", id)
		}
		if w.state == WinNormal {
			w.nTop, w.nLeft, w.nH, w.nW = w.top, w.left, w.h, w.w
		}
		w.state = WinMaximized
		t, l, h, wd := sc.maxRect()
		w.top, w.left = t, l
		sc.resizeWin(w, h, wd)
		sc.raise(w)
	case WinNormal:
		if w.state == WinMinimized && w.prev == WinMaximized {
			w.state = WinMaximized
			sc.raise(w)
			break
		}
		if w.state == WinMaximized {
			w.top, w.left = w.nTop, w.nLeft
			sc.resizeWin(w, w.nH, w.nW)
			sc.fitOnScreen(w)
		}
		w.state = WinNormal
		sc.raise(w)
	default:
		return fmt.Errorf("unknown window state %d", s)
	}
	sc.paintAll()
	sc.showCursor()
	return nil
}

func (sc *Screen) WSetTitle(id int, title string) error {
	sc.invalidate()
	defer sc.invalidate()
	w, err := sc.getNumbered(id)
	if err != nil {
		return err
	}
	w.title = title
	sc.paint(rect{w.top, w.left, w.top, w.left + w.w - 1})
	sc.paintStubRow()
	return nil
}

func (sc *Screen) WSetSizable(id int, on bool) error {
	w, err := sc.getNumbered(id)
	if err != nil {
		return err
	}
	if !on && w.state == WinMaximized {
		sc.WSetState(id, WinNormal)
	}
	w.sizable = on
	sc.paint(sc.outer(w))
	return nil
}

func (sc *Screen) WInfo(id int) (WinInfo, bool) {
	w, err := sc.get(id)
	if err != nil {
		return WinInfo{}, false
	}
	return WinInfo{ID: w.id, Top: w.top, Left: w.left, Bottom: w.top + w.h - 1, Right: w.left + w.w - 1,
		Rows: w.ih, Cols: w.iw, State: w.state, Title: w.title, Sizable: w.sizable}, true
}

func (sc *Screen) WList() []int {
	var ids []int
	for i := 1; i <= MaxWindows && len(ids) < len(sc.wins); i++ {
		if _, ok := sc.wins[i]; ok {
			ids = append(ids, i)
		}
	}
	return ids
}

func (sc *Screen) WTop() int { return sc.active().id }

func (sc *Screen) SetOnEvent(f func(e WinEvent)) { sc.onEvent = f }

func (sc *Screen) emit(e WinEvent) {
	if sc.onEvent != nil {
		sc.onEvent(e)
	}
}

// resizeWin changes a window's frame size and reports it.
func (sc *Screen) resizeWin(w *window, h, wd int) {
	if h == w.h && wd == w.w {
		return
	}
	w.resize(h, wd)
	if w.id != 0 {
		sc.emit(WinEvent{ID: w.id, Kind: WinResized})
	}
}

// fitOnScreen moves (and if need be shrinks) a window to fit the screen.
func (sc *Screen) fitOnScreen(w *window) {
	h, wd := min(w.h, sc.base.h), min(w.w, sc.base.w)
	sc.resizeWin(w, max(h, 3), max(wd, 3))
	w.top = max(0, min(w.top, sc.base.h-w.h))
	w.left = max(0, min(w.left, sc.base.w-w.w))
}

// updateMouse turns mouse reporting on only while there are windows, so
// the terminal's own text selection works the rest of the time.
func (sc *Screen) updateMouse() {
	want := len(sc.wins) > 0
	if want == sc.mouseOn {
		return
	}
	sc.mouseOn = want
	if want {
		sc.s.EnableMouse(tcell.MouseButtonEvents | tcell.MouseDragEvents)
	} else {
		sc.s.DisableMouse()
	}
}

// ---------- mouse ----------

func (sc *Screen) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	if b := ev.Buttons(); b&(tcell.WheelUp|tcell.WheelDown) != 0 {
		if w, stub := sc.hit(y, x); w != nil && w.id != 0 && !stub {
			d := 3
			if b&tcell.WheelUp != 0 {
				d = -3
			}
			sc.emit(WinEvent{ID: w.id, Kind: WinScroll, Delta: d})
		}
		return
	}
	if ev.Buttons()&tcell.Button1 == 0 {
		sc.pressed, sc.drag = false, nil
		return
	}
	if sc.pressed {
		if d := sc.drag; d != nil {
			sc.dragTo(d, y, x)
		}
		return
	}
	sc.pressed = true
	w, stub := sc.hit(y, x)
	if w == nil || w.id == 0 {
		return
	}
	if stub {
		sc.WSetState(w.id, WinNormal)
		return
	}
	sc.raise(w)
	ry, rx := y-w.top, x-w.left
	if ry == 0 {
		if cs := ctrlStart(w); cs >= 0 && rx >= cs && rx < cs+len(controls(w)) {
			sc.control(w, (rx-cs)/3)
			return
		}
		if w.state == WinNormal {
			sc.drag = &dragState{w: w, dy: ry, dx: rx}
		}
		return
	}
	if w.sizable && w.state == WinNormal && ry == w.h-1 && rx == w.w-1 {
		sc.drag = &dragState{w: w, resize: true}
	}
}

// control runs title bar control i (0 = minimise, then maximise for
// sizable windows, then close).
func (sc *Screen) control(w *window, i int) {
	if !w.sizable && i == 1 {
		i = 2
	}
	switch i {
	case 0:
		sc.WSetState(w.id, WinMinimized)
	case 1:
		if w.state == WinMaximized {
			sc.WSetState(w.id, WinNormal)
		} else {
			sc.WSetState(w.id, WinMaximized)
		}
	case 2:
		id, cur := w.id, sc.cur == w
		sc.WClose(id)
		sc.emit(WinEvent{ID: id, Kind: WinClosed, WasCurrent: cur})
	}
}

func (sc *Screen) dragTo(d *dragState, y, x int) {
	w := d.w
	if _, ok := sc.wins[w.id]; !ok || w.state != WinNormal {
		sc.drag = nil
		return
	}
	old := sc.outer(w)
	if d.resize {
		h := max(3, min(y-w.top+1, sc.base.h-w.top))
		wd := max(3, min(x-w.left+1, sc.base.w-w.left))
		if h == w.h && wd == w.w {
			return
		}
		sc.resizeWin(w, h, wd)
	} else {
		top := max(0, min(y-d.dy, sc.base.h-w.h))
		left := max(0, min(x-d.dx, sc.base.w-w.w))
		if top == w.top && left == w.left {
			return
		}
		w.top, w.left = top, left
	}
	sc.repaintAround(w, old)
	sc.showCursor()
}

// checkSize follows a change of terminal size.
func (sc *Screen) checkSize() {
	tw, th := sc.s.Size()
	if tw == sc.base.w && th == sc.base.h || tw <= 0 || th <= 0 {
		return
	}
	sc.base.resize(th, tw)
	sc.reserve = min(sc.reserve, max(th-1, 0))
	for _, w := range sc.wins {
		if w.state == WinMaximized {
			t, l, h, wd := sc.maxRect()
			w.top, w.left = t, l
			sc.resizeWin(w, h, wd)
		} else {
			sc.fitOnScreen(w)
		}
	}
	sc.paintAll()
	sc.showCursor()
}
