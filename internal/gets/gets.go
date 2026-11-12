// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package gets is the GET system: @ ... GET and READ, the GET object,
// @ ... PROMPT / MENU TO, Alert() and AChoice().
package gets

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"nantucket/internal/vm"

	"nantucket/internal/term"
	"nantucket/internal/value"
)

// session is the GET system's state for one runtime.
type session struct {
	read    *readState // the READ in progress
	prompts []prompt   // @ ... PROMPT items waiting for MENU TO
}

type sessionKey struct{}

func state(m *vm.VM) *session {
	return m.Ext(sessionKey{}, func() any { return &session{} }).(*session)
}

// ---------- GET object ----------

type Get struct {
	Row, Col  int
	Block     value.Value
	Name      string
	Pic       vm.Picture
	PicStr    string
	Valid     value.Value
	When      value.Value
	Lo, Hi    value.Value
	Color     term.Attr
	HasColor  bool
	Cargo     value.Value
	typ       byte
	buf       []byte
	pos       int
	changed   bool
	firstKey  bool
	tmpl      string
	exitState int
}

func (g *Get) ClassName() string { return "GET" }

// Changed reports whether the user changed the GET's value.
func (g *Get) Changed() bool { return g.changed }

func (g *Get) Send(m *vm.VM, msg string, args []value.Value) value.Value {
	switch msg {
	case "ROW":
		return value.Int(g.Row)
	case "COL":
		return value.Int(g.Col)
	case "NAME":
		return value.Str(g.Name)
	case "VARGET":
		return m.EvalBlock(g.Block)
	case "VARPUT":
		if len(args) > 0 {
			m.EvalBlock(g.Block, args[0])
		}
		return vm.Arg(args, 0)
	case "DISPLAY":
		DisplayGet(m, g, false)
		return value.Obj(g)
	case "PICTURE":
		return value.Str(g.PicStr)
	case "_PICTURE":
		g.PicStr = vm.ArgStr(args, 0, "")
		g.Pic = vm.ParsePicture(g.PicStr)
		return vm.Arg(args, 0)
	case "POSTBLOCK":
		return g.Valid
	case "_POSTBLOCK":
		g.Valid = vm.Arg(args, 0)
		return g.Valid
	case "PREBLOCK":
		return g.When
	case "_PREBLOCK":
		g.When = vm.Arg(args, 0)
		return g.When
	case "CARGO":
		return g.Cargo
	case "_CARGO":
		g.Cargo = vm.Arg(args, 0)
		return g.Cargo
	case "BLOCK":
		return g.Block
	case "BUFFER":
		return value.Str(string(g.buf))
	case "CHANGED":
		return value.Bool(g.changed)
	case "EXITSTATE":
		return value.Int(g.exitState)
	case "TYPE":
		return value.Str(string(g.typ))
	case "COLORSPEC", "_COLORSPEC":
		return value.Str("")
	}
	return m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 5, SubCode: 1004, Description: "No exported method", Operation: msg})
}

