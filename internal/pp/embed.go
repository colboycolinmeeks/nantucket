// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package pp

import (
	"embed"
	"strings"
	"sync"
)

//go:embed include/*.ch
var headers embed.FS

// Header returns the text of a built-in header file.
func Header(name string) (string, bool) {
	b, err := headers.ReadFile("include/" + strings.ToLower(name))
	if err != nil {
		return "", false
	}
	return string(b), true
}

var (
	stdOnce sync.Once
	stdPP   *PP
	stdErr  error
)

// NewStd returns a preprocessor preloaded with std.ch. include resolves
// user header files; built-in headers are always available.
func NewStd(include func(name string) (string, string, error)) (*PP, error) {
	stdOnce.Do(func() {
		p := New()
		p.Include = builtinInclude(nil)
		src, _ := Header("std.ch")
		stdErr = p.LoadSource(src, "std.ch")
		stdPP = p
	})
	if stdErr != nil {
		return nil, stdErr
	}
	n := stdPP.Clone()
	n.Include = builtinInclude(include)
	return n, nil
}

func builtinInclude(user func(string) (string, string, error)) func(string) (string, string, error) {
	return func(name string) (string, string, error) {
		if user != nil {
			if s, p, err := user(name); err == nil {
				return s, p, nil
			}
		}
		if s, ok := Header(name); ok {
			return s, name, nil
		}
		return "", "", errNotFound
	}
}

type notFound struct{}

func (notFound) Error() string { return "include file not found" }

var errNotFound = notFound{}
