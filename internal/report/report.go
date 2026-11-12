// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package report reads dBase III / Clipper report (.FRM) and label (.LBL)
// definitions. The runtime prints them (REPORT FORM, LABEL FORM).
//
// Both are fixed-size binary files. The layouts follow the published
// Clipper format, which Harbour also reads. Encode writes the same layouts
// (the report and label designers, CREATE REPORT and CREATE LABEL, save
// with it).
package report

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// ErrBadForm means the file is not a report or label definition.
var ErrBadForm = errors.New("not a valid report or label file")

// ---------- reports (.FRM) ----------

// .FRM layout (offsets from 0). Expressions are stored once, NUL
// terminated, in the expression area, and referred to by number through
// two tables: their lengths (including the NUL) and their offsets in the
// area. Number 65535 means "none".
const (
	frmSize      = 1990
	frmLengths   = 4    // 55 two-byte expression lengths
	frmOffsets   = 114  // 55 two-byte expression offsets
	frmExprs     = 224  // expression area
	frmExprSize  = 1440 //
	frmFields    = 1666 // columns: column i (from 1) at frmFields + i*12
	frmParams    = 1964 // report settings
	frmMaxExprs  = 55
	frmMaxCols   = 24
	frmNoExpr    = 65535
	colWidth     = 1 - 1 // offsets within a column entry
	colTotal     = 6 - 1
	colDecimals  = 7 - 1
	colExpr      = 9 - 1
	colHeader    = 11 - 1
	parHeading   = 1 - 1 // offsets within the settings
	parGroup     = 3 - 1
	parSub       = 5 - 1
	parGroupHead = 7 - 1
	parSubHead   = 9 - 1
	parWidth     = 11 - 1
	parLines     = 13 - 1
	parLeft      = 15 - 1
	parRight     = 17 - 1
	parCols      = 19 - 1
	parDouble    = 21 - 1
	parSummary   = 22 - 1
	parGroupEj   = 23 - 1
	parOptions   = 24 - 1
)

// Column is one report column.
type Column struct {
	Header   []string // header lines
	Expr     string   // contents
	Width    int
	Decimals int
	Total    bool // total this (numeric) column
}

// Report is a report definition.
type Report struct {
	Heading     []string // page heading lines, centred
	GroupExpr   string   // group on this expression ("" for none)
	GroupHeader string
	SubExpr     string // subgroup
	SubHeader   string
	PageWidth   int
	Lines       int // lines per page
	LeftMargin  int
	RightMargin int
	DoubleSpace bool
	Summary     bool // totals only
	GroupEject  bool // new page after each group
	Plain       bool // no page number, date or page breaks
	EjectBefore bool
	EjectAfter  bool
	Columns     []Column
}

// MaxColumns is the most columns a report can have.
const MaxColumns = frmMaxCols

// NewReport returns an empty report with Clipper's default settings.
func NewReport() *Report {
	return &Report{PageWidth: 80, Lines: 58, LeftMargin: 8, EjectBefore: true}
}

func word(b []byte, off int) int { return int(binary.LittleEndian.Uint16(b[off:])) }

func yes(b byte) bool { return strings.ContainsRune("YyTt", rune(b)) }

