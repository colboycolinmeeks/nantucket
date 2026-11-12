// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package term

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
)

// Screen is a full screen console backed by tcell. Output goes to the
// selected window (see window.go); window 0 is the whole screen.
type Screen struct {
	s         tcell.Screen
	keys      chan int
	events    chan tcell.Event // mouse and resize, handled while reading keys
	mu        sync.Mutex
	typeahead []int
	cursor    int
	// OnInterrupt is called (from the input goroutine) on Ctrl-C / Alt-C.
	OnInterrupt func()
	// OnDebug is called (from the input goroutine) on Alt-D. When it
	// returns true the key opens the debugger instead of going to the
	// program.
	OnDebug func() bool
	// Idle is called about once a second while waiting for a key (the
	// status bar clock uses it).
	Idle       func()
	quit       chan struct{}
	suspended  atomic.Bool
	aheadLimit atomic.Int32 // SET TYPEAHEAD limit for keys typed ahead (0 = 256)
	mono       bool         // render in black and white
	fill       Attr         // colour for lines scrolled in at the bottom
	reserve    int          // bottom rows of window 0 kept out of scrolling (status bar)
	// TermPalette uses the terminal's own 16 colours instead of the fixed
	// VGA colours. Terminal themes often remap these, which can make
	// classic colour schemes hard to read, so it is off by default.
	TermPalette bool

	base    *window         // window 0, the whole screen
	cur     *window         // window receiving output
	wins    map[int]*window // numbered windows
	z       []*window       // stacking order, bottom first; z[0] is base
	drag    *dragState
	pressed bool
	mouseOn bool
	onEvent func(e WinEvent)

	waitHook func(bool)  // see WaitNotifier
	extKeys  atomic.Bool // report shifted keys (see ExtendedKeys)

	stubCache   []tab   // layout of minimised tabs (see stubs)
	activeCache *window // front window (see active)
}

// SetExtendedKeys turns reporting of shifted editing keys on or off.
func (sc *Screen) SetExtendedKeys(on bool) { sc.extKeys.Store(on) }

// extendedKey maps Shift+movement keys, Ctrl-Ins, Shift-Ins and Shift-Del
// to their own codes (0 if the key isn't one of them).
func extendedKey(e *tcell.EventKey) int {
	mod := e.Modifiers()
	if mod&tcell.ModShift != 0 && mod&(tcell.ModCtrl|tcell.ModAlt) == 0 {
		switch e.Key() {
		case tcell.KeyUp:
			return KShUp
		case tcell.KeyDown:
			return KShDown
		case tcell.KeyLeft:
			return KShLeft
		case tcell.KeyRight:
			return KShRight
		case tcell.KeyHome:
			return KShHome
		case tcell.KeyEnd:
			return KShEnd
		case tcell.KeyPgUp:
			return KShPgUp
		case tcell.KeyPgDn:
			return KShPgDn
		case tcell.KeyInsert:
			return KShIns
		case tcell.KeyDelete:
			return KShDel
		}
	}
	if mod&tcell.ModCtrl != 0 && e.Key() == tcell.KeyInsert {
		return KCtrlIns
	}
	if e.Key() == tcell.KeyCtrlZ {
		return KCtrlZ
	}
	return 0
}

// SetWaitHook sets the function told when the console waits for input.
func (sc *Screen) SetWaitHook(f func(bool)) { sc.waitHook = f }

func (sc *Screen) waiting(w bool) {
	if sc.waitHook != nil {
		sc.waitHook(w)
	}
}

// vgaPalette is the IBM VGA text mode palette, so colour schemes look the
// same whatever palette the terminal theme uses.
var vgaPalette = []tcell.Color{
	tcell.NewHexColor(0x000000), tcell.NewHexColor(0x0000AA), tcell.NewHexColor(0x00AA00), tcell.NewHexColor(0x00AAAA),
	tcell.NewHexColor(0xAA0000), tcell.NewHexColor(0xAA00AA), tcell.NewHexColor(0xAA5500), tcell.NewHexColor(0xAAAAAA),
	tcell.NewHexColor(0x555555), tcell.NewHexColor(0x5555FF), tcell.NewHexColor(0x55FF55), tcell.NewHexColor(0x55FFFF),
	tcell.NewHexColor(0xFF5555), tcell.NewHexColor(0xFF55FF), tcell.NewHexColor(0xFFFF55), tcell.NewHexColor(0xFFFFFF),
}