func init() {
	vm.Register("__GETNEW", func(m *vm.VM, a []value.Value) value.Value {
		r, c := m.Con.Pos()
		g := &Get{Row: r, Col: c, Block: vm.Arg(a, 0), Name: vm.ArgStr(a, 1, ""), PicStr: vm.ArgStr(a, 2, ""),
			Valid: vm.Arg(a, 3), When: vm.Arg(a, 4), Lo: vm.Arg(a, 5), Hi: vm.Arg(a, 6)}
		g.Pic = vm.ParsePicture(g.PicStr)
		if s := vm.ArgStr(a, 7, ""); s != "" {
			parts := strings.Split(s, ",")
			g.Color = term.ParseColorPair(parts[0], m.Colors().Unsel)
			g.HasColor = true
		}
		DisplayGet(m, g, false)
		return value.Obj(g)
	})
	vm.Register("GETNEW", func(m *vm.VM, a []value.Value) value.Value {
		g := &Get{Row: vm.ArgInt(a, 0, 0), Col: vm.ArgInt(a, 1, 0), Block: vm.Arg(a, 2), Name: vm.ArgStr(a, 3, ""), PicStr: vm.ArgStr(a, 4, "")}
		g.Pic = vm.ParsePicture(g.PicStr)
		return value.Obj(g)
	})
	vm.Register("READMODAL", func(m *vm.VM, a []value.Value) value.Value {
		gl := vm.Arg(a, 0)
		if gl.K != value.ARRAY {
			return value.False
		}
		var gets []*Get
		for _, it := range gl.Arr().Items {
			if g, ok := it.P.(*Get); ok {
				gets = append(gets, g)
			}
		}
		return value.Bool(ReadModal(m, gets))
	})
	vm.Register("UPDATED", func(m *vm.VM, a []value.Value) value.Value {
		return value.Bool(state(m).read != nil && state(m).read.updated)
	})
	vm.Register("READKILL", func(m *vm.VM, a []value.Value) value.Value { return value.False })
	vm.Register("READEXIT", func(m *vm.VM, a []value.Value) value.Value {
		old := m.Setting(30)
		if vm.Has(a, 0) {
			m.SetSetting(30, vm.Arg(a, 0))
		}
		return old
	})
	vm.Register("READINSERT", func(m *vm.VM, a []value.Value) value.Value {
		old := m.Setting(29)
		if vm.Has(a, 0) {
			m.SetSetting(29, vm.Arg(a, 0))
		}
		return old
	})
	vm.Register("READVAR", func(m *vm.VM, a []value.Value) value.Value {
		if state(m).read != nil && state(m).read.cur != nil {
			return value.Str(strings.ToUpper(state(m).read.cur.Name))
		}
		return value.Str("")
	})
}

type readState struct {
	updated bool
	cur     *Get
	exitKey int
	sb      *scoreboard // nil when SET SCOREBOARD is OFF
}

// getWidth/prepare build the edit buffer from the current value.
func PrepareGet(m *vm.VM, g *Get) {
	v := m.EvalBlock(g.Block)
	g.typ = v.TypeChar()[0]
	if g.typ == 'M' {
		g.typ = 'C' // memo values are edited as text
	}
	g.tmpl = g.Pic.Tmpl
	switch g.typ {
	case 'C':
		s := v.S
		if g.Pic.Has('!') {
			s = strings.ToUpper(s)
		}
		if g.tmpl != "" {
			s = vm.ApplyCharTemplate(s, g.tmpl, g.Pic.Has('R'))
		}
		g.buf = []byte(s)
	case 'N':
		if g.tmpl == "" {
			w := v.DefaultWidth()
			dec := int(v.Dec)
			if dec > 0 {
				g.tmpl = strings.Repeat("9", w-dec-1) + "." + strings.Repeat("9", dec)
			} else {
				g.tmpl = strings.Repeat("9", w)
			}
		}
		g.buf = []byte(m.Transform(v, g.PicStr))
		if len(g.buf) < len(g.tmpl) {
			g.buf = []byte(vm.PadTo(string(g.buf), len(g.tmpl), 'L', " "))
		}
	case 'D':
		g.buf = []byte(m.DTOC(v.J))
		var t strings.Builder
		for _, c := range g.buf {
			if c >= '0' && c <= '9' || c == ' ' {
				t.WriteByte('9')
			} else {
				t.WriteByte(c)
			}
		}
		g.tmpl = t.String()
	case 'L':
		g.buf = []byte(m.Transform(v, g.PicStr))
		g.tmpl = "L"
		if strings.ContainsAny(strings.ToUpper(g.PicStr), "Y") {
			g.tmpl = "Y"
		}
	default:
		g.buf = []byte(m.Display(v))
	}
	g.pos = 0
	g.firstKey = true
	g.skipLiterals(1)
}

func (g *Get) editable(i int) bool {
	if i < 0 || i >= len(g.buf) {
		return false
	}
	if g.tmpl == "" {
		return true
	}
	if i >= len(g.tmpl) {
		return g.typ == 'C' && !g.Pic.Has('R')
	}
	c := g.tmpl[i]
	switch g.typ {
	case 'N':
		return c == '9' || c == '#' || c == '$' || c == '*'
	case 'D':
		return c == '9'
	}
	return vm.IsTmplChar(c, 'C')
}

func (g *Get) skipLiterals(dir int) {
	for g.pos >= 0 && g.pos < len(g.buf) && !g.editable(g.pos) {
		g.pos += dir
	}
	if g.pos < 0 {
		g.pos = 0
		g.skipLiterals(1)
	}
}

