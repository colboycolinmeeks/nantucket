// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"os"
	"strings"
	"testing"
)

// TestIndexFromFileCantRunCode is Sec #4: an index whose key runs code
// (the review's example creates a file) is refused when opened, unless
// SET TRUSTED is ON.
func TestIndexFromFileCantRunCode(t *testing.T) {
	t.Chdir(t.TempDir())
	s := newSession(t)
	s.do(`dbCreate("t", {{"NAME", "C", 10, 0}})
USE t
APPEND BLANK
REPLACE name WITH "ALPHA"
INDEX ON name + iif(FCreate("PWNED.txt") > 0, "", "") TO tn
CLOSE ALL`)
	os.Remove("PWNED.txt")
	victim := newSession(t)
	victim.fails(`USE t INDEX tn`, "calls FCREATE(), which isn't allowed in a data file")
	if _, err := os.Stat("PWNED.txt"); err == nil {
		t.Fatal("opening the index ran its code")
	}
	victim.do(`SET TRUSTED ON
USE t INDEX tn`)
	if _, err := os.Stat("PWNED.txt"); err != nil {
		t.Error("with SET TRUSTED ON the index should open normally")
	}
}

func TestDataExprCheck(t *testing.T) {
	s := newSession(t)
	// a program's own function is allowed in an index key
	if _, err := s.vm.LoadSource("KEYS", "FUNCTION MyKey(c)\nRETURN Upper(c)\n"); err != nil {
		t.Fatal(err)
	}
	for expr, ok := range map[string]bool{
		`Upper(name) + DToS(joined)`:           true,
		`iif(active, "A", "B") + Str(bal, 10)`: true,
		`Descend(balance)`:                     true,
		`cust->name + Left(city, 3)`:           true,
		`MyKey(name)`:                          true,
		`name + __Run("ls")`:                   false,
		`MemoWrit("x", name)`:                  false,
		`&cExpr`:                               false,
		`name + (x := "a")`:                    false,
		`Eval({|| 1})`:                         false,
		`oObj:Run()`:                           false,
	} {
		err := s.vm.CheckDataExpr(expr)
		if (err == nil) != ok {
			t.Errorf("%s: allowed = %v, want %v (%v)", expr, err == nil, ok, err)
		}
	}
	if err := s.vm.CheckDataExpr(`FErase("x")`); err == nil || !strings.Contains(err.Error(), "FERASE") {
		t.Errorf("error should name the function: %v", err)
	}
}
