// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package pp implements a Clipper-compatible preprocessor: #define,
// #include, #ifdef, #command/#xcommand and #translate/#xtranslate.
package pp

import (
	"fmt"
	"strconv"
	"strings"

	"nantucket/internal/lex"
)

type elemKind uint8

const (
	eLit elemKind = iota
	eMarker
	eOpt
)

type mKind uint8

const (
	mRegular mKind = iota
	mList
	mRestrict
	mWild
	mExtended
	// result kinds
	rRegular
	rDumb
	rNormal
	rSmart
	rBlock
	rLogify
	rBlank
)

type elem struct {
	kind  elemKind
	tok   lex.Token
	name  string
	mk    mKind
	words [][]lex.Token
	sub   []*elem
	space bool
}

type Rule struct {
	command bool
	exact   bool
	match   []*elem
	result  []*elem
	first   string // first literal keyword (upper) or ""
}

type define struct {
	params []string
	isFunc bool
	body   []lex.Token
}

// PP holds preprocessor state.
type PP struct {
	commands []*Rule
	trans    []*Rule
	defines  map[string]*define
	// Include resolves an #include name to (source, path).
	Include func(name string) (string, string, error)
	cond    []condState
}

type condState struct {
	active   bool // current branch active
	taken    bool // some branch taken
	parentOn bool
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

func New() *PP {
	return &PP{defines: map[string]*define{}}
}

// Clone copies rules so that a compile unit can add its own without
// affecting the base set.
func (p *PP) Clone() *PP {
	n := &PP{
		commands: append([]*Rule(nil), p.commands...),
		trans:    append([]*Rule(nil), p.trans...),
		defines:  make(map[string]*define, len(p.defines)),
		Include:  p.Include,
	}
	for k, v := range p.defines {
		n.defines[k] = v
	}
	return n
}

// Define adds an object-like #define.
func (p *PP) Define(name, value string) {
	toks, _ := lex.LexLine(value)
	p.defines[name] = &define{body: toks}
}

// LoadSource processes directives from a header source (rules/defines only).
func (p *PP) LoadSource(src, file string) error {
	_, err := p.ProcessSource(src, file)
	return err
}

// ProcessSource lexes and preprocesses a whole source.
func (p *PP) ProcessSource(src, file string) ([]lex.Stmt, error) {
	stmts, err := lex.Lex(src, file)
	if err != nil {
		return nil, err
	}
	return p.Process(stmts)
}

func (p *PP) active() bool {
	if len(p.cond) == 0 {
		return true
	}
	c := p.cond[len(p.cond)-1]
	return c.active && c.parentOn
}

// Process expands a list of statements.
func (p *PP) Process(stmts []lex.Stmt) ([]lex.Stmt, error) {
	var out []lex.Stmt
	for _, st := range stmts {
		if len(st.Toks) == 1 && st.Toks[0].Kind == lex.Directive {
			res, err := p.directive(st)
			if err != nil {
				return nil, err
			}
			out = append(out, res...)
			continue
		}
		if !p.active() {
			continue
		}
		res, err := p.expand(st, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, res...)
	}
	return out, nil
}

func (p *PP) errAt(st lex.Stmt, f string, a ...any) error {
	return &Error{File: st.File, Line: st.Line, Msg: fmt.Sprintf(f, a...)}
}

func (p *PP) directive(st lex.Stmt) ([]lex.Stmt, error) {
	text := strings.TrimSpace(st.Toks[0].Text)
	text = strings.TrimSpace(text[1:])
	word := text
	rest := ""
	if i := strings.IndexAny(text, " \t"); i >= 0 {
		word, rest = text[:i], strings.TrimSpace(text[i+1:])
	}
	word = strings.ToLower(word)
	switch word {
	case "ifdef", "ifndef":
		_, def := p.defines[strings.TrimSpace(rest)]
		on := def == (word == "ifdef")
		p.cond = append(p.cond, condState{active: on, taken: on, parentOn: p.active()})
		return nil, nil
	case "else":
		if len(p.cond) == 0 {
			return nil, p.errAt(st, "#else without #ifdef")
		}
		c := &p.cond[len(p.cond)-1]
		c.active = !c.taken
		c.taken = true
		return nil, nil
	case "endif":
		if len(p.cond) == 0 {
			return nil, p.errAt(st, "#endif without #ifdef")
		}
		p.cond = p.cond[:len(p.cond)-1]
		return nil, nil
	}
	if !p.active() {
		return nil, nil
	}
	switch word {
	case "define":
		return nil, p.parseDefine(st, rest)
	case "undef":
		delete(p.defines, strings.TrimSpace(rest))
		return nil, nil
	case "include":
		name := strings.Trim(strings.TrimSpace(rest), "\"'<>")
		if p.Include == nil {
			return nil, p.errAt(st, "cannot include %s", name)
		}
		src, path, err := p.Include(name)
		if err != nil {
			return nil, p.errAt(st, "cannot open include file %s", name)
		}
		return p.ProcessSource(src, path)
	case "command", "xcommand", "translate", "xtranslate":
		r, err := parseRule(rest, word == "command" || word == "xcommand", word[0] == 'x')
		if err != nil {
			return nil, p.errAt(st, "%v", err)
		}
		if r.command {
			p.commands = append(p.commands, r)
		} else {
			p.trans = append(p.trans, r)
		}
		return nil, nil
	case "error":
		return nil, p.errAt(st, "#error %s", rest)
	case "stdout", "warning", "pragma":
		return nil, nil
	}
	return nil, p.errAt(st, "unknown directive #%s", word)
}

func (p *PP) parseDefine(st lex.Stmt, rest string) error {
	i := 0
	for i < len(rest) && (isIdentByte(rest[i])) {
		i++
	}
	name := rest[:i]
	if name == "" {
		return p.errAt(st, "bad #define")
	}
	d := &define{}
	body := rest[i:]
	if strings.HasPrefix(body, "(") {
		e := strings.IndexByte(body, ')')
		if e < 0 {
			return p.errAt(st, "bad #define")
		}
		for _, a := range strings.Split(body[1:e], ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				d.params = append(d.params, strings.ToUpper(a))
			}
		}
		d.isFunc = true
		body = body[e+1:]
	}
	body = strings.TrimSpace(body)
	if body != "" {
		toks, err := lex.LexLine("_ " + body)
		if err != nil {
			return p.errAt(st, "%v", err)
		}
		d.body = toks[1:]
	}
	p.defines[name] = d
	return nil
}

