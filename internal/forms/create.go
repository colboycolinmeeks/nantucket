// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package forms

// CREATE REPORT <file> FROM <fields> and CREATE LABEL <file> FROM
// <fields>: make a simple report or label from fields of the current
// table, without the designers.

import (
	"os"
	"path/filepath"
	"strings"

	"nantucket/internal/db"
	"nantucket/internal/gets"
	"nantucket/internal/report"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

// FieldColumn returns a report column showing a field of wa, with the
// field's width and decimals (or its name's length, if longer), its name
// as the heading, and a total for numbers. ok is false if wa has no such
// field.
func FieldColumn(m *vm.VM, wa *db.WorkArea, name string) (col report.Column, ok bool) {
	if wa == nil {
		return col, false
	}
	i := wa.T.FieldIndex(strings.TrimSpace(name))
	if i < 0 {
		return col, false
	}
	fd := wa.T.Fields[i]
	col = report.Column{Expr: fd.Name, Header: []string{fd.Name}, Width: m.FieldWidth(fd)}
	switch fd.Type {
	case 'M':
		col.Width = 30
	case 'N', 'F':
		col.Decimals = fd.Dec
		col.Total = true
	}
	col.Width = max(col.Width, len(fd.Name))
	return col, true
}

// SaveForm encodes and writes a report or label file, raising an error
// if it can't. It reports whether it was written.
func SaveForm(m *vm.VM, path string, enc func() ([]byte, error)) bool {
	data, err := enc()
	if err != nil {
		m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 20, SubCode: 2013, Description: "Create error: " + err.Error(), Filename: path})
		return false
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 20, SubCode: 2013, Description: "Create error: " + err.Error(), Filename: path})
		return false
	}
	return true
}

// ConfirmReplace asks before replacing a file when SET SAFETY is ON.
func ConfirmReplace(m *vm.VM, path string) bool {
	if !db.FileExists(path) || !m.Setting(101).Truthy() || !m.Con.Interactive() {
		return true
	}
	return gets.Alert(m, filepath.Base(path)+" already exists;Overwrite it?", []string{"No", "Yes"}) == 2
}

// fieldNames reads the field list argument, raising an error for a name
// the current table doesn't have.
func fieldNames(m *vm.VM, wa *db.WorkArea, a value.Value, what string) ([]report.Column, bool) {
	var cols []report.Column
	if a.K != value.ARRAY {
		return nil, true
	}
	for _, it := range a.Arr().Items {
		name := strings.TrimSpace(it.Deref().S)
		if name == "" {
			continue
		}
		col, ok := FieldColumn(m, wa, name)
		if !ok {
			m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 14, SubCode: 1003, Description: what + ": no field " + strings.ToUpper(name)})
			return nil, false
		}
		cols = append(cols, col)
	}
	return cols, true
}

func init() {
	// __ReportCreate(cFile, aFields)
	vm.Register("__REPORTCREATE", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("CREATE REPORT")
		if wa == nil {
			return value.Nil
		}
		cols, ok := fieldNames(m, wa, vm.Arg(a, 1), "CREATE REPORT")
		if !ok {
			return value.Nil
		}
		if len(cols) > report.MaxColumns {
			return m.Raise(vm.ErrMsg("CREATE REPORT: a report can have at most 24 columns"))
		}
		r := report.NewReport()
		r.Columns = cols
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".frm")
		if ConfirmReplace(m, path) {
			SaveForm(m, path, r.Encode)
		}
		return value.Nil
	})
	// __LabelCreate(cFile, aFields): one field to a line, character
	// fields trimmed.
	vm.Register("__LABELCREATE", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("CREATE LABEL")
		if wa == nil {
			return value.Nil
		}
		cols, ok := fieldNames(m, wa, vm.Arg(a, 1), "CREATE LABEL")
		if !ok {
			return value.Nil
		}
		if len(cols) > report.MaxLabelLines {
			return m.Raise(vm.ErrMsg("CREATE LABEL: a label can have at most 16 lines"))
		}
		l := report.NewLabel()
		l.Height = max(l.Height, len(cols))
		l.Lines = make([]string, l.Height)
		for i, c := range cols {
			l.Lines[i] = c.Expr
			if fd := wa.T.Fields[wa.T.FieldIndex(c.Expr)]; fd.Type == 'C' {
				l.Lines[i] = "Trim(" + c.Expr + ")"
			}
			l.Width = min(max(l.Width, c.Width), report.MaxLabelWidth)
		}
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".lbl")
		if ConfirmReplace(m, path) {
			SaveForm(m, path, l.Encode)
		}
		return value.Nil
	})
}