func DisplayGet(m *vm.VM, g *Get, active bool) {
	if g.buf == nil || !active {
		prepareGetSafe(m, g)
	}
	attr := m.Colors().Unsel
	if active {
		attr = m.Colors().Enh
	}
	if !m.Setting(31).Truthy() { // SET INTENSITY OFF
		attr = m.Colors().Std
	}
	if g.HasColor && !active {
		attr = g.Color
	}
	s := string(g.buf)
	width := len(s)
	off := 0
	if g.Pic.Scroll > 0 && g.Pic.Scroll < width {
		width = g.Pic.Scroll
		if g.pos >= width {
			off = g.pos - width + 1
		}
		s = s[off : off+width]
	}
	col := g.Col
	if m.Setting(33).Truthy() {
		d := m.Setting(34).S + "  "
		m.Con.Put(g.Row, col, d[:1], m.Colors().Std)
		col++
		m.Con.Put(g.Row, col+width, d[1:2], m.Colors().Std)
	}
	m.Con.Put(g.Row, col, s, attr)
	if active {
		m.Con.SetPos(g.Row, col+g.pos-off)
	}
}

func prepareGetSafe(m *vm.VM, g *Get) {
	PrepareGet(m, g)
}

// assignGet converts the buffer back to a value and stores it.
func GetValue(m *vm.VM, g *Get) (value.Value, bool) {
	s := string(g.buf)
	switch g.typ {
	case 'C':
		if g.Pic.Has('R') && g.tmpl != "" {
			var b strings.Builder
			for i := 0; i < len(s); i++ {
				if i >= len(g.tmpl) || vm.IsTmplChar(g.tmpl[i], 'C') {
					b.WriteByte(s[i])
				}
			}
			s = b.String()
		}
		return value.Str(s), true
	case 'N':
		t := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' || r == '.' || r == '-' {
				return r
			}
			if r == ',' && g.Pic.Has('E') {
				return '.'
			}
			return -1
		}, s)
		if g.Pic.Has('E') {
			t = strings.ReplaceAll(t, ".", "")
		}
		n, _ := strconv.ParseFloat(t, 64)
		if strings.Contains(s, "(") {
			n = -math.Abs(n)
		}
		old := m.EvalBlock(g.Block)
		return value.NumW(n, int(old.Width), int(old.Dec)), true
	case 'D':
		if strings.Trim(s, " /.-") == "" {
			return value.Date(0), true
		}
		j := m.CTOD(s)
		if j == 0 {
			return value.Nil, false
		}
		return value.Date(j), true
	case 'L':
		c := strings.ToUpper(strings.TrimSpace(s))
		return value.Bool(c == "T" || c == "Y"), true
	}
	return value.Str(s), true
}

const (
	gexNone = iota
	gexUp
	gexDown
	gexEnter
	gexWrite
	GexEscape
	gexTop
	gexBottom
)

