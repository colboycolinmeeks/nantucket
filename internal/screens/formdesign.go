// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package screens

// The report and label designers: CREATE REPORT / MODIFY REPORT and
// CREATE LABEL / MODIFY LABEL. They edit .FRM and .LBL files, which
// REPORT FORM and LABEL FORM print.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nantucket/internal/forms"
	"nantucket/internal/gets"
	"nantucket/internal/report"
	"nantucket/internal/term"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

func init() {
	// __ReportDesign(cFile, lCreate)
	vm.Register("__REPORTDESIGN", func(m *vm.VM, a []value.Value) value.Value {
		designForm(m, vm.ArgStr(a, 0, ""), vm.ArgBool(a, 1, true), true)
		return value.Nil
	})
	// __LabelDesign(cFile, lCreate)
	vm.Register("__LABELDESIGN", func(m *vm.VM, a []value.Value) value.Value {
		designForm(m, vm.ArgStr(a, 0, ""), vm.ArgBool(a, 1, true), false)
		return value.Nil
	})
}

// designForm opens the report or label designer on a file, new or
// existing, and saves what the user makes.
func designForm(m *vm.VM, name string, create, isReport bool) {
	verb, ext, kind := "MODIFY", ".lbl", "LABEL"
	if create {
		verb = "CREATE"
	}
	if isReport {
		ext, kind = ".frm", "REPORT"
	}
	what := verb + " " + kind
	if !needInteractive(m, what) {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		prompt := "Label file name: "
		if isReport {
			prompt = "Report file name: "
		}
		m.Out(prompt, true)
		s, ok := term.ReadLine(m.Con, m.Colors().Std, "", 128, nil)
		if !ok || strings.TrimSpace(s) == "" {
			return
		}
		name = strings.TrimSpace(s)
	}
	path := m.FindFile(name, ext)
	if path == "" {
		path = m.DefaultPath(name, ext)
	}
	var data []byte
	if _, err := os.Stat(path); err == nil {
		if data, err = os.ReadFile(path); err != nil {
			m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2005, Description: "Open error: " + err.Error(), Filename: path})
			return
		}
	}
	title := what + "  " + filepath.Base(path)
	if isReport {
		r := report.NewReport()
		if data != nil {
			var err error
			if r, err = report.ParseFRM(data); err != nil {
				m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 32, SubCode: 2015, Description: err.Error(), Filename: path})
				return
			}
		}
		if r, ok := ReportDesigner(m, title, r); ok {
			forms.SaveForm(m, path, r.Encode)
		}
		return
	}
	l := report.NewLabel()
	if data != nil {
		var err error
		if l, err = report.ParseLBL(data); err != nil {
			m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 32, SubCode: 2015, Description: err.Error(), Filename: path})
			return
		}
	}
	if l, ok := LabelDesigner(m, title, l); ok {
		forms.SaveForm(m, path, l.Encode)
	}
}

// exprProblem says what is wrong with an expression in a form, or "".
func exprProblem(m *vm.VM, src string) string {
	if _, err := m.CompileBlockText(src); err != nil {
		return "not a valid expression: " + src
	}
	if err := m.CheckDataExpr(src); err != nil {
		return err.Error() + " (SET TRUSTED ON allows it)"
	}
	return ""
}

// clip cuts s to w characters.
func clip(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s
}

// ---------- a page of settings ----------

// setting is one line of a settings page.
type setting struct {
	label  string
	kind   byte // 'C' text, 'N' number, 'L' yes or no
	s      *string
	n      *int
	b      *bool
	lo, hi int // range of a number
	size   int // longest text (0 for no limit)
}

func (it setting) text() string {
	switch it.kind {
	case 'N':
		return strconv.Itoa(*it.n)
	case 'L':
		if *it.b {
			return "Yes"
		}
		return "No"
	}
	return *it.s
}

// settingsPage is a list of settings, one to a line, edited in place.
type settingsPage struct {
	cur, top int
}

const settingLabelX, settingValueX = 2, 26

func (p *settingsPage) draw(m *vm.VM, items []setting, y0, h int) {
	_, cols := m.Con.Size()
	std, hi := m.Colors().Std, m.Colors().Enh
	p.cur = min(max(p.cur, 0), len(items)-1)
	if p.cur < p.top {
		p.top = p.cur
	}
	if p.cur >= p.top+h {
		p.top = p.cur - h + 1
	}
	w := cols - settingValueX - 1
	for i := 0; i < h && p.top+i < len(items); i++ {
		it := items[p.top+i]
		m.Con.Put(y0+i, settingLabelX, it.label, std)
		at := std
		if p.top+i == p.cur {
			at = hi
		}
		vw := w
		if it.kind != 'C' {
			vw = 5
		} else if it.size > 0 {
			vw = min(w, it.size)
		}
		m.Con.Put(y0+i, settingValueX, vm.PadTo(clip(it.text(), vw), vw, 'R', " "), at)
	}
	m.Con.SetPos(y0+p.cur-p.top, settingValueX)
}

