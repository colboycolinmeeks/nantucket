// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package app packages a program as a single executable: a copy of the
// Nantucket runtime with the program's source files appended as a zip
// archive, followed by a short trailer. When Nantucket starts it looks for
// the trailer at the end of its own executable and, if it is there, runs
// the packaged program instead of the dot prompt.
//
// Layout of a packaged executable:
//
//	runtime executable | zip archive | zip length (8 bytes, LE) | magic (8 bytes)
//
// The zip holds the program files under their base names and a manifest,
// app.json, naming the main program.
package app

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

// magic marks the end of a packaged executable.
const magic = "NTKTAPP1"

const trailerLen = 16

// manifestName is the manifest inside the archive.
const manifestName = "app.json"

// Bundle is a packaged program.
type Bundle struct {
	// Main is the main program's file name, e.g. "invoice.prg".
	Main string `json:"main"`
	// Version is the Nantucket version it was built with.
	Version string `json:"version"`
	// Files maps file names (base names, such as "invoice.prg" or
	// "app.ch") to their contents.
	Files map[string][]byte `json:"-"`
}

// Names lists the bundle's files in order.
func (b *Bundle) Names() []string {
	var names []string
	for n := range b.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Write creates out as a copy of the runtime executable with the bundle
// appended. A runtime that is itself a packaged program has its old
// bundle removed first.
func Write(out, runtime string, b *Bundle) error {
	exe, err := os.ReadFile(runtime)
	if err != nil {
		return fmt.Errorf("reading runtime: %w", err)
	}
	if n, ok := payloadStart(exe); ok {
		exe = exe[:n]
	}
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	man, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := addFile(zw, manifestName, man); err != nil {
		return err
	}
	for _, name := range b.Names() {
		if err := addFile(zw, name, b.Files[name]); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	var trailer [trailerLen]byte
	binary.LittleEndian.PutUint64(trailer[:8], uint64(zbuf.Len()))
	copy(trailer[8:], magic)

	tmp := out + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	for _, part := range [][]byte{exe, zbuf.Bytes(), trailer[:]} {
		if _, err := f.Write(part); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, out)
}

func addFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// payloadStart reports where an appended bundle starts in exe, if it has
// one.
func payloadStart(exe []byte) (int, bool) {
	if len(exe) < trailerLen || string(exe[len(exe)-8:]) != magic {
		return 0, false
	}
	n := binary.LittleEndian.Uint64(exe[len(exe)-trailerLen:])
	start := int64(len(exe)) - trailerLen - int64(n)
	if n == 0 || start < 0 {
		return 0, false
	}
	return int(start), true
}

// ErrNoBundle means the executable has no packaged program.
var ErrNoBundle = errors.New("no packaged program")

// Read loads the bundle appended to an executable, or returns ErrNoBundle.
func Read(exe string) (*Bundle, error) {
	f, err := os.Open(exe)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size < trailerLen {
		return nil, ErrNoBundle
	}
	var trailer [trailerLen]byte
	if _, err := f.ReadAt(trailer[:], size-trailerLen); err != nil {
		return nil, err
	}
	if string(trailer[8:]) != magic {
		return nil, ErrNoBundle
	}
	n := int64(binary.LittleEndian.Uint64(trailer[:8]))
	start := size - trailerLen - n
	if n <= 0 || start < 0 {
		return nil, fmt.Errorf("packaged program is damaged")
	}
	zr, err := zip.NewReader(io.NewSectionReader(f, start, n), n)
	if err != nil {
		return nil, fmt.Errorf("packaged program is damaged: %w", err)
	}
	b := &Bundle{Files: map[string][]byte{}}
	for _, zf := range zr.File {
		rc, err := zf.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		if zf.Name == manifestName {
			if err := json.Unmarshal(data, b); err != nil {
				return nil, fmt.Errorf("packaged program is damaged: %w", err)
			}
			continue
		}
		b.Files[zf.Name] = data
	}
	if b.Main == "" || b.Files[b.Main] == nil {
		return nil, fmt.Errorf("packaged program has no main program")
	}
	return b, nil
}

// Self loads the bundle appended to the running executable, if any.
func Self() (*Bundle, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, ErrNoBundle
	}
	return Read(exe)
}
