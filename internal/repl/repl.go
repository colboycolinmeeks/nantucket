// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package repl implements the interactive dot prompt.
package repl

import (
	"os"
	"path/filepath"
	"strings"

	"nantucket/internal/lex"
	"nantucket/internal/parse"
	"nantucket/internal/screens"
	"nantucket/internal/term"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

// Banner and Hint are shown when the dot prompt starts.
const (
	Banner = "Nantucket " + vm.Version + "  -  dBase III / Clipper compatible"
	Hint   = "Type HELP for commands, VERSION FULL for what's new, QUIT to exit."
)

// Report prints an error the way the dot prompt shows it. Returns an exit code.
func Report(m *vm.VM, err error) int {
	if err == nil {
		return 0
	}
	switch e := err.(type) {
	case *vm.QuitSignal:
		return 0
	case *vm.CancelSignal:
		m.Println(e.Error())
		return 1
	}
	m.Println(vm.ErrorReport(err))
	return 1
}

// Run runs the dot prompt until QUIT or end of input.
func Run(m *vm.VM, con term.Console) {
	m.PushBaseFrame()
	if con.Interactive() {
		// the status bar is on by default at the dot prompt, as in dBase
		m.SetValue(104, []value.Value{value.True})
		con.Scroll(0, 0, 999, 999, 0, m.StdAttr())
		con.SetPos(0, 0)
		con.Out(Banner, m.Theme().Banner)
		con.NewLine()
		con.Out(Hint, m.StdAttr())
	}
	hist := openHistory(con.Interactive())
	defer hist.close()
	history := hist.lines
	for {
		screens.RefreshStatus(m)
		before := len(history)
		src, ok := readStatement(m, con, &history)
		hist.add(history[before:])
		if !ok {
			return
		}
		if strings.TrimSpace(src) == "" {
			continue
		}
		trim := strings.TrimSpace(src)
		if strings.HasPrefix(trim, "!") || strings.EqualFold(firstWord(trim), "RUN") {
			cmd := strings.TrimPrefix(trim, "!")
			if strings.EqualFold(firstWord(trim), "RUN") {
				cmd = strings.TrimSpace(trim[3:])
			}
			m.Protect(func() { m.ExecStatements("__Run(" + quote(cmd) + ")") })
			continue
		}
		var err error
		if _, cerr := m.CompileStatements(src); cerr != nil {
			m.Println(cerr.Error())
			continue
		}
		m.ResetDebugStep()
		err = m.Protect(func() { m.ExecStatements(src) })
		for _, note := range m.TakeNotes() {
			if err == nil {
				m.Println(note)
			}
		}
		if err != nil {
			if _, ok := err.(*vm.QuitSignal); ok {
				return
			}
			Report(m, err)
		}
	}
}

func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

func quote(s string) string {
	if !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	if !strings.Contains(s, `'`) {
		return `'` + s + `'`
	}
	return "[" + s + "]"
}

// readStatement reads one dot-prompt entry, continuing over multiple
// lines while a control structure (IF, DO WHILE, FOR...) is open.
// promptKeys are handled by the dot prompt itself while a line is typed:
// F1 toggles HELP, F6 arranges windows from the keyboard.
var promptKeys = map[int]bool{term.KF1: true, term.KF6: true}

func readStatement(m *vm.VM, con term.Console, history *[]string) (string, bool) {
	var lines []string
	depth := 0
	pending := "" // text typed before F1 / F6, put back afterwards
	for {
		r, c := con.Pos()
		_ = r
		if c > 0 || len(lines) > 0 || true {
			con.NewLine()
		}
		prompt := ". "
		if len(lines) > 0 {
			prompt = strings.Repeat("  ", depth) + "  "
		}
		con.Out(prompt, m.StdAttr())
		con.Flush()
		line, key, ok := term.ReadLineExit(con, m.StdAttr(), pending, 0, *history, promptKeys)
		pending = ""
		switch key {
		case term.KF1:
			screens.ToggleHelp(m)
			pending = line
			continue
		case term.KF6:
			if !screens.WindowMode(m) {
				m.Println("No windows are open (F1 opens HELP)")
			}
			pending = line
			continue
		}
		// Esc on an empty line closes the front HELP or VERSION FULL window
		if !ok && key == 0 && line == "" && len(lines) == 0 && con.Interactive() && screens.CloseFrontText(m) {
			continue
		}
		if !ok {
			if !con.Interactive() {
				if len(lines) > 0 {
					return strings.Join(lines, "\n"), true
				}
				return "", false
			}
			if len(lines) > 0 {
				return "", true // Esc cancels a multi-line entry
			}
			continue
		}
		if !con.Interactive() {
			con.Out(line, m.StdAttr())
		}
		if strings.TrimSpace(line) != "" {
			if len(*history) == 0 || (*history)[len(*history)-1] != line {
				*history = append(*history, line)
			}
		}
		lines = append(lines, line)
		depth += blockDelta(line)
		if depth <= 0 {
			return strings.Join(lines, "\n"), true
		}
	}
}

func blockDelta(line string) int {
	stmts, err := lex.Lex(line, "")
	if err != nil {
		return 0
	}
	d := 0
	for _, st := range stmts {
		d += parse.IsBlockStart(st.Toks)
	}
	return d
}

// ---------- history file ----------

// maxHistory is how many lines the history file keeps.
const maxHistory = 1000

// history keeps the dot prompt's history in a file, so that Up/Down can
// recall lines from earlier sessions: $NANTUCKET_HISTORY, or
// ~/.nantucket_history. NANTUCKET_HISTORY=off keeps history in memory only.
type history struct {
	lines []string
	f     *os.File
}

func historyPath() string {
	p := os.Getenv("NANTUCKET_HISTORY")
	if strings.EqualFold(p, "off") {
		return ""
	}
	if p == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		p = filepath.Join(home, ".nantucket_history")
	}
	return p
}

// openHistory loads the history file (when interactive) and opens it for
// adding lines, trimming it to the last maxHistory lines.
func openHistory(interactive bool) *history {
	h := &history{}
	path := historyPath()
	if !interactive || path == "" {
		return h
	}
	if data, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if l = strings.TrimRight(l, "\r"); l != "" {
				h.lines = append(h.lines, l)
			}
		}
		if len(h.lines) > maxHistory {
			h.lines = h.lines[len(h.lines)-maxHistory:]
			os.WriteFile(path, []byte(strings.Join(h.lines, "\n")+"\n"), 0o600)
		}
	}
	h.f, _ = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	return h
}

func (h *history) add(lines []string) {
	if h.f == nil {
		return
	}
	for _, l := range lines {
		h.f.WriteString(l + "\n")
	}
}

func (h *history) close() {
	if h.f != nil {
		h.f.Close()
	}
}
