// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package editor implements the full screen text editor used by
// MODIFY COMMAND and MemoEdit().
package editor

import (
	"fmt"
	"strconv"
	"strings"

	"nantucket/internal/term"
)

type Options struct {
	Text       string
	Title      string
	Top, Left  int
	Bottom     int // 0 = full screen
	Right      int
	Attr       term.Attr
	StatusAttr term.Attr
	HelpAttr   term.Attr // help popup
	ReadOnly   bool
	Status     bool // show the status line
	Highlight  bool // xBase syntax colouring
	TabWidth   int
	// OnSave is called by F2 (save and keep editing).
	OnSave func(text string) error
	// Check, if set, is called when the user saves and exits. If it reports
	// a problem (line > 0), the editor stays open on that line and shows
	// the message; saving and exiting again without changes leaves anyway.
	Check func(text string) (line int, msg string)
}

// pos is a place in the document.
type pos struct{ r, c int }

func (a pos) before(b pos) bool { return a.r < b.r || a.r == b.r && a.c < b.c }

// snap is an undo step: the text and cursor before an edit.
type snap struct {
	lines    []string
	row, col int
}

// mark modes
const (
	markNone  = iota
	markShift // Shift+movement: from the anchor to the cursor
	markBlock // Ctrl-K B / Ctrl-K K: fixed until hidden
)

type editor struct {
	con        term.Console
	o          Options
	lines      [][]rune
	row, col   int // cursor in the document
	top, left  int // scroll offsets
	ins        bool
	modified   bool
	crlf       bool
	find       string
	msg        string
	t, l, b, r int // text area
	statusRow  int

	undo     []snap
	lastKind string // kind of the last edit, to group typing into one undo
	markMode int
	markA    pos // anchor (markShift) or block start (markBlock)
	markB    pos // block end (markBlock)
	clip     string
	checked  string // text that last failed Check, so a second save leaves
}

// Edit runs the editor. It returns the final text and whether the user
// chose to save (Ctrl-W / Ctrl-End / F10).
func Edit(con term.Console, o Options) (string, bool) {
	rows, cols := con.Size()
	if o.Bottom == 0 && o.Right == 0 {
		o.Top, o.Left, o.Bottom, o.Right = 0, 0, rows-1, cols-1
	}
	if o.TabWidth == 0 {
		o.TabWidth = 4
	}
	if o.Attr == 0 {
		o.Attr = term.MkAttr(7, 1)
	}
	if o.StatusAttr == 0 {
		o.StatusAttr = term.MkAttr(0, 3)
	}
	if o.HelpAttr == 0 {
		o.HelpAttr = term.MkAttr(15, 3)
	}
	e := &editor{con: con, o: o, ins: true}
	e.t, e.l, e.b, e.r = o.Top, o.Left, o.Bottom, o.Right
	if o.Status {
		e.statusRow = e.b
		e.b--
	}
	e.crlf = strings.Contains(o.Text, "\r\n")
	text := strings.ReplaceAll(o.Text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\x1a")
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.ReplaceAll(ln, "\t", strings.Repeat(" ", o.TabWidth))
		e.lines = append(e.lines, []rune(ln))
	}
	if len(e.lines) > 1 && len(e.lines[len(e.lines)-1]) == 0 && strings.HasSuffix(text, "\n") {
		e.lines = e.lines[:len(e.lines)-1]
	}
	if len(e.lines) == 0 {
		e.lines = [][]rune{{}}
	}
	save := con.Save(o.Top, o.Left, o.Bottom, o.Right)
	if x, ok := con.(term.ExtendedKeys); ok {
		x.SetExtendedKeys(true)
		defer x.SetExtendedKeys(false)
	}
	defer func() {
		con.Restore(o.Top, o.Left, o.Bottom, o.Right, save)
		con.Flush()
	}()
	return e.loop()
}