// editGet runs the keyboard loop for one GET. Returns an exit code.
func EditGet(m *vm.VM, g *Get) int {
	ins := m.Setting(29).Truthy()
	scoreboardIns(m, ins)
	for {
		DisplayGet(m, g, true)
		if ins {
			m.Con.SetCursor(2)
		} else {
			m.Con.SetCursor(1)
		}
		k := m.Inkey(-1, true)
		switch k {
		case term.KEsc:
			if !m.Setting(28).Truthy() { // SET ESCAPE OFF
				continue
			}
			return GexEscape
		case term.KEnter:
			return gexEnter
		case term.KUp, term.KShTab:
			return gexUp
		case term.KDown, term.KTab:
			return gexDown
		case term.KPgDn, term.KPgUp, term.KCtrlW:
			return gexWrite
		case term.KCtrlHome:
			return gexTop
		case term.KLeft:
			g.firstKey = false
			if g.pos > 0 {
				g.pos--
				g.skipLiterals(-1)
			}
		case term.KRight:
			g.firstKey = false
			if g.pos < len(g.buf)-1 {
				g.pos++
				g.skipLiterals(1)
				if !g.editable(g.pos) {
					g.pos--
				}
			}
		case term.KHome:
			g.firstKey = false
			g.pos = 0
			g.skipLiterals(1)
		case term.KEnd:
			g.firstKey = false
			e := len(strings.TrimRight(string(g.buf), " "))
			if e >= len(g.buf) {
				e = len(g.buf) - 1
			}
			g.pos = e
			if g.pos < 0 {
				g.pos = 0
			}
		case term.KIns:
			ins = !ins
			m.SetSetting(29, value.Bool(ins))
			scoreboardIns(m, ins)
		case term.KDel:
			g.firstKey = false
			if g.pos < len(g.buf) && g.editable(g.pos) {
				g.deleteAt(g.pos)
				g.changed = true
			}
		case term.KBS:
			g.firstKey = false
			if g.pos > 0 {
				g.pos--
				g.skipLiterals(-1)
				if g.editable(g.pos) {
					g.deleteAt(g.pos)
					g.changed = true
				}
			}
		case term.KCtrlY:
			for i := range g.buf {
				if g.editable(i) {
					g.buf[i] = ' '
				}
			}
			g.pos = 0
			g.skipLiterals(1)
			g.changed = true
		case term.KCtrlT:
			// delete word right
			for g.pos < len(g.buf) && g.buf[g.pos] != ' ' {
				g.deleteAt(g.pos)
			}
			g.changed = true
		default:
			if k < 32 || k > 255 {
				continue
			}
			c := byte(k)
			if g.typ == 'N' && (c == '.' || c == ',') && !g.Pic.Has('E') && c == '.' {
				// align integer part and move after the decimal point
				if dp := strings.IndexByte(g.tmpl, '.'); dp >= 0 {
					intPart := strings.TrimSpace(strings.Map(func(r rune) rune {
						if r >= '0' && r <= '9' || r == '-' {
							return r
						}
						return -1
					}, string(g.buf[:min(g.pos, dp)])))
					if g.firstKey {
						intPart = ""
					}
					for i := 0; i < len(g.buf); i++ {
						if g.editable(i) {
							g.buf[i] = ' '
						}
					}
					r := []byte(vm.PadTo(intPart, dp, 'L', " "))
					copy(g.buf, r)
					for i := dp + 1; i < len(g.buf); i++ {
						if g.editable(i) {
							g.buf[i] = '0'
						}
					}
					g.buf[dp] = '.'
					g.pos = dp + 1
					g.firstKey = false
					g.changed = true
					continue
				}
			}
			if !g.accept(&c) {
				getBell(m)
				continue
			}
			if g.firstKey && (g.typ == 'N' || g.Pic.Has('K')) {
				for i := range g.buf {
					if g.editable(i) {
						g.buf[i] = ' '
					}
				}
				if g.typ == 'N' {
					g.pos = 0
					g.skipLiterals(1)
				}
			}
			g.firstKey = false
			if ins && g.typ == 'C' {
				g.insertAt(g.pos, c)
			} else if g.pos < len(g.buf) {
				g.buf[g.pos] = c
			}
			g.changed = true
			g.pos++
			g.skipLiterals(1)
			if g.pos >= len(g.buf) || !g.editable(g.pos) && g.pos >= len(g.buf)-1 {
				g.pos = len(g.buf) - 1
				if g.pos < 0 {
					g.pos = 0
				}
				getBell(m)
				if !m.Setting(27).Truthy() {
					return gexDown
				}
			}
		}
	}
}

func (g *Get) deleteAt(p int) {
	// shift editable positions left until a literal
	end := p
	for end+1 < len(g.buf) && g.editable(end+1) {
		end++
	}
	copy(g.buf[p:end], g.buf[p+1:end+1])
	g.buf[end] = ' '
}

func (g *Get) insertAt(p int, c byte) {
	end := p
	for end+1 < len(g.buf) && g.editable(end+1) {
		end++
	}
	copy(g.buf[p+1:end+1], g.buf[p:end])
	g.buf[p] = c
}