// palette is the terminal's own 16 colours (used for mono and TermPalette).
var palette = []tcell.Color{
	tcell.ColorBlack, tcell.ColorNavy, tcell.ColorGreen, tcell.ColorTeal,
	tcell.ColorMaroon, tcell.ColorPurple, tcell.ColorOlive, tcell.ColorSilver,
	tcell.ColorGray, tcell.ColorBlue, tcell.ColorLime, tcell.ColorAqua,
	tcell.ColorRed, tcell.ColorFuchsia, tcell.ColorYellow, tcell.ColorWhite,
}

func (sc *Screen) style(a Attr) tcell.Style {
	if sc.mono {
		a = MonoAttr(a)
	}
	pal := vgaPalette
	if sc.mono || sc.TermPalette {
		pal = palette
	}
	st := tcell.StyleDefault.Foreground(pal[a.Fg()]).Background(pal[a.Bg()])
	if sc.mono && a.Fg() == 15 {
		st = st.Bold(true)
	}
	if a&AttrUnderline != 0 {
		st = st.Underline(true)
	}
	if a&AttrBlink != 0 {
		st = st.Blink(true)
	}
	if a&AttrReverse != 0 {
		st = st.Reverse(true)
	}
	return st
}

func attrOf(st tcell.Style) Attr {
	fg, bg, at := st.Decompose()
	idx := func(c tcell.Color) int {
		for i := range palette {
			if palette[i] == c || vgaPalette[i] == c {
				return i
			}
		}
		return 7
	}
	a := MkAttr(idx(fg), idx(bg))
	if at&tcell.AttrUnderline != 0 {
		a |= AttrUnderline
	}
	if at&tcell.AttrReverse != 0 {
		a |= AttrReverse
	}
	return a
}

func NewScreen() (*Screen, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	return NewScreenWith(s)
}

// NewScreenWith wraps an existing tcell screen (tests use a simulation screen).
func NewScreenWith(s tcell.Screen) (*Screen, error) {
	if err := s.Init(); err != nil {
		return nil, err
	}
	sc := &Screen{s: s, keys: make(chan int, 256), events: make(chan tcell.Event, 64), cursor: 1,
		quit: make(chan struct{}), fill: DefaultAttr, wins: map[int]*window{}}
	w, h := s.Size()
	sc.base = newWindow(0, 0, 0, h, w, false, DefaultAttr)
	sc.cur = sc.base
	sc.z = []*window{sc.base}
	s.SetStyle(sc.style(DefaultAttr))
	s.Clear()
	go sc.pump()
	return sc, nil
}

