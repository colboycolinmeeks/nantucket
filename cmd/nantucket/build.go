// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"nantucket/internal/app"
	"nantucket/internal/pp"
	"nantucket/internal/repl"
	"nantucket/internal/term"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

// listFlag collects a flag given several times or as a comma list.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error {
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			*l = append(*l, s)
		}
	}
	return nil
}

// buildMain is "nantucket build": package a program and the files it uses
// as a single executable.
func buildMain(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	out := fs.String("o", "", "output file (default: the program's name)")
	runtime := fs.String("runtime", "", "Nantucket executable to build on, e.g. one from \"make dist\" for another system (default: this one)")
	var with listFlag
	fs.Var(&with, "with", "extra files to include, comma separated or repeated (for programs the scan can't see, such as DO &cName)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `usage: nantucket build [options] program[.prg]

Packages program.prg, and the .prg and header files it uses (found by
following DO, SET PROCEDURE TO and #include), into a single executable
that runs the program without Nantucket installed. Every file is compiled
first, so errors are reported now rather than when the program runs.

Data files (.dbf, .ntx...) are not packaged: the program finds them in the
directory it is run from, as usual.

The source is not hidden: the files are stored as an ordinary zip archive
at the end of the executable, and anyone can unzip it and read them.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	main := fs.Arg(0)
	if filepath.Ext(main) == "" {
		main += ".prg"
	}
	if _, err := os.Stat(main); err != nil {
		fmt.Fprintf(os.Stderr, "nantucket build: cannot find %s\n", main)
		return 2
	}
	if *runtime == "" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "nantucket build: cannot find this executable:", err)
			return 2
		}
		*runtime = exe
	}
	if *out == "" {
		*out = strings.TrimSuffix(filepath.Base(main), filepath.Ext(main))
		if strings.EqualFold(filepath.Ext(*runtime), ".exe") {
			*out += ".exe"
		}
	}

	b, warnings, err := app.Collect(main, with, func(name string) bool {
		_, ok := pp.Header(name)
		return ok
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "nantucket build:", err)
		return 1
	}
	b.Version = vm.Version
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if errs := compileAll(b); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		fmt.Fprintln(os.Stderr, "nantucket build: not built because of the errors above")
		return 1
	}
	if err := app.Write(*out, *runtime, b); err != nil {
		fmt.Fprintln(os.Stderr, "nantucket build:", err)
		return 1
	}
	size := int64(0)
	if st, err := os.Stat(*out); err == nil {
		size = st.Size()
	}
	fmt.Printf("Built %s (%.1f MB) from %s\n", *out, float64(size)/(1<<20), strings.Join(b.Names(), ", "))
	return 0
}

// loadBundle gives a runtime the bundle's files.
func loadBundle(m *vm.VM, b *app.Bundle) {
	m.Packaged = true
	m.Embedded = map[string]string{}
	m.EmbeddedHeaders = map[string]string{}
	for name, data := range b.Files {
		if strings.EqualFold(filepath.Ext(name), ".prg") {
			m.Embedded[strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))] = string(data)
		} else {
			m.EmbeddedHeaders[strings.ToUpper(name)] = string(data)
		}
	}
}

// compileAll compiles every .prg in the bundle and returns the errors.
func compileAll(b *app.Bundle) []string {
	m, err := vm.New(term.NewStream(strings.NewReader(""), io.Discard))
	if err != nil {
		return []string{err.Error()}
	}
	loadBundle(m, b)
	var errs []string
	for _, name := range b.Names() {
		if !strings.EqualFold(filepath.Ext(name), ".prg") {
			continue
		}
		if _, err := m.CompileSource(string(b.Files[name]), name); err != nil {
			errs = append(errs, err.Error())
		}
	}
	return errs
}

// runBundle runs a packaged program. The theme comes from
// $NANTUCKET_THEME, and output is plain lines when not on a terminal.
func runBundle(b *app.Bundle, args []string) int {
	theme, err := pickTheme("")
	if err != nil {
		theme = term.Theme{}
	}
	m, con, interactive := start(false, theme)
	loadBundle(m, b)
	var vargs []value.Value
	for _, a := range args {
		vargs = append(vargs, value.Str(a))
	}
	m.PushBaseFrame()
	err = m.RunEmbedded(b.Main, vargs)
	code := 0
	if _, ok := err.(*vm.QuitSignal); ok {
		err = nil
	}
	if err != nil {
		if !interactive {
			fmt.Fprintln(os.Stderr, "\n"+vm.ErrorReport(err))
			code = 1
		} else {
			code = repl.Report(m, err)
			// keep the error on screen until a key is pressed
			m.Println("")
			m.Println("Press any key to exit...")
			con.Flush()
			con.ReadKey(-1)
		}
	}
	m.Close()
	con.Close()
	if !interactive {
		fmt.Println()
	}
	return code
}