// key handles a key on the page. It reports whether a setting changed,
// and a message for the user.
func (p *settingsPage) key(m *vm.VM, items []setting, k int, y0 int) (changed bool, msg string) {
	it := items[p.cur]
	switch k {
	case term.KUp, term.KShTab:
		if p.cur > 0 {
			p.cur--
		}
		return
	case term.KDown, term.KTab:
		if p.cur < len(items)-1 {
			p.cur++
		}
		return
	case term.KHome, term.KCtrlPgUp:
		p.cur = 0
		return
	case term.KEnd, term.KCtrlPgDn:
		p.cur = len(items) - 1
		return
	}
	if it.kind == 'L' {
		switch {
		case k == ' ':
			*it.b = !*it.b
			return true, ""
		case k == 'y' || k == 'Y' || k == 't' || k == 'T':
			changed = !*it.b
			*it.b = true
		case k == 'n' || k == 'N' || k == 'f' || k == 'F':
			changed = *it.b
			*it.b = false
		case k == term.KEnter:
		default:
			return
		}
		if p.cur < len(items)-1 {
			p.cur++
		}
		return changed, ""
	}
	if k != term.KEnter && (k < 32 || k > 255) {
		return
	}
	if k != term.KEnter {
		m.Con.PushKeys([]int{term.KCtrlY, k})
	}
	m.Con.SetPos(y0+p.cur-p.top, settingValueX)
	s, ok := term.ReadLine(m.Con, m.Colors().Enh, it.text(), it.size, nil)
	if !ok {
		return
	}
	if it.kind == 'N' {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < it.lo || n > it.hi {
			m.Con.Bell()
			return false, fmt.Sprintf("%s must be %d to %d", it.label, it.lo, it.hi)
		}
		changed = n != *it.n
		*it.n = n
	} else {
		s = strings.TrimRight(s, " ")
		changed = s != *it.s
		*it.s = s
	}
	if p.cur < len(items)-1 {
		p.cur++
	}
	return changed, ""
}

// designerFrame draws the title, the page tabs and the key line.
func designerFrame(m *vm.VM, title string, tabs []string, page int, keys string) {
	rows, cols := m.Con.Size()
	std, hdr, hi := m.Colors().Std, m.Theme().Header, m.Colors().Enh
	m.Con.Scroll(0, 0, rows-1, cols-1, 0, std)
	m.Con.Put(0, 0, vm.PadTo(" "+title, cols, 'R', " "), hdr)
	x := 1
	for i, t := range tabs {
		at := std
		if i == page {
			at = hi
		}
		s := fmt.Sprintf(" F%d %s ", i+2, t)
		m.Con.Put(1, x, s, at)
		x += len(s) + 1
	}
	m.Con.Put(2, 0, strings.Repeat("─", cols), std)
	m.Con.Put(rows-1, 0, vm.PadTo(keys, cols, 'R', " "), hdr)
}

// abandon asks whether to leave without saving, if anything changed.
func abandon(m *vm.VM, changed bool, what string) bool {
	return !changed || gets.Alert(m, "Abandon "+what+" changes?", []string{"No", "Yes"}) == 2
}

// ---------- the report designer ----------

// dcol is a report column being edited.
type dcol struct {
	expr, head string
	width, dec int
	total      bool
}

func (c dcol) blank() bool { return strings.TrimSpace(c.expr) == "" && strings.TrimSpace(c.head) == "" }

var (
	colX     = []int{6, 38, 60, 67, 72}
	colW     = []int{30, 20, 5, 3, 5}
	colNames = []string{"Contents", "Heading", "Width", "Dec", "Total"}
)

func (c dcol) cell(i int) string {
	switch i {
	case 0:
		return c.expr
	case 1:
		return c.head
	case 2:
		if c.width == 0 {
			return ""
		}
		return strconv.Itoa(c.width)
	case 3:
		return strconv.Itoa(c.dec)
	}
	if c.total {
		return "Yes"
	}
	return "No"
}