func (e *editor) text() string {
	parts := make([]string, len(e.lines))
	for i, l := range e.lines {
		parts[i] = strings.TrimRight(string(l), " ")
	}
	nl := "\n"
	if e.crlf {
		nl = "\r\n"
	}
	return strings.Join(parts, nl) + nl
}

func (e *editor) height() int { return e.b - e.t + 1 }
func (e *editor) width() int  { return e.r - e.l + 1 }

func (e *editor) clampCol() {
	if e.col > len(e.lines[e.row]) {
		e.col = len(e.lines[e.row])
	}
}

func (e *editor) scroll() {
	h, w := e.height(), e.width()
	if e.row < e.top {
		e.top = e.row
	}
	if e.row >= e.top+h {
		e.top = e.row - h + 1
	}
	if e.col < e.left {
		e.left = e.col
	}
	if e.col >= e.left+w {
		e.left = e.col - w + 1
	}
}

func (e *editor) draw() {
	e.scroll()
	h, w := e.height(), e.width()
	for i := 0; i < h; i++ {
		ln := e.top + i
		var cells []rune
		var attrs []term.Attr
		if ln < len(e.lines) {
			cells = e.lines[ln]
			if e.o.Highlight {
				attrs = highlight(cells, e.o.Attr)
			}
			attrs = e.markAttrs(ln, cells, attrs)
		}
		e.drawLine(e.t+i, cells, attrs, w)
	}
	if e.o.Status {
		mode := "Ins"
		if !e.ins {
			mode = "Ovr"
		}
		mod := " "
		if e.modified {
			mod = "*"
		}
		left := fmt.Sprintf(" %s%s  Line %d/%d  Col %d  %s", e.o.Title, mod, e.row+1, len(e.lines), e.col+1, mode)
		right := "^W Save  Esc Quit  F1 Help "
		if e.msg != "" {
			right = e.msg + " "
		}
		pad := e.width() - len([]rune(left)) - len([]rune(right))
		if pad < 1 {
			pad = 1
		}
		status := left + strings.Repeat(" ", pad) + right
		rs := []rune(status)
		if len(rs) > e.width() {
			rs = rs[:e.width()]
		}
		e.con.Put(e.statusRow, e.l, string(rs), e.o.StatusAttr)
	}
	e.con.SetPos(e.t+e.row-e.top, e.l+e.col-e.left)
	e.con.Flush()
}

func (e *editor) drawLine(screenRow int, cells []rune, attrs []term.Attr, w int) {
	// group runs of the same attribute
	start := e.left
	x := 0
	for x < w {
		i := start + x
		a := e.o.Attr
		if i < len(cells) && attrs != nil {
			a = attrs[i]
		}
		j := x
		var run []rune
		for j < w {
			k := start + j
			b := e.o.Attr
			if k < len(cells) && attrs != nil {
				b = attrs[k]
			}
			if b != a {
				break
			}
			if k < len(cells) {
				run = append(run, cells[k])
			} else {
				run = append(run, ' ')
			}
			j++
		}
		e.con.Put(screenRow, e.l+x, string(run), a)
		x = j
	}
}

func (e *editor) ask(prompt string) (string, bool) {
	row := e.statusRow
	if !e.o.Status {
		row = e.b
	}
	e.con.Put(row, e.l, strings.Repeat(" ", e.width()), e.o.StatusAttr)
	e.con.Put(row, e.l, prompt, e.o.StatusAttr)
	e.con.SetPos(row, e.l+len(prompt))
	return term.ReadLine(e.con, e.o.StatusAttr, "", e.width()-len(prompt)-1, nil)
}

func (e *editor) confirm(prompt string) bool {
	row := e.statusRow
	if !e.o.Status {
		row = e.b
	}
	e.con.Put(row, e.l, strings.Repeat(" ", e.width()), e.o.StatusAttr)
	e.con.Put(row, e.l, prompt, e.o.StatusAttr)
	e.con.SetPos(row, e.l+len(prompt))
	for {
		k, ok := e.con.ReadKey(-1)
		if !ok {
			return true
		}
		switch k {
		case 'y', 'Y':
			return true
		case 'n', 'N', term.KEsc:
			return false
		}
	}
}

