// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package screens

import (
	"nantucket/internal/term"
	"nantucket/internal/vm"
)

// Keyboard control of windows, for when there is no mouse. The dot prompt
// calls these: F1 toggles HELP, Esc on an empty line closes the front
// HELP or VERSION FULL window, and F6 enters window mode.

// ToggleHelp opens the HELP window, or closes it if it is open.
func ToggleHelp(m *vm.VM) {
	w := windower(m)
	if w != nil {
		if id := scr(m).namedWins["help"]; id != 0 {
			if _, ok := w.WInfo(id); ok {
				closeWindow(m, id)
				return
			}
		}
	}
	showHelp(m)
}

// CloseFrontText closes the front HELP or VERSION FULL window, and
// reports whether there was one.
func CloseFrontText(m *vm.VM) bool {
	w := windower(m)
	if w == nil {
		return false
	}
	if id := w.WTop(); id != 0 && scr(m).textWins[id] != nil {
		closeWindow(m, id)
		return true
	}
	return false
}

// windowModeHint is shown on the bottom line in window mode.
const windowModeHint = " Window: arrows move  Shift+arrows size  Tab next  M minimise  Z zoom  X close  Enter done "

// WindowMode lets the user arrange windows with the keyboard: arrows move
// the front window, Shift+arrows resize it, Tab brings the next window to
// the front (restoring it if minimised), M minimises, Z maximises or
// restores, X closes, and Enter or Esc finish. It reports false if there
// are no windows.
func WindowMode(m *vm.VM) bool {
	w := windower(m)
	if w == nil || len(w.WList()) == 0 {
		return false
	}
	if x, ok := m.Con.(term.ExtendedKeys); ok {
		x.SetExtendedKeys(true) // for Shift+arrows
		defer x.SetExtendedKeys(false)
	}
	// the hint goes over the bottom line, which is put back afterwards
	var saved []term.Cell
	var rows, cols int
	onWindow(m, 0, func() {
		rows, cols = m.Con.Size()
		saved = m.Con.Save(rows-1, 0, rows-1, cols-1)
		m.Con.Put(rows-1, 0, vm.PadTo(windowModeHint, cols, 'R', " "), m.Theme().Status)
	})
	defer onWindow(m, 0, func() { m.Con.Restore(rows-1, 0, rows-1, cols-1, saved) })

	raise := func(id int) {
		prev := w.WCurrent()
		w.WSelect(id, true)
		w.WSelect(prev, false)
	}
	cur := w.WTop()
	if cur == 0 { // all minimised: bring one back
		cur = w.WList()[0]
		w.WSetState(cur, term.WinNormal)
		raise(cur)
	}
	for {
		m.Con.Flush()
		k, ok := m.Con.ReadKey(-1)
		if !ok {
			return true
		}
		in, ok := w.WInfo(cur)
		if !ok {
			return true
		}
		switch k {
		case term.KUp:
			w.WMove(cur, in.Top-1, in.Left)
		case term.KDown:
			w.WMove(cur, in.Top+1, in.Left)
		case term.KLeft:
			w.WMove(cur, in.Top, in.Left-1)
		case term.KRight:
			w.WMove(cur, in.Top, in.Left+1)
		case term.KShUp, term.KShDown, term.KShLeft, term.KShRight:
			if !in.Sizable {
				break
			}
			h, wd := in.Bottom-in.Top+1, in.Right-in.Left+1
			switch k {
			case term.KShUp:
				h--
			case term.KShDown:
				h++
			case term.KShLeft:
				wd--
			case term.KShRight:
				wd++
			}
			w.WResize(cur, h, wd)
		case term.KTab, term.KShTab:
			ids := w.WList()
			next := ids[0]
			for i, id := range ids {
				if id == cur {
					next = ids[(i+1)%len(ids)]
					if k == term.KShTab {
						next = ids[(i+len(ids)-1)%len(ids)]
					}
				}
			}
			cur = next
			if in, ok := w.WInfo(cur); ok && in.State == term.WinMinimized {
				w.WSetState(cur, term.WinNormal)
			}
			raise(cur)
		case 'm', 'M':
			w.WSetState(cur, term.WinMinimized)
			if cur = w.WTop(); cur == 0 {
				return true
			}
		case 'z', 'Z':
			if in.State == term.WinMaximized {
				w.WSetState(cur, term.WinNormal)
			} else if in.Sizable {
				w.WSetState(cur, term.WinMaximized)
			}
		case 'x', 'X', term.KDel:
			closeWindow(m, cur)
			if cur = w.WTop(); cur == 0 {
				if ids := w.WList(); len(ids) > 0 {
					cur = ids[0]
					w.WSetState(cur, term.WinNormal)
					raise(cur)
				} else {
					return true
				}
			}
		case term.KEnter, term.KEsc:
			return true
		}
	}
}