func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// ---------------- rule parsing ----------------

func parseRule(text string, command, exact bool) (*Rule, error) {
	sep := findArrow(text)
	if sep < 0 {
		return nil, fmt.Errorf("missing => in rule")
	}
	mt, rt := strings.TrimSpace(text[:sep]), strings.TrimSpace(text[sep+2:])
	m, err := parsePattern(mt, false)
	if err != nil {
		return nil, err
	}
	r, err := parsePattern(rt, true)
	if err != nil {
		return nil, err
	}
	rule := &Rule{command: command, exact: exact, match: m, result: r}
	if len(m) > 0 && m[0].kind == eLit {
		rule.first = m[0].tok.Text
	}
	return rule, nil
}

func findArrow(s string) int {
	var q byte
	for i := 0; i < len(s)-1; i++ {
		c := s[i]
		if q != 0 {
			if c == q {
				q = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			q = c
			continue
		}
		if c == '\\' {
			i++
			continue
		}
		if c == '=' && s[i+1] == '>' {
			return i
		}
	}
	return -1
}

func lexChunk(s string) ([]lex.Token, error) {
	toks, err := lex.LexLine("_ " + s)
	if err != nil {
		return nil, err
	}
	return toks[1:], nil
}

func parsePattern(s string, result bool) ([]*elem, error) {
	type frame struct{ elems []*elem }
	stack := []*frame{{}}
	top := func() *frame { return stack[len(stack)-1] }
	space := true
	i := 0
	var lit strings.Builder
	litSpace := false
	flushLit := func() error {
		if lit.Len() == 0 {
			return nil
		}
		toks, err := lexChunk(lit.String())
		if err != nil {
			return err
		}
		for j, t := range toks {
			if j == 0 {
				t.Space = litSpace
			}
			top().elems = append(top().elems, &elem{kind: eLit, tok: t, space: t.Space})
		}
		lit.Reset()
		return nil
	}
	addLit := func(c byte) {
		if lit.Len() == 0 {
			litSpace = space
		}
		lit.WriteByte(c)
		space = false
	}
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			if lit.Len() > 0 {
				lit.WriteByte(' ')
			}
			space = true
			i++
		case c == '\\' && i+1 < len(s):
			addLit(s[i+1])
			i += 2
		case c == '"' || c == '\'':
			e := strings.IndexByte(s[i+1:], c)
			if e < 0 {
				return nil, fmt.Errorf("unterminated string in rule")
			}
			if lit.Len() == 0 {
				litSpace = space
			}
			lit.WriteString(s[i : i+e+2])
			space = false
			i += e + 2
		case c == '[':
			if err := flushLit(); err != nil {
				return nil, err
			}
			stack = append(stack, &frame{})
			i++
		case c == ']':
			if err := flushLit(); err != nil {
				return nil, err
			}
			if len(stack) < 2 {
				addLit(']')
				i++
				continue
			}
			f := top()
			stack = stack[:len(stack)-1]
			top().elems = append(top().elems, &elem{kind: eOpt, sub: f.elems, space: true})
			i++
		case c == ';':
			if err := flushLit(); err != nil {
				return nil, err
			}
			top().elems = append(top().elems, &elem{kind: eLit, tok: lex.Token{Kind: lex.Op, Text: ";", Raw: ";", Space: true}, space: true})
			space = true
			i++
		case c == '#' && result && i+1 < len(s) && s[i+1] == '<':
			if m, n := parseMarker(s[i+1:], result); m != nil {
				if err := flushLit(); err != nil {
					return nil, err
				}
				m.mk = rDumb
				m.space = space
				top().elems = append(top().elems, m)
				i += 1 + n
				space = false
				continue
			}
			addLit(c)
			i++
		case c == '<':
			if m, n := parseMarker(s[i:], result); m != nil {
				if err := flushLit(); err != nil {
					return nil, err
				}
				m.space = space
				top().elems = append(top().elems, m)
				i += n
				space = false
				continue
			}
			addLit(c)
			i++
		default:
			addLit(c)
			i++
		}
	}
	if err := flushLit(); err != nil {
		return nil, err
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("unbalanced [ ] in rule")
	}
	return stack[0].elems, nil
}