// ParseFRM reads a .FRM file.
func ParseFRM(data []byte) (*Report, error) {
	if len(data) < frmSize || word(data, 0) != 2 {
		return nil, ErrBadForm
	}
	expr := func(n int) (string, error) {
		if n == frmNoExpr {
			return "", nil
		}
		if n < 0 || n >= frmMaxExprs {
			return "", fmt.Errorf("%w: expression %d out of range", ErrBadForm, n)
		}
		length, off := word(data, frmLengths+2*n), word(data, frmOffsets+2*n)
		if length == 0 {
			return "", nil
		}
		if off+length > frmExprSize {
			return "", fmt.Errorf("%w: expression %d runs past the expression area", ErrBadForm, n)
		}
		s := string(data[frmExprs+off : frmExprs+off+length])
		if i := strings.IndexByte(s, 0); i >= 0 {
			s = s[:i]
		}
		return s, nil
	}
	p := data[frmParams:]
	r := &Report{
		PageWidth:   word(p, parWidth),
		Lines:       word(p, parLines),
		LeftMargin:  word(p, parLeft),
		RightMargin: word(p, parRight),
		DoubleSpace: yes(p[parDouble]),
		Summary:     yes(p[parSummary]),
		GroupEject:  yes(p[parGroupEj]),
		EjectBefore: true,
	}
	opt := p[parOptions]
	r.Plain = opt&4 != 0
	r.EjectAfter = opt&2 != 0
	r.EjectBefore = opt&1 == 0
	var err error
	var heading string
	if heading, err = expr(word(p, parHeading)); err != nil {
		return nil, err
	}
	r.Heading = splitLines(heading)
	for _, f := range []struct {
		dst *string
		at  int
	}{{&r.GroupExpr, parGroup}, {&r.SubExpr, parSub}, {&r.GroupHeader, parGroupHead}, {&r.SubHeader, parSubHead}} {
		if *f.dst, err = expr(word(p, f.at)); err != nil {
			return nil, err
		}
	}
	n := word(p, parCols)
	if n > frmMaxCols {
		return nil, fmt.Errorf("%w: %d columns", ErrBadForm, n)
	}
	for i := 1; i <= n; i++ {
		c := data[frmFields+i*12:]
		col := Column{Width: word(c, colWidth), Decimals: word(c, colDecimals), Total: yes(c[colTotal])}
		if col.Expr, err = expr(word(c, colExpr)); err != nil {
			return nil, err
		}
		var h string
		if h, err = expr(word(c, colHeader)); err != nil {
			return nil, err
		}
		col.Header = splitLines(h)
		if col.Width <= 0 || col.Width > 1000 {
			return nil, fmt.Errorf("%w: column %d is %d wide", ErrBadForm, i, col.Width)
		}
		r.Columns = append(r.Columns, col)
	}
	if r.PageWidth <= 0 {
		r.PageWidth = 80
	}
	if r.Lines <= 0 {
		r.Lines = 58
	}
	return r, nil
}

// splitLines splits text on ";" into lines ("" gives none).
func splitLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ";")
}

// Encode writes the report as a .FRM file.
func (r *Report) Encode() ([]byte, error) {
	data := make([]byte, frmSize)
	put := func(off, v int) { binary.LittleEndian.PutUint16(data[off:], uint16(v)) }
	put(0, 2)
	put(frmSize-2, 2)
	next, used := 0, 0
	add := func(s string) (int, error) {
		if s == "" {
			return frmNoExpr, nil
		}
		if next >= frmMaxExprs || used+len(s)+1 > frmExprSize {
			return 0, errors.New("report has too many expressions")
		}
		n := next
		put(frmLengths+2*n, len(s)+1)
		put(frmOffsets+2*n, used)
		copy(data[frmExprs+used:], s)
		used += len(s) + 1
		next++
		return n, nil
	}
	if len(r.Columns) > frmMaxCols {
		return nil, fmt.Errorf("a report can have at most %d columns", frmMaxCols)
	}
	p := frmParams
	for _, f := range []struct {
		s  string
		at int
	}{{strings.Join(r.Heading, ";"), parHeading}, {r.GroupExpr, parGroup}, {r.SubExpr, parSub},
		{r.GroupHeader, parGroupHead}, {r.SubHeader, parSubHead}} {
		n, err := add(f.s)
		if err != nil {
			return nil, err
		}
		put(p+f.at, n)
	}
	for i, col := range r.Columns {
		if col.Width < 1 || col.Width > 1000 {
			return nil, fmt.Errorf("column %d is %d wide", i+1, col.Width)
		}
		c := frmFields + (i+1)*12
		put(c+colWidth, col.Width)
		put(c+colDecimals, col.Decimals)
		data[c+colTotal] = 'N'
		if col.Total {
			data[c+colTotal] = 'Y'
		}
		n, err := add(col.Expr)
		if err != nil {
			return nil, err
		}
		put(c+colExpr, n)
		if n, err = add(strings.Join(col.Header, ";")); err != nil {
			return nil, err
		}
		put(c+colHeader, n)
	}
	put(2, used)
	put(p+parWidth, r.PageWidth)
	put(p+parLines, r.Lines)
	put(p+parLeft, r.LeftMargin)
	put(p+parRight, r.RightMargin)
	put(p+parCols, len(r.Columns))
	yn := func(b bool) byte {
		if b {
			return 'Y'
		}
		return 'N'
	}
	data[p+parDouble] = yn(r.DoubleSpace)
	data[p+parSummary] = yn(r.Summary)
	data[p+parGroupEj] = yn(r.GroupEject)
	var opt byte
	if r.Plain {
		opt |= 4
	}
	if r.EjectAfter {
		opt |= 2
	}
	if !r.EjectBefore {
		opt |= 1
	}
	data[p+parOptions] = opt
	return data, nil
}