func (g *Get) accept(c *byte) bool {
	t := byte('X')
	if g.tmpl != "" && g.pos < len(g.tmpl) {
		t = g.tmpl[g.pos]
	}
	ch := *c
	upper := func() { *c = byte(strings.ToUpper(string(ch))[0]) }
	if g.Pic.Has('!') {
		upper()
		ch = *c
	}
	switch g.typ {
	case 'N':
		return ch >= '0' && ch <= '9' || ch == '-' || ch == ' '
	case 'D':
		return ch >= '0' && ch <= '9' || ch == ' '
	case 'L':
		upper()
		if t == 'Y' {
			return *c == 'Y' || *c == 'N'
		}
		return strings.IndexByte("TFYN", *c) >= 0
	}
	switch t {
	case '9', '#':
		return ch >= '0' && ch <= '9' || (t == '#' && (ch == ' ' || ch == '-' || ch == '+'))
	case 'A', 'a':
		return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
	case 'N', 'n':
		return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
	case 'L', 'l':
		upper()
		return strings.IndexByte("TFYN", *c) >= 0
	case 'Y', 'y':
		upper()
		return *c == 'Y' || *c == 'N'
	case '!':
		upper()
	}
	return true
}

func evalCond(m *vm.VM, b value.Value, g *Get) bool {
	if b.K != value.BLOCK {
		return true
	}
	return m.EvalBlock(b, value.Obj(g)).Truthy()
}

// ReadModal edits a list of GETs. Returns true if anything was updated.
func ReadModal(m *vm.VM, gets []*Get) bool {
	if len(gets) == 0 {
		return false
	}
	prev := state(m).read
	rs := &readState{}
	state(m).read = rs
	defer func() {
		if prev != nil {
			prev.updated = prev.updated || rs.updated
			state(m).read = prev
		}
	}()
	if !m.Con.Interactive() {
		return readStream(m, gets, rs)
	}
	saveCursor := m.Setting(16).Int()
	defer m.Con.SetCursor(saveCursor)
	rs.sb = openScoreboard(m)
	defer closeScoreboard(m, rs.sb)
	for _, g := range gets {
		PrepareGet(m, g)
		DisplayGet(m, g, false)
	}
	pos := 0
	dir := 1
	for pos >= 0 && pos < len(gets) {
		g := gets[pos]
		rs.cur = g
		if !evalCond(m, g.When, g) {
			pos += dir
			if pos < 0 {
				pos, dir = 0, 1
			}
			continue
		}
		PrepareGet(m, g)
		g.changed = false
		code := EditGet(m, g)
		g.exitState = code
		rs.exitKey = m.LastKey()
		if code == GexEscape {
			DisplayGet(m, g, false)
			return rs.updated
		}
		if g.changed {
			v, ok := GetValue(m, g)
			if !ok {
				getBell(m)
				scoreboardMsg(m, "Invalid date")
				continue
			}
			m.EvalBlock(g.Block, v)
			rs.updated = true
		}
		// RANGE and VALID
		if g.Lo.K != value.NIL || g.Hi.K != value.NIL {
			v := m.EvalBlock(g.Block)
			if (g.Lo.K != value.NIL && m.CompareLess(v, g.Lo)) || (g.Hi.K != value.NIL && m.CompareLess(g.Hi, v)) {
				getBell(m)
				scoreboardMsg(m, fmt.Sprintf("Range: %s - %s", strings.TrimSpace(m.Display(g.Lo)), strings.TrimSpace(m.Display(g.Hi))))
				continue
			}
		}
		if code != gexUp && !evalCond(m, g.Valid, g) {
			getBell(m)
			continue
		}
		scoreboardMsg(m, "")
		PrepareGet(m, g)
		DisplayGet(m, g, false)
		switch code {
		case gexUp:
			dir = -1
			if pos > 0 {
				pos--
			} else if m.Setting(30).Truthy() {
				return rs.updated
			}
		case gexDown, gexEnter:
			dir = 1
			pos++
		case gexWrite:
			return rs.updated
		case gexTop:
			pos, dir = 0, 1
		}
	}
	return rs.updated
}

// readStream implements READ when not on a terminal: one line per GET.
func readStream(m *vm.VM, gets []*Get, rs *readState) bool {
	for _, g := range gets {
		if !evalCond(m, g.When, g) {
			continue
		}
		PrepareGet(m, g)
		line, ok := m.Con.(term.LineReader)
		if !ok {
			break
		}
		s, got := line.ReadLine()
		if !got {
			break
		}
		if s == "" {
			continue
		}
		g.buf = []byte(s)
		if g.typ == 'C' {
			old := m.EvalBlock(g.Block).S
			if len(s) < len(old) {
				g.buf = []byte(vm.PadTo(s, len(old), 'R', " "))
			}
		}
		if v, ok := GetValue(m, g); ok {
			m.EvalBlock(g.Block, v)
			rs.updated = true
		}
	}
	return rs.updated
}