// parseMarker parses a marker starting at '<'. Returns nil if not a marker.
func parseMarker(s string, result bool) (*elem, int) {
	e := strings.IndexByte(s, '>')
	if e < 0 {
		return nil, 0
	}
	inner := strings.TrimSpace(s[1:e])
	n := e + 1
	if inner == "" {
		return nil, 0
	}
	ident := func(x string) bool {
		x = strings.TrimSpace(x)
		if x == "" {
			return false
		}
		for i := 0; i < len(x); i++ {
			if !isIdentByte(x[i]) {
				return false
			}
		}
		return true
	}
	wrap := func(open, close byte) (string, bool) {
		if len(inner) >= 2 && inner[0] == open && inner[len(inner)-1] == close && ident(inner[1:len(inner)-1]) {
			return strings.ToUpper(strings.TrimSpace(inner[1 : len(inner)-1])), true
		}
		return "", false
	}
	if result {
		if nm, ok := wrap('"', '"'); ok {
			return &elem{kind: eMarker, name: nm, mk: rNormal}, n
		}
		if nm, ok := wrap('(', ')'); ok {
			return &elem{kind: eMarker, name: nm, mk: rSmart}, n
		}
		if nm, ok := wrap('{', '}'); ok {
			return &elem{kind: eMarker, name: nm, mk: rBlock}, n
		}
		if nm, ok := wrap('.', '.'); ok {
			return &elem{kind: eMarker, name: nm, mk: rLogify}, n
		}
		if nm, ok := wrap('!', '!'); ok {
			return &elem{kind: eMarker, name: nm, mk: rSmart}, n
		}
		if strings.HasPrefix(inner, "-") && strings.HasSuffix(inner, "-") && len(inner) > 2 && ident(inner[1:len(inner)-1]) {
			return &elem{kind: eMarker, name: strings.ToUpper(inner[1 : len(inner)-1]), mk: rBlank}, n
		}
		if ident(inner) {
			return &elem{kind: eMarker, name: strings.ToUpper(inner), mk: rRegular}, n
		}
		return nil, 0
	}
	if nm, ok := wrap('*', '*'); ok {
		return &elem{kind: eMarker, name: nm, mk: mWild}, n
	}
	if nm, ok := wrap('(', ')'); ok {
		return &elem{kind: eMarker, name: nm, mk: mExtended}, n
	}
	if nm, ok := wrap('!', '!'); ok {
		return &elem{kind: eMarker, name: nm, mk: mRegular}, n
	}
	if ident(inner) {
		return &elem{kind: eMarker, name: strings.ToUpper(inner), mk: mRegular}, n
	}
	if c := strings.IndexByte(inner, ','); c > 0 && strings.TrimSpace(inner[c+1:]) == "..." && ident(inner[:c]) {
		return &elem{kind: eMarker, name: strings.ToUpper(strings.TrimSpace(inner[:c])), mk: mList}, n
	}
	if c := strings.IndexByte(inner, ':'); c > 0 && ident(inner[:c]) {
		m := &elem{kind: eMarker, name: strings.ToUpper(strings.TrimSpace(inner[:c])), mk: mRestrict}
		for _, w := range strings.Split(inner[c+1:], ",") {
			w = strings.TrimSpace(w)
			if w == "" {
				continue
			}
			toks, err := lexChunk(w)
			if err != nil || len(toks) == 0 {
				return nil, 0
			}
			m.words = append(m.words, toks)
		}
		return m, n
	}
	return nil, 0
}

