// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package screens

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nantucket/internal/vm"

	"nantucket/internal/term"
	"nantucket/internal/value"
)

// The status bar sits on the bottom row at the dot prompt while SET STATUS
// is ON. It shows the current work area, the table open there, the record
// position and the total record count (from the table header, so it costs
// nothing to show), the controlling index and filter, the directory, and
// the date and time (kept up to date while waiting for a key).

// reserver is implemented by consoles that can keep bottom rows out of
// scrolling (the full screen).
type reserver interface{ SetReserve(n int) }

// RefreshStatus draws, or with SET STATUS OFF removes, the status bar.
// The dot prompt calls it before each prompt. It is always on window 0,
// whichever window is selected.
func RefreshStatus(m *vm.VM) {
	onWindow(m, 0, func() { refreshStatus(m) })
}

func refreshStatus(m *vm.VM) {
	r, ok := m.Con.(reserver)
	if !ok || !m.Con.Interactive() {
		return
	}
	rows, cols := m.Con.Size()
	if rows < 3 {
		return
	}
	if !m.Setting(104).Truthy() {
		if scr(m).statusShown {
			r.SetReserve(0)
			m.Con.Scroll(rows-1, 0, rows-1, cols-1, 0, m.Colors().Std)
			scr(m).statusShown = false
		}
		return
	}
	if !scr(m).statusShown {
		// make room if the cursor is on the bottom row
		if cr, cc := m.Con.Pos(); cr >= rows-1 {
			r.SetReserve(0)
			m.Con.Scroll(0, 0, rows-1, cols-1, 1, m.Colors().Std)
			m.Con.SetPos(rows-2, cc)
		}
		r.SetReserve(1)
		scr(m).statusShown = true
		if i, ok := m.Con.(interface{ SetIdle(func()) }); ok {
			i.SetIdle(func() { tickStatus(m) })
		}
	}
	scr(m).lastStatus = statusText(m, cols)
	m.Con.Put(rows-1, 0, scr(m).lastStatus, m.Theme().Status)
}

// tickStatus keeps the clock up to date while waiting for a key. It only
// redraws when the bar is still as it was drawn, so it never paints over a
// program or full screen command that has used the bottom line.
func tickStatus(m *vm.VM) {
	if !scr(m).statusShown || !m.Setting(104).Truthy() {
		return
	}
	onWindow(m, 0, func() {
		rows, cols := m.Con.Size()
		var sb strings.Builder
		for _, c := range m.Con.Save(rows-1, 0, rows-1, cols-1) {
			sb.WriteRune(c.R)
		}
		if sb.String() != scr(m).lastStatus {
			return
		}
		if txt := statusText(m, cols); txt != scr(m).lastStatus {
			scr(m).lastStatus = txt
			m.Con.Put(rows-1, 0, txt, m.Theme().Status)
		}
	})
}

// statusText builds the status bar, exactly cols characters wide.
func statusText(m *vm.VM, cols int) string {
	parts := []string{"Area " + strconv.Itoa(m.CurArea())}
	if wa := m.CurWA(); wa == nil {
		parts = append(parts, "No table in use")
	} else {
		file := strings.ToUpper(filepath.Base(wa.T.Path))
		name := file
		if base := strings.TrimSuffix(file, filepath.Ext(file)); base != wa.Alias {
			name += " (" + wa.Alias + ")"
		}
		parts = append(parts, name)
		rec := strconv.Itoa(wa.Recno)
		if wa.EOF {
			rec = "EOF"
		}
		parts = append(parts, "Rec "+rec+"/"+strconv.Itoa(wa.T.RecCount()))
		if ix := wa.CurIndex(); ix != nil {
			parts = append(parts, "Order "+strings.ToUpper(filepath.Base(ix.Path)))
		}
		if wa.FilterText != "" {
			parts = append(parts, "Filter")
		}
		if !wa.EOF && wa.Deleted() {
			parts = append(parts, "Del")
		}
	}
	left := " " + strings.Join(parts, " │ ") + " "
	now := time.Now()
	clock := m.DTOC(value.ToJulian(now.Year(), int(now.Month()), now.Day())) + " " + now.Format("15:04")
	right := " " + clock + " "
	if dir, err := os.Getwd(); err == nil {
		if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(dir, home) {
			dir = "~" + dir[len(home):]
		}
		right = " " + dir + " │" + right
	}
	lw, rw := term.DisplayLen(left), term.DisplayLen(right)
	if lw >= cols {
		return string([]rune(left)[:cols])
	}
	if room := cols - lw; rw > room {
		// keep the end of the path, which is the useful part
		if room < 6 {
			right = ""
		} else {
			rr := []rune(right)
			right = " …" + string(rr[len(rr)-(room-2):])
		}
		rw = term.DisplayLen(right)
	}
	return left + strings.Repeat(" ", cols-lw-rw) + right
}
