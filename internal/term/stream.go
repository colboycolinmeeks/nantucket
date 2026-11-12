// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package term

import (
	"bufio"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Stream is a line oriented console used when not attached to a terminal
// (scripts, pipes, tests). Positioned output is written sequentially.
type Stream struct {
	w        io.Writer
	in       *bufio.Reader
	row      int
	col      int
	mu       sync.Mutex
	keys     []int
	Rows     int
	Cols     int
	eof      bool
	waitHook func(bool) // see WaitNotifier
}

func NewStream(in io.Reader, out io.Writer) *Stream {
	return &Stream{w: out, in: bufio.NewReader(in), Rows: 25, Cols: 80}
}

func NewStdStream() *Stream { return NewStream(os.Stdin, os.Stdout) }

func (s *Stream) Out(str string, a Attr) {
	io.WriteString(s.w, str)
	if i := strings.LastIndexByte(str, '\n'); i >= 0 {
		s.col = len(str) - i - 1
	} else {
		s.col += len(str)
	}
}

func (s *Stream) NewLine() {
	io.WriteString(s.w, "\n")
	s.col = 0
	if s.row < s.Rows-1 {
		s.row++
	}
}

func (s *Stream) SetPos(r, c int) {
	if r != s.row && s.col > 0 {
		io.WriteString(s.w, "\n")
		s.col = 0
	}
	if r > s.row && s.col == 0 && r-s.row > 1 {
		// keep vertical spacing modest in stream mode
	}
	if c > s.col {
		io.WriteString(s.w, strings.Repeat(" ", c-s.col))
		s.col = c
	}
	s.row = r
}

func (s *Stream) Pos() (int, int)  { return s.row, s.col }
func (s *Stream) Size() (int, int) { return s.Rows, s.Cols }
func (s *Stream) Put(r, c int, str string, a Attr) {
	s.SetPos(r, c)
	s.Out(str, a)
}
func (s *Stream) Scroll(top, left, bottom, right, n int, a Attr) {}

func (s *Stream) ReadKey(timeout time.Duration) (int, bool) {
	s.mu.Lock()
	if len(s.keys) > 0 {
		k := s.keys[0]
		s.keys = s.keys[1:]
		s.mu.Unlock()
		return k, true
	}
	s.mu.Unlock()
	if timeout == 0 || s.eof {
		return 0, false
	}
	s.waiting(true)
	b, err := s.in.ReadByte()
	s.waiting(false)
	if err != nil {
		s.eof = true
		return 0, false
	}
	if b == '\n' {
		return KEnter, true
	}
	return int(b), true
}

// ReadLine reads a whole line (used by ACCEPT/INPUT in stream mode).
func (s *Stream) ReadLine() (string, bool) {
	s.mu.Lock()
	if len(s.keys) > 0 {
		var sb strings.Builder
		for len(s.keys) > 0 {
			k := s.keys[0]
			s.keys = s.keys[1:]
			if k == KEnter {
				s.mu.Unlock()
				return sb.String(), true
			}
			if k > 0 && k < 256 {
				sb.WriteByte(byte(k))
			}
		}
		s.mu.Unlock()
		return sb.String(), true
	}
	s.mu.Unlock()
	s.waiting(true)
	line, err := s.in.ReadString('\n')
	s.waiting(false)
	if err != nil && line == "" {
		s.eof = true
		return "", false
	}
	s.col = 0
	return strings.TrimRight(line, "\r\n"), true
}

func (s *Stream) PushKeys(k []int) {
	s.mu.Lock()
	s.keys = append(s.keys, k...)
	s.mu.Unlock()
}
func (s *Stream) UnreadKey(k int) {
	s.mu.Lock()
	s.keys = append([]int{k}, s.keys...)
	s.mu.Unlock()
}
func (s *Stream) ClearKeys() {
	s.mu.Lock()
	s.keys = nil
	s.mu.Unlock()
}

// SetWaitHook sets the function told when the console waits for input.
func (s *Stream) SetWaitHook(f func(bool)) { s.waitHook = f }

func (s *Stream) waiting(w bool) {
	if s.waitHook != nil {
		s.waitHook(w)
	}
}

func (s *Stream) Flush()                           {}
func (s *Stream) SetCursor(int)                    {}
func (s *Stream) Save(t, l, b, r int) []Cell       { return nil }
func (s *Stream) Restore(t, l, b, r int, c []Cell) {}
func (s *Stream) Interactive() bool                { return false }
func (s *Stream) Bell()                            {}
func (s *Stream) Close()                           {}
func (s *Stream) Suspend()                         {}
func (s *Stream) Resume()                          {}

// LineReader is implemented by consoles that can read a whole line natively.
type LineReader interface {
	ReadLine() (string, bool)
}