// ---------------- matching ----------------

type bind struct {
	name  string
	elems [][]lex.Token
}

type matcher struct {
	toks  []lex.Token
	exact bool
	binds []bind
}

func litEq(rule, in lex.Token, exact bool) bool {
	if rule.Kind != in.Kind {
		// numbers/strings mismatch
		return false
	}
	switch rule.Kind {
	case lex.Ident:
		if rule.Text == in.Text {
			return true
		}
		if !exact && len(in.Text) >= 4 && strings.HasPrefix(rule.Text, in.Text) {
			return true
		}
		return false
	case lex.String:
		return rule.Text == in.Text
	case lex.Macro:
		return strings.EqualFold(rule.Text, in.Text)
	}
	return rule.Text == in.Text
}

// leading literal ident of an element sequence
func leadKeyword(es []*elem) (lex.Token, bool) {
	if len(es) > 0 && es[0].kind == eLit && es[0].tok.Kind == lex.Ident {
		return es[0].tok, true
	}
	if len(es) > 0 && es[0].kind == eMarker && es[0].mk == mRestrict {
		for _, w := range es[0].words {
			if len(w) > 0 && w[0].Kind == lex.Ident {
				return w[0], true
			}
		}
	}
	return lex.Token{}, false
}

func (m *matcher) isStop(pos int, stops []lex.Token) bool {
	if pos >= len(m.toks) {
		return true
	}
	t := m.toks[pos]
	if t.Kind != lex.Ident {
		return false
	}
	for _, s := range stops {
		if litEq(s, t, m.exact) {
			// a keyword followed by an assignment is still an expression
			return true
		}
	}
	return false
}

// collect stop keywords for a run of optional elements plus the following literal
func stopsFor(es []*elem, from int) []lex.Token {
	var stops []lex.Token
	j := from
	for j < len(es) && es[j].kind == eOpt {
		if k, ok := leadKeyword(es[j].sub); ok {
			stops = append(stops, k)
		}
		j++
	}
	if j < len(es) {
		if k, ok := leadKeyword(es[j:]); ok {
			stops = append(stops, k)
		}
	}
	return stops
}