// ---------- labels (.LBL) ----------

const (
	lblSize    = 1034
	lblRemark  = 1  // 60 bytes
	lblHeight  = 61 // label height in lines
	lblWidth   = 63
	lblMargin  = 65
	lblLines   = 67 // blank lines between labels
	lblSpaces  = 69 // spaces between labels across
	lblAcross  = 71
	lblText    = 73 // one 60 byte line of contents per label line
	lblLineLen = 60
	lblMaxLine = 16
)

// Label is a label definition.
type Label struct {
	Remark        string
	Height        int // lines per label
	Width         int
	LeftMargin    int
	LinesBetween  int
	SpacesBetween int
	Across        int
	// Lines holds the contents of each label line: expressions separated
	// by commas, printed joined by single spaces. Lines that come out
	// empty are dropped and the rest move up.
	Lines []string
}

// Label limits.
const (
	MaxLabelLines  = lblMaxLine // lines per label
	MaxLabelLine   = lblLineLen // characters in a line's contents
	MaxLabelWidth  = 250
	MaxLabelAcross = 20
	MaxRemark      = 60
)

// NewLabel returns a label with dBase's standard size: 3 1/2 by 15/16
// inches, one across (35 characters by 5 lines).
func NewLabel() *Label {
	return &Label{Height: 5, Width: 35, LinesBetween: 1, Across: 1, Lines: make([]string, 5)}
}

// SplitLine splits a label line into its expressions: on commas that are
// not inside quotes, brackets or parentheses.
func SplitLine(s string) []string {
	var parts []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// ParseLBL reads a .LBL file.
func ParseLBL(data []byte) (*Label, error) {
	if len(data) < lblSize || data[0] != 2 {
		return nil, ErrBadForm
	}
	l := &Label{
		Remark:        strings.TrimRight(string(data[lblRemark:lblRemark+60]), "\x00 "),
		Height:        word(data, lblHeight),
		Width:         word(data, lblWidth),
		LeftMargin:    word(data, lblMargin),
		LinesBetween:  word(data, lblLines),
		SpacesBetween: word(data, lblSpaces),
		Across:        word(data, lblAcross),
	}
	if l.Height < 1 || l.Height > lblMaxLine || l.Width < 1 || l.Width > MaxLabelWidth || l.Across < 1 || l.Across > MaxLabelAcross {
		return nil, fmt.Errorf("%w: label size %d x %d, %d across", ErrBadForm, l.Width, l.Height, l.Across)
	}
	for i := 0; i < l.Height; i++ {
		off := lblText + i*lblLineLen
		l.Lines = append(l.Lines, strings.TrimRight(string(data[off:off+lblLineLen]), "\x00 "))
	}
	return l, nil
}

// Encode writes the label as a .LBL file.
func (l *Label) Encode() ([]byte, error) {
	if l.Height < 1 || l.Height > lblMaxLine || len(l.Lines) > l.Height {
		return nil, fmt.Errorf("a label has 1 to %d lines", lblMaxLine)
	}
	if l.Width < 1 || l.Width > MaxLabelWidth || l.Across < 1 || l.Across > MaxLabelAcross {
		return nil, fmt.Errorf("a label is 1 to %d wide, 1 to %d across", MaxLabelWidth, MaxLabelAcross)
	}
	if len(l.Remark) > MaxRemark {
		return nil, fmt.Errorf("the remark is longer than %d characters", MaxRemark)
	}
	data := make([]byte, lblSize)
	for i := range data[lblRemark:lblText] {
		data[lblRemark+i] = ' '
	}
	data[0], data[lblSize-1] = 2, 2
	copy(data[lblRemark:lblRemark+60], l.Remark)
	put := func(off, v int) { binary.LittleEndian.PutUint16(data[off:], uint16(v)) }
	put(lblHeight, l.Height)
	put(lblWidth, l.Width)
	put(lblMargin, l.LeftMargin)
	put(lblLines, l.LinesBetween)
	put(lblSpaces, l.SpacesBetween)
	put(lblAcross, l.Across)
	for i := 0; i < l.Height; i++ {
		off := lblText + i*lblLineLen
		line := ""
		if i < len(l.Lines) {
			line = l.Lines[i]
		}
		if len(line) > lblLineLen {
			return nil, fmt.Errorf("label line %d is longer than %d characters", i+1, lblLineLen)
		}
		copy(data[off:off+lblLineLen], line+strings.Repeat(" ", lblLineLen-len(line)))
	}
	return data, nil
}
