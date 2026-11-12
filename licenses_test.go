// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package nantucket

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestThirdPartyLicenses checks that every module in go.mod is attributed
// in NOTICE and THIRD_PARTY_LICENSES, so adding a dependency without its
// licence fails the build. When it fails, add the module's copyright
// notice and licence text to THIRD_PARTY_LICENSES and a line to NOTICE.
func TestThirdPartyLicenses(t *testing.T) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	licences, err := os.ReadFile("THIRD_PARTY_LICENSES")
	if err != nil {
		t.Fatal(err)
	}
	notice, err := os.ReadFile("NOTICE")
	if err != nil {
		t.Fatal(err)
	}
	// module paths in require lines and blocks
	re := regexp.MustCompile(`(?m)^(?:require\s+|\t)([a-z0-9.\-]+\.[a-z]+/[^\s]+)\s+v`)
	found := 0
	for _, m := range re.FindAllStringSubmatch(string(mod), -1) {
		path := m[1]
		found++
		// golang.org/x/sys is listed as "golang.org/x/sys" in
		// THIRD_PARTY_LICENSES and as "Go x/sys" in NOTICE.
		short := strings.TrimSuffix(path, "/v2")
		if !strings.Contains(string(licences), short) {
			t.Errorf("%s is not in THIRD_PARTY_LICENSES", path)
		}
		name := short[strings.LastIndex(short, "/")+1:]
		if !strings.Contains(string(notice), short) && !strings.Contains(string(notice), "x/"+name) {
			t.Errorf("%s is not in NOTICE", path)
		}
	}
	if found == 0 {
		t.Fatal("no modules found in go.mod")
	}
}