func (e *editor) loop() (string, bool) {
	for {
		e.draw()
		e.msg = ""
		k, ok := e.con.ReadKey(-1)
		if !ok {
			return e.text(), false
		}
		// Shift+movement selects; plain movement drops a Shift selection
		if plain, shifted := shiftedMove[k]; shifted {
			if e.markMode != markShift {
				e.markMode, e.markA = markShift, pos{e.row, e.col}
			}
			k = plain
		} else if isMove(k) && e.markMode == markShift {
			e.markMode = markNone
		}
		if isMove(k) {
			e.lastKind = ""
		}
		switch k {
		case term.KUp:
			if e.row > 0 {
				e.row--
				e.clampCol()
			}
		case term.KDown:
			if e.row < len(e.lines)-1 {
				e.row++
				e.clampCol()
			}
		case term.KLeft:
			if e.col > 0 {
				e.col--
			} else if e.row > 0 {
				e.row--
				e.col = len(e.lines[e.row])
			}
		case term.KRight:
			if e.col < len(e.lines[e.row]) {
				e.col++
			} else if e.row < len(e.lines)-1 {
				e.row++
				e.col = 0
			}
		case term.KHome:
			// first non-blank, then column 0
			fnb := 0
			for fnb < len(e.lines[e.row]) && e.lines[e.row][fnb] == ' ' {
				fnb++
			}
			if e.col == fnb {
				e.col = 0
			} else {
				e.col = fnb
			}
		case term.KEnd:
			e.col = len(e.lines[e.row])
		case term.KPgUp:
			e.row -= e.height() - 1
			if e.row < 0 {
				e.row = 0
			}
			e.top -= e.height() - 1
			if e.top < 0 {
				e.top = 0
			}
			e.clampCol()
		case term.KPgDn:
			e.row += e.height() - 1
			if e.row >= len(e.lines) {
				e.row = len(e.lines) - 1
			}
			e.top += e.height() - 1
			if e.top > len(e.lines)-1 {
				e.top = len(e.lines) - 1
			}
			e.clampCol()
		case term.KCtrlPgUp, term.KCtrlHome:
			e.row, e.col = 0, 0
		case term.KCtrlPgDn:
			e.row = len(e.lines) - 1
			e.col = len(e.lines[e.row])
		case term.KCtrlLeft:
			e.wordLeft()
		case term.KCtrlRight:
			e.wordRight()
		case term.KIns:
			e.ins = !e.ins
			if e.ins {
				e.con.SetCursor(1)
			} else {
				e.con.SetCursor(2)
			}
		case term.KCtrlW, term.KF10:
			if e.o.ReadOnly {
				return e.text(), false
			}
			if e.o.Check != nil && e.text() != e.checked {
				if line, msg := e.o.Check(e.text()); line > 0 {
					// keep the work, but show the problem
					if e.o.OnSave != nil && e.o.OnSave(e.text()) == nil {
						e.modified = false
					}
					e.checked = e.text()
					e.row, e.col = min(line, len(e.lines))-1, 0
					e.msg = fmt.Sprintf("Line %d: %s  (^W again to leave)", line, msg)
					continue
				}
			}
			return e.text(), true
		case term.KCtrlU, term.KCtrlZ:
			e.undoLast()
		case term.KCtrlK:
			e.blockCommand()
		case term.KCtrlIns:
			if a, b, ok := e.region(); ok {
				e.clip = e.textRange(a, b)
				e.msg = "Copied"
			}
		case term.KShDel:
			if a, b, ok := e.region(); ok && !e.o.ReadOnly {
				e.clip = e.textRange(a, b)
				e.snapshot("block")
				e.deleteRange(a, b)
				e.markMode = markNone
			}
		case term.KShIns:
			if e.clip != "" && !e.o.ReadOnly {
				e.snapshot("block")
				if a, b, ok := e.region(); ok && e.markMode == markShift {
					e.deleteRange(a, b)
				}
				e.markMode = markNone
				e.insertText(pos{e.row, e.col}, e.clip)
			}
		case term.KEsc, term.KCtrlQ:
			if e.modified && !e.o.ReadOnly {
				if !e.confirm("Abandon changes? (Y/N) ") {
					continue
				}
			}
			return e.text(), false
		case term.KF2:
			if e.o.OnSave != nil && !e.o.ReadOnly {
				if err := e.o.OnSave(e.text()); err != nil {
					e.msg = "Save failed: " + err.Error()
				} else {
					e.modified = false
					e.msg = "Saved"
				}
			}
		case term.KF1:
			e.help()
		case term.KF7:
			if s, ok := e.ask("Find: "); ok && s != "" {
				e.find = s
				e.findNext(false)
			}
		case term.KCtrlL:
			if e.find != "" {
				e.findNext(true)
			}
		case term.KF5:
			if s, ok := e.ask("Go to line: "); ok {
				if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
					if n < 1 {
						n = 1
					}
					if n > len(e.lines) {
						n = len(e.lines)
					}
					e.row, e.col = n-1, 0
				}
			}
		default:
			if e.o.ReadOnly {
				continue
			}
			e.editKey(k)
		}
	}
}