func (sc *Screen) pump() {
	for {
		ev := sc.s.PollEvent()
		if ev == nil {
			if sc.suspended.Load() {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			return
		}
		switch e := ev.(type) {
		case *tcell.EventKey:
			k := keyCode(e)
			if sc.extKeys.Load() {
				if x := extendedKey(e); x != 0 {
					k = x
				}
			}
			if k == KAltC || (e.Key() == tcell.KeyCtrlC) {
				if sc.OnInterrupt != nil {
					sc.OnInterrupt()
				}
				continue
			}
			if k == KAltD && sc.OnDebug != nil && sc.OnDebug() {
				select {
				case sc.keys <- KDebug:
				default:
				}
				continue
			}
			if k != 0 {
				if n := int(sc.aheadLimit.Load()); n > 0 && len(sc.keys) >= n {
					sc.s.Beep() // keyboard buffer full
					continue
				}
				select {
				case sc.keys <- k:
				default:
				}
			}
		case *tcell.EventResize:
			sc.s.Sync()
			select {
			case sc.events <- e:
			default:
			}
		case *tcell.EventMouse:
			select {
			case sc.events <- e:
			default: // drop mouse moves if the program is busy
			}
		}
	}
}

// handleEvent deals with a mouse or resize event (on the caller's goroutine).
func (sc *Screen) handleEvent(ev tcell.Event) {
	switch e := ev.(type) {
	case *tcell.EventMouse:
		sc.checkSize()
		sc.handleMouse(e)
	case *tcell.EventResize:
		sc.checkSize()
	}
}

func keyCode(e *tcell.EventKey) int {
	mod := e.Modifiers()
	switch e.Key() {
	case tcell.KeyRune:
		r := e.Rune()
		if mod&tcell.ModAlt != 0 {
			switch r {
			case 'c', 'C':
				return KAltC
			case 'd', 'D':
				return KAltD
			case 'x', 'X':
				return 301
			}
			return 0
		}
		if r < 256 {
			return int(r)
		}
		return int(r)
	case tcell.KeyUp:
		return KUp
	case tcell.KeyDown:
		return KDown
	case tcell.KeyLeft:
		if mod&tcell.ModCtrl != 0 {
			return KCtrlLeft
		}
		return KLeft
	case tcell.KeyRight:
		if mod&tcell.ModCtrl != 0 {
			return KCtrlRight
		}
		return KRight
	case tcell.KeyHome:
		if mod&tcell.ModCtrl != 0 {
			return KCtrlHome
		}
		return KHome
	case tcell.KeyEnd:
		if mod&tcell.ModCtrl != 0 {
			return KCtrlEnd
		}
		return KEnd
	case tcell.KeyPgUp:
		if mod&tcell.ModCtrl != 0 {
			return KCtrlPgUp
		}
		return KPgUp
	case tcell.KeyPgDn:
		if mod&tcell.ModCtrl != 0 {
			return KCtrlPgDn
		}
		return KPgDn
	case tcell.KeyEnter:
		return KEnter
	case tcell.KeyEscape:
		return KEsc
	case tcell.KeyInsert:
		return KIns
	case tcell.KeyDelete:
		return KDel
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return KBS
	case tcell.KeyTab:
		return KTab
	case tcell.KeyBacktab:
		return KShTab
	case tcell.KeyF1:
		return KF1
	case tcell.KeyF2:
		return KF2
	case tcell.KeyF3:
		return KF3
	case tcell.KeyF4:
		return KF4
	case tcell.KeyF5:
		return KF5
	case tcell.KeyF6:
		return KF6
	case tcell.KeyF7:
		return KF7
	case tcell.KeyF8:
		return KF8
	case tcell.KeyF9:
		return KF9
	case tcell.KeyF10:
		return KF10
	case tcell.KeyF11:
		return KF11
	case tcell.KeyF12:
		return KF12
	}
	k := e.Key()
	if k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		return int(k-tcell.KeyCtrlA) + 1
	}
	return 0
}

func (sc *Screen) Size() (int, int) {
	sc.checkSize()
	return sc.cur.ih, sc.cur.iw
}

// Put writes text at a position in the current window without scrolling.
func (sc *Screen) Put(r, c int, s string, a Attr) {
	w := sc.cur
	if r < 0 || r >= w.ih {
		return
	}
	c0 := c
	for _, ch := range Runes(s) {
		if c >= 0 && c < w.iw {
			w.cells[r*w.iw+c] = Cell{ch, a}
		}
		c++
	}
	sc.paintInside(w, r, max(c0, 0), r, min(c, w.iw)-1)
}

// bottom is the last row of the current window that text output scrolls in.
func (sc *Screen) bottom() int {
	w := sc.cur
	if w == sc.base {
		return max(0, w.ih-1-sc.reserve)
	}
	return max(0, w.ih-1)
}

// SetReserve keeps the bottom n rows of window 0 out of scrolling output,
// so a status bar there stays put. Positioned output (@ SAY) can still
// write there.
func (sc *Screen) SetReserve(n int) {
	sc.reserve = min(max(n, 0), max(sc.base.h-1, 0))
	sc.invalidate()
	sc.paintStubRow()
}

func (sc *Screen) Out(s string, a Attr) {
	w := sc.cur
	for _, ch := range Runes2(s) {
		if ch == '\n' {
			sc.NewLine()
			continue
		}
		if ch == '\r' {
			w.col = 0
			continue
		}
		if w.col >= w.iw {
			sc.NewLine()
		}
		if w.row >= w.ih {
			w.row = w.ih - 1
		}
		if w.row < 0 || w.iw == 0 {
			continue
		}
		w.cells[w.row*w.iw+w.col] = Cell{ch, a}
		sc.paintInside(w, w.row, w.col, w.row, w.col)
		w.col++
	}
	sc.showCursor()
}

