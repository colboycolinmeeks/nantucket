// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeDir(t *testing.T) {
	e := newSim(t)
	base, _ := filepath.EvalSymlinks(e.dir)
	os.MkdirAll(filepath.Join(e.dir, "sub", "deeper"), 0o755)
	os.WriteFile(filepath.Join(e.dir, "afile"), nil, 0o644)
	wd := func() string { d, _ := os.Getwd(); d, _ = filepath.EvalSymlinks(d); return d }

	steps := []struct{ src, want string }{
		{`CD sub`, filepath.Join(base, "sub")},
		{`cd deeper`, filepath.Join(base, "sub", "deeper")},
		{`CD ..`, filepath.Join(base, "sub")},
		{`CD -`, filepath.Join(base, "sub", "deeper")},
		{`__Run("cd ../..")`, base}, // what !cd and RUN cd become
		{`RUN cd sub`, filepath.Join(base, "sub")},
		{`CHDIR "` + base + `"`, base},
	}
	for _, s := range steps {
		if err := e.wait(e.run(s.src)); err != nil {
			t.Fatalf("%s: %v", s.src, err)
		}
		if got := wd(); got != s.want {
			t.Fatalf("%s: in %s, want %s", s.src, got, s.want)
		}
	}
	for _, bad := range []string{`CD nowhere`, `CD afile`} {
		err := e.wait(e.run(bad))
		if err == nil {
			t.Errorf("%s: expected an error", bad)
			continue
		}
		if !strings.Contains(ErrorReport(err), "irectory") {
			t.Errorf("%s: error %q", bad, ErrorReport(err))
		}
		if wd() != base {
			t.Errorf("%s moved to %s", bad, wd())
		}
	}
	// CD on its own shows the directory
	if err := e.wait(e.run("CLS\nCD")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.screen(), filepath.Base(base)) {
		t.Errorf("CD did not show the directory:\n%s", e.screen())
	}
	// a variable called cd is still a variable
	if err := e.wait(e.run(`cd := 5`)); err != nil {
		t.Fatal(err)
	}
	if e.str("cd") != "5" || wd() != base {
		t.Errorf("cd := 5 gave cd=%s, dir %s", e.str("cd"), wd())
	}
}

func TestShellCD(t *testing.T) {
	for in, want := range map[string]string{"cd examples": "examples", "  CD ../x ": "../x", "cd": "", "chdir /tmp": "/tmp"} {
		if got, ok := shellCD(in); !ok || got != want {
			t.Errorf("shellCD(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"cd x && ls", "ls", "cdx", "cd x; pwd", "cd $(pwd)"} {
		if _, ok := shellCD(in); ok {
			t.Errorf("shellCD(%q) should go to the shell", in)
		}
	}
}