func (e *editor) editKey(k int) {
	kind := "edit"
	if k >= 32 && k != 127 && k != term.KShTab && k < 1000 {
		kind = "type"
	}
	if (k < 32 || k == 127) && k != term.KEnter && k != term.KBS && k != term.KDel &&
		k != term.KTab && k != term.KShTab && k != term.KCtrlY && k != term.KCtrlN && k != term.KCtrlT {
		return // not an editing key
	}
	if k >= 1000 {
		return
	}
	e.snapshot(kind)
	// a Shift selection is replaced by what is typed, or deleted by BS/Del
	if a, b, ok := e.region(); ok && e.markMode == markShift {
		e.deleteRange(a, b)
		e.markMode = markNone
		e.modified = true
		if k == term.KBS || k == term.KDel {
			return
		}
	}
	line := e.lines[e.row]
	switch k {
	case term.KEnter:
		indent := 0
		for indent < len(line) && line[indent] == ' ' {
			indent++
		}
		if indent > e.col {
			indent = e.col
		}
		rest := append([]rune(strings.Repeat(" ", indent)), line[e.col:]...)
		e.lines[e.row] = append([]rune{}, line[:e.col]...)
		e.lines = append(e.lines, nil)
		copy(e.lines[e.row+2:], e.lines[e.row+1:])
		e.lines[e.row+1] = rest
		e.row++
		e.col = indent
	case term.KBS:
		if e.col > 0 {
			e.lines[e.row] = append(line[:e.col-1], line[e.col:]...)
			e.col--
		} else if e.row > 0 {
			prev := e.lines[e.row-1]
			e.col = len(prev)
			e.lines[e.row-1] = append(prev, line...)
			e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
			e.row--
		}
	case term.KDel:
		if e.col < len(line) {
			e.lines[e.row] = append(line[:e.col], line[e.col+1:]...)
		} else if e.row < len(e.lines)-1 {
			e.lines[e.row] = append(line, e.lines[e.row+1]...)
			e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
		}
	case term.KTab:
		n := e.o.TabWidth - e.col%e.o.TabWidth
		for i := 0; i < n; i++ {
			e.insertRune(' ')
		}
	case term.KShTab:
		n := e.col % e.o.TabWidth
		if n == 0 {
			n = e.o.TabWidth
		}
		for i := 0; i < n && e.col > 0 && e.lines[e.row][e.col-1] == ' '; i++ {
			e.col--
			e.lines[e.row] = append(e.lines[e.row][:e.col], e.lines[e.row][e.col+1:]...)
		}
	case term.KCtrlY:
		if len(e.lines) > 1 {
			e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
			if e.row >= len(e.lines) {
				e.row = len(e.lines) - 1
			}
		} else {
			e.lines[0] = nil
		}
		e.col = 0
	case term.KCtrlN:
		e.lines = append(e.lines, nil)
		copy(e.lines[e.row+1:], e.lines[e.row:])
		e.lines[e.row] = []rune{}
		e.col = 0
	case term.KCtrlT:
		end := e.col
		for end < len(line) && line[end] != ' ' {
			end++
		}
		for end < len(line) && line[end] == ' ' {
			end++
		}
		e.lines[e.row] = append(line[:e.col], line[end:]...)
	default:
		if k < 32 || k == 127 {
			return
		}
		e.insertRune(rune(k))
	}
	e.modified = true
}

