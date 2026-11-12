// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"os"
	"strings"

	"nantucket/internal/db"
	"nantucket/internal/value"
)

// SET CODEPAGE TO <name> chooses the code page for tables whose header
// doesn't name one, and for new tables. The default is "AUTO" (see
// db.CodePage), or the NANTUCKET_CODEPAGE environment variable.

// CodePage is the session's code page.
func (vm *VM) CodePage() *db.CodePage {
	if vm.codePage == nil {
		cp, ok := db.LookupCodePage(os.Getenv("NANTUCKET_CODEPAGE"))
		if !ok {
			cp, _ = db.LookupCodePage("AUTO")
		}
		vm.codePage = cp
	}
	return vm.codePage
}

func init() {
	// __SETCODEPAGE( [<name>] ) -> old name; "" or no name: AUTO
	reg("__SETCODEPAGE", func(vm *VM, a []value.Value) value.Value {
		old := vm.CodePage().Name
		if has(a, 0) {
			cp, ok := db.LookupCodePage(argStr(a, 0, ""))
			if !ok {
				return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 1, SubCode: 2017,
					Description: "Unknown code page", Operation: argStr(a, 0, ""),
					Hint: "Code pages: " + strings.Join(db.CodePageNames(), ", ")})
			}
			vm.codePage = cp
		}
		return value.Str(old)
	})
	// DBCODEPAGE() -> the code page of the table in the current work area,
	// or of the session if none is open
	reg("DBCODEPAGE", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.areas[vm.curArea]; wa != nil && wa.T.CodePage() != nil {
			return value.Str(wa.T.CodePage().Name)
		}
		return value.Str(vm.CodePage().Name)
	})
}