func (m *matcher) seq(es []*elem, pos int, outerStops []lex.Token) (int, bool) {
	for i := 0; i < len(es); i++ {
		e := es[i]
		switch e.kind {
		case eOpt:
			j := i
			for j < len(es) && es[j].kind == eOpt {
				j++
			}
			run := es[i:j]
			stops := append(stopsFor(es, i), outerStops...)
			used := make([]bool, len(run))
			for {
				progressed := false
				for k, o := range run {
					mark := len(m.binds)
					// avoid expression markers swallowing keywords of other clauses
					if len(o.sub) > 0 && o.sub[0].kind == eMarker && o.sub[0].mk != mRestrict && m.isStop(pos, stops) {
						continue
					}
					// non-repeatable clauses (those without markers repeat harmlessly)
					_ = used
					if np, ok := m.seq(o.sub, pos, stops); ok && np > pos {
						pos = np
						progressed = true
						used[k] = true
						break
					}
					m.binds = m.binds[:mark]
				}
				if !progressed {
					break
				}
			}
			i = j - 1
		case eLit:
			if pos < len(m.toks) && litEq(e.tok, m.toks[pos], m.exact) {
				pos++
			} else {
				return pos, false
			}
		case eMarker:
			var stops []lex.Token
			if i+1 < len(es) {
				stops = stopsFor(es, i+1)
			}
			stops = append(stops, outerStops...)
			np, ok := m.marker(e, pos, stops)
			if !ok {
				return pos, false
			}
			pos = np
		}
	}
	return pos, true
}

func (m *matcher) marker(e *elem, pos int, stops []lex.Token) (int, bool) {
	toks := m.toks
	switch e.mk {
	case mWild:
		m.binds = append(m.binds, bind{e.name, [][]lex.Token{toks[pos:]}})
		return len(toks), true
	case mRestrict:
		for _, w := range e.words {
			if pos+len(w) <= len(toks) {
				ok := true
				for k := range w {
					if w[k].Kind == lex.Op && w[k].Text == "&" && toks[pos+k].Kind == lex.Macro {
						continue
					}
					if !litEq(w[k], toks[pos+k], m.exact) {
						ok = false
						break
					}
				}
				if ok {
					m.binds = append(m.binds, bind{e.name, [][]lex.Token{toks[pos : pos+len(w)]}})
					return pos + len(w), true
				}
			}
		}
		return pos, false
	case mExtended:
		if pos >= len(toks) {
			return pos, false
		}
		t := toks[pos]
		if t.Is("(") || t.Kind == lex.String {
			end := scanExpr(toks, pos)
			if end <= pos {
				return pos, false
			}
			m.binds = append(m.binds, bind{e.name, [][]lex.Token{toks[pos:end]}})
			return end, true
		}
		if m.isStop(pos, stops) {
			return pos, false
		}
		end := pos + 1
		for end < len(toks) && !toks[end].Space && !toks[end].Is(",") && !toks[end].Is(")") && !toks[end].Is(";") {
			end++
		}
		m.binds = append(m.binds, bind{e.name, [][]lex.Token{toks[pos:end]}})
		return end, true
	case mList:
		var elems [][]lex.Token
		p := pos
		for {
			if p < len(toks) && toks[p].Is(",") {
				elems = append(elems, nil)
				p++
				continue
			}
			if m.isStop(p, stops) {
				if len(elems) > 0 {
					elems = append(elems, nil)
				}
				break
			}
			end := scanExpr(toks, p)
			if end <= p {
				if len(elems) > 0 {
					elems = append(elems, nil)
				}
				break
			}
			elems = append(elems, toks[p:end])
			p = end
			if p < len(toks) && toks[p].Is(",") {
				p++
				if p >= len(toks) {
					elems = append(elems, nil)
					break
				}
				continue
			}
			break
		}
		if len(elems) == 0 {
			return pos, false
		}
		m.binds = append(m.binds, bind{e.name, elems})
		return p, true
	default: // regular
		if m.isStop(pos, stops) {
			return pos, false
		}
		end := scanExpr(toks, pos)
		if end <= pos {
			return pos, false
		}
		m.binds = append(m.binds, bind{e.name, [][]lex.Token{toks[pos:end]}})
		return end, true
	}
}

// ---------------- expression extent scanner ----------------