func (e *editor) insertRune(r rune) {
	line := e.lines[e.row]
	for len(line) < e.col {
		line = append(line, ' ')
	}
	if e.ins || e.col >= len(line) {
		line = append(line, 0)
		copy(line[e.col+1:], line[e.col:])
	}
	line[e.col] = r
	e.lines[e.row] = line
	e.col++
	e.modified = true
}

func (e *editor) wordLeft() {
	line := e.lines[e.row]
	if e.col == 0 {
		if e.row > 0 {
			e.row--
			e.col = len(e.lines[e.row])
		}
		return
	}
	c := e.col
	for c > 0 && !isWord(line[c-1]) {
		c--
	}
	for c > 0 && isWord(line[c-1]) {
		c--
	}
	e.col = c
}

func (e *editor) wordRight() {
	line := e.lines[e.row]
	if e.col >= len(line) {
		if e.row < len(e.lines)-1 {
			e.row++
			e.col = 0
		}
		return
	}
	c := e.col
	for c < len(line) && isWord(line[c]) {
		c++
	}
	for c < len(line) && !isWord(line[c]) {
		c++
	}
	e.col = c
}

func isWord(r rune) bool {
	return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127
}

func (e *editor) findNext(skip bool) {
	needle := strings.ToUpper(e.find)
	startCol := e.col
	if skip {
		startCol++
	}
	for i := 0; i < len(e.lines); i++ {
		r := (e.row + i) % len(e.lines)
		hay := strings.ToUpper(string(e.lines[r]))
		from := 0
		if i == 0 {
			from = len(string(e.lines[r][:min(startCol, len(e.lines[r]))]))
		}
		if from > len(hay) {
			continue
		}
		if j := strings.Index(hay[from:], needle); j >= 0 {
			e.row = r
			e.col = len([]rune(hay[:from+j]))
			return
		}
	}
	e.msg = "Not found: " + e.find
}

func (e *editor) help() {
	lines := []string{
		"  Editor keys",
		"",
		"  Arrows, PgUp/PgDn    move           Ctrl-Left/Right  word left/right",
		"  Home / End           line start/end Ctrl-PgUp/PgDn   top/bottom",
		"  Ins                  insert/overtype",
		"  Ctrl-Y               delete line    Ctrl-N           insert line",
		"  Ctrl-T               delete word    Tab / Shift-Tab  indent",
		"  F7                   find           Ctrl-L           find again",
		"  F5                   go to line     F2               save, keep editing",
		"  Ctrl-Z, Ctrl-U       undo           Shift+arrows     select",
		"  Ctrl-Ins / Shift-Del copy / cut     Shift-Ins        paste",
		"  Ctrl-K B / K         mark block start / end",
		"  Ctrl-K C / V / Y / H copy / move / delete / hide block",
		"  Ctrl-W, Ctrl-End, F10  save and exit",
		"  Esc, Ctrl-Q          exit without saving",
		"",
		"  Press any key...",
	}
	rows, cols := e.con.Size()
	w := 76
	if w > cols {
		w = cols
	}
	h := len(lines) + 2
	top := (rows - h) / 2
	left := (cols - w) / 2
	save := e.con.Save(top, left, top+h-1, left+w-1)
	at := e.o.HelpAttr
	e.con.Scroll(top, left, top+h-1, left+w-1, 0, at)
	for i, l := range lines {
		e.con.Put(top+1+i, left+1, l, at)
	}
	e.con.Flush()
	e.con.ReadKey(-1)
	e.con.Restore(top, left, top+h-1, left+w-1, save)
}

