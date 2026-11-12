// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWriteRead(t *testing.T) {
	dir := t.TempDir()
	rt := write(t, dir, "runtime", "#!fake runtime\x00\x01\x02")
	if _, err := Read(rt); !errors.Is(err, ErrNoBundle) {
		t.Fatalf("plain runtime: %v, want ErrNoBundle", err)
	}
	b := &Bundle{Main: "a.prg", Version: "0.2", Files: map[string][]byte{"a.prg": []byte("? 1"), "x.ch": []byte("#define X 1")}}
	out := filepath.Join(dir, "a")
	if err := Write(out, rt, b); err != nil {
		t.Fatal(err)
	}
	got, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Main != "a.prg" || got.Version != "0.2" || string(got.Files["x.ch"]) != "#define X 1" || len(got.Files) != 2 {
		t.Fatalf("read back %+v", got)
	}
	data, _ := os.ReadFile(out)
	if !strings.HasPrefix(string(data), "#!fake runtime") {
		t.Error("the runtime should come first")
	}
	if st, _ := os.Stat(out); st.Mode()&0o100 == 0 {
		t.Error("output should be executable")
	}
	// building on a packaged program replaces its bundle
	b2 := &Bundle{Main: "b.prg", Files: map[string][]byte{"b.prg": []byte("? 2")}}
	out2 := filepath.Join(dir, "b")
	if err := Write(out2, out, b2); err != nil {
		t.Fatal(err)
	}
	got, err = Read(out2)
	if err != nil || got.Main != "b.prg" || len(got.Files) != 1 {
		t.Fatalf("rebuilt: %+v %v", got, err)
	}
	data2, _ := os.ReadFile(out2)
	if strings.Count(string(data2), magic) != 1 {
		t.Error("old bundle was not removed")
	}
	// a damaged trailer is reported, not run
	os.WriteFile(out2, append(data2[:len(data2)-20], data2[len(data2)-16:]...), 0o755)
	if _, err := Read(out2); err == nil || errors.Is(err, ErrNoBundle) {
		t.Errorf("damaged bundle: %v", err)
	}
}

func TestCollect(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "lib")
	os.Mkdir(sub, 0o755)
	main := write(t, dir, "main.prg", `#include "app.ch"
#include "inkey.ch"
* DO commented
SET PROCEDURE TO lib/util
DO WHILE .T.
   EXIT
ENDDO
DO CASE
ENDCASE
DO Report WITH 1
DO nowhere
SET PROCEDURE TO missing
`)
	write(t, dir, "app.ch", `#include "more.ch"`)
	write(t, dir, "more.ch", `#define X 1`)
	write(t, sub, "util.prg", `DO helper`)
	write(t, sub, "helper.prg", `? "help"`)
	write(t, dir, "report.prg", `? "r"`)
	write(t, dir, "commented.prg", `? "not used"`)
	extra := write(t, dir, "extra.prg", `? "x"`)
	b, warns, err := Collect(main, []string{extra}, func(n string) bool { return n == "inkey.ch" })
	if err != nil {
		t.Fatal(err)
	}
	want := "app.ch extra.prg helper.prg main.prg more.ch report.prg util.prg"
	if got := strings.Join(b.Names(), " "); got != want {
		t.Errorf("files = %s\nwant    %s", got, want)
	}
	if b.Main != "main.prg" {
		t.Errorf("main = %s", b.Main)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "missing") {
		t.Errorf("warnings = %q", warns)
	}
	// two different files with one name can't both be packaged
	write(t, sub, "report.prg", `? "other"`)
	write(t, sub, "util.prg", `DO helper
DO report`)
	os.Remove(filepath.Join(dir, "report.prg"))
	write(t, dir, "main.prg", `SET PROCEDURE TO lib/util
DO report`)
	write(t, dir, "report.prg", `? "r"`)
	if _, _, err := Collect(main, nil, func(string) bool { return false }); err == nil || !strings.Contains(err.Error(), "two files") {
		t.Errorf("clash: %v", err)
	}
}