var binOps = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "%": true, "**": true, "=": true, "==": true, "!=": true,
	"<": true, ">": true, "<=": true, ">=": true, "$": true, ".AND.": true, ".OR.": true,
	":=": true, "+=": true, "-=": true, "*=": true, "/=": true, "%=": true, "**=": true,
}

func skipBalanced(toks []lex.Token, p int) int {
	depth := 0
	for p < len(toks) {
		t := toks[p]
		if t.Kind == lex.Op {
			switch t.Text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
				if depth == 0 {
					return p + 1
				}
				if depth < 0 {
					return p
				}
			}
		}
		p++
	}
	return p
}

func scanOperand(toks []lex.Token, p int) int {
	if p >= len(toks) {
		return p
	}
	t := toks[p]
	switch t.Kind {
	case lex.Number, lex.String, lex.Logical, lex.Date, lex.Macro:
		p++
	case lex.Ident:
		p++
		if p < len(toks) && toks[p].Is("(") {
			p = skipBalanced(toks, p)
		}
	case lex.Op:
		switch t.Text {
		case "+", "-", ".NOT.", "++", "--", "@":
			q := scanOperand(toks, p+1)
			if q == p+1 {
				return p
			}
			return q
		case "(", "{":
			p = skipBalanced(toks, p)
		case "&":
			if p+1 < len(toks) && toks[p+1].Is("(") {
				p = skipBalanced(toks, p+1)
			} else {
				return p
			}
		default:
			return p
		}
	default:
		return p
	}
	// postfix
	for p < len(toks) {
		t := toks[p]
		if t.Kind != lex.Op {
			break
		}
		switch t.Text {
		case "[":
			p = skipBalanced(toks, p)
			continue
		case "(":
			if !t.Space {
				p = skipBalanced(toks, p)
				continue
			}
		case "->":
			q := scanOperand(toks, p+1)
			if q == p+1 {
				return p
			}
			return q
		case ":":
			if p+1 < len(toks) && toks[p+1].Kind == lex.Ident {
				p += 2
				if p < len(toks) && toks[p].Is("(") {
					p = skipBalanced(toks, p)
				}
				continue
			}
		case "++", "--":
			p++
			continue
		}
		break
	}
	return p
}

func scanExpr(toks []lex.Token, p int) int {
	q := scanOperand(toks, p)
	if q == p {
		return p
	}
	for q < len(toks) && toks[q].Kind == lex.Op && binOps[toks[q].Text] {
		r := scanOperand(toks, q+1)
		if r == q+1 {
			return q
		}
		q = r
	}
	return q
}

// ---------------- result generation ----------------

func (m *matcher) lookup(name string) []bind {
	var r []bind
	for _, b := range m.binds {
		if b.name == name {
			r = append(r, b)
		}
	}
	return r
}

func markersIn(es []*elem, out map[string]bool) {
	for _, e := range es {
		if e.kind == eMarker {
			out[e.name] = true
		} else if e.kind == eOpt {
			markersIn(e.sub, out)
		}
	}
}

func strTok(s string, space bool) lex.Token {
	return lex.Token{Kind: lex.String, Text: s, Raw: strconv.Quote(s), Space: space}
}
func opTok(s string) lex.Token { return lex.Token{Kind: lex.Op, Text: s, Raw: s, Space: true} }
func identTok(s string) lex.Token {
	return lex.Token{Kind: lex.Ident, Text: strings.ToUpper(s), Raw: s, Space: true}
}

