// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The source lines that bring in other files, as the interpreter finds
// them at run time: DO <name>, SET PROCEDURE TO <file> and #include.
var (
	doRe      = regexp.MustCompile(`(?i)^\s*DO\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:WITH\b.*)?$`)
	setProcRe = regexp.MustCompile(`(?i)^\s*SET\s+PROCEDURE\s+TO\s+["']?([^\s"']+)`)
	includeRe = regexp.MustCompile(`(?i)^\s*#\s*include\s+["<]([^">]+)[">]`)
)

// Collect gathers the main program and every .prg and header file it
// uses, following DO, SET PROCEDURE TO and #include from file to file.
// Files are looked for next to the file that uses them, then next to the
// main program. extra adds files the scan can't see (for example a
// program run with a macro: DO &cName). isStdHeader reports headers built
// into Nantucket, which need not be packaged.
//
// It returns the bundle and warnings about references it couldn't find.
func Collect(mainPath string, extra []string, isStdHeader func(string) bool) (*Bundle, []string, error) {
	mainDir := filepath.Dir(mainPath)
	b := &Bundle{Main: filepath.Base(mainPath), Files: map[string][]byte{}}
	from := map[string]string{} // upper-case name -> path, to catch clashes
	var warnings []string
	var queue []string

	add := func(path string) error {
		name := filepath.Base(path)
		key := strings.ToUpper(name)
		if prev, ok := from[key]; ok {
			if sameFile(prev, path) {
				return nil
			}
			return fmt.Errorf("two files are both called %s: %s and %s", name, prev, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		from[key] = path
		b.Files[name] = data
		if isSource(name) {
			queue = append(queue, path)
		}
		return nil
	}
	if err := add(mainPath); err != nil {
		return nil, nil, err
	}
	b.Main = filepath.Base(mainPath)
	for _, x := range extra {
		if err := add(x); err != nil {
			return nil, nil, err
		}
	}

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		dirs := []string{filepath.Dir(path)}
		if filepath.Dir(path) != mainDir {
			dirs = append(dirs, mainDir)
		}
		for i, line := range strings.Split(string(b.Files[filepath.Base(path)]), "\n") {
			line = strings.TrimRight(line, "\r")
			where := fmt.Sprintf("%s:%d", filepath.Base(path), i+1)
			if m := includeRe.FindStringSubmatch(line); m != nil {
				if p := find(dirs, m[1], ""); p != "" {
					if err := add(p); err != nil {
						return nil, nil, err
					}
				} else if !isStdHeader(m[1]) {
					warnings = append(warnings, where+": include file "+m[1]+" not found")
				}
				continue
			}
			line = stripComment(line)
			if m := setProcRe.FindStringSubmatch(line); m != nil {
				if p := find(dirs, m[1], ".prg"); p != "" {
					if err := add(p); err != nil {
						return nil, nil, err
					}
				} else {
					warnings = append(warnings, where+": SET PROCEDURE TO "+m[1]+": file not found")
				}
				continue
			}
			if m := doRe.FindStringSubmatch(line); m != nil {
				name := strings.ToUpper(m[1])
				if name == "WHILE" || name == "CASE" {
					continue
				}
				// DO can also call a procedure defined in a file already
				// collected, so a missing file is not an error
				if p := find(dirs, m[1], ".prg"); p != "" {
					if err := add(p); err != nil {
						return nil, nil, err
					}
				}
			}
		}
	}
	return b, warnings, nil
}

func isSource(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".prg", ".ch":
		return true
	}
	return false
}

// stripComment drops * and // comment lines and trailing && or //
// comments (outside strings, roughly).
func stripComment(line string) string {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "*") || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "&&") {
		return ""
	}
	if i := strings.Index(line, "&&"); i >= 0 {
		line = line[:i]
	}
	return line
}

// find looks for name (adding ext if it has none) in dirs, trying the name
// as written, lower case and upper case.
func find(dirs []string, name, ext string) string {
	if ext != "" && filepath.Ext(name) == "" {
		name += ext
	}
	if filepath.IsAbs(name) {
		dirs = []string{""}
	}
	for _, d := range dirs {
		dir, file := filepath.Split(filepath.Join(d, name))
		for _, f := range []string{file, strings.ToLower(file), strings.ToUpper(file)} {
			p := filepath.Join(dir, f)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

func sameFile(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}
