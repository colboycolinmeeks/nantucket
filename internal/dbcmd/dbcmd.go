// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package dbcmd has the commands that work through the records of a
// table: LOCATE and CONTINUE, COUNT, SUM and AVERAGE, LIST and DISPLAY,
// DISPLAY STRUCTURE and STATUS, COPY TO, APPEND FROM, COPY STRUCTURE,
// CREATE FROM, SORT and TOTAL. Opening tables, moving through them,
// fields, indexes, filters, relations and locks are part of the core
// (internal/vm), which needs them to run programs at all.
package dbcmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nantucket/internal/compile"
	"nantucket/internal/db"
	"nantucket/internal/term"
	"nantucket/internal/value"
	"nantucket/internal/vm"
)

func init() {
	// LOCATE / CONTINUE
	vm.Register("__DBLOCATE", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("LOCATE")
		if wa == nil {
			return value.Nil
		}
		s := vm.ScopeArgs(a, 0)
		forB := s.For
		wa.LocateFor = nil
		wa.LocateWhile = nil
		if forB.K == value.BLOCK {
			wa.LocateFor = func() bool { return m.EvalBlock(forB).Truthy() }
		}
		if s.While.K == value.BLOCK {
			wb := s.While
			wa.LocateWhile = func() bool { return m.EvalBlock(wb).Truthy() }
		}
		wa.Found = false
		found := false
		m.EachRecord(wa, s, func() bool {
			found = true
			return false
		})
		wa.Found = found
		if !found && !wa.EOF {
			m.DBErr(wa.GoTo(0), "LOCATE")
		}
		if !found && m.AtPrompt() {
			m.Println("End of LOCATE scope")
		}
		return value.Nil
	})
	vm.Register("__DBCONTINUE", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("CONTINUE")
		if wa == nil || wa.LocateFor == nil {
			return value.Nil
		}
		wa.Found = false
		for {
			m.DBErr(wa.Skip(1), "CONTINUE")
			if wa.EOF {
				break
			}
			if wa.LocateWhile != nil && !wa.LocateWhile() {
				m.DBErr(wa.GoTo(0), "CONTINUE")
				break
			}
			if wa.LocateFor() {
				wa.Found = true
				break
			}
		}
		if !wa.Found && m.AtPrompt() {
			m.Println("End of LOCATE scope")
		}
		return value.Nil
	})

	// aggregate commands
	vm.Register("__DBCOUNT", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("COUNT")
		if wa == nil {
			return value.Nil
		}
		n := 0
		m.EachRecord(wa, vm.ScopeArgs(a, 1), func() bool { n++; return true })
		if b := vm.Arg(a, 0); b.K == value.BLOCK {
			m.EvalBlock(b, value.Int(n))
		} else {
			m.Println(fmt.Sprintf("%6d records", n))
		}
		return value.Int(n)
	})
	vm.Register("__DBSUM", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("SUM")
		if wa == nil {
			return value.Nil
		}
		avg := vm.ArgBool(a, 0, false)
		exprs := vm.Arg(a, 1)
		names := vm.Arg(a, 2)
		setters := vm.Arg(a, 3)
		var blocks []value.Value
		var labels []string
		if exprs.K == value.ARRAY && len(exprs.Arr().Items) > 0 && exprs.Arr().Items[0].K == value.BLOCK {
			blocks = exprs.Arr().Items
			for _, n := range names.Arr().Items {
				labels = append(labels, n.S)
			}
		} else {
			// all numeric fields
			for i, fd := range wa.T.Fields {
				if fd.Type == 'N' || fd.Type == 'F' {
					idx := i
					blocks = append(blocks, value.Value{K: value.BLOCK, P: &value.Block{Fn: vm.NativeBlock(func(m *vm.VM, _ []value.Value) value.Value {
						return m.CurWA().GetField(idx)
					})}})
					labels = append(labels, fd.Name)
				}
			}
		}
		sums := make([]value.Value, len(blocks))
		for i := range sums {
			sums[i] = value.Int(0)
		}
		count := 0
		m.EachRecord(wa, vm.ScopeArgs(a, 4), func() bool {
			count++
			for i, b := range blocks {
				v := m.EvalBlock(b)
				sums[i] = m.Arith(compile.ADD, sums[i], v)
			}
			return true
		})
		if avg {
			for i := range sums {
				if count > 0 {
					sums[i] = m.Arith(compile.DIV, sums[i], value.Int(count))
				}
			}
		}
		if setters.K == value.ARRAY {
			for i, s := range setters.Arr().Items {
				if i < len(sums) {
					m.EvalBlock(s, sums[i])
				}
			}
		} else {
			m.Println(fmt.Sprintf("%6d records %s", count, map[bool]string{true: "averaged", false: "summed"}[avg]))
			var hdr, line strings.Builder
			for i, s := range sums {
				d := m.Display(s)
				w := len(d)
				if len(labels[i]) > w {
					w = len(labels[i])
				}
				hdr.WriteString(vm.PadTo(labels[i], w, 'L', " ") + " ")
				line.WriteString(vm.PadTo(d, w, 'L', " ") + " ")
			}
			m.Println(hdr.String())
			m.Println(line.String())
		}
		return value.Nil
	})

	// LIST / DISPLAY
	vm.Register("__DBLIST", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("LIST")
		if wa == nil {
			return value.Nil
		}
		off := vm.ArgBool(a, 0, false)
		blocks := vm.Arg(a, 1).Arr()
		names := vm.Arg(a, 2).Arr()
		all := vm.ArgBool(a, 3, false)
		s := vm.ScopeArgs(a, 4)
		toPrint := vm.ArgBool(a, 9, false)
		toFile := vm.ArgStr(a, 10, "")
		var fileOut *os.File
		if strings.TrimSpace(toFile) != "" {
			f, err := os.Create(m.DefaultPath(toFile, ".txt"))
			if err != nil {
				return m.Raise(&vm.RTError{SubSystem: "DBCMD", GenCode: 20, SubCode: 2013, Description: "Create error", Filename: toFile})
			}
			fileOut = f
			defer f.Close()
		}
		emit := func(line string) {
			if fileOut != nil {
				fileOut.WriteString(line + "\r\n")
				return
			}
			m.Println(line)
			if toPrint {
				m.WritePrinter(line + "\r\n")
			}
		}
		allFields := blocks == nil || len(blocks.Items) == 0
		type col struct {
			head  string
			width int
			get   func() string
			memo  bool // wrap at SET MEMOWIDTH
		}
		var cols []col
		if allFields {
			for i, fd := range wa.T.Fields {
				idx := i
				w := m.FieldWidth(fd)
				if len(fd.Name) > w {
					w = len(fd.Name)
				}
				cols = append(cols, col{head: fd.Name, width: w, get: func() string { return m.FieldDisplay(wa, idx, true) }})
			}
		} else {
			for i, b := range blocks.Items {
				bb := b
				head := ""
				if names != nil && i < len(names.Items) {
					head = names.Items[i].S
				}
				c := col{head: head, get: func() string { return m.Display(m.EvalBlock(bb)) }}
				// size the column from the field definition or a sample value
				if fi := wa.T.FieldIndex(strings.TrimSpace(head)); fi >= 0 {
					c.width = m.FieldWidth(wa.T.Fields[fi])
					idx := fi
					c.get = func() string { return m.FieldDisplay(wa, idx, false) }
					if wa.T.Fields[fi].Type == 'M' {
						c.width = m.MemoWidth()
						c.memo = true
					}
				} else if !wa.EOF {
					c.width = len(c.get())
				}
				if c.width > 0 && len(head) > c.width {
					c.width = len(head)
				}
				cols = append(cols, c)
			}
		}
		if m.Setting(102).Truthy() {
			var sb strings.Builder
			if !off {
				sb.WriteString("Record# ")
			}
			for _, c := range cols {
				w := c.width
				if w == 0 {
					w = len(c.head)
				}
				sb.WriteString(vm.PadTo(strings.ToUpper(c.head), w, 'R', " ") + " ")
			}
			emit(strings.TrimRight(sb.String(), " "))
		}
		lines := 0
		pause := !all && (s.HasNext || s.Rest || s.For.K == value.BLOCK || s.While.K == value.BLOCK) || (!all && false)
		_ = pause
		_, rows := 0, 0
		if r, _ := m.Con.Size(); r > 0 {
			rows = r
		}
		line := func() {
			var sb strings.Builder
			if !off {
				sb.WriteString(fmt.Sprintf("%7d", wa.Recno))
				if wa.Deleted() {
					sb.WriteString("*")
				} else {
					sb.WriteString(" ")
				}
			}
			indent := sb.Len()
			// memo columns wrap onto extra lines under their column
			wrapped := make([][]string, len(cols))
			extra := 0
			for i, c := range cols {
				v := c.get()
				if c.memo {
					wrapped[i] = vm.MemoLines(v, c.width)
					v = ""
					if len(wrapped[i]) > 0 {
						v = wrapped[i][0]
					}
					extra = max(extra, len(wrapped[i])-1)
				}
				if c.width > 0 {
					v = vm.PadTo(v, c.width, 'R', " ")
				}
				sb.WriteString(v + " ")
			}
			emit(strings.TrimRight(sb.String(), " "))
			for n := 1; n <= extra; n++ {
				var more strings.Builder
				more.WriteString(strings.Repeat(" ", indent))
				for i, c := range cols {
					v := ""
					if n < len(wrapped[i]) {
						v = wrapped[i][n]
					}
					w := c.width
					if w == 0 {
						w = len(c.get())
					}
					more.WriteString(vm.PadTo(v, w, 'R', " ") + " ")
				}
				emit(strings.TrimRight(more.String(), " "))
			}
			lines++
			// DISPLAY pauses each screenful in interactive mode
			if !all && fileOut == nil && m.Con.Interactive() && rows > 4 && lines%(rows-3) == 0 {
				m.Out("Press any key to continue...", true)
				if m.Inkey(-1, false) == term.KEsc {
					panic(&vm.CancelSignal{})
				}
			}
		}
		if !all && !s.HasNext && !s.HasRec && !s.Rest && s.For.K != value.BLOCK && s.While.K != value.BLOCK {
			// DISPLAY with no scope shows the current record
			if !wa.EOF {
				line()
			}
			return value.Nil
		}
		m.EachRecord(wa, s, func() bool { line(); return true })
		return value.Nil
	})
	vm.Register("__DBDISPSTRU", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("DISPLAY STRUCTURE")
		if wa == nil {
			return value.Nil
		}
		abs, _ := filepath.Abs(wa.T.Path)
		m.Println("Structure for database: " + abs)
		m.Println(fmt.Sprintf("Number of data records: %7d", wa.T.RecCount()))
		y, mo, d := wa.T.LastUpdate()
		lu := ""
		if value.ValidDate(y, mo, d) {
			lu = m.DTOC(value.ToJulian(y, mo, d))
		}
		m.Println("Date of last update   : " + lu)
		for i, ix := range wa.Indexes {
			mark := " "
			if i+1 == wa.Order {
				mark = "*"
			}
			m.Println(fmt.Sprintf("Index file %s %s  Key: %s", mark, filepath.Base(ix.Path), ix.KeyExpr))
		}
		m.Println("Field  Field Name  Type       Width    Dec")
		types := map[byte]string{'C': "Character", 'N': "Numeric", 'F': "Float", 'D': "Date", 'L': "Logical", 'M': "Memo",
			'I': "Integer", 'B': "Double", 'Y': "Currency", 'T': "DateTime", 'G': "General"}
		for i, fd := range wa.T.Fields {
			dec := ""
			if fd.Type == 'N' || fd.Type == 'F' || fd.Type == 'B' {
				dec = fmt.Sprintf("%7d", fd.Dec)
			}
			m.Println(fmt.Sprintf("%5d  %-10s  %-10s %5s%s", i+1, fd.Name, types[fd.Type], fmt.Sprint(fd.Len), dec))
		}
		m.Println(fmt.Sprintf("** Total **                %7d", wa.T.RecLen))
		return value.Nil
	})
	vm.Register("__DISPSTATUS", func(m *vm.VM, a []value.Value) value.Value {
		for i := 1; i < 256; i++ {
			wa := m.Area(i)
			if wa == nil {
				continue
			}
			cur := ""
			if i == m.CurArea() {
				cur = "Currently Selected "
			}
			abs, _ := filepath.Abs(wa.T.Path)
			m.Println(fmt.Sprintf("%sDatabase in Select area %d: %s  Alias: %s", cur, i, abs, wa.Alias))
			for k, ix := range wa.Indexes {
				mark := " "
				if k+1 == wa.Order {
					mark = "Master"
				}
				m.Println(fmt.Sprintf("   %s index file: %s  Key: %s", mark, filepath.Base(ix.Path), ix.KeyExpr))
			}
			if wa.FilterText != "" {
				m.Println("   Filter: " + wa.FilterText)
			}
			for _, r := range wa.Relations {
				m.Println(fmt.Sprintf("   Related into: %s  Relation: %s", r.Child.Alias, r.KeyText))
			}
		}
		d, _ := os.Getwd()
		m.Println("")
		m.Println("File search path: " + m.Setting(6).S)
		m.Println("Default directory: " + d + " " + m.Setting(7).S)
		onoff := func(n int) string {
			if m.Setting(n).Truthy() {
				return "ON "
			}
			return "OFF"
		}
		m.Println(fmt.Sprintf("DELETED - %s  EXACT   - %s  SOFTSEEK - %s  UNIQUE - %s", onoff(11), onoff(1), onoff(9), onoff(10)))
		m.Println(fmt.Sprintf("CONFIRM - %s  ESCAPE  - %s  FIXED    - %s  CENTURY- %s", onoff(27), onoff(28), onoff(2), onoff(103)))
		m.Println(fmt.Sprintf("DATE format: %s   DECIMALS: %d   THEME: %s   COLOR: %s", m.Setting(4).S, m.Setting(3).Int(), m.Theme().Name, m.Colors().Text))
		return value.Nil
	})

	// COPY / APPEND FROM / structures
	vm.Register("__DBCOPY", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("COPY")
		if wa == nil {
			return value.Nil
		}
		fields := selectFields(m, wa, vm.Arg(a, 1))
		s := vm.ScopeArgs(a, 2)
		sdf, delim := vm.ArgBool(a, 7, false), vm.ArgBool(a, 8, false)
		if sdf || delim {
			ext := ".txt"
			path := m.DefaultPath(vm.ArgStr(a, 0, ""), ext)
			f, err := os.Create(path)
			if err != nil {
				return m.Raise(&vm.RTError{SubSystem: "DBCMD", GenCode: 20, SubCode: 2013, Description: "Create error", Filename: path})
			}
			defer f.Close()
			w := bufio.NewWriter(f)
			defer w.Flush()
			sep, quote := delimFormat(vm.ArgStr(a, 9, ""))
			m.EachRecord(wa, s, func() bool {
				var parts []string
				for _, i := range fields {
					fd := wa.T.Fields[i]
					v := wa.GetField(i)
					if sdf {
						raw := wa.Buf()[fd.Offset : fd.Offset+fd.Len]
						parts = append(parts, string(raw))
						continue
					}
					switch fd.Type {
					case 'C', 'M':
						parts = append(parts, quote+strings.TrimRight(v.S, " ")+quote)
					case 'D':
						parts = append(parts, value.DTOS(v.J))
					case 'L':
						if v.B {
							parts = append(parts, "T")
						} else {
							parts = append(parts, "F")
						}
					default:
						parts = append(parts, strings.TrimSpace(m.FieldDisplay(wa, i, false)))
					}
				}
				if sdf {
					w.WriteString(strings.Join(parts, "") + "\r\n")
				} else {
					w.WriteString(strings.Join(parts, sep) + "\r\n")
				}
				return true
			})
			return value.Nil
		}
		var defs []db.FieldDef
		for _, i := range fields {
			defs = append(defs, wa.T.Fields[i])
		}
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".dbf")
		if err := db.CreateTable(path, vm.CloneDefs(defs), wa.T.CodePage()); err != nil {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		}
		dst := m.OpenTemp(path)
		defer m.CloseTemp(dst)
		dwa := m.Area(dst)
		count := 0
		m.EachRecord(wa, s, func() bool {
			m.DBErr(dwa.Append(), "COPY")
			for k, i := range fields {
				dwa.PutField(k, wa.GetField(i))
			}
			if wa.Deleted() {
				dwa.SetDeleted(true)
			}
			count++
			return true
		})
		if m.Setting(100).Truthy() {
			m.Println(fmt.Sprintf("%d records copied", count))
		}
		return value.Int(count)
	})
	vm.Register("__DBAPP", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("APPEND FROM")
		if wa == nil {
			return value.Nil
		}
		sdf, delim := vm.ArgBool(a, 7, false), vm.ArgBool(a, 8, false)
		s := vm.ScopeArgs(a, 2)
		if sdf || delim {
			path := m.FindFile(vm.ArgStr(a, 0, ""), ".txt")
			if path == "" {
				return m.Raise(&vm.RTError{SubSystem: "DBCMD", GenCode: 21, SubCode: 2004, Description: "File does not exist", Filename: vm.ArgStr(a, 0, "")})
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return m.Raise(&vm.RTError{SubSystem: "DBCMD", GenCode: 21, SubCode: 2004, Description: "Open error", Filename: path})
			}
			for _, ln := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
				if ln == "" || ln == "\x1a" {
					continue
				}
				m.DBErr(wa.Append(), "APPEND FROM")
				if sdf {
					for i, fd := range wa.T.Fields {
						seg := ""
						if fd.Offset-1 < len(ln) {
							end := fd.Offset - 1 + fd.Len
							if end > len(ln) {
								end = len(ln)
							}
							seg = ln[fd.Offset-1 : end]
						}
						putText(m, wa, i, seg)
					}
				} else {
					vals := splitDelimited(ln, strings.TrimSpace(vm.ArgStr(a, 9, "")))
					for i := range wa.T.Fields {
						if i < len(vals) {
							putText(m, wa, i, vals[i])
						}
					}
				}
			}
			m.DBErr(wa.Commit(), "APPEND FROM")
			return value.Nil
		}
		path := m.FindFile(vm.ArgStr(a, 0, ""), ".dbf")
		if path == "" {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1001, Description: "Open error", Filename: vm.ArgStr(a, 0, "")})
		}
		src := m.OpenSource(path)
		defer m.CloseTemp(src)
		swa := m.Area(src)
		var pairs [][2]int
		wanted := selectFields(m, wa, vm.Arg(a, 1))
		for _, di := range wanted {
			if si := swa.T.FieldIndex(wa.T.Fields[di].Name); si >= 0 {
				pairs = append(pairs, [2]int{si, di})
			}
		}
		save := m.CurArea()
		m.SelectArea(src)
		var recs [][]value.Value
		var dels []bool
		m.EachRecord(swa, s, func() bool {
			row := make([]value.Value, len(pairs))
			for k, p := range pairs {
				row[k] = swa.GetField(p[0])
			}
			recs = append(recs, row)
			dels = append(dels, swa.Deleted())
			return true
		})
		m.SelectArea(save)
		for r, row := range recs {
			m.DBErr(wa.Append(), "APPEND FROM")
			for k, p := range pairs {
				v := row[k]
				if v.K == value.NUMBER || wa.T.Fields[p[1]].Type == swa.T.Fields[p[0]].Type {
					wa.PutField(p[1], v)
				}
			}
			if dels[r] {
				wa.SetDeleted(true)
			}
		}
		m.DBErr(wa.Commit(), "APPEND FROM")
		if m.Setting(100).Truthy() {
			m.Println(fmt.Sprintf("%d records added", len(recs)))
		}
		return value.Int(len(recs))
	})
	vm.Register("__DBCOPYSTRUCT", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("COPY STRUCTURE")
		if wa == nil {
			return value.Nil
		}
		var defs []db.FieldDef
		for _, i := range selectFields(m, wa, vm.Arg(a, 1)) {
			defs = append(defs, wa.T.Fields[i])
		}
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".dbf")
		if err := db.CreateTable(path, vm.CloneDefs(defs), wa.T.CodePage()); err != nil {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		}
		return value.Nil
	})
	vm.Register("__DBCOPYXSTRUCT", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("COPY STRUCTURE EXTENDED")
		if wa == nil {
			return value.Nil
		}
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".dbf")
		defs := []db.FieldDef{{Name: "FIELD_NAME", Type: 'C', Len: 10}, {Name: "FIELD_TYPE", Type: 'C', Len: 1},
			{Name: "FIELD_LEN", Type: 'N', Len: 3}, {Name: "FIELD_DEC", Type: 'N', Len: 3}}
		if err := db.CreateTable(path, defs, m.CodePage()); err != nil {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		}
		dst := m.OpenTemp(path)
		defer m.CloseTemp(dst)
		dwa := m.Area(dst)
		for _, fd := range wa.T.Fields {
			dwa.Append()
			dwa.PutField(0, value.Str(fd.Name))
			dwa.PutField(1, value.Str(string(fd.Type)))
			dwa.PutField(2, value.Int(fd.Len))
			dwa.PutField(3, value.Int(fd.Dec))
		}
		return value.Nil
	})
	vm.Register("__DBCREATE", func(m *vm.VM, a []value.Value) value.Value {
		from := m.FindFile(vm.ArgStr(a, 1, ""), ".dbf")
		if from == "" {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1001, Description: "Open error", Filename: vm.ArgStr(a, 1, "")})
		}
		src := m.OpenSource(from)
		swa := m.Area(src)
		var defs []db.FieldDef
		for r := 1; r <= swa.T.RecCount(); r++ {
			swa.GoTo(r)
			defs = append(defs, db.FieldDef{Name: strings.TrimSpace(swa.GetField(0).S), Type: strings.ToUpper(swa.GetField(1).S + " ")[0],
				Len: swa.GetField(2).Int(), Dec: swa.GetField(3).Int()})
		}
		m.CloseTemp(src)
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".dbf")
		if err := db.CreateTable(path, defs, m.CodePage()); err != nil {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		}
		m.UseArea(vm.ArgBool(a, 3, false), path, vm.ArgStr(a, 4, ""), false)
		return value.Nil
	})
	vm.Register("__DBSORT", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("SORT")
		if wa == nil {
			return value.Nil
		}
		type key struct {
			idx         int
			desc, icase bool
		}
		var keys []key
		for _, f := range vm.Arg(a, 1).Arr().Items {
			spec := strings.ToUpper(f.S)
			k := key{}
			if i := strings.IndexByte(spec, '/'); i >= 0 {
				opts := spec[i:]
				k.desc = strings.Contains(opts, "D")
				k.icase = strings.Contains(opts, "C")
				spec = spec[:i]
			}
			k.idx = wa.T.FieldIndex(strings.TrimSpace(spec))
			if k.idx < 0 {
				return m.Raise(vm.ErrNoVar(spec))
			}
			keys = append(keys, k)
		}
		type row struct {
			vals []value.Value
			del  bool
		}
		var rows []row
		m.EachRecord(wa, vm.ScopeArgs(a, 2), func() bool {
			r := row{vals: make([]value.Value, len(wa.T.Fields)), del: wa.Deleted()}
			for i := range wa.T.Fields {
				r.vals[i] = wa.GetField(i)
			}
			rows = append(rows, r)
			return true
		})
		sort.SliceStable(rows, func(i, j int) bool {
			for _, k := range keys {
				x, y := rows[i].vals[k.idx], rows[j].vals[k.idx]
				if k.icase && x.K == value.STRING {
					x, y = value.Str(strings.ToUpper(x.S)), value.Str(strings.ToUpper(y.S))
				}
				c := 0
				switch {
				case m.CompareLess(x, y):
					c = -1
				case m.CompareLess(y, x):
					c = 1
				}
				if k.desc {
					c = -c
				}
				if c != 0 {
					return c < 0
				}
			}
			return false
		})
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".dbf")
		if err := db.CreateTable(path, vm.CloneDefs(wa.T.Fields), wa.T.CodePage()); err != nil {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		}
		dst := m.OpenTemp(path)
		defer m.CloseTemp(dst)
		dwa := m.Area(dst)
		for _, r := range rows {
			dwa.Append()
			for i, v := range r.vals {
				dwa.PutField(i, v)
			}
			if r.del {
				dwa.SetDeleted(true)
			}
		}
		return value.Nil
	})
	vm.Register("__DBTOTAL", func(m *vm.VM, a []value.Value) value.Value {
		wa := m.NeedWA("TOTAL")
		if wa == nil {
			return value.Nil
		}
		keyB := vm.Arg(a, 1)
		var sumIdx []int
		if f := vm.Arg(a, 2); f.K == value.ARRAY && len(f.Arr().Items) > 0 {
			sumIdx = selectFields(m, wa, f)
		} else {
			for i, fd := range wa.T.Fields {
				if fd.Type == 'N' || fd.Type == 'F' {
					sumIdx = append(sumIdx, i)
				}
			}
		}
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".dbf")
		var defs []db.FieldDef
		for _, fd := range wa.T.Fields {
			if fd.Type != 'M' {
				defs = append(defs, fd)
			}
		}
		if err := db.CreateTable(path, vm.CloneDefs(defs), wa.T.CodePage()); err != nil {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		}
		dst := m.OpenTemp(path)
		defer m.CloseTemp(dst)
		dwa := m.Area(dst)
		var curKey value.Value
		have := false
		var acc []value.Value
		flush := func() {
			if !have {
				return
			}
			dwa.Append()
			for i, v := range acc {
				if di := dwa.T.FieldIndex(defs[i].Name); di >= 0 {
					dwa.PutField(di, v)
				}
			}
		}
		m.EachRecord(wa, vm.ScopeArgs(a, 3), func() bool {
			k := m.EvalBlock(keyB)
			row := make([]value.Value, len(defs))
			for i, fd := range defs {
				row[i] = wa.GetField(wa.T.FieldIndex(fd.Name))
			}
			if !have || !m.EqualExact(k, curKey) {
				flush()
				curKey, have, acc = k, true, row
				return true
			}
			for _, si := range sumIdx {
				name := wa.T.Fields[si].Name
				for i, fd := range defs {
					if fd.Name == name {
						acc[i] = m.Arith(compile.ADD, acc[i], row[i])
					}
				}
			}
			return true
		})
		flush()
		return value.Nil
	})
}

