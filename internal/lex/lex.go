// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package lex tokenizes xBase source into statements.
package lex

import (
	"fmt"
	"strconv"
	"strings"
)

type Kind uint8

const (
	EOF Kind = iota
	Ident
	Number
	String
	Logical
	Date
	Op
	Macro
	Directive // whole preprocessor line in Text
)

func (k Kind) String() string {
	return [...]string{"EOF", "Ident", "Number", "String", "Logical", "Date", "Op", "Macro", "Directive"}[k]
}

type Token struct {
	Kind  Kind
	Text  string // canonical: upper ident, op canonical form, string value, YYYYMMDD
	Raw   string // source text
	Num   float64
	Dec   int
	Space bool // preceded by whitespace
	Line  int
}

func (t Token) String() string { return t.Raw }

// Is reports whether the token is the given op.
func (t Token) Is(op string) bool { return t.Kind == Op && t.Text == op }

// Stmt is one logical statement (a source line, or part of one split by ';').
type Stmt struct {
	Toks []Token
	Line int
	File string
}

type Error struct {
	File string
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.File != "" {
		return fmt.Sprintf("%s(%d): %s", e.File, e.Line, e.Msg)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

type lexer struct {
	src   string
	pos   int
	line  int
	file  string
	stmts []Stmt
	cur   []Token
	start int // line where current stmt started
	space bool
}

// Lex splits source into statements.
func Lex(src, file string) ([]Stmt, error) {
	// Ctrl-Z (0x1A) is the DOS end-of-file marker; dBase and Clipper
	// ignore it and anything after it.
	if i := strings.IndexByte(src, 0x1A); i >= 0 {
		src = src[:i]
	}
	lx := &lexer{src: strings.ReplaceAll(src, "\r\n", "\n"), line: 1, file: file, start: 1}
	if err := lx.run(); err != nil {
		return nil, err
	}
	return lx.stmts, nil
}

// LexLine tokenizes a single fragment (no statement splitting).
func LexLine(src string) ([]Token, error) {
	st, err := Lex(src, "")
	if err != nil {
		return nil, err
	}
	var out []Token
	for i, s := range st {
		if i > 0 {
			out = append(out, Token{Kind: Op, Text: ";", Raw: ";", Space: true})
		}
		out = append(out, s.Toks...)
	}
	return out, nil
}

func (lx *lexer) errf(format string, a ...any) error {
	return &Error{File: lx.file, Line: lx.line, Msg: fmt.Sprintf(format, a...)}
}

func (lx *lexer) flush() {
	if len(lx.cur) > 0 {
		lx.stmts = append(lx.stmts, Stmt{Toks: lx.cur, Line: lx.start, File: lx.file})
		lx.checkText()
	}
	lx.cur = nil
	lx.start = lx.line
}

func (lx *lexer) emit(t Token) {
	if len(lx.cur) == 0 {
		lx.start = lx.line
	}
	t.Space = lx.space || len(lx.cur) == 0
	t.Line = lx.line
	lx.cur = append(lx.cur, t)
	lx.space = false
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}
func isIdentChar(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }
func isDigit(c byte) bool     { return c >= '0' && c <= '9' }

func (lx *lexer) peek(off int) byte {
	if lx.pos+off < len(lx.src) {
		return lx.src[lx.pos+off]
	}
	return 0
}

// restOfLineBlank reports whether only whitespace/comments remain on this line.
func (lx *lexer) restOfLineBlank(p int) bool {
	for p < len(lx.src) {
		c := lx.src[p]
		switch {
		case c == '\n':
			return true
		case c == ' ' || c == '\t' || c == '\r':
			p++
		case c == '&' && p+1 < len(lx.src) && lx.src[p+1] == '&':
			return true
		case c == '/' && p+1 < len(lx.src) && lx.src[p+1] == '/':
			return true
		default:
			return false
		}
	}
	return true
}

func (lx *lexer) skipToEOL() {
	for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' {
		lx.pos++
	}
}

// operators, longest first
var ops = []string{
	"**=", "...", "??", "^=", "%=", "+=", "-=", "*=", "/=", ":=", "==", "!=", "<>", "<=", ">=", "->", "++", "--", "**", "=>", "::",
	"+", "-", "*", "/", "%", "^", "=", "<", ">", "#", "$", "(", ")", "[", "]", "{", "}", ",", ":", "|", "@", "!", ".", "&", "\\", "~", "?",
}

func canonOp(s string) string {
	switch s {
	case "<>", "#":
		return "!="
	case "!":
		return ".NOT."
	case "^":
		return "**"
	case "^=":
		return "**="
	}
	return s
}

// bracketIsString decides whether '[' starts a string.
func (lx *lexer) bracketIsString() bool {
	if len(lx.cur) == 0 {
		return true
	}
	p := lx.cur[len(lx.cur)-1]
	switch p.Kind {
	case Ident, Macro:
		return false
	case Op:
		return !(p.Text == ")" || p.Text == "]" || p.Text == "}")
	}
	return true
}

func (lx *lexer) run() error {
	for {
		// start of statement handling
		if len(lx.cur) == 0 {
			// skip whitespace
			for lx.pos < len(lx.src) && (lx.src[lx.pos] == ' ' || lx.src[lx.pos] == '\t' || lx.src[lx.pos] == '\r') {
				lx.pos++
			}
			if lx.pos >= len(lx.src) {
				break
			}
			c := lx.src[lx.pos]
			if c == '*' && !(lx.peek(1) == '=') {
				lx.skipToEOL()
				continue
			}
			if c == '#' && isIdentStart(lx.peek(1)) || c == '#' && lx.peek(1) == ' ' {
				if err := lx.directive(); err != nil {
					return err
				}
				continue
			}
			if c == 'n' || c == 'N' {
				if len(lx.src) >= lx.pos+4 && strings.EqualFold(lx.src[lx.pos:lx.pos+4], "note") && !isIdentChar(lx.peek(4)) {
					lx.skipToEOL()
					continue
				}
			}
			lx.start = lx.line
		}
		if lx.pos >= len(lx.src) {
			break
		}
		c := lx.src[lx.pos]
		switch {
		case c == '\n':
			lx.pos++
			lx.line++
			lx.flush()
			lx.start = lx.line
			lx.space = true
		case c == ' ' || c == '\t' || c == '\r':
			lx.pos++
			lx.space = true
		case c == '&' && lx.peek(1) == '&':
			lx.skipToEOL()
		case c == '/' && lx.peek(1) == '/':
			lx.skipToEOL()
		case c == '/' && lx.peek(1) == '*':
			end := strings.Index(lx.src[lx.pos+2:], "*/")
			if end < 0 {
				return lx.errf("unterminated comment")
			}
			lx.line += strings.Count(lx.src[lx.pos:lx.pos+2+end], "\n")
			lx.pos += end + 4
			lx.space = true
		case c == ';':
			if lx.restOfLineBlank(lx.pos + 1) {
				// continuation
				lx.skipToEOL()
				if lx.pos < len(lx.src) {
					lx.pos++
					lx.line++
				}
				lx.space = true
			} else {
				lx.pos++
				lx.flush()
				lx.space = true
			}
		case c == '"' || c == '\'':
			end := strings.IndexByte(lx.src[lx.pos+1:], c)
			nl := strings.IndexByte(lx.src[lx.pos+1:], '\n')
			if end < 0 || (nl >= 0 && nl < end) {
				return lx.errf("unterminated string")
			}
			s := lx.src[lx.pos+1 : lx.pos+1+end]
			lx.emit(Token{Kind: String, Text: s, Raw: lx.src[lx.pos : lx.pos+end+2]})
			lx.pos += end + 2
		case c == '[' && lx.bracketIsString():
			end := strings.IndexByte(lx.src[lx.pos+1:], ']')
			nl := strings.IndexByte(lx.src[lx.pos+1:], '\n')
			if end < 0 || (nl >= 0 && nl < end) {
				return lx.errf("unterminated string")
			}
			s := lx.src[lx.pos+1 : lx.pos+1+end]
			lx.emit(Token{Kind: String, Text: s, Raw: lx.src[lx.pos : lx.pos+end+2]})
			lx.pos += end + 2
		case isDigit(c) || (c == '.' && isDigit(lx.peek(1))):
			if err := lx.number(); err != nil {
				return err
			}
		case c == '.' && lx.dotWord():
			// handled
		case isIdentStart(c):
			st := lx.pos
			for lx.pos < len(lx.src) && isIdentChar(lx.src[lx.pos]) {
				lx.pos++
			}
			if lx.pos < len(lx.src) && lx.src[lx.pos] == '&' && isIdentStart(lx.peek(1)) {
				// name&suffix: an identifier built by a macro, e.g. opt&sub
				lx.macroRun()
				raw := lx.src[st:lx.pos]
				lx.emit(Token{Kind: Macro, Text: raw, Raw: raw})
				break
			}
			raw := lx.src[st:lx.pos]
			lx.emit(Token{Kind: Ident, Text: strings.ToUpper(raw), Raw: raw})
		case c == '&' && (isIdentStart(lx.peek(1))):
			st := lx.pos
			lx.macroRun()
			raw := lx.src[st:lx.pos]
			lx.emit(Token{Kind: Macro, Text: raw, Raw: raw})
		default:
			matched := false
			for _, op := range ops {
				if strings.HasPrefix(lx.src[lx.pos:], op) {
					lx.emit(Token{Kind: Op, Text: canonOp(op), Raw: op})
					lx.pos += len(op)
					matched = true
					break
				}
			}
			if !matched {
				return lx.errf("unexpected character %q", c)
			}
		}
	}
	lx.flush()
	return nil
}

// macroRun consumes &name[.][suffix][&name2...] with no whitespace.
func (lx *lexer) macroRun() {
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		if c == '&' && isIdentStart(lx.peek(1)) {
			lx.pos++
			for lx.pos < len(lx.src) && isIdentChar(lx.src[lx.pos]) {
				lx.pos++
			}
			if lx.pos < len(lx.src) && lx.src[lx.pos] == '.' && !lx.isDotWordAt(lx.pos) {
				lx.pos++
			}
			continue
		}
		if isIdentChar(c) {
			lx.pos++
			continue
		}
		break
	}
}