func (m *matcher) gen(es []*elem, iter int, out []lex.Token) []lex.Token {
	for _, e := range es {
		switch e.kind {
		case eLit:
			t := e.tok
			t.Space = e.space
			out = append(out, t)
		case eOpt:
			names := map[string]bool{}
			markersIn(e.sub, names)
			count := 0
			for n := range names {
				if c := len(m.lookup(n)); c > count {
					count = c
				}
			}
			for k := 0; k < count; k++ {
				out = m.gen(e.sub, k, out)
			}
		case eMarker:
			bs := m.lookup(e.name)
			var b *bind
			if iter < len(bs) {
				b = &bs[iter]
			} else if len(bs) > 0 && iter >= 0 {
				b = &bs[len(bs)-1]
			}
			if e.mk == rLogify {
				if b != nil {
					out = append(out, lex.Token{Kind: lex.Logical, Text: ".T.", Raw: ".T.", Space: true})
				} else {
					out = append(out, lex.Token{Kind: lex.Logical, Text: ".F.", Raw: ".F.", Space: true})
				}
				continue
			}
			if b == nil || e.mk == rBlank {
				continue
			}
			switch e.mk {
			case rRegular:
				for k, el := range b.elems {
					if k > 0 {
						out = append(out, opTok(","))
					}
					for j, t := range el {
						if j == 0 {
							t.Space = true
						}
						out = append(out, t)
					}
				}
			case rDumb:
				var parts []string
				for _, el := range b.elems {
					parts = append(parts, lex.Join(el))
				}
				out = append(out, strTok(strings.Join(parts, ","), true))
			case rNormal:
				for k, el := range b.elems {
					if k > 0 {
						out = append(out, opTok(","))
					}
					if len(el) > 0 {
						out = append(out, strTok(lex.Join(el), true))
					}
				}
			case rSmart:
				for k, el := range b.elems {
					if k > 0 {
						out = append(out, opTok(","))
					}
					out = append(out, smart(el)...)
				}
			case rBlock:
				for k, el := range b.elems {
					if k > 0 {
						out = append(out, opTok(","))
					}
					if len(el) == 0 {
						continue
					}
					if len(el) >= 2 && el[0].Is("{") && el[1].Is("|") {
						out = append(out, el...)
						continue
					}
					out = append(out, opTok("{"), opTok("|"), opTok("|"))
					out = append(out, el...)
					out = append(out, opTok("}"))
				}
			}
		}
	}
	return out
}

func smart(el []lex.Token) []lex.Token {
	if len(el) == 0 {
		return nil
	}
	if el[0].Is("(") || (len(el) == 1 && el[0].Kind == lex.String) {
		return el
	}
	if len(el) == 1 && el[0].Kind == lex.Macro {
		raw := el[0].Text
		name := strings.TrimSuffix(raw[1:], ".")
		if isPlainIdent(name) {
			return []lex.Token{opTok("("), identTok(name), opTok(")")}
		}
		return []lex.Token{identTok("__MACROSTR"), opTok("("), strTok(raw, false), opTok(")")}
	}
	// name containing macros -> runtime substitution
	text := lex.Join(el)
	if strings.Contains(text, "&") {
		return []lex.Token{identTok("__MACROSTR"), opTok("("), strTok(text, false), opTok(")")}
	}
	return []lex.Token{strTok(text, true)}
}

func isPlainIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isIdentByte(s[i]) {
			return false
		}
	}
	return true
}

// ---------------- expansion ----------------

func (p *PP) applyDefines(toks []lex.Token) ([]lex.Token, bool) {
	if len(p.defines) == 0 {
		return toks, false
	}
	changed := false
	var out []lex.Token
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Kind == lex.Ident {
			if d, ok := p.defines[t.Raw]; ok {
				if !d.isFunc {
					for j, b := range d.body {
						if j == 0 {
							b.Space = t.Space
						}
						b.Line = t.Line
						out = append(out, b)
					}
					changed = true
					continue
				}
				if i+1 < len(toks) && toks[i+1].Is("(") {
					end := skipBalanced(toks, i+1)
					args := splitArgs(toks[i+2 : end-1])
					if len(args) == len(d.params) || (len(d.params) == 0 && len(args) == 1 && len(args[0]) == 0) {
						for _, b := range d.body {
							if b.Kind == lex.Ident {
								found := false
								for k, pn := range d.params {
									if pn == b.Text {
										out = append(out, args[k]...)
										found = true
										break
									}
								}
								if found {
									continue
								}
							}
							b.Line = t.Line
							out = append(out, b)
						}
						i = end - 1
						changed = true
						continue
					}
				}
			}
		}
		out = append(out, t)
	}
	return out, changed
}

