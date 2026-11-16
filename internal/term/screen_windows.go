// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package term

import "github.com/gdamore/tcell/v2"

// newTcellScreen uses tcell's Win32 console backend on Windows. The default
// (tcell.NewScreen) picks the VT backend, whose console reader passes on the
// character from every key record, key releases included, so unless the
// console honours win32-input-mode every key press arrives twice (or more
// with auto-repeat). The console backend ignores key releases.
func newTcellScreen() (tcell.Screen, error) {
	return tcell.NewConsoleScreen()
}
