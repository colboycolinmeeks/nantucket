// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package testrun implements "nantucket test": it runs *_test.prg files,
// checks their assertions and compares their output with .out files.
package testrun

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"nantucket/internal/compile"
	_ "nantucket/internal/features" // the commands outside the core
	"nantucket/internal/term"
	"nantucket/internal/vm"
)

// Options control a test run.
type Options struct {
	Update  bool           // write .out files from the actual output
	Verbose bool           // list every test and show program output
	Run     *regexp.Regexp // only run tests whose name matches
	Out     io.Writer      // report destination
}

// Main runs "nantucket test [flags] [paths...]" and returns the exit code.
func Main(args []string) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	update := fs.Bool("update", false, "write or refresh the expected .out files from the actual output")
	verbose := fs.Bool("v", false, "list every test, and show each file's output")
	run := fs.String("run", "", "only run tests whose name (file or Test procedure) matches this regular expression")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: nantucket test [options] [files or directories...]\n\n"+
			"Runs every *_test.prg file (searching directories recursively; default: the\n"+
			"current directory). A test passes when it raises no errors, all its\n"+
			"assertions hold, and its output matches <name>.out if that file exists.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := Options{Update: *update, Verbose: *verbose, Out: os.Stdout}
	if *run != "" {
		re, err := regexp.Compile("(?i)" + *run)
		if err != nil {
			fmt.Fprintln(os.Stderr, "nantucket test: bad -run pattern:", err)
			return 2
		}
		opts.Run = re
	}
	files, err := Find(fs.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "nantucket test:", err)
		return 2
	}
	if len(files) == 0 {
		fmt.Fprintln(opts.Out, "no test files found (test files are named *_test.prg)")
		if hint := testPrgHint(fs.Args()); hint != "" {
			fmt.Fprintln(opts.Out, hint)
		}
		return 0
	}
	if RunAll(files, opts) {
		return 0
	}
	return 1
}

