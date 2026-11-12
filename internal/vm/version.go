// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"strings"

	"nantucket"
)

// VersionLine is what VERSION shows.
const VersionLine = "Nantucket " + Version + " - dBase III / Clipper compatible environment"

// CopyrightLine is the licence notice VERSION FULL shows.
const CopyrightLine = "Copyright 2026 Colin Meeks. Apache License 2.0: see LICENSE and NOTICE."

// WhatsNew returns the release notes for one version from WHATSNEW.md
// (the lines under its "## <version>" heading), or "" if there are none.
func WhatsNew(version string) string {
	var out []string
	in := false
	for _, l := range strings.Split(nantucket.WhatsNew, "\n") {
		if h, ok := strings.CutPrefix(l, "## "); ok {
			if in {
				break
			}
			in = strings.TrimSpace(h) == version
			continue
		}
		if in {
			out = append(out, strings.TrimRight(l, " \r"))
		}
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}
