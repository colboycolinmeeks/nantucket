// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package gets

import (
	"nantucket/internal/term"
	"nantucket/internal/vm"
)

// scoreboard shows READ status on row 0 while SET SCOREBOARD is ON: "Ins"
// in insert mode, and messages such as "Invalid date" or the RANGE limits.
// Whatever was on row 0 is put back when the READ ends.
type scoreboard struct {
	saved []term.Cell
	cols  int
	ins   bool
	msg   string
}

func openScoreboard(m *vm.VM) *scoreboard {
	if !m.Setting(32).Truthy() || !m.Con.Interactive() {
		return nil
	}
	_, cols := m.Con.Size()
	return &scoreboard{saved: m.Con.Save(0, 0, 0, cols-1), cols: cols}
}

func drawScoreboard(m *vm.VM, sb *scoreboard) {
	if sb == nil {
		return
	}
	r, c := m.Con.Pos()
	m.Con.Restore(0, 0, 0, sb.cols-1, sb.saved)
	if sb.ins {
		m.Con.Put(0, sb.cols-4, "Ins", m.Colors().Std)
	}
	if sb.msg != "" {
		m.Con.Put(0, max(0, sb.cols-6-len(sb.msg)), sb.msg, m.Colors().Std)
	}
	m.Con.SetPos(r, c)
}

func closeScoreboard(m *vm.VM, sb *scoreboard) {
	if sb != nil {
		m.Con.Restore(0, 0, 0, sb.cols-1, sb.saved)
	}
}

// scoreboardMsg shows (or with "" clears) a READ message.
func scoreboardMsg(m *vm.VM, msg string) {
	if rs := state(m).read; rs != nil && rs.sb != nil && rs.sb.msg != msg {
		rs.sb.msg = msg
		drawScoreboard(m, rs.sb)
	}
}

// scoreboardIns shows or hides the insert-mode indicator.
func scoreboardIns(m *vm.VM, ins bool) {
	if rs := state(m).read; rs != nil && rs.sb != nil && rs.sb.ins != ins {
		rs.sb.ins = ins
		drawScoreboard(m, rs.sb)
	}
}

// getBell beeps during READ when SET BELL is ON.
func getBell(m *vm.VM) {
	if m.Setting(26).Truthy() {
		m.Con.Bell()
	}
}
