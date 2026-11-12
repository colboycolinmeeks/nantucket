// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

// Logpoints: breakpoints that write a message to the debugger's log
// instead of stopping the program. The log is kept in memory (the last
// maxLog lines), shown by the debugger, and also written to a log file
// (LOGFILE in the debugger) and to the -trace output when they are set.
// It is never written to the program's screen.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nantucket/internal/value"
)

const maxLog = 1000

type debugLog struct {
	lines []string // the last maxLog lines
	total int      // lines logged since the log was cleared
	seen  int      // total when the debugger last showed the log
	file  *os.File // LOGFILE
	path  string
}

// logpointHit counts a hit on a logpoint and writes its message when its
// condition and hit count say so. It returns a problem to log instead
// when the condition can't be evaluated.
func (vm *VM) logpointHit(fr *Frame, b *breakpoint) string {
	if b.cond != "" {
		v, err := vm.quietEval(fr, b.cond)
		switch {
		case err != nil:
			return "condition error: " + err.Error()
		case v.K != value.LOGICAL:
			return "condition isn't logical: " + b.cond
		case !v.B:
			return ""
		}
	}
	b.hits++
	if b.hit.stops(b.hits) {
		vm.writeLog(fr, vm.logMessage(fr, b.log))
	}
	return ""
}

// quietEval evaluates an expression in a call without letting anything it
// runs open the debugger.
func (vm *VM) quietEval(fr *Frame, src string) (value.Value, error) {
	active := vm.dbg.active
	vm.dbg.active = true
	defer func() { vm.dbg.active = active }()
	return vm.DebugEval(DebugFrame{fr: fr}, src)
}

// logParts splits a logpoint message into text and {expressions}: the
// expressions are at the odd indexes. {{ and }} stand for braces.
func logParts(msg string) ([]string, error) {
	var parts []string
	var text strings.Builder
	for i := 0; i < len(msg); i++ {
		c := msg[i]
		switch {
		case c == '{' && i+1 < len(msg) && msg[i+1] == '{', c == '}' && i+1 < len(msg) && msg[i+1] == '}':
			text.WriteByte(c)
			i++
		case c == '{':
			end := strings.IndexByte(msg[i+1:], '}')
			if end < 0 {
				return nil, fmt.Errorf("a { in the message has no }")
			}
			parts = append(parts, text.String(), msg[i+1:i+1+end])
			text.Reset()
			i += end + 1
		default:
			text.WriteByte(c)
		}
	}
	return append(parts, text.String()), nil
}

// checkLogMessage makes sure each {expression} in a message compiles.
func (vm *VM) checkLogMessage(msg string) error {
	parts, err := logParts(msg)
	if err != nil {
		return err
	}
	for i := 1; i < len(parts); i += 2 {
		if strings.TrimSpace(parts[i]) == "" {
			return fmt.Errorf("{} in the message has no expression")
		}
		if _, err := vm.compileMacro(parts[i], false); err != nil {
			return fmt.Errorf("not a valid expression in the message: {%s}", parts[i])
		}
	}
	return nil
}

// logMessage fills in a message's {expressions}, evaluated in the call.
// Text values appear as they are, without quotes.
func (vm *VM) logMessage(fr *Frame, msg string) string {
	parts, err := logParts(msg)
	if err != nil {
		return msg
	}
	var sb strings.Builder
	for i, p := range parts {
		if i%2 == 0 {
			sb.WriteString(p)
			continue
		}
		v, err := vm.quietEval(fr, p)
		switch {
		case err != nil:
			sb.WriteString("{" + p + ": " + err.Error() + "}")
		case v.K == value.STRING:
			sb.WriteString(v.S)
		default:
			sb.WriteString(vm.debugShow(v))
		}
	}
	return sb.String()
}

// writeLog adds a line to the log, saying where it was logged.
func (vm *VM) writeLog(fr *Frame, msg string) {
	l := &vm.dbg.log
	line := fmt.Sprintf("%s(%d) %s: %s", filepath.Base(fr.fn.File), fr.line, fr.fn.Name, msg)
	l.lines = append(l.lines, line)
	if len(l.lines) > maxLog {
		l.lines = append(l.lines[:0], l.lines[len(l.lines)-maxLog:]...)
	}
	l.total++
	if l.file != nil {
		fmt.Fprintln(l.file, line)
	}
	if vm.traceW != nil {
		fmt.Fprintln(vm.traceW, "LOG "+line)
	}
}

// DebugLog returns the log (its last lines, oldest first), and how many of
// them are new since the last call.
func (vm *VM) DebugLog() (lines []string, unseen int) {
	l := &vm.dbg.log
	unseen = min(l.total-l.seen, len(l.lines))
	l.seen = l.total
	return l.lines, unseen
}

// UnseenLog returns the number of lines logged since DebugLog was last
// called, and the latest line.
func (vm *VM) UnseenLog() (int, string) {
	l := &vm.dbg.log
	if l.total == l.seen || len(l.lines) == 0 {
		return 0, ""
	}
	return l.total - l.seen, l.lines[len(l.lines)-1]
}

// ClearDebugLog empties the log (not the log file).
func (vm *VM) ClearDebugLog() {
	l := &vm.dbg.log
	l.lines, l.total, l.seen = nil, 0, 0
}

// SetDebugLogFile also appends the log to a file from now on ("" stops).
func (vm *VM) SetDebugLogFile(name string) (string, error) {
	l := &vm.dbg.log
	if l.file != nil {
		l.file.Close()
		l.file, l.path = nil, ""
	}
	if strings.TrimSpace(name) == "" {
		return "", nil
	}
	path := vm.DefaultPath(name, ".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	l.file, l.path = f, path
	return path, nil
}

// DebugLogFile returns the log file's path, or "".
func (vm *VM) DebugLogFile() string { return vm.dbg.log.path }