func message(m *vm.VM, s string) {
	row := m.Setting(36).Int()
	if row <= 0 {
		row = 0
	}
	_, cols := m.Con.Size()
	r, c := m.Con.Pos()
	align := byte('R')          // left-aligned: pad on the right
	if m.Setting(37).Truthy() { // SET MESSAGE TO ... CENTER
		align = 'C'
	}
	m.Con.Put(row, 0, vm.PadTo(s, cols, align, " "), m.Colors().Std)
	m.Con.SetPos(r, c)
}

func clearMessage(m *vm.VM) {
	if m.Setting(36).Int() > 0 {
		message(m, "")
	}
}

// ---------- @ PROMPT / MENU TO ----------

type prompt struct {
	row, col int
	text     string
	msg      string
}

func init() {
	vm.Register("__ATPROMPT", func(m *vm.VM, a []value.Value) value.Value {
		p := prompt{vm.ArgInt(a, 0, 0), vm.ArgInt(a, 1, 0), vm.ArgStr(a, 2, ""), vm.ArgStr(a, 3, "")}
		state(m).prompts = append(state(m).prompts, p)
		m.Con.Put(p.row, p.col, p.text, m.Colors().Std)
		return value.Nil
	})
	vm.Register("__MENUTO", func(m *vm.VM, a []value.Value) value.Value {
		ps := state(m).prompts
		state(m).prompts = nil
		if len(ps) == 0 {
			return value.Int(0)
		}
		cur := m.EvalBlock(vm.Arg(a, 0)).Int()
		if cur < 1 || cur > len(ps) {
			cur = 1
		}
		if !m.Con.Interactive() {
			if lr, ok := m.Con.(term.LineReader); ok {
				s, ok := lr.ReadLine()
				if !ok {
					return value.Int(0)
				}
				if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
					return value.Int(n)
				}
			}
			return value.Int(cur)
		}
		for {
			for i, p := range ps {
				at := m.Colors().Std
				if i+1 == cur {
					at = m.Colors().Enh
				}
				m.Con.Put(p.row, p.col, p.text, at)
			}
			if ps[cur-1].msg != "" && m.Setting(36).Int() > 0 {
				message(m, ps[cur-1].msg)
			}
			m.Con.SetPos(ps[cur-1].row, ps[cur-1].col)
			k := m.Inkey(-1, true)
			switch k {
			case term.KUp, term.KLeft:
				cur--
				if cur < 1 {
					if m.Setting(35).Truthy() {
						cur = len(ps)
					} else {
						cur = 1
					}
				}
			case term.KDown, term.KRight:
				cur++
				if cur > len(ps) {
					if m.Setting(35).Truthy() {
						cur = 1
					} else {
						cur = len(ps)
					}
				}
			case term.KHome:
				cur = 1
			case term.KEnd:
				cur = len(ps)
			case term.KEnter, term.KPgDn, term.KPgUp:
				clearMessage(m)
				return value.Int(cur)
			case term.KEsc:
				clearMessage(m)
				return value.Int(0)
			default:
				if k > 32 && k < 256 {
					up := strings.ToUpper(string(rune(k)))
					for i, p := range ps {
						t := strings.TrimSpace(p.text)
						if t != "" && strings.ToUpper(t[:1]) == up {
							for j, q := range ps {
								at := m.Colors().Std
								if j == i {
									at = m.Colors().Enh
								}
								m.Con.Put(q.row, q.col, q.text, at)
							}
							return value.Int(i + 1)
						}
					}
				}
			}
		}
	})
	vm.Register("ALERT", func(m *vm.VM, a []value.Value) value.Value {
		msg := m.Display(vm.Arg(a, 0))
		var opts []string
		if o := vm.Arg(a, 1); o.K == value.ARRAY {
			for _, it := range o.Arr().Items {
				opts = append(opts, it.Deref().S)
			}
		}
		if len(opts) == 0 {
			opts = []string{"Ok"}
		}
		return value.Int(Alert(m, msg, opts))
	})
	vm.Register("ACHOICE", func(m *vm.VM, a []value.Value) value.Value {
		items := vm.Arg(a, 4)
		if items.K != value.ARRAY {
			return value.Int(0)
		}
		var list []string
		for _, it := range items.Arr().Items {
			if it.Deref().K == value.STRING && it.Deref().S == "" {
				break // an empty string ends the list, as in Clipper
			}
			list = append(list, m.Display(it))
		}
		rows, cols := m.Con.Size()
		return value.Int(AChoice(m, vm.ArgInt(a, 0, 0), vm.ArgInt(a, 1, 0), vm.ArgInt(a, 2, rows-1), vm.ArgInt(a, 3, cols-1),
			list, achoiceSelectable(m, vm.Arg(a, 5)), strings.TrimSuffix(strings.TrimSpace(vm.ArgStr(a, 6, "")), "()"),
			vm.ArgInt(a, 7, 1), vm.ArgInt(a, 8, 0)))
	})
}

