// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package features links in every optional part of the runtime. Programs
// that run xBase code (the nantucket command, the test runner) import it
// for its side effects; each package registers its built-in functions with
// vm when it is initialised.
package features

import (
	_ "nantucket/internal/dbcmd"    // LIST, COUNT, SUM, COPY TO, APPEND FROM, SORT, TOTAL...
	_ "nantucket/internal/debugger" // the debugger (Alt-D, AltD())
	_ "nantucket/internal/forms"    // REPORT FORM, LABEL FORM
	_ "nantucket/internal/gets"     // @ ... GET, READ, MENU TO, Alert(), AChoice()
	_ "nantucket/internal/screens"  // BROWSE, EDIT, CREATE, MODIFY, windows, HELP
)
