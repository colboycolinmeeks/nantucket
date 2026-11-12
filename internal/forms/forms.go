// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package forms prints the current table using dBase III / Clipper report
// (.FRM) and label (.LBL) forms: the REPORT FORM and LABEL FORM commands.
// The forms themselves are read by internal/report.
package forms

import (
	"fmt"
	"os"
	"strings"
	"time"

	"nantucket/internal/report"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

// formOut sends report lines to the screen, the printer and a file.
type formOut struct {
	m       *vm.VM
	console bool
	printer bool
	file    *os.File
}

func (o *formOut) line(s string) {
	if o.console {
		o.m.Println(s)
	}
	if o.printer {
		o.m.WritePrinter(s + "\r\n")
	}
	if o.file != nil {
		o.file.WriteString(s + "\r\n")
	}
}

// paged reports whether output goes somewhere with pages.
func (o *formOut) paged() bool { return o.printer || o.file != nil }

func (o *formOut) eject() {
	if o.printer {
		o.m.WritePrinter("\f")
	}
	if o.file != nil {
		o.file.WriteString("\f")
	}
}

// openForm reads a .FRM or .LBL file and sets up the output.
func openForm(m *vm.VM, name, ext string, toPrinter bool, toFile string, noConsole bool) ([]byte, *formOut, bool) {
	path := m.FindFile(name, ext)
	if path == "" {
		m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2005, Description: "Open error", Filename: m.DefaultPath(name, ext)})
		return nil, nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2005, Description: "Open error: " + err.Error(), Filename: path})
		return nil, nil, false
	}
	o := &formOut{m: m, console: !noConsole, printer: toPrinter}
	if strings.TrimSpace(toFile) != "" {
		f, err := os.Create(m.DefaultPath(toFile, ".txt"))
		if err != nil {
			m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 20, SubCode: 2013, Description: "Create error", Filename: toFile})
			return nil, nil, false
		}
		o.file = f
	}
	return data, o, true
}

// formBlock compiles an expression from a form, reporting which.
func formBlock(m *vm.VM, expr, what, file string) value.Value {
	if err := m.CheckDataExpr(expr); err != nil {
		m.Raise(vm.DataExprError(what, file, expr, err))
		return value.Nil
	}
	if strings.TrimSpace(expr) == "" {
		return value.Nil
	}
	b, err := m.CompileBlockText(expr)
	if err != nil {
		m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 32, SubCode: 2014, Description: fmt.Sprintf("Bad %s expression in %s: %s", what, file, expr)})
	}
	return b
}

// formText shows a value as a report does.
func formText(m *vm.VM, v value.Value, width, dec int) string {
	v = v.Deref()
	switch v.K {
	case value.NUMBER:
		return value.FormatNum(v.N, width, dec)
	case value.DATE:
		return m.DTOC(v.J)
	case value.LOGICAL:
		if v.B {
			return ".T."
		}
		return ".F."
	case value.STRING:
		return v.S
	case value.NIL:
		return ""
	}
	return m.Display(v)
}

func init() {
	// __ReportForm(cFile, lPrinter, cToFile, lNoConsole, bFor, bWhile,
	// nNext, nRecord, lRest, lPlain, cHeading, lNoEject, lSummary)
	vm.Register("__REPORTFORM", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("REPORT FORM")
		if wa == nil {
			return value.Nil
		}
		data, out, ok := openForm(m, vm.ArgStr(a, 0, ""), ".frm", vm.ArgBool(a, 1, false), vm.ArgStr(a, 2, ""), vm.ArgBool(a, 3, false))
		if !ok {
			return value.Nil
		}
		if out.file != nil {
			defer out.file.Close()
		}
		r, err := report.ParseFRM(data)
		if err != nil {
			return m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 32, SubCode: 2015, Description: err.Error(), Filename: vm.ArgStr(a, 0, "")})
		}
		if vm.ArgBool(a, 9, false) {
			r.Plain = true
		}
		if vm.ArgBool(a, 11, false) {
			r.EjectBefore = false
		}
		if vm.ArgBool(a, 12, false) {
			r.Summary = true
		}
		runReport(m, wa.Num, r, vm.ArgStr(a, 0, ""), out, vm.ArgStr(a, 10, ""), vm.ScopeArgs(a, 4))
		return value.Nil
	})
	// __LabelForm(cFile, lPrinter, cToFile, lNoConsole, bFor, bWhile,
	// nNext, nRecord, lRest, lSample)
	vm.Register("__LABELFORM", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("LABEL FORM")
		if wa == nil {
			return value.Nil
		}
		data, out, ok := openForm(m, vm.ArgStr(a, 0, ""), ".lbl", vm.ArgBool(a, 1, false), vm.ArgStr(a, 2, ""), vm.ArgBool(a, 3, false))
		if !ok {
			return value.Nil
		}
		if out.file != nil {
			defer out.file.Close()
		}
		l, err := report.ParseLBL(data)
		if err != nil {
			return m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 32, SubCode: 2015, Description: err.Error(), Filename: vm.ArgStr(a, 0, "")})
		}
		runLabels(m, wa.Num, l, vm.ArgStr(a, 0, ""), out, vm.ScopeArgs(a, 4), vm.ArgBool(a, 9, false))
		return value.Nil
	})
}

