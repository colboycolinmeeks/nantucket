// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package forms

import (
	"os"
	"strings"
	"testing"

	"nantucket/internal/report"
)

func TestCreateReportFrom(t *testing.T) {
	s := formSetup(t)
	s.Do(`SET SAFETY OFF
CREATE REPORT quick FROM name, bal`)
	data, err := os.ReadFile("quick.frm")
	if err != nil {
		t.Fatal(err)
	}
	r, err := report.ParseFRM(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Columns) != 2 || r.Columns[0].Expr != "NAME" || r.Columns[0].Width != 12 ||
		r.Columns[1].Width != 8 || r.Columns[1].Decimals != 2 || !r.Columns[1].Total || r.Columns[0].Total {
		t.Fatalf("columns: %+v", r.Columns)
	}
	// it prints
	s.Do(`REPORT FORM quick TO FILE out PLAIN NOCONSOLE`)
	got := readOut(t, "out.txt")
	if !strings.Contains(got, "Abbott") || !strings.Contains(got, "175.50") {
		t.Errorf("report:\n%s", got)
	}
	s.Fails(`CREATE REPORT bad FROM name, nosuch`, "no field NOSUCH")
}

func TestCreateLabelFrom(t *testing.T) {
	s := formSetup(t)
	s.Do(`SET SAFETY OFF
CREATE LABEL mail FROM name, street, bal`)
	data, err := os.ReadFile("mail.lbl")
	if err != nil {
		t.Fatal(err)
	}
	l, err := report.ParseLBL(data)
	if err != nil {
		t.Fatal(err)
	}
	if l.Height != 5 || l.Width != 35 || l.Lines[0] != "Trim(NAME)" || l.Lines[2] != "BAL" {
		t.Fatalf("label: %+v", l)
	}
	s.Do(`GO TOP
LABEL FORM mail TO FILE labels NOCONSOLE NEXT 1`)
	if got := readOut(t, "labels.txt"); !strings.HasPrefix(got, "Abbott\n1 High St\n100\n") {
		t.Errorf("labels:\n%q", got)
	}
}