// ReportDesigner edits a report on three pages: the page layout, groups
// and columns. It returns the report, and false if the user cancelled.
func ReportDesigner(m *vm.VM, title string, r *report.Report) (*report.Report, bool) {
	rows, cols := m.Con.Size()
	saveScr := m.Con.Save(0, 0, rows-1, cols-1)
	defer m.Con.Restore(0, 0, rows-1, cols-1, saveScr)

	heading := make([]string, max(4, len(r.Heading)))
	copy(heading, r.Heading)
	opts := *r
	var columns []dcol
	for _, c := range r.Columns {
		columns = append(columns, dcol{c.Expr, strings.Join(c.Header, ";"), c.Width, c.Decimals, c.Total})
	}
	if len(columns) == 0 {
		columns = []dcol{{}}
	}
	layout := func() []setting {
		var items []setting
		for i := range heading {
			items = append(items, setting{label: fmt.Sprintf("Page heading line %d", i+1), kind: 'C', s: &heading[i], size: 120})
		}
		return append(items,
			setting{label: "Page width", kind: 'N', n: &opts.PageWidth, lo: 1, hi: 999},
			setting{label: "Lines per page", kind: 'N', n: &opts.Lines, lo: 1, hi: 999},
			setting{label: "Left margin", kind: 'N', n: &opts.LeftMargin, lo: 0, hi: 999},
			setting{label: "Right margin", kind: 'N', n: &opts.RightMargin, lo: 0, hi: 999},
			setting{label: "Double space", kind: 'L', b: &opts.DoubleSpace},
			setting{label: "Summary only", kind: 'L', b: &opts.Summary},
			setting{label: "Plain (no page numbers)", kind: 'L', b: &opts.Plain},
			setting{label: "New page before", kind: 'L', b: &opts.EjectBefore},
			setting{label: "New page after", kind: 'L', b: &opts.EjectAfter},
		)
	}
	groups := func() []setting {
		return []setting{
			{label: "Group on expression", kind: 'C', s: &opts.GroupExpr, size: 200},
			{label: "Group heading", kind: 'C', s: &opts.GroupHeader, size: 50},
			{label: "New page after group", kind: 'L', b: &opts.GroupEject},
			{label: "Subgroup on expression", kind: 'C', s: &opts.SubExpr, size: 200},
			{label: "Subgroup heading", kind: 'C', s: &opts.SubHeader, size: 50},
		}
	}
	tabs := []string{"Layout", "Groups", "Columns"}
	pages := []*settingsPage{{}, {}}
	page := 2
	if len(r.Columns) == 0 {
		page = 0
	}
	cur, col, top := 0, 0, 0
	changed := false
	msg := ""
	listKeys := " ↑↓ move  Enter edit  Space yes/no  F2-F4 page  ^W save  Esc cancel"
	gridKeys := " Enter edit  ^N insert  ^U delete  F2-F4 page  ^W save  Esc cancel"

	// build checks the report and returns it, or says what is wrong.
	build := func() (*report.Report, string, int) {
		out := opts
		out.Heading = nil
		last := -1
		for i, h := range heading {
			if strings.TrimSpace(h) != "" {
				last = i
			}
		}
		out.Heading = append([]string(nil), heading[:last+1]...)
		for _, g := range []struct{ what, src string }{{"Group", opts.GroupExpr}, {"Subgroup", opts.SubExpr}} {
			if src := strings.TrimSpace(g.src); src != "" {
				if p := exprProblem(m, src); p != "" {
					return nil, g.what + ": " + p, 1
				}
			}
		}
		out.Columns = nil
		for i, c := range columns {
			if c.blank() {
				continue
			}
			src := strings.TrimSpace(c.expr)
			if src == "" {
				return nil, fmt.Sprintf("Column %d has no contents", i+1), 2
			}
			if p := exprProblem(m, src); p != "" {
				return nil, fmt.Sprintf("Column %d: %s", i+1, p), 2
			}
			out.Columns = append(out.Columns, report.Column{Expr: src, Header: splitHead(c.head),
				Width: max(c.width, 1), Decimals: c.dec, Total: c.total})
		}
		if len(out.Columns) == 0 {
			return nil, "Add at least one column", 2
		}
		if _, err := out.Encode(); err != nil {
			return nil, err.Error(), 2
		}
		return &out, "", 0
	}

	for {
		var items []setting
		switch page {
		case 0:
			items = layout()
		case 1:
			items = groups()
		}
		keys := listKeys
		if page == 2 {
			keys = gridKeys
		}
		designerFrame(m, title, tabs, page, keys)
		width, used := 0, 0
		for _, c := range columns {
			if !c.blank() {
				width += c.width
				used++
			}
		}
		if used > 1 {
			width += used - 1
		}
		status := fmt.Sprintf(" Columns: %d   Report width: %d   Page: %d - margins %d = %d",
			used, width, opts.PageWidth, opts.LeftMargin+opts.RightMargin, opts.PageWidth-opts.LeftMargin-opts.RightMargin)
		if msg != "" {
			status = " " + msg
		}
		at := m.Colors().Std
		if msg != "" {
			at = m.Colors().Enh
		}
		m.Con.Put(rows-2, 0, vm.PadTo(clip(status, cols), cols, 'R', " "), at)
		msg = ""
		h := rows - 6
		if page < 2 {
			pages[page].draw(m, items, 3, rows-5)
		} else {
			cur = min(max(cur, 0), len(columns)-1)
			if cur < top {
				top = cur
			}
			if cur >= top+h {
				top = cur - h + 1
			}
			std, hi := m.Colors().Std, m.Colors().Enh
			m.Con.Put(3, 0, " Num", std)
			for c, n := range colNames {
				m.Con.Put(3, colX[c], n, std)
			}
			for i := 0; i < h && top+i < len(columns); i++ {
				r := top + i
				m.Con.Put(4+i, 0, fmt.Sprintf("%4d", r+1), std)
				for c := range colX {
					at := std
					if r == cur && c == col {
						at = hi
					}
					m.Con.Put(4+i, colX[c], vm.PadTo(clip(columns[r].cell(c), colW[c]), colW[c], 'R', " "), at)
				}
			}
			m.Con.SetPos(4+cur-top, colX[col])
		}
		k := m.Inkey(-1, false)
		switch k {
		case term.KF2, term.KF3, term.KF4:
			page = map[int]int{term.KF2: 0, term.KF3: 1, term.KF4: 2}[k]
			continue
		case term.KEsc:
			if abandon(m, changed, "report") {
				return nil, false
			}
			continue
		case term.KCtrlW:
			out, problem, where := build()
			if problem != "" {
				msg = problem
				page = where
				m.Con.Bell()
				continue
			}
			return out, true
		}
		if page < 2 {
			c, mm := pages[page].key(m, items, k, 3)
			changed = changed || c
			msg = mm
			continue
		}
		// the columns grid
		c := &columns[cur]
		next := func() {
			col = 0
			if cur == len(columns)-1 {
				if c.blank() || len(columns) >= report.MaxColumns {
					return
				}
				columns = append(columns, dcol{})
			}
			cur++
		}
		switch k {
		case term.KUp:
			if cur > 0 {
				cur--
			}
		case term.KDown:
			if cur < len(columns)-1 {
				cur++
			} else if !c.blank() && len(columns) < report.MaxColumns {
				columns = append(columns, dcol{})
				cur++
				col = 0
			}
		case term.KLeft, term.KShTab:
			if col > 0 {
				col--
			}
		case term.KRight, term.KTab:
			if col < len(colX)-1 {
				col++
			}
		case term.KHome:
			col = 0
		case term.KEnd:
			col = len(colX) - 1
		case term.KPgUp:
			cur = max(cur-h, 0)
		case term.KPgDn:
			cur = min(cur+h, len(columns)-1)
		case term.KCtrlN:
			if len(columns) >= report.MaxColumns {
				msg = fmt.Sprintf("A report can have at most %d columns", report.MaxColumns)
				m.Con.Bell()
				continue
			}
			columns = append(columns, dcol{})
			copy(columns[cur+1:], columns[cur:])
			columns[cur] = dcol{}
			col = 0
			changed = true
		case term.KCtrlU:
			if len(columns) > 1 {
				columns = append(columns[:cur], columns[cur+1:]...)
			} else {
				columns[0] = dcol{}
			}
			changed = true
		default:
			if col == 4 {
				switch k {
				case ' ':
					c.total = !c.total
					changed = true
				case 'y', 'Y', 't', 'T':
					changed = changed || !c.total
					c.total = true
					next()
				case 'n', 'N', 'f', 'F':
					changed = changed || c.total
					c.total = false
					next()
				case term.KEnter:
					next()
				}
				continue
			}
			if k != term.KEnter && (k < 32 || k > 255) {
				continue
			}
			if k != term.KEnter {
				m.Con.PushKeys([]int{term.KCtrlY, k})
			}
			m.Con.SetPos(4+cur-top, colX[col])
			size := []int{200, 200, 4, 2}[col]
			s, ok := term.ReadLine(m.Con, m.Colors().Enh, c.cell(col), size, nil)
			if !ok {
				continue
			}
			before := *c
			s = strings.TrimSpace(s)
			switch col {
			case 0:
				c.expr = s
				if c.width == 0 && s != "" {
					// a new column: take its size from the field it shows
					if fc, ok := forms.FieldColumn(m, m.CurWA(), s); ok {
						c.width, c.dec, c.total = fc.Width, fc.Decimals, fc.Total
						if c.head == "" {
							c.head = strings.Join(fc.Header, ";")
						}
					} else {
						c.width = 10
					}
				}
			case 1:
				c.head = s
			case 2:
				n, err := strconv.Atoi(s)
				if err != nil || n < 1 || n > 254 {
					msg = "Width must be 1 to 254"
					m.Con.Bell()
					continue
				}
				c.width = n
			case 3:
				n, err := strconv.Atoi(s)
				if s == "" {
					n, err = 0, nil
				}
				if err != nil || n < 0 || n > 15 {
					msg = "Decimals must be 0 to 15"
					m.Con.Bell()
					continue
				}
				c.dec = n
			}
			changed = changed || *c != before
			col++
		}
	}
}

