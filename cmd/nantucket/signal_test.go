// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestStoppedProcessKeepsIndex is the Arch #1 report: a process adds a
// record to an indexed table and is then stopped. Whether it is killed
// outright or told to end, the record must still be found through the
// index afterwards.
func TestStoppedProcessKeepsIndex(t *testing.T) {
	bin := buildBinary(t)
	cases := []struct {
		name      string
		sig       syscall.Signal
		busy      bool // stopped while running code, not waiting for a key
		wantClean bool // the index was saved before exiting
	}{
		{"killed", syscall.SIGKILL, false, false},
		{"terminated while waiting", syscall.SIGTERM, false, true},
		{"terminal closed", syscall.SIGHUP, false, true},
		{"terminated while busy", syscall.SIGTERM, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			nant := func(args ...string) string {
				cmd := exec.Command(bin, append([]string{"-stream", "-q"}, args...)...)
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return string(out)
			}
			nant("-c", `dbCreate("t", {{"NAME", "C", 10, 0}})
USE t
APPEND BLANK
REPLACE name WITH "ALPHA"
INDEX ON name TO tn
CLOSE ALL`)
			wait := `Inkey(0)`
			if c.busy {
				wait = `DO WHILE .T.
   n++
ENDDO`
			}
			os.WriteFile(filepath.Join(dir, "writer.prg"), []byte(`n := 0
USE t INDEX tn
APPEND BLANK
REPLACE name WITH "BRAVO"
SKIP 0
? "READY"
? ""
`+wait+"\n"), 0o644)

			cmd := exec.Command(bin, "-stream", "-q", "writer.prg")
			cmd.Dir = dir
			stdin, _ := cmd.StdinPipe() // kept open: Inkey(0) waits
			defer stdin.Close()
			stdout, _ := cmd.StdoutPipe()
			cmd.Stderr = io.Discard
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			ready := make(chan bool, 1)
			go func() {
				sc := bufio.NewScanner(stdout)
				for sc.Scan() {
					if strings.Contains(sc.Text(), "READY") {
						ready <- true
					}
				}
			}()
			select {
			case <-ready:
			case <-time.After(10 * time.Second):
				cmd.Process.Kill()
				t.Fatal("writer did not start")
			}
			time.Sleep(50 * time.Millisecond)
			cmd.Process.Signal(c.sig)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				cmd.Process.Kill()
				t.Fatal("writer did not stop")
			}

			hdr := make([]byte, 1024)
			f, err := os.Open(filepath.Join(dir, "tn.ntx"))
			if err != nil {
				t.Fatal(err)
			}
			f.ReadAt(hdr, 0)
			f.Close()
			if clean := hdr[1008] == 0; clean != c.wantClean {
				t.Errorf("index saved cleanly = %v, want %v", clean, c.wantClean)
			}
			out := nant("-c", `USE t INDEX tn
SEEK "BRAVO"
? "found", Found(), "records", LastRec(), "keys", ordKeyCount()`)
			if !strings.Contains(out, "found .T. records          2 keys          2") {
				t.Errorf("after stopping: %s", strings.TrimSpace(out))
			}
		})
	}
}