// Alert shows a centered dialog and returns the chosen option (0 = Esc).
func Alert(m *vm.VM, msg string, opts []string) int {
	lines := strings.Split(msg, ";")
	if !m.Con.Interactive() {
		for _, l := range lines {
			m.Println(l)
		}
		m.Println("[" + strings.Join(opts, "] [") + "]")
		return 1
	}
	rows, cols := m.Con.Size()
	btn := ""
	for _, o := range opts {
		btn += " " + o + " "
	}
	w := len(btn) + 4
	for _, l := range lines {
		if len(l)+4 > w {
			w = len(l) + 4
		}
	}
	if w > cols-2 {
		w = cols - 2
	}
	h := len(lines) + 4
	top := (rows - h) / 2
	left := (cols - w) / 2
	save := m.Con.Save(top, left, top+h-1, left+w-1)
	defer m.Con.Restore(top, left, top+h-1, left+w-1, save)
	at := m.Theme().Alert
	hi := m.Theme().AlertHi
	m.Con.Scroll(top, left, top+h-1, left+w-1, 0, at)
	m.DispBox(top, left, top+h-1, left+w-1, value.Int(2), at)
	for i, l := range lines {
		m.Con.Put(top+1+i, left+(w-len(l))/2, l, at)
	}
	cur := 0
	for {
		x := left + (w-len(btn))/2
		for i, o := range opts {
			t := " " + o + " "
			c := at
			if i == cur {
				c = hi
			}
			m.Con.Put(top+h-2, x, t, c)
			x += len(t)
		}
		k := m.Inkey(-1, false)
		switch k {
		case term.KLeft, term.KShTab:
			if cur > 0 {
				cur--
			}
		case term.KRight, term.KTab:
			if cur < len(opts)-1 {
				cur++
			}
		case term.KEnter:
			return cur + 1
		case term.KEsc:
			return 0
		default:
			for i, o := range opts {
				if k > 32 && k < 256 && o != "" && strings.EqualFold(o[:1], string(rune(k))) {
					return i + 1
				}
			}
		}
	}
}

// AChoice is a simple scrolling pick list.
// achoiceSelectable turns AChoice's selectable argument into a test: a
// logical for all items, or an array of logicals or condition strings.
func achoiceSelectable(m *vm.VM, v value.Value) func(int) bool {
	v = v.Deref()
	switch v.K {
	case value.LOGICAL:
		all := v.B
		return func(int) bool { return all }
	case value.ARRAY:
		flags := v.Arr().Items
		return func(i int) bool {
			if i >= len(flags) {
				return true
			}
			f := flags[i].Deref()
			switch f.K {
			case value.LOGICAL:
				return f.B
			case value.STRING:
				return m.MacroEval(f.S).Truthy()
			}
			return true
		}
	}
	return nil
}

// AChoice modes passed to the user function, and its return values.
const (
	acIdle      = 0
	acHitTop    = 1
	acHitBottom = 2
	acExcept    = 3
	acNoItem    = 4

	acAbort  = 0
	acSelect = 1
	acCont   = 2
	acGoto   = 3
)