// ---------- syntax highlighting ----------

var keywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`IF ELSE ELSEIF ENDIF END DO WHILE ENDDO CASE OTHERWISE ENDCASE FOR NEXT TO STEP EACH IN
		FUNCTION PROCEDURE RETURN LOCAL STATIC PRIVATE PUBLIC PARAMETERS MEMVAR FIELD EXIT LOOP
		BEGIN SEQUENCE RECOVER USING BREAK SWITCH ENDSWITCH TRY CATCH FINALLY ENDTRY WITH
		USE SELECT GO GOTO SKIP APPEND BLANK REPLACE DELETE RECALL PACK ZAP LIST DISPLAY LOCATE CONTINUE
		SEEK FIND INDEX ON REINDEX SET CLEAR CLOSE STORE READ SAY GET PICTURE VALID WHEN COLOR
		ACCEPT INPUT WAIT QUIT CANCEL COUNT SUM AVERAGE TOTAL COPY SORT CREATE MODIFY COMMAND
		TEXT ENDTEXT NIL ALL REST FOR`) {
		keywords[k] = true
	}
}

func highlight(line []rune, base term.Attr) []term.Attr {
	attrs := make([]term.Attr, len(line))
	for i := range attrs {
		attrs[i] = base
	}
	bg := base.Bg()
	kw := term.MkAttr(15, bg)
	str := term.MkAttr(14, bg)
	com := term.MkAttr(3, bg)
	num := term.MkAttr(11, bg)
	if bg == 3 {
		com = term.MkAttr(8, bg)
	}
	// whole-line comments
	trim := strings.TrimLeft(string(line), " ")
	if strings.HasPrefix(trim, "*") || strings.HasPrefix(trim, "//") || strings.HasPrefix(strings.ToUpper(trim), "NOTE ") {
		for i := range attrs {
			attrs[i] = com
		}
		return attrs
	}
	first := true
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case c == '"' || c == '\'' || (c == '[' && (i == 0 || !isWord(line[i-1]) && line[i-1] != ']' && line[i-1] != ')')):
			end := c
			if c == '[' {
				end = ']'
			}
			j := i + 1
			for j < len(line) && line[j] != end {
				j++
			}
			if j < len(line) {
				j++
			}
			for k := i; k < j; k++ {
				attrs[k] = str
			}
			i = j
		case c == '&' && i+1 < len(line) && line[i+1] == '&', c == '/' && i+1 < len(line) && line[i+1] == '/':
			for k := i; k < len(line); k++ {
				attrs[k] = com
			}
			return attrs
		case c >= '0' && c <= '9':
			j := i
			for j < len(line) && (line[j] >= '0' && line[j] <= '9' || line[j] == '.') {
				j++
			}
			for k := i; k < j; k++ {
				attrs[k] = num
			}
			i = j
		case isWord(c):
			j := i
			for j < len(line) && isWord(line[j]) {
				j++
			}
			w := strings.ToUpper(string(line[i:j]))
			if keywords[w] || (first && len(w) >= 4 && prefixKeyword(w)) {
				for k := i; k < j; k++ {
					attrs[k] = kw
				}
			}
			first = false
			i = j
		case c == '?' || c == '@':
			attrs[i] = kw
			first = false
			i++
		default:
			if c != ' ' {
				first = false
			}
			i++
		}
	}
	return attrs
}