func splitArgs(toks []lex.Token) [][]lex.Token {
	var args [][]lex.Token
	depth := 0
	start := 0
	for i, t := range toks {
		if t.Kind == lex.Op {
			switch t.Text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case ",":
				if depth == 0 {
					args = append(args, toks[start:i])
					start = i + 1
				}
			}
		}
	}
	args = append(args, toks[start:])
	return args
}

func (p *PP) applyTranslates(toks []lex.Token) ([]lex.Token, bool) {
	if len(p.trans) == 0 {
		return toks, false
	}
	for i := 0; i < len(toks); i++ {
		for r := len(p.trans) - 1; r >= 0; r-- {
			rule := p.trans[r]
			if rule.first != "" && !(toks[i].Kind == lex.Ident && litEq(rule.match[0].tok, toks[i], rule.exact)) &&
				!(toks[i].Kind != lex.Ident && toks[i].Text == rule.first) {
				continue
			}
			m := &matcher{toks: toks, exact: rule.exact}
			end, ok := m.seq(rule.match, i, nil)
			if !ok || end == i {
				continue
			}
			res := m.gen(rule.result, 0, nil)
			for k := range res {
				res[k].Line = toks[i].Line
			}
			if len(res) > 0 {
				res[0].Space = toks[i].Space
			}
			out := append([]lex.Token{}, toks[:i]...)
			out = append(out, res...)
			out = append(out, toks[end:]...)
			return out, true
		}
	}
	return toks, false
}

// assignOps are operators that, straight after a leading name, make a
// statement an assignment rather than a command: "run := 1" sets a variable
// called run; it is not RUN ":= 1".
var assignOps = map[string]bool{"=": true, ":=": true, "+=": true, "-=": true, "*=": true,
	"/=": true, "^=": true, "%=": true, "**=": true, "++": true, "--": true}

func (p *PP) applyCommand(toks []lex.Token) ([]lex.Token, bool) {
	if len(toks) == 0 {
		return toks, false
	}
	if len(toks) > 1 && toks[0].Kind == lex.Ident && toks[1].Kind == lex.Op && assignOps[toks[1].Text] {
		return toks, false
	}
	for r := len(p.commands) - 1; r >= 0; r-- {
		rule := p.commands[r]
		if rule.first != "" {
			if len(rule.match) == 0 || !litEq(rule.match[0].tok, toks[0], rule.exact) {
				continue
			}
		}
		m := &matcher{toks: toks, exact: rule.exact}
		end, ok := m.seq(rule.match, 0, nil)
		if !ok || end != len(toks) {
			continue
		}
		res := m.gen(rule.result, 0, nil)
		for k := range res {
			res[k].Line = toks[0].Line
		}
		if len(res) > 0 {
			res[0].Space = true
		}
		return res, true
	}
	return toks, false
}

func (p *PP) expand(st lex.Stmt, depth int) ([]lex.Stmt, error) {
	if depth > 50 {
		return nil, p.errAt(st, "circularity in preprocessor rules")
	}
	toks := st.Toks
	for iter := 0; iter < 200; iter++ {
		var c1, c2, c3 bool
		toks, c1 = p.applyDefines(toks)
		toks, c2 = p.applyTranslates(toks)
		if c1 || c2 {
			continue
		}
		toks, c3 = p.applyCommand(toks)
		if !c3 {
			break
		}
		// split on ';' and re-expand each part
		parts := splitSemi(toks)
		if len(parts) > 1 {
			var out []lex.Stmt
			for _, part := range parts {
				if len(part) == 0 {
					continue
				}
				res, err := p.expand(lex.Stmt{Toks: part, Line: st.Line, File: st.File}, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, res...)
			}
			return out, nil
		}
	}
	parts := splitSemi(toks)
	var out []lex.Stmt
	for _, part := range parts {
		if len(part) > 0 {
			out = append(out, lex.Stmt{Toks: part, Line: st.Line, File: st.File})
		}
	}
	return out, nil
}

func splitSemi(toks []lex.Token) [][]lex.Token {
	var parts [][]lex.Token
	start := 0
	for i, t := range toks {
		if t.Is(";") {
			parts = append(parts, toks[start:i])
			start = i + 1
		}
	}
	return append(parts, toks[start:])
}