var dotWords = []struct {
	w, canon string
	kind     Kind
}{
	{".AND.", ".AND.", Op}, {".OR.", ".OR.", Op}, {".NOT.", ".NOT.", Op},
	{".T.", ".T.", Logical}, {".F.", ".F.", Logical}, {".Y.", ".T.", Logical}, {".N.", ".F.", Logical},
}

func (lx *lexer) isDotWordAt(p int) bool {
	for _, dw := range dotWords {
		if len(lx.src) >= p+len(dw.w) && strings.EqualFold(lx.src[p:p+len(dw.w)], dw.w) {
			return true
		}
	}
	return false
}

func (lx *lexer) dotWord() bool {
	for _, dw := range dotWords {
		if len(lx.src) >= lx.pos+len(dw.w) && strings.EqualFold(lx.src[lx.pos:lx.pos+len(dw.w)], dw.w) {
			lx.emit(Token{Kind: dw.kind, Text: dw.canon, Raw: lx.src[lx.pos : lx.pos+len(dw.w)]})
			lx.pos += len(dw.w)
			return true
		}
	}
	return false
}

func (lx *lexer) number() error {
	st := lx.pos
	// 0x hex, 0d date
	if lx.src[lx.pos] == '0' && (lx.peek(1) == 'x' || lx.peek(1) == 'X') {
		lx.pos += 2
		for lx.pos < len(lx.src) && strings.IndexByte("0123456789abcdefABCDEF", lx.src[lx.pos]) >= 0 {
			lx.pos++
		}
		n, err := strconv.ParseInt(lx.src[st+2:lx.pos], 16, 64)
		if err != nil {
			return lx.errf("bad hex number")
		}
		lx.emit(Token{Kind: Number, Text: lx.src[st:lx.pos], Raw: lx.src[st:lx.pos], Num: float64(n)})
		return nil
	}
	if lx.src[lx.pos] == '0' && (lx.peek(1) == 'd' || lx.peek(1) == 'D') && isDigit(lx.peek(2)) {
		lx.pos += 2
		for lx.pos < len(lx.src) && isDigit(lx.src[lx.pos]) {
			lx.pos++
		}
		d := lx.src[st+2 : lx.pos]
		lx.emit(Token{Kind: Date, Text: d, Raw: lx.src[st:lx.pos]})
		return nil
	}
	for lx.pos < len(lx.src) && isDigit(lx.src[lx.pos]) {
		lx.pos++
	}
	dec := 0
	if lx.pos < len(lx.src) && lx.src[lx.pos] == '.' && isDigit(lx.peek(1)) {
		lx.pos++
		ds := lx.pos
		for lx.pos < len(lx.src) && isDigit(lx.src[lx.pos]) {
			lx.pos++
		}
		dec = lx.pos - ds
	}
	if lx.pos < len(lx.src) && (lx.src[lx.pos] == 'e' || lx.src[lx.pos] == 'E') &&
		(isDigit(lx.peek(1)) || ((lx.peek(1) == '-' || lx.peek(1) == '+') && isDigit(lx.peek(2)))) {
		lx.pos += 2
		for lx.pos < len(lx.src) && isDigit(lx.src[lx.pos]) {
			lx.pos++
		}
	}
	raw := lx.src[st:lx.pos]
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return lx.errf("bad number %s", raw)
	}
	lx.emit(Token{Kind: Number, Text: raw, Raw: raw, Num: n, Dec: dec})
	return nil
}