func prefixKeyword(w string) bool {
	for k := range keywords {
		if len(k) > len(w) && strings.HasPrefix(k, w) {
			return true
		}
	}
	return false
}

// ---------- undo ----------

const maxUndo = 200

// snapshot records the text before an edit. Runs of typing make a single
// undo step.
func (e *editor) snapshot(kind string) {
	if kind == "type" && e.lastKind == "type" {
		return
	}
	e.lastKind = kind
	lines := make([]string, len(e.lines))
	for i, l := range e.lines {
		lines[i] = string(l)
	}
	e.undo = append(e.undo, snap{lines, e.row, e.col})
	if len(e.undo) > maxUndo {
		e.undo = e.undo[len(e.undo)-maxUndo:]
	}
}

func (e *editor) undoLast() {
	if len(e.undo) == 0 {
		e.msg = "Nothing to undo"
		return
	}
	u := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.lines = make([][]rune, len(u.lines))
	for i, l := range u.lines {
		e.lines[i] = []rune(l)
	}
	e.row, e.col = u.row, u.col
	e.clampCol()
	if e.markMode == markShift {
		e.markMode = markNone // a Ctrl-K block stays marked, as in WordStar
	}
	e.lastKind = ""
	e.modified = true
}

// ---------- marking and blocks ----------

var shiftedMove = map[int]int{
	term.KShUp: term.KUp, term.KShDown: term.KDown, term.KShLeft: term.KLeft, term.KShRight: term.KRight,
	term.KShHome: term.KHome, term.KShEnd: term.KEnd, term.KShPgUp: term.KPgUp, term.KShPgDn: term.KPgDn,
}

func isMove(k int) bool {
	switch k {
	case term.KUp, term.KDown, term.KLeft, term.KRight, term.KHome, term.KEnd, term.KPgUp, term.KPgDn,
		term.KCtrlPgUp, term.KCtrlHome, term.KCtrlPgDn, term.KCtrlLeft, term.KCtrlRight:
		return true
	}
	return false
}

// region is the marked text, start before end.
func (e *editor) region() (pos, pos, bool) {
	var a, b pos
	switch e.markMode {
	case markShift:
		a, b = e.markA, pos{e.row, e.col}
	case markBlock:
		a, b = e.markA, e.markB
	default:
		return a, b, false
	}
	if b.before(a) {
		a, b = b, a
	}
	a, b = e.clampPos(a), e.clampPos(b)
	return a, b, a != b
}

func (e *editor) clampPos(p pos) pos {
	p.r = max(0, min(p.r, len(e.lines)-1))
	p.c = max(0, min(p.c, len(e.lines[p.r])))
	return p
}

func (e *editor) textRange(a, b pos) string {
	if a.r == b.r {
		return string(e.lines[a.r][a.c:b.c])
	}
	var sb strings.Builder
	sb.WriteString(string(e.lines[a.r][a.c:]))
	for r := a.r + 1; r < b.r; r++ {
		sb.WriteString("\n" + string(e.lines[r]))
	}
	sb.WriteString("\n" + string(e.lines[b.r][:b.c]))
	return sb.String()
}

// deleteRange removes the text between a and b and puts the cursor at a.
func (e *editor) deleteRange(a, b pos) {
	head := append([]rune{}, e.lines[a.r][:a.c]...)
	merged := append(head, e.lines[b.r][b.c:]...)
	e.lines = append(e.lines[:a.r+1], e.lines[b.r+1:]...)
	e.lines[a.r] = merged
	e.row, e.col = a.r, a.c
	e.modified = true
}