// Runes2 is like Runes but keeps newlines.
func Runes2(s string) []rune {
	out := make([]rune, 0, len(s))
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			out = append(out, Runes(s[start:i])...)
			out = append(out, rune(s[i]))
			start = i + 1
		}
	}
	return append(out, Runes(s[start:])...)
}

func (sc *Screen) NewLine() {
	w := sc.cur
	bottom := sc.bottom()
	w.col = 0
	w.row++
	switch {
	case w.row == bottom+1:
		sc.Scroll(0, 0, bottom, w.iw-1, 1, sc.fill)
		w.row = bottom
	case w.row >= w.ih:
		// writing below the scroll region (e.g. @ MaxRow() SAY ... then ?)
		sc.Scroll(0, 0, w.ih-1, w.iw-1, 1, sc.fill)
		w.row = w.ih - 1
	}
	sc.showCursor()
}

func (sc *Screen) SetPos(r, c int) {
	sc.cur.row, sc.cur.col = r, c
	sc.showCursor()
}

func (sc *Screen) Pos() (int, int) { return sc.cur.row, sc.cur.col }

// Scroll scrolls (n > 0 up, n < 0 down) or with n = 0 clears a region of
// the current window.
func (sc *Screen) Scroll(top, left, bottom, right, n int, a Attr) {
	w := sc.cur
	bottom, right = min(bottom, w.ih-1), min(right, w.iw-1)
	top, left = max(top, 0), max(left, 0)
	if top > bottom || left > right {
		return
	}
	at := func(r, c int) *Cell { return &w.cells[r*w.iw+c] }
	h := bottom - top + 1
	switch {
	case n == 0 || n >= h || -n >= h:
		for r := top; r <= bottom; r++ {
			for c := left; c <= right; c++ {
				*at(r, c) = Cell{' ', a}
			}
		}
	case n > 0:
		for r := top; r <= bottom; r++ {
			for c := left; c <= right; c++ {
				if r+n <= bottom {
					*at(r, c) = *at(r+n, c)
				} else {
					*at(r, c) = Cell{' ', a}
				}
			}
		}
	default:
		n = -n
		for r := bottom; r >= top; r-- {
			for c := left; c <= right; c++ {
				if r-n >= top {
					*at(r, c) = *at(r-n, c)
				} else {
					*at(r, c) = Cell{' ', a}
				}
			}
		}
	}
	sc.paintInside(w, top, left, bottom, right)
}

// showCursor puts the terminal cursor at the current window's cursor, or
// hides it when that spot is not visible.
func (sc *Screen) showCursor() {
	w := sc.cur
	if sc.cursor == 0 || w.state == WinMinimized || w.row < 0 || w.row >= w.ih || w.col < 0 || w.iw == 0 {
		sc.s.HideCursor()
		return
	}
	in := w.inset()
	y, x := w.top+in+w.row, w.left+in+min(w.col, max(w.iw-1, 0))
	if top, stub := sc.hit(y, x); top != w || stub {
		sc.s.HideCursor()
		return
	}
	sc.s.ShowCursor(x, y)
}

func (sc *Screen) SetCursor(shape int) {
	sc.cursor = shape
	switch shape {
	case 0:
		sc.s.HideCursor()
		return
	case 2, 3:
		sc.s.SetCursorStyle(tcell.CursorStyleSteadyBlock)
	default:
		sc.s.SetCursorStyle(tcell.CursorStyleDefault)
	}
	sc.showCursor()
}

// SetIdle sets the function called about once a second while waiting for
// a key.
func (sc *Screen) SetIdle(f func()) { sc.Idle = f }

func (sc *Screen) Flush() {
	sc.checkSize()
	sc.showCursor()
	sc.s.Show()
}

