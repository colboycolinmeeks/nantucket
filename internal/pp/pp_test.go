// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package pp

import (
	"strings"
	"testing"

	"nantucket/internal/lex"
)

func expand(t *testing.T, src string) string {
	p, err := NewStd(nil)
	if err != nil {
		t.Fatal(err)
	}
	st, err := p.ProcessSource(src, "t")
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, s := range st {
		parts = append(parts, lex.Join(s.Toks))
	}
	return strings.Join(parts, " | ")
}

func TestStd(t *testing.T) {
	cases := []string{
		"? 1, 'a'",
		"USE customer",
		"USE (cFile) ALIAS cust NEW INDEX a, b",
		"REPLACE name WITH 'x', age WITH 5",
		"REPLACE ALL age WITH age + 1 FOR age > 3",
		"LIST name, age FOR age > 30",
		"LIST",
		"DISP STRU",
		"LIST MEMORY",
		"@ 5, 10 SAY 'Name' GET cName PICTURE '@!' VALID !Empty(cName)",
		"@ 1,2 TO 10,40 DOUBLE",
		"STORE 0 TO a, b, c",
		"SET EXACT ON",
		"SET COLOR TO W/B,N/W",
		"INDEX ON name + city TO custname",
		"SUM amount, tax TO nAmt, nTax FOR paid",
		"COUNT TO n",
		"DELETE ALL FOR age < 18",
		"SET RELATION TO custid INTO cust",
		"x = 5",
		"count = 5",
		"MODI COMM test",
		"SELECT 2",
		"GO TOP",
		"SKIP -1",
		"SEEK 'Smith'",
		"#define MAXV 10\n? MAXV",
		"#define SQ(x) ((x)*(x))\n? SQ(3)",
		"SET FILTER TO &cf",
		"DEFAULT a TO 1, b TO 2",
	}
	for _, c := range cases {
		t.Logf("%-50s => %s", strings.ReplaceAll(c, "\n", "\\n"), expand(t, c))
	}
}
