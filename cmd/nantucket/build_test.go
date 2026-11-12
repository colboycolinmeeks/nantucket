// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary builds the nantucket executable for a test (skipped with
// -short or without the Go toolchain).
func buildBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the binary")
	}
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not found")
	}
	bin := filepath.Join(t.TempDir(), "nantucket")
	if out, err := exec.Command(gobin, "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// TestBuildAndRun builds the nantucket binary, packages a small program
// with "nantucket build", and runs the result from another directory.
func TestBuildAndRun(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	os.Mkdir(src, 0o755)
	files := map[string]string{
		"hello.prg": `#include "hello.ch"
PARAMETERS cName
SET PROCEDURE TO lib
? GREETING, cName, Twice(21)
DO footer
`,
		"hello.ch":   `#define GREETING "Hello"`,
		"lib.prg":    "FUNCTION Twice(n)\nRETURN n * 2\n",
		"footer.prg": `? "bye"`,
		"broken.prg": "? 1 +\n",
	}
	for name, body := range files {
		os.WriteFile(filepath.Join(src, name), []byte(body), 0o644)
	}
	run := func(dir string, name string, args ...string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run(src, bin, "build", "-o", filepath.Join(dir, "hello"), "hello")
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if !strings.Contains(out, "footer.prg, hello.ch, hello.prg, lib.prg") {
		t.Errorf("build output: %s", out)
	}
	elsewhere := t.TempDir() // no sources here
	out, err = run(elsewhere, filepath.Join(dir, "hello"), "world")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Hello world         42") || !strings.Contains(out, "bye") {
		t.Errorf("packaged program printed:\n%s", out)
	}
	// compile errors stop the build
	out, err = run(src, bin, "build", "-o", filepath.Join(dir, "broken"), "broken.prg")
	if err == nil || !strings.Contains(out, "not built") {
		t.Errorf("broken program: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "broken")); err == nil {
		t.Error("a broken program should not be written")
	}
}
