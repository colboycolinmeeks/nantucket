// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package nantucket holds files from the repository root that are built
// into the binary.
package nantucket

import _ "embed"

// WhatsNew is WHATSNEW.md: release notes, newest release first.
//
//go:embed WHATSNEW.md
var WhatsNew string
