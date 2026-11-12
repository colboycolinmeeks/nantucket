// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package term

// ReadLine edits a single line at the cursor position. Returns the text
// and false if the user pressed Esc. maxLen <= 0 means unlimited.
// history may be nil; it is navigated with Up/Down.
func ReadLine(con Console, a Attr, initial string, maxLen int, history []string) (string, bool) {
	s, _, ok := ReadLineExit(con, a, initial, maxLen, history, nil)
	return s, ok
}

// ReadLineExit is ReadLine that also stops when one of the exit keys is
// pressed, returning the text so far, the key and false (the dot prompt
// uses it for F1 and F6). exitKey is 0 otherwise.
func ReadLineExit(con Console, a Attr, initial string, maxLen int, history []string, exits map[int]bool) (s string, exitKey int, ok bool) {
	if lr, ok := con.(LineReader); ok && !con.Interactive() {
		s, ok := lr.ReadLine()
		return s, 0, ok
	}
	row, col := con.Pos()
	_, cols := con.Size()
	width := cols - col - 1
	if width < 1 {
		width = 1
	}
	buf := []rune(initial)
	pos := len(buf)
	off := 0
	hist := len(history)
	ins := true
	draw := func() {
		if pos < off {
			off = pos
		}
		if pos-off >= width {
			off = pos - width + 1
		}
		end := off + width
		if end > len(buf) {
			end = len(buf)
		}
		vis := string(buf[off:end])
		line := []rune(vis)
		for len(line) < width {
			line = append(line, ' ')
		}
		con.Put(row, col, string(line), a)
		con.SetPos(row, col+pos-off)
		con.Flush()
	}
	for {
		draw()
		k, ok := con.ReadKey(-1)
		if !ok {
			return string(buf), 0, false
		}
		if exits[k] {
			return string(buf), k, false
		}
		switch k {
		case KEnter:
			con.SetPos(row, col+len(buf)-off)
			return string(buf), 0, true
		case KEsc:
			return string(buf), 0, false
		case KLeft:
			if pos > 0 {
				pos--
			}
		case KRight:
			if pos < len(buf) {
				pos++
			}
		case KHome:
			pos = 0
		case KEnd:
			pos = len(buf)
		case KBS:
			if pos > 0 {
				buf = append(buf[:pos-1], buf[pos:]...)
				pos--
			}
		case KDel:
			if pos < len(buf) {
				buf = append(buf[:pos], buf[pos+1:]...)
			}
		case KCtrlY:
			buf = buf[:0]
			pos = 0
		case KIns:
			ins = !ins
		case KUp:
			if hist > 0 {
				hist--
				buf = []rune(history[hist])
				pos = len(buf)
			}
		case KDown:
			if hist < len(history)-1 {
				hist++
				buf = []rune(history[hist])
				pos = len(buf)
			} else {
				hist = len(history)
				buf = buf[:0]
				pos = 0
			}
		case KCtrlLeft:
			for pos > 0 && buf[pos-1] == ' ' {
				pos--
			}
			for pos > 0 && buf[pos-1] != ' ' {
				pos--
			}
		case KCtrlRight:
			for pos < len(buf) && buf[pos] != ' ' {
				pos++
			}
			for pos < len(buf) && buf[pos] == ' ' {
				pos++
			}
		default:
			if k >= 32 && k != 127 {
				r := rune(k)
				if maxLen > 0 && len(buf) >= maxLen && (ins || pos >= len(buf)) {
					con.Bell()
					continue
				}
				if ins || pos >= len(buf) {
					buf = append(buf, 0)
					copy(buf[pos+1:], buf[pos:])
				}
				buf[pos] = r
				pos++
			}
		}
	}
}
