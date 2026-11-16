// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package term

import "github.com/gdamore/tcell/v2"

func newTcellScreen() (tcell.Screen, error) {
	return tcell.NewScreen()
}