func putText(m *vm.VM, wa *db.WorkArea, i int, s string) {
	fd := wa.T.Fields[i]
	var v value.Value
	switch fd.Type {
	case 'N', 'F':
		n := 0.0
		fmt.Sscan(strings.TrimSpace(s), &n)
		v = value.Num(n, fd.Dec)
	case 'D':
		v = value.Date(value.STOD(strings.TrimSpace(s)))
	case 'L':
		t := strings.ToUpper(strings.TrimSpace(s))
		v = value.Bool(t == "T" || t == "Y" || t == ".T.")
	default:
		v = value.Str(s)
	}
	wa.PutField(i, v)
}

// delimFormat reads the WITH clause of COPY TO / APPEND FROM ... DELIMITED
// and returns the field separator and the quote around character fields.
// WITH BLANK separates with spaces and no quotes, WITH TAB and WITH PIPE
// separate with a tab or "|", and WITH <c> quotes with <c>.
func delimFormat(with string) (sep, quote string) {
	with = strings.TrimSpace(with)
	switch strings.ToUpper(with) {
	case "":
		return ",", "\""
	case "BLANK":
		return " ", ""
	case "TAB":
		return "\t", "\""
	case "PIPE":
		return "|", "\""
	}
	return ",", with[:1]
}

func splitDelimited(ln, delim string) []string {
	sep, quote := delimFormat(delim)
	if sep == " " {
		return strings.Fields(ln)
	}
	q := byte(0)
	if quote != "" {
		q = quote[0]
	}
	var out []string
	var cur strings.Builder
	inq := false
	for i := 0; i < len(ln); i++ {
		c := ln[i]
		switch {
		case q != 0 && c == q:
			inq = !inq
		case c == sep[0] && !inq:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(out, cur.String())
}

// selectFields maps a FIELDS list (array of names) to field indexes.
func selectFields(m *vm.VM, wa *db.WorkArea, list value.Value) []int {
	var out []int
	if list.K == value.ARRAY && len(list.Arr().Items) > 0 {
		for _, it := range list.Arr().Items {
			name := strings.TrimSpace(it.S)
			if i := strings.LastIndex(name, "->"); i >= 0 {
				name = name[i+2:]
			}
			if i := wa.T.FieldIndex(name); i >= 0 {
				out = append(out, i)
			}
		}
		return out
	}
	for i := range wa.T.Fields {
		out = append(out, i)
	}
	return out
}