func (sc *Screen) ReadKey(timeout time.Duration) (int, bool) {
	sc.mu.Lock()
	if len(sc.typeahead) > 0 {
		k := sc.typeahead[0]
		sc.typeahead = sc.typeahead[1:]
		sc.mu.Unlock()
		return k, true
	}
	sc.mu.Unlock()
	sc.Flush()
	if timeout == 0 {
		for {
			select {
			case ev := <-sc.events:
				sc.handleEvent(ev)
				sc.Flush()
				continue
			default:
			}
			break
		}
		select {
		case k := <-sc.keys:
			return k, true
		default:
			return 0, false
		}
	}
	var deadline <-chan time.Time
	if timeout > 0 {
		deadline = time.After(timeout)
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	// while blocked here the program is idle; handling events and the
	// clock runs program code, so it is done outside the idle state
	sc.waiting(true)
	defer sc.waiting(false)
	for {
		select {
		case k := <-sc.keys:
			return k, true
		case ev := <-sc.events:
			sc.waiting(false)
			sc.handleEvent(ev)
			sc.Flush()
			sc.waiting(true)
		case <-tick.C:
			if sc.Idle != nil {
				sc.waiting(false)
				sc.Idle()
				sc.Flush()
				sc.waiting(true)
			}
		case <-deadline:
			return 0, false
		}
	}
}

func (sc *Screen) PushKeys(k []int) {
	sc.mu.Lock()
	sc.typeahead = append(sc.typeahead, k...)
	sc.mu.Unlock()
}

func (sc *Screen) UnreadKey(k int) {
	sc.mu.Lock()
	sc.typeahead = append([]int{k}, sc.typeahead...)
	sc.mu.Unlock()
}

func (sc *Screen) ClearKeys() {
	sc.mu.Lock()
	sc.typeahead = nil
	sc.mu.Unlock()
	for {
		select {
		case <-sc.keys:
		default:
			return
		}
	}
}

// Save copies a region of the current window.
func (sc *Screen) Save(top, left, bottom, right int) []Cell {
	w := sc.cur
	var cells []Cell
	for r := top; r <= bottom; r++ {
		for c := left; c <= right; c++ {
			if r >= 0 && r < w.ih && c >= 0 && c < w.iw {
				cl := w.cells[r*w.iw+c]
				if cl.R == 0 {
					cl.R = ' '
				}
				cells = append(cells, cl)
			} else {
				cells = append(cells, Cell{' ', sc.fill})
			}
		}
	}
	return cells
}

// Restore puts back a region saved with Save.
func (sc *Screen) Restore(top, left, bottom, right int, cells []Cell) {
	w := sc.cur
	i := 0
	for r := top; r <= bottom; r++ {
		for c := left; c <= right; c++ {
			if i >= len(cells) {
				break
			}
			if r >= 0 && r < w.ih && c >= 0 && c < w.iw {
				w.cells[r*w.iw+c] = cells[i]
			}
			i++
		}
	}
	sc.paintInside(w, max(top, 0), max(left, 0), min(bottom, w.ih-1), min(right, w.iw-1))
}

// ApplyTheme switches black and white rendering on or off and sets the
// colour used for unpainted areas and lines scrolled in. Text already on
// the screen keeps its colours; switching to or from mono repaints it.
func (sc *Screen) ApplyTheme(t Theme, std Attr) {
	repaint := sc.mono != t.Mono
	sc.mono = t.Mono
	sc.fill = std
	sc.s.SetStyle(sc.style(std))
	if repaint {
		sc.paintAll()
	}
}

// SetTypeahead limits how many keys can be typed ahead (SET TYPEAHEAD).
func (sc *Screen) SetTypeahead(n int) {
	sc.aheadLimit.Store(int32(min(max(n, 1), cap(sc.keys))))
}

// Colors reports how many colours the terminal supports.
func (sc *Screen) Colors() int { return sc.s.Colors() }

func (sc *Screen) Interactive() bool { return true }
func (sc *Screen) Bell()             { sc.s.Beep() }
func (sc *Screen) Close()            { sc.s.Fini() }

func (sc *Screen) Suspend() {
	sc.suspended.Store(true)
	sc.s.Suspend()
}

func (sc *Screen) Resume() {
	sc.s.Resume()
	sc.suspended.Store(false)
	sc.s.Sync()
	sc.paintAll()
}