// Find returns the *_test.prg files named by paths. Directories are searched
// recursively, skipping hidden ones. No paths means the current directory.
func Find(paths []string) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	var files []string
	seen := map[string]bool{}
	add := func(p string) {
		if abs, err := filepath.Abs(p); err == nil && !seen[abs] {
			seen[abs] = true
			files = append(files, p)
		}
	}
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			if st2, err2 := os.Stat(p + ".prg"); err2 == nil && !st2.IsDir() {
				add(p + ".prg")
				continue
			}
			return nil, err
		}
		if !st.IsDir() {
			add(p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != p && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), "_test.prg") {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

// Result is the outcome of one test file.
type Result struct {
	File     string
	Tests    []TestResult // one per Test procedure, or one for the whole file
	Output   string
	OutDiff  []string // differences from the .out file
	Updated  bool     // .out file written with -update
	Skipped  bool     // filtered out by -run
	Duration time.Duration
}

// TestResult is the outcome of one Test procedure (or of the whole file).
type TestResult struct {
	Name     string
	Failures []string
}

// Passed reports whether the file and all its tests passed.
func (r *Result) Passed() bool {
	if len(r.OutDiff) > 0 {
		return false
	}
	for _, t := range r.Tests {
		if len(t.Failures) > 0 {
			return false
		}
	}
	return true
}

// RunAll runs the files, prints a report and returns true if all passed.
func RunAll(files []string, opts Options) bool {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	var nFiles, nTests, nFailed, nUpdated int
	start := time.Now()
	for _, f := range files {
		r := RunFile(f, opts)
		if r.Skipped {
			continue
		}
		nFiles++
		report(opts, r)
		for _, t := range r.Tests {
			nTests++
			if len(t.Failures) > 0 {
				nFailed++
			}
		}
		if len(r.OutDiff) > 0 && !hasFailures(r) {
			nFailed++ // count an output mismatch once for the file
		}
		if r.Updated {
			nUpdated++
		}
	}
	w := opts.Out
	fmt.Fprintln(w)
	summary := fmt.Sprintf("%d %s, %d %s", nFiles, plural(nFiles, "file", "files"), nTests, plural(nTests, "test", "tests"))
	if nUpdated > 0 {
		summary += fmt.Sprintf(", %d .out %s written", nUpdated, plural(nUpdated, "file", "files"))
	}
	if nFailed == 0 {
		fmt.Fprintf(w, "PASS  %s (%.2fs)\n", summary, time.Since(start).Seconds())
		return true
	}
	fmt.Fprintf(w, "FAIL  %s, %d failed (%.2fs)\n", summary, nFailed, time.Since(start).Seconds())
	return false
}

func hasFailures(r *Result) bool {
	for _, t := range r.Tests {
		if len(t.Failures) > 0 {
			return true
		}
	}
	return false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func report(opts Options, r *Result) {
	w := opts.Out
	status := "ok  "
	if !r.Passed() {
		status = "FAIL"
	}
	fmt.Fprintf(w, "%s  %s (%.2fs)\n", status, r.File, r.Duration.Seconds())
	for _, t := range r.Tests {
		if len(t.Failures) == 0 {
			if opts.Verbose && t.Name != "" {
				fmt.Fprintf(w, "      ok    %s\n", t.Name)
			}
			continue
		}
		if t.Name != "" {
			fmt.Fprintf(w, "      FAIL  %s\n", t.Name)
		}
		for _, f := range t.Failures {
			for i, line := range strings.Split(f, "\n") {
				if i == 0 {
					fmt.Fprintf(w, "            %s\n", line)
				} else {
					fmt.Fprintf(w, "              %s\n", line)
				}
			}
		}
	}
	if r.Updated {
		fmt.Fprintf(w, "      wrote %s\n", outFile(r.File))
	}
	if len(r.OutDiff) > 0 {
		fmt.Fprintf(w, "      output differs from %s (- expected, + actual):\n", filepath.Base(outFile(r.File)))
		for _, l := range r.OutDiff {
			fmt.Fprintf(w, "            %s\n", l)
		}
	}
	if opts.Verbose && r.Output != "" {
		fmt.Fprintln(w, "      output:")
		for _, l := range strings.Split(r.Output, "\n") {
			fmt.Fprintf(w, "        | %s\n", l)
		}
	}
}

func outFile(prg string) string { return strings.TrimSuffix(prg, filepath.Ext(prg)) + ".out" }
func inFile(prg string) string  { return strings.TrimSuffix(prg, filepath.Ext(prg)) + ".in" }

// RunFile runs one test file in a fresh runtime. The working directory is
// the file's directory while it runs, so tests can use data files beside them.
func RunFile(file string, opts Options) *Result {
	r := &Result{File: file}
	start := time.Now()
	defer func() { r.Duration = time.Since(start) }()

	abs, err := filepath.Abs(file)
	if err != nil {
		r.Tests = []TestResult{{Failures: []string{err.Error()}}}
		return r
	}
	base := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	fileMatch := opts.Run == nil || opts.Run.MatchString(base)

	input := ""
	if b, err := os.ReadFile(inFile(abs)); err == nil {
		input = string(b)
	}
	var out bytes.Buffer
	con := term.NewStream(strings.NewReader(input), &out)
	m, err := vm.New(con)
	if err != nil {
		r.Tests = []TestResult{{Failures: []string{err.Error()}}}
		return r
	}
	old, _ := os.Getwd()
	os.Chdir(filepath.Dir(abs))
	defer os.Chdir(old)
	defer m.Close()

	var cur *TestResult
	m.SetAssertHandler(func(f vm.AssertFailure) {
		cur.Failures = append(cur.Failures, f.String())
	})
	m.PushBaseFrame()

	srcBytes, _ := os.ReadFile(abs)
	src := strings.Split(strings.ReplaceAll(string(srcBytes), "\r\n", "\n"), "\n")
	mod, err := m.LoadFile(abs)
	if err != nil {
		r.Tests = []TestResult{{Failures: []string{"compile error: " + err.Error()}}}
		return r
	}
	var tests []*compile.Func
	var setup *compile.Func
	for i, f := range mod.Funcs {
		switch {
		case strings.HasPrefix(strings.ToUpper(f.Name), "TEST"):
			tests = append(tests, f)
		case i == 0 && strings.EqualFold(f.Name, base):
			setup = f // statements before the first FUNCTION/PROCEDURE
		}
	}
	filtered := false
	if len(tests) == 0 {
		// no Test procedures: the whole file is one test
		if !fileMatch {
			r.Skipped = true
			return r
		}
		r.Tests = []TestResult{{}}
		cur = &r.Tests[0]
		if err := m.RunFile(abs, nil); err != nil {
			addError(cur, err)
		}
	} else {
		var run []*compile.Func
		for _, f := range tests {
			if !fileMatch && !opts.Run.MatchString(f.Name) {
				filtered = true
				continue
			}
			run = append(run, f)
			r.Tests = append(r.Tests, TestResult{Name: displayName(src, f)})
		}
		if len(run) == 0 {
			r.Skipped = true
			return r
		}
		if setup != nil {
			// failures in the setup code are reported against the first test
			cur = &r.Tests[0]
			// transparent, so PRIVATE variables it creates stay visible to the tests
			if err := m.Protect(func() { m.Call(setup, nil, nil, true) }); err != nil {
				addError(cur, err)
			}
		}
		for i, f := range run {
			cur = &r.Tests[i]
			fn := f
			if err := m.Protect(func() { m.Call(fn, nil, nil, false) }); err != nil {
				addError(cur, err)
			}
		}
	}
	r.Output = normalize(out.String())

	// compare with the expected output (only when the whole file ran)
	if filtered {
		return r
	}
	of := outFile(abs)
	if opts.Update {
		if err := os.WriteFile(of, []byte(r.Output+"\n"), 0644); err != nil {
			cur.Failures = append(cur.Failures, "cannot write "+of+": "+err.Error())
		} else {
			r.Updated = true
		}
		return r
	}
	if b, err := os.ReadFile(of); err == nil {
		if want := normalize(string(b)); want != r.Output {
			r.OutDiff = diff(strings.Split(want, "\n"), strings.Split(r.Output, "\n"))
		}
	}
	return r
}

// displayName finds how a function's name is spelled in the source (names
// are stored in upper case), falling back to the stored name.
func displayName(src []string, f *compile.Func) string {
	if f.Line >= 1 && f.Line <= len(src) {
		line := src[f.Line-1]
		if i := strings.Index(strings.ToUpper(line), f.Name); i >= 0 {
			return line[i : i+len(f.Name)]
		}
	}
	return f.Name
}

func addError(t *TestResult, err error) {
	switch err.(type) {
	case *vm.QuitSignal:
		return // QUIT ends the test normally
	}
	t.Failures = append(t.Failures, "runtime error: "+vm.ErrorReport(err))
}

// normalize makes output comparable: Unix line ends, no trailing spaces,
// no leading blank line (from the first ?) and no trailing blank lines.
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	s = strings.Join(lines, "\n")
	s = strings.TrimPrefix(s, "\n")
	return strings.TrimRight(s, "\n")
}

// diff returns a line diff of want and got: "- " lines are missing from
// got, "+ " lines are extra, and up to 2 unchanged lines of context are
// kept around each change.
func diff(want, got []string) []string {
	n, m := len(want), len(got)
	// longest common subsequence table
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if want[i] == got[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	type op struct {
		kind byte
		text string
	}
	var ops []op
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && want[i] == got[j]:
			ops = append(ops, op{' ', want[i]})
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', want[i]})
			i++
		default:
			ops = append(ops, op{'+', got[j]})
			j++
		}
	}
	// keep each change plus 2 unchanged lines either side
	const context, limit = 2, 60
	keep := make([]bool, len(ops))
	for k, o := range ops {
		if o.kind != ' ' {
			for c := max(0, k-context); c <= min(len(ops)-1, k+context); c++ {
				keep[c] = true
			}
		}
	}
	var out []string
	for k, o := range ops {
		if !keep[k] {
			if len(out) > 0 && out[len(out)-1] != "..." {
				out = append(out, "...")
			}
			continue
		}
		out = append(out, string(o.kind)+" "+o.text)
		if len(out) >= limit {
			return append(out, "... (more differences)")
		}
	}
	if len(out) > 0 && out[len(out)-1] == "..." {
		out = out[:len(out)-1]
	}
	return out
}

// testPrgHint explains what to do when "nantucket test" finds no tests
// but there is a program called test.prg, which the user probably meant
// to run.
func testPrgHint(args []string) string {
	if len(args) > 0 {
		return ""
	}
	for _, name := range []string{"test.prg", "TEST.PRG", "Test.prg"} {
		if st, err := os.Stat(name); err == nil && !st.IsDir() {
			return "To run your program " + name + ", type: nantucket " + name
		}
	}
	return ""
}
