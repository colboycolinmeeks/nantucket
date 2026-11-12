// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"reflect"
	"testing"
)

func sampleReport() *Report {
	return &Report{
		Heading: []string{"Customer balances", "by state"}, GroupExpr: "state", GroupHeader: "State",
		PageWidth: 80, Lines: 58, LeftMargin: 2, RightMargin: 0, EjectBefore: true, EjectAfter: true,
		Columns: []Column{
			{Header: []string{"Name"}, Expr: "name", Width: 20},
			{Header: []string{"Balance", "(USD)"}, Expr: "balance", Width: 10, Decimals: 2, Total: true},
		},
	}
}

func TestReportRoundTrip(t *testing.T) {
	r := sampleReport()
	data, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1990 {
		t.Fatalf("size %d", len(data))
	}
	got, err := ParseFRM(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, r) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, r)
	}
}

func TestLabelRoundTrip(t *testing.T) {
	l := &Label{Remark: "3 across", Height: 5, Width: 35, LeftMargin: 1, LinesBetween: 1, SpacesBetween: 2, Across: 3,
		Lines: []string{"Trim(first), Trim(last)", "street", "", "city", "zip"}}
	data, err := l.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseLBL(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, l) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, l)
	}
}

func FuzzParseFRM(f *testing.F) {
	data, _ := sampleReport().Encode()
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) { ParseFRM(data) })
}

func FuzzParseLBL(f *testing.F) {
	data, _ := (&Label{Height: 3, Width: 30, Across: 2, Lines: []string{"a", "b"}}).Encode()
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) { ParseLBL(data) })
}