// runReport prints a report, as Clipper's REPORT FORM does: page number
// and date, headings, column headers, a line per record (character
// values wrap within their column), group and subgroup headings with
// subtotals, and a grand total.
func runReport(m *vm.VM, area int, r *report.Report, file string, out *formOut, heading string, s vm.Scope) {
	wa := m.Area(area)
	cols := make([]value.Value, len(r.Columns))
	for i, c := range r.Columns {
		cols[i] = formBlock(m, c.Expr, "column", file)
	}
	group := formBlock(m, r.GroupExpr, "group", file)
	sub := formBlock(m, r.SubExpr, "subgroup", file)
	margin := strings.Repeat(" ", r.LeftMargin)
	textWidth := max(r.PageWidth-r.LeftMargin-r.RightMargin, 1)
	center := func(s string) string {
		s = strings.TrimSpace(s)
		return strings.Repeat(" ", max((textWidth-len(s))/2, 0)) + s
	}
	hasTotals := false
	for _, c := range r.Columns {
		hasTotals = hasTotals || c.Total
	}

	page, lines := 0, 0
	emit := func(s string) {
		out.line(strings.TrimRight(margin+s, " "))
		lines++
	}
	header := func() {
		page++
		lines = 0
		if !r.Plain {
			emit(fmt.Sprintf("Page No. %5d", page))
			now := time.Now()
			emit(m.DTOC(value.ToJulian(now.Year(), int(now.Month()), now.Day())))
		}
		for _, h := range splitHeading(heading) {
			emit(center(h))
		}
		for _, h := range r.Heading {
			emit(center(h))
		}
		emit("")
		depth := 0
		for _, c := range r.Columns {
			depth = max(depth, len(c.Header))
		}
		for i := 0; i < depth; i++ {
			var sb strings.Builder
			for k, c := range r.Columns {
				h := ""
				if i < len(c.Header) {
					h = strings.TrimSpace(c.Header[i])
				}
				side := byte('R') // text: header on the left
				if c.Decimals > 0 || c.Total {
					side = 'L' // numbers: on the right
				}
				if k > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(vm.PadTo(h, c.Width, side, " "))
			}
			emit(sb.String())
		}
		emit("")
	}
	// newLine starts a new page first when the current one is full
	newLine := func() {
		if out.paged() && !r.Plain && lines >= r.Lines {
			out.eject()
			header()
		}
	}
	line := func(s string) {
		newLine()
		emit(s)
	}
	totalsLine := func(sums []float64) string {
		var sb strings.Builder
		for k, c := range r.Columns {
			if k > 0 {
				sb.WriteString(" ")
			}
			if c.Total {
				sb.WriteString(value.FormatNum(sums[k], c.Width, c.Decimals))
			} else {
				sb.WriteString(strings.Repeat(" ", c.Width))
			}
		}
		return sb.String()
	}
	grand := make([]float64, len(cols))
	groupSums := make([]float64, len(cols))
	subSums := make([]float64, len(cols))
	var groupKey, subKey string
	started := false
	closeSub := func() {
		if sub.K == value.BLOCK && started && hasTotals {
			line("* Subsubtotal *")
			line(totalsLine(subSums))
			line("")
		}
		subSums = make([]float64, len(cols))
	}
	closeGroup := func() {
		closeSub()
		if group.K == value.BLOCK && started && hasTotals {
			line("** Subtotal **")
			line(totalsLine(groupSums))
			line("")
		}
		groupSums = make([]float64, len(cols))
	}

	if r.EjectBefore && out.paged() && !r.Plain {
		out.eject()
	}
	header()
	sel := func(b value.Value) string { return formText(m, m.AreaEval(area, b), 0, 0) }
	m.EachRecord(wa, s, func() bool {
		if group.K == value.BLOCK {
			if k := sel(group); !started || k != groupKey {
				if started {
					closeGroup()
					if r.GroupEject && out.paged() {
						out.eject()
						header()
					}
				}
				groupKey, subKey = k, ""
				line("** " + strings.TrimSpace(r.GroupHeader+" "+k))
				if sub.K == value.BLOCK {
					subKey = sel(sub)
					line("* " + strings.TrimSpace(r.SubHeader+" "+subKey))
				}
			}
		}
		if sub.K == value.BLOCK && started {
			if k := sel(sub); k != subKey {
				closeSub()
				subKey = k
				line("* " + strings.TrimSpace(r.SubHeader+" "+k))
			}
		}
		started = true
		// the record: each column, wrapped within its width
		cells := make([][]string, len(cols))
		height := 1
		for k, c := range r.Columns {
			v := m.AreaEval(area, cols[k]).Deref()
			if v.K == value.NUMBER && c.Total {
				grand[k] += v.N
				groupSums[k] += v.N
				subSums[k] += v.N
			}
			text := formText(m, v, c.Width, c.Decimals)
			if v.K == value.STRING && len(text) > c.Width {
				cells[k] = vm.MemoLines(strings.TrimRight(text, " "), c.Width)
			} else {
				cells[k] = []string{text}
			}
			height = max(height, len(cells[k]))
		}
		if !r.Summary {
			for i := 0; i < height; i++ {
				var sb strings.Builder
				for k, c := range r.Columns {
					if k > 0 {
						sb.WriteString(" ")
					}
					piece := ""
					if i < len(cells[k]) {
						piece = cells[k][i]
					}
					side := byte('R')
					if c.Decimals > 0 || c.Total {
						side = 'L'
					}
					sb.WriteString(vm.PadTo(piece, c.Width, side, " "))
				}
				line(sb.String())
			}
			if r.DoubleSpace {
				line("")
			}
		}
		return true
	})
	closeGroup()
	if hasTotals {
		line("*** Total ***")
		line(totalsLine(grand))
	}
	if r.EjectAfter && out.paged() {
		out.eject()
	}
}

