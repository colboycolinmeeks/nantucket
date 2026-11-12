// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"reflect"
	"testing"
)

func TestLogParts(t *testing.T) {
	got, err := logParts("a {x} b {{c}} {Len(s)}")
	want := []string{"a ", "x", " b {c} ", "Len(s)", ""}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %q %v, want %q", got, err, want)
	}
	if _, err := logParts("a {x"); err == nil {
		t.Error("unclosed { accepted")
	}
}