// splitHead splits a column heading on ";" into lines.
func splitHead(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ";")
}

// ---------- the label designer ----------

// LabelDesigner edits a label's size and contents. It returns the label,
// and false if the user cancelled.
func LabelDesigner(m *vm.VM, title string, l *report.Label) (*report.Label, bool) {
	rows, cols := m.Con.Size()
	saveScr := m.Con.Save(0, 0, rows-1, cols-1)
	defer m.Con.Restore(0, 0, rows-1, cols-1, saveScr)
	lab := *l
	lab.Lines = make([]string, report.MaxLabelLines)
	copy(lab.Lines, l.Lines)
	p := &settingsPage{}
	changed := false
	msg := ""
	items := func() []setting {
		list := []setting{
			{label: "Remarks", kind: 'C', s: &lab.Remark, size: report.MaxRemark},
			{label: "Label height (lines)", kind: 'N', n: &lab.Height, lo: 1, hi: report.MaxLabelLines},
			{label: "Label width", kind: 'N', n: &lab.Width, lo: 1, hi: report.MaxLabelWidth},
			{label: "Left margin", kind: 'N', n: &lab.LeftMargin, lo: 0, hi: 250},
			{label: "Lines between labels", kind: 'N', n: &lab.LinesBetween, lo: 0, hi: 16},
			{label: "Spaces between labels", kind: 'N', n: &lab.SpacesBetween, lo: 0, hi: 250},
			{label: "Labels across", kind: 'N', n: &lab.Across, lo: 1, hi: report.MaxLabelAcross},
		}
		for i := 0; i < lab.Height; i++ {
			list = append(list, setting{label: fmt.Sprintf("Line %d", i+1), kind: 'C', s: &lab.Lines[i], size: report.MaxLabelLine})
		}
		return list
	}
	for {
		list := items()
		designerFrame(m, title, nil, -1, " ↑↓ move  Enter edit  ^W save  Esc cancel   Lines: expressions, comma separated")
		m.Con.Put(1, 1, "Label size and contents", m.Colors().Std)
		status := fmt.Sprintf(" Label: %d x %d, %d across   Page width used: %d", lab.Width, lab.Height, lab.Across,
			lab.LeftMargin+lab.Across*lab.Width+(lab.Across-1)*lab.SpacesBetween)
		at := m.Colors().Std
		if msg != "" {
			status, at = " "+msg, m.Colors().Enh
		}
		m.Con.Put(rows-2, 0, vm.PadTo(clip(status, cols), cols, 'R', " "), at)
		msg = ""
		p.draw(m, list, 3, rows-5)
		k := m.Inkey(-1, false)
		switch k {
		case term.KEsc:
			if abandon(m, changed, "label") {
				return nil, false
			}
			continue
		case term.KCtrlW:
			out := lab
			out.Lines = append([]string(nil), lab.Lines[:lab.Height]...)
			problem := ""
			for i, line := range out.Lines {
				for _, part := range report.SplitLine(line) {
					if src := strings.TrimSpace(part); src != "" {
						if pr := exprProblem(m, src); pr != "" {
							problem = fmt.Sprintf("Line %d: %s", i+1, pr)
							break
						}
					}
				}
				if problem != "" {
					p.cur = 7 + i
					break
				}
			}
			if problem == "" {
				if _, err := out.Encode(); err != nil {
					problem = err.Error()
				}
			}
			if problem != "" {
				msg = problem
				m.Con.Bell()
				continue
			}
			return &out, true
		}
		c, mm := p.key(m, list, k, 3)
		changed = changed || c
		msg = mm
	}
}