// AChoice shows a scrolling pick list and returns the chosen item (1-based)
// or 0. selectable reports whether an item can be chosen; userFunc, if set,
// is called as userFunc(nMode, nCurrent, nRow) and decides what happens on
// keys AChoice doesn't handle itself. winRow is the initial row of the
// highlight within the window.
func AChoice(m *vm.VM, t, l, b, r int, items []string, selectable func(int) bool, userFunc string, initial, winRow int) int {
	n := len(items)
	if n == 0 {
		return 0
	}
	if selectable == nil {
		selectable = func(int) bool { return true }
	}
	if !m.Con.Interactive() {
		return initial
	}
	h := b - t + 1
	w := r - l + 1
	call := func(mode, cur, top int) int {
		res := m.CallFunc(userFunc, []value.Value{value.Int(mode), value.Int(cur + 1), value.Int(cur - top)})
		if res.K != value.NUMBER {
			return acCont
		}
		return res.Int()
	}
	// next finds the nearest selectable item from i in direction d (or -1).
	next := func(i, d int) int {
		for ; i >= 0 && i < n; i += d {
			if selectable(i) {
				return i
			}
		}
		return -1
	}
	first := next(0, 1)
	if first < 0 {
		if userFunc != "" {
			call(acNoItem, 0, 0)
		}
		return 0
	}
	cur := initial - 1
	if cur < 0 || cur >= n {
		cur = 0
	}
	if c := next(cur, 1); c >= 0 {
		cur = c
	} else {
		cur = next(cur, -1)
	}
	top := max(0, min(cur-winRow, n-h))
	if top < 0 {
		top = 0
	}
	// letterJump moves to the next selectable item starting with key k.
	letterJump := func(k int) {
		if k <= 32 || k >= 256 {
			return
		}
		for i := 1; i <= n; i++ {
			j := (cur + i) % n
			s := strings.TrimSpace(items[j])
			if s != "" && selectable(j) && strings.EqualFold(s[:1], string(rune(k))) {
				cur = j
				return
			}
		}
	}
	mode := acIdle
	for {
		if cur < top {
			top = cur
		}
		if cur >= top+h {
			top = cur - h + 1
		}
		for i := 0; i < h; i++ {
			text := ""
			idx := top + i
			at := m.Colors().Std
			if idx < n {
				text = items[idx]
				if !selectable(idx) {
					at = m.Colors().Unsel
				}
			}
			if idx == cur {
				at = m.Colors().Enh
			}
			m.Con.Put(t+i, l, vm.PadTo(text, w, 'R', " "), at)
		}
		m.Con.SetPos(t+cur-top, l)
		if userFunc != "" && mode != acIdle {
			switch call(mode, cur, top) {
			case acAbort:
				return 0
			case acSelect:
				return cur + 1
			case acGoto:
				letterJump(m.LastKey())
				mode = acIdle
				continue
			}
		}
		mode = acIdle
		if userFunc != "" {
			// let the user function react while waiting for a key
			switch call(acIdle, cur, top) {
			case acAbort:
				return 0
			case acSelect:
				return cur + 1
			}
		}
		k := m.Inkey(-1, true)
		move := func(target, dir int) {
			if c := next(target, dir); c >= 0 {
				cur = c
			} else if c := next(target, -dir); c >= 0 {
				cur = c
			}
		}
		switch k {
		case term.KUp:
			if c := next(cur-1, -1); c >= 0 {
				cur = c
			} else {
				mode = acHitTop
			}
		case term.KDown:
			if c := next(cur+1, 1); c >= 0 {
				cur = c
			} else {
				mode = acHitBottom
			}
		case term.KPgUp:
			move(max(0, cur-h), 1)
		case term.KPgDn:
			move(min(n-1, cur+h), -1)
		case term.KHome, term.KCtrlHome, term.KCtrlPgUp:
			move(0, 1)
		case term.KEnd, term.KCtrlEnd, term.KCtrlPgDn:
			move(n-1, -1)
		default:
			if userFunc != "" {
				mode = acExcept
				continue
			}
			switch k {
			case term.KEnter:
				return cur + 1
			case term.KEsc, term.KLeft, term.KRight:
				return 0
			default:
				letterJump(k)
			}
		}
		if (mode == acHitTop || mode == acHitBottom) && userFunc == "" {
			mode = acIdle
		}
	}
}