func splitHeading(h string) []string {
	if strings.TrimSpace(h) == "" {
		return nil
	}
	return strings.Split(h, ";")
}

// runLabels prints labels: each label line is a list of expressions
// separated by commas, joined with single spaces; lines that come out
// empty are dropped and the rest move up. SAMPLE first prints a row of
// labels filled with asterisks, for lining up the paper.
func runLabels(m *vm.VM, area int, l *report.Label, file string, out *formOut, s vm.Scope, sample bool) {
	wa := m.Area(area)
	blocks := make([][]value.Value, len(l.Lines))
	for i, line := range l.Lines {
		for _, part := range report.SplitLine(line) {
			if strings.TrimSpace(part) != "" {
				blocks[i] = append(blocks[i], formBlock(m, part, "label", file))
			}
		}
	}
	margin := strings.Repeat(" ", l.LeftMargin)
	var row [][]string
	flush := func() {
		if len(row) == 0 {
			return
		}
		for i := 0; i < l.Height; i++ {
			var sb strings.Builder
			sb.WriteString(margin)
			for k, lab := range row {
				if k > 0 {
					sb.WriteString(strings.Repeat(" ", l.SpacesBetween))
				}
				sb.WriteString(vm.PadTo(lab[i], l.Width, 'R', " "))
			}
			out.line(strings.TrimRight(sb.String(), " "))
		}
		for i := 0; i < l.LinesBetween; i++ {
			out.line("")
		}
		row = nil
	}
	add := func(lab []string) {
		row = append(row, lab)
		if len(row) == l.Across {
			flush()
		}
	}
	if sample {
		stars := make([]string, l.Height)
		for i := range stars {
			stars[i] = strings.Repeat("*", l.Width)
		}
		for i := 0; i < l.Across; i++ {
			add(stars)
		}
	}
	m.EachRecord(wa, s, func() bool {
		var lab []string
		for _, items := range blocks {
			var parts []string
			for _, b := range items {
				if t := strings.TrimSpace(formText(m, m.AreaEval(area, b), 0, 0)); t != "" {
					parts = append(parts, t)
				}
			}
			if len(parts) > 0 {
				lab = append(lab, strings.Join(parts, " "))
			}
		}
		for len(lab) < l.Height {
			lab = append(lab, "")
		}
		add(lab)
		return true
	})
	flush()
}