// insertText puts text at p, leaves the cursor after it and returns where
// it ends.
func (e *editor) insertText(p pos, text string) pos {
	p = e.clampPos(p)
	parts := strings.Split(text, "\n")
	line := e.lines[p.r]
	tail := append([]rune{}, line[p.c:]...)
	first := append(append([]rune{}, line[:p.c]...), []rune(parts[0])...)
	if len(parts) == 1 {
		e.lines[p.r] = append(first, tail...)
		e.row, e.col = p.r, len(first)
		e.modified = true
		return pos{e.row, e.col}
	}
	newLines := [][]rune{first}
	for _, mid := range parts[1 : len(parts)-1] {
		newLines = append(newLines, []rune(mid))
	}
	last := []rune(parts[len(parts)-1])
	end := pos{p.r + len(parts) - 1, len(last)}
	newLines = append(newLines, append(last, tail...))
	rest := append([][]rune{}, e.lines[p.r+1:]...)
	e.lines = append(append(e.lines[:p.r], newLines...), rest...)
	e.row, e.col = end.r, end.c
	e.modified = true
	return end
}

// blockCommand runs a WordStar / dBase Ctrl-K block command.
func (e *editor) blockCommand() {
	e.msg = "^K"
	e.draw()
	k, ok := e.con.ReadKey(-1)
	if !ok {
		return
	}
	e.msg = ""
	switch k {
	case 'b', 'B', 2:
		end := e.markB
		if e.markMode != markBlock {
			end = pos{e.row, e.col}
		}
		e.markMode, e.markA, e.markB = markBlock, pos{e.row, e.col}, end
	case 'k', 'K', 11:
		start := e.markA
		if e.markMode != markBlock {
			start = pos{e.row, e.col}
		}
		e.markMode, e.markA, e.markB = markBlock, start, pos{e.row, e.col}
	case 'h', 'H', 8:
		e.markMode = markNone
	case 'c', 'C', 3, 'v', 'V', 22, 'y', 'Y', 25:
		a, b, ok := e.region()
		if !ok {
			e.msg = "No block marked (^K B, ^K K)"
			return
		}
		if e.o.ReadOnly {
			return
		}
		text := e.textRange(a, b)
		cur := e.clampPos(pos{e.row, e.col})
		e.snapshot("block")
		switch k {
		case 'y', 'Y', 25:
			e.deleteRange(a, b)
			e.markMode = markNone
		case 'c', 'C', 3:
			end := e.insertText(cur, text)
			e.markMode, e.markA, e.markB = markBlock, cur, end
			e.row, e.col = cur.r, cur.c
		default: // move
			if !cur.before(a) && cur.before(b) {
				e.msg = "Can't move a block into itself"
				e.undo = e.undo[:len(e.undo)-1]
				return
			}
			e.deleteRange(a, b)
			if !cur.before(b) { // the cursor was after the block
				if cur.r == b.r {
					cur.c = a.c + (cur.c - b.c)
				}
				cur.r -= b.r - a.r
			}
			end := e.insertText(cur, text)
			e.markMode, e.markA, e.markB = markBlock, cur, end
			e.row, e.col = cur.r, cur.c
		}
	}
}

// markAttrs shows the marked text in reverse colours on line ln.
func (e *editor) markAttrs(ln int, cells []rune, attrs []term.Attr) []term.Attr {
	a, b, ok := e.region()
	if !ok || ln < a.r || ln > b.r {
		return attrs
	}
	from, to := 0, len(cells)
	if ln == a.r {
		from = a.c
	}
	if ln == b.r {
		to = b.c
	}
	if from >= to {
		return attrs
	}
	if attrs == nil {
		attrs = make([]term.Attr, len(cells))
		for i := range attrs {
			attrs[i] = e.o.Attr
		}
	}
	rev := term.MkAttr(e.o.Attr.Bg(), e.o.Attr.Fg())
	for i := from; i < to && i < len(attrs); i++ {
		attrs[i] = rev
	}
	return attrs
}