// directive reads a '#' line including continuations.
func (lx *lexer) directive() error {
	var sb strings.Builder
	line := lx.line
	for lx.pos < len(lx.src) {
		e := strings.IndexByte(lx.src[lx.pos:], '\n')
		var l string
		if e < 0 {
			l = lx.src[lx.pos:]
			lx.pos = len(lx.src)
		} else {
			l = lx.src[lx.pos : lx.pos+e]
			lx.pos += e + 1
		}
		lx.line++
		t := strings.TrimRight(l, " \t\r")
		if strings.HasSuffix(t, ";") {
			sb.WriteString(t[:len(t)-1])
			sb.WriteString(" ")
			continue
		}
		sb.WriteString(t)
		break
	}
	lx.stmts = append(lx.stmts, Stmt{Toks: []Token{{Kind: Directive, Text: sb.String(), Raw: sb.String(), Line: line}}, Line: line, File: lx.file})
	lx.start = lx.line
	return nil
}

// checkText handles TEXT ... ENDTEXT after a statement was flushed.
func (lx *lexer) checkText() {
	last := lx.stmts[len(lx.stmts)-1]
	t := last.Toks
	if len(t) == 0 || t[0].Kind != Ident || t[0].Text != "TEXT" {
		return
	}
	if len(t) > 1 && !(t[1].Kind == Ident && (t[1].Text == "TO" || t[1].Text == "INTO")) {
		return
	}
	if len(t) > 1 && t[1].Text == "INTO" {
		return // TEXT INTO var not supported here
	}
	lx.stmts = lx.stmts[:len(lx.stmts)-1]
	// TEXT TO PRINTER / TEXT TO FILE <file>: send the lines there as well
	call := func(ln int, args ...string) Stmt {
		toks := []Token{{Kind: Ident, Text: "__TEXTTO", Raw: "__TextTo", Line: ln, Space: true},
			{Kind: Op, Text: "(", Raw: "(", Line: ln}}
		for i, a := range args {
			if i > 0 {
				toks = append(toks, Token{Kind: Op, Text: ",", Raw: ",", Line: ln})
			}
			toks = append(toks, Token{Kind: String, Text: a, Raw: strconv.Quote(a), Line: ln})
		}
		return Stmt{Line: ln, File: lx.file, Toks: append(toks, Token{Kind: Op, Text: ")", Raw: ")", Line: ln})}
	}
	redirected := false
	if len(t) >= 3 && t[1].Text == "TO" {
		switch {
		case t[2].Kind == Ident && strings.HasPrefix("PRINTER", t[2].Text) && len(t[2].Text) >= 4:
			lx.stmts = append(lx.stmts, call(last.Line, "PRINTER"))
			redirected = true
		case t[2].Kind == Ident && t[2].Text == "FILE" && len(t) >= 4:
			name := t[3].Text
			if t[3].Kind == Ident {
				name = t[3].Raw
				for _, x := range t[4:] { // file names such as out.txt
					name += x.Raw
				}
			}
			lx.stmts = append(lx.stmts, call(last.Line, "FILE", name))
			redirected = true
		}
	}
	defer func() {
		if redirected {
			lx.stmts = append(lx.stmts, call(lx.line-1))
		}
	}()
	// we are positioned at the start of the next line
	for lx.pos < len(lx.src) {
		e := strings.IndexByte(lx.src[lx.pos:], '\n')
		var l string
		if e < 0 {
			l = lx.src[lx.pos:]
			lx.pos = len(lx.src)
		} else {
			l = lx.src[lx.pos : lx.pos+e]
			lx.pos += e + 1
		}
		ln := lx.line
		lx.line++
		up := strings.ToUpper(strings.TrimSpace(l))
		if strings.HasPrefix(up, "ENDT") {
			break
		}
		l = strings.TrimRight(l, "\r")
		lx.stmts = append(lx.stmts, Stmt{Line: ln, File: lx.file, Toks: []Token{
			{Kind: Ident, Text: "__TEXTLINE", Raw: "__TextLine", Line: ln, Space: true},
			{Kind: Op, Text: "(", Raw: "(", Line: ln},
			{Kind: String, Text: l, Raw: strconv.Quote(l), Line: ln},
			{Kind: Op, Text: ")", Raw: ")", Line: ln},
		}})
	}
}

// Join renders tokens back to source text.
func Join(toks []Token) string {
	var sb strings.Builder
	for i, t := range toks {
		if i > 0 && t.Space {
			sb.WriteByte(' ')
		}
		sb.WriteString(t.Raw)
	}
	return sb.String()
}
