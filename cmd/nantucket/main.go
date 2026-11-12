// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Command nantucket is a dBase/Clipper compatible interpreter with a dot prompt.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	xterm "golang.org/x/term"

	"nantucket/internal/app"
	_ "nantucket/internal/features" // the commands outside the core
	"nantucket/internal/repl"
	"nantucket/internal/term"
	"nantucket/internal/testrun"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

func main() {
	// a program packaged with "nantucket build" runs straight away, and
	// gets every argument
	if b, err := app.Self(); err == nil {
		os.Exit(runBundle(b, os.Args[1:]))
	} else if !errors.Is(err, app.ErrNoBundle) {
		fmt.Fprintln(os.Stderr, "nantucket:", err)
		os.Exit(2)
	}
	if len(os.Args) > 1 && os.Args[1] == "test" {
		os.Exit(testrun.Main(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "build" {
		os.Exit(buildMain(os.Args[2:]))
	}
	stream := flag.Bool("stream", false, "use plain line output instead of the full screen console")
	cmds := flag.String("c", "", "execute commands (separated by newlines or ;) and exit")
	noPause := flag.Bool("q", false, "do not wait for a key when a program ends")
	cpuProfile := flag.String("cpuprofile", "", "write a Go CPU profile to this file (for developers)")
	traceFile := flag.String("trace", "", "write a trace of every procedure call and program line to this file (- for standard error)")
	debugStart := flag.Bool("debug", false, "open the debugger at the program's first line (Alt-D opens it while a program runs)")
	themeName := flag.String("theme", "", "colour theme: "+strings.Join(term.ThemeNames(), ", ")+" (default: $NANTUCKET_THEME, else picked from the terminal)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: nantucket [options] [program[.prg] [args...]]\n       nantucket test [options] [files or directories...]\n       nantucket build [options] program[.prg]\n\nWith no program, starts the interactive dot prompt.\nRun \"nantucket test -h\" or \"nantucket build -h\" for their options.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	theme, err := pickTheme(*themeName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nantucket:", err)
		os.Exit(2)
	}
	machine, con, interactive := start(*stream, theme)
	stopProfile := func() {}
	if *cpuProfile != "" {
		if f, err := os.Create(*cpuProfile); err == nil && pprof.StartCPUProfile(f) == nil {
			stopProfile = pprof.StopCPUProfile
		}
	}
	if *traceFile != "" {
		if *traceFile == "-" {
			machine.SetTrace(os.Stderr)
		} else if f, err := os.Create(*traceFile); err != nil {
			fmt.Fprintln(os.Stderr, "nantucket: cannot create trace file:", err)
		} else {
			defer f.Close()
			machine.SetTrace(f)
		}
	}
	if *debugStart {
		if !interactive {
			fmt.Fprintln(os.Stderr, "nantucket: -debug needs the full screen console")
		}
		machine.DebugAtStart()
	}
	code := 0
	switch {
	case *cmds != "":
		machine.PushBaseFrame()
		code = runCommands(machine, *cmds)
	case flag.NArg() > 0:
		code = runProgram(machine, con, flag.Arg(0), flag.Args()[1:], interactive && !*noPause)
	default:
		repl.Run(machine, con)
	}
	machine.Close()
	con.Close()
	if !interactive {
		fmt.Println()
	}
	stopProfile()
	os.Exit(code)
}

// pickTheme finds the theme named by -theme, else $NANTUCKET_THEME. An
// empty Theme means: choose from the terminal.
func pickTheme(name string) (term.Theme, error) {
	if name == "" {
		name = os.Getenv("NANTUCKET_THEME")
	}
	if name == "" {
		return term.Theme{}, nil
	}
	t, ok := term.LookupTheme(name)
	if !ok {
		return t, fmt.Errorf("unknown theme %q (choose from %s)", name, strings.Join(term.ThemeNames(), ", "))
	}
	return t, nil
}

// start sets up the console (full screen on a terminal, otherwise plain
// lines) and the runtime.
func start(stream bool, theme term.Theme) (*vm.VM, term.Console, bool) {
	interactive := !stream && xterm.IsTerminal(int(os.Stdin.Fd())) && xterm.IsTerminal(int(os.Stdout.Fd()))
	var con term.Console
	var scr *term.Screen
	if interactive {
		s, err := term.NewScreen()
		if err != nil {
			interactive = false
		} else {
			scr = s
			s.TermPalette = strings.EqualFold(os.Getenv("NANTUCKET_PALETTE"), "terminal")
			con = s
		}
	}
	if con == nil {
		con = term.NewStdStream()
	}
	machine, err := vm.New(con)
	if err != nil {
		con.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if theme.Name == "" {
		colors := 256
		if scr != nil {
			colors = scr.Colors()
		}
		theme = term.AutoTheme(colors)
	}
	machine.SetStartTheme(theme)
	if scr != nil {
		scr.OnInterrupt = machine.Interrupt
		scr.OnDebug = machine.RequestDebug
	} else {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		go func() {
			for range sig {
				machine.Interrupt()
			}
		}()
	}
	handleTermination(machine, con)
	return machine, con, interactive
}

// handleTermination saves everything and exits when Nantucket is told to
// end: SIGTERM (kill, a system shutdown) or SIGHUP (the terminal window
// was closed). Without this, changes still held in memory, such as index
// updates, would be lost.
func handleTermination(machine *vm.VM, con term.Console) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		s := <-sig
		saved := machine.Shutdown(3 * time.Second)
		con.Close()
		if !saved {
			fmt.Fprintln(os.Stderr, "nantucket: stopped before changes could be saved; out of date indexes will be rebuilt when next opened")
		}
		code := 128 + 15
		if s == syscall.SIGHUP {
			code = 128 + 1
		}
		os.Exit(code)
	}()
}

func runCommands(m *vm.VM, src string) int {
	err := m.Protect(func() { m.ExecStatements(src) })
	return repl.Report(m, err)
}

func runProgram(m *vm.VM, con term.Console, file string, args []string, pause bool) int {
	path := m.FindFile(file, ".prg")
	if path == "" {
		con.Close()
		fmt.Fprintf(os.Stderr, "nantucket: cannot find program %s\n", file)
		os.Exit(2)
	}
	var vargs []value.Value
	for _, a := range args {
		vargs = append(vargs, value.Str(a))
	}
	m.PushBaseFrame()
	err := m.RunFile(path, vargs)
	quit := false
	if _, ok := err.(*vm.QuitSignal); ok {
		quit = true
		err = nil
	}
	code := 0
	if err != nil {
		if !con.Interactive() {
			// make errors visible on stderr in batch use
			fmt.Fprintln(os.Stderr, "\n"+vm.ErrorReport(err))
			return 1
		}
		code = repl.Report(m, err)
	}
	if pause && !quit {
		m.Println("")
		m.Println("Press any key to exit...")
		con.Flush()
		con.ReadKey(-1)
	}
	return code
}
