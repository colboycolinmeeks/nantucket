// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package screens has the full screen tools: BROWSE, EDIT and APPEND,
// CREATE and MODIFY STRUCTURE, MODIFY COMMAND, windows (WOpen() and the
// WINDOW commands), the HELP and VERSION FULL windows and the dot
// prompt's status bar.
package screens

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"nantucket/internal/gets"
	"nantucket/internal/vm"

	"nantucket/internal/db"
	"nantucket/internal/editor"
	"nantucket/internal/term"
	"nantucket/internal/value"
)

func needInteractive(m *vm.VM, what string) bool {
	if m.Con.Interactive() {
		return true
	}
	m.Raise(vm.ErrMsg(what + " requires an interactive terminal"))
	return false
}

// ModifyFile opens a file in the full screen editor.
// errLineRe finds the line number in a compile error: "file.prg(12): ...".
var errLineRe = regexp.MustCompile(`\((\d+)\):\s*(.*)$`)

// errorLine splits a compile error into its line number (1 if there is
// none) and message.
func errorLine(msg string) (int, string) {
	if m := errLineRe.FindStringSubmatch(msg); m != nil {
		n, _ := strconv.Atoi(m[1])
		return max(n, 1), m[2]
	}
	return 1, msg
}

func ModifyFile(m *vm.VM, name, ext, title string) {
	if !needInteractive(m, title) {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		m.Out("File name: ", true)
		s, ok := term.ReadLine(m.Con, m.Colors().Std, "", 128, nil)
		if !ok || strings.TrimSpace(s) == "" {
			return
		}
		name = strings.TrimSpace(s)
	}
	path := m.FindFile(name, ext)
	if path == "" {
		path = m.DefaultPath(name, ext)
	}
	data, _ := os.ReadFile(path)
	save := func(text string) error { return os.WriteFile(path, []byte(text), 0644) }
	// a program is compiled when saved, so that the editor can go straight
	// to an error
	var check func(string) (int, string)
	if strings.EqualFold(filepath.Ext(path), ".prg") {
		check = func(text string) (int, string) {
			_, err := m.CompileSource(text, path)
			if err == nil {
				return 0, ""
			}
			return errorLine(err.Error())
		}
	}
	text, saved := editor.Edit(m.Con, editor.Options{
		Text:       string(data),
		Title:      title + "  " + filepath.Base(path),
		Status:     true,
		Attr:       m.Theme().Edit,
		StatusAttr: m.Theme().EditStatus,
		HelpAttr:   m.Theme().EditHelp,
		Highlight:  strings.EqualFold(ext, ".prg") || strings.EqualFold(filepath.Ext(path), ".prg") || strings.EqualFold(filepath.Ext(path), ".ch"),
		OnSave:     save,
		Check:      check,
	})
	if saved {
		if err := save(text); err != nil {
			m.Raise(&vm.RTError{SubSystem: "BASE", GenCode: 20, SubCode: 2012, Description: "Write error", Filename: path})
			return
		}
		if check != nil {
			// the user chose to leave with an error still there
			if line, msg := check(text); line > 0 {
				m.Println(fmt.Sprintf("Compile error: %s(%d): %s", filepath.Base(path), line, msg))
			}
		}
	}
}

func init() {
	vm.Register("__MODICOMM", func(m *vm.VM, a []value.Value) value.Value {
		ModifyFile(m, vm.ArgStr(a, 0, ""), ".prg", "MODIFY COMMAND")
		return value.Nil
	})
	vm.Register("__MODIFILE", func(m *vm.VM, a []value.Value) value.Value {
		ModifyFile(m, vm.ArgStr(a, 0, ""), "", "MODIFY FILE")
		return value.Nil
	})
	vm.Register("MEMOEDIT", func(m *vm.VM, a []value.Value) value.Value {
		text := vm.ArgStr(a, 0, "")
		if !m.Con.Interactive() {
			return value.Str(text)
		}
		rows, cols := m.Con.Size()
		t, l, b, r := vm.ArgInt(a, 1, 0), vm.ArgInt(a, 2, 0), vm.ArgInt(a, 3, rows-1), vm.ArgInt(a, 4, cols-1)
		res, saved := editor.Edit(m.Con, editor.Options{Text: text, Top: t, Left: l, Bottom: b, Right: r,
			Attr: m.Colors().Std, ReadOnly: !vm.ArgBool(a, 5, true), StatusAttr: m.Colors().Enh})
		if !saved {
			return value.Str(text)
		}
		return value.Str(strings.TrimSuffix(res, "\n"))
	})
	vm.Register("BROWSE", func(m *vm.VM, a []value.Value) value.Value {
		if !needInteractive(m, "BROWSE") {
			return value.False
		}
		rows, cols := m.Con.Size()
		var names []string
		if f := vm.Arg(a, 4); f.K == value.ARRAY {
			for _, it := range f.Arr().Items {
				if s := strings.TrimSpace(it.Deref().S); s != "" {
					names = append(names, s)
				}
			}
		}
		Browse(m, vm.ArgInt(a, 0, 1), vm.ArgInt(a, 1, 0), vm.ArgInt(a, 2, rows-1), vm.ArgInt(a, 3, cols-1), names)
		return value.True
	})
	vm.Register("DBEDIT", func(m *vm.VM, a []value.Value) value.Value {
		if !needInteractive(m, "DBEDIT") {
			return value.False
		}
		rows, cols := m.Con.Size()
		var names []string
		if f := vm.Arg(a, 4); f.K == value.ARRAY {
			for _, it := range f.Arr().Items {
				names = append(names, it.Deref().S)
			}
		}
		Browse(m, vm.ArgInt(a, 0, 0), vm.ArgInt(a, 1, 0), vm.ArgInt(a, 2, rows-1), vm.ArgInt(a, 3, cols-1), names)
		return value.Nil
	})
	vm.Register("__DBEDIT", func(m *vm.VM, a []value.Value) value.Value {
		if needInteractive(m, "EDIT") {
			EditRecords(m, false)
		}
		return value.Nil
	})
	vm.Register("__DBAPPENDEDIT", func(m *vm.VM, a []value.Value) value.Value {
		if needInteractive(m, "APPEND") {
			EditRecords(m, true)
		}
		return value.Nil
	})
	vm.Register("__DBCREATEINTERACTIVE", func(m *vm.VM, a []value.Value) value.Value {
		if !needInteractive(m, "CREATE") {
			return value.Nil
		}
		path := m.DefaultPath(vm.ArgStr(a, 0, ""), ".dbf")
		if db.FileExists(path) && m.Setting(101).Truthy() {
			if gets.Alert(m, filepath.Base(path)+" already exists;Overwrite it?", []string{"No", "Yes"}) != 2 {
				return value.Nil
			}
		}
		fields, ok := StructureEditor(m, "CREATE "+filepath.Base(path), nil)
		if !ok {
			return value.Nil
		}
		if err := db.CreateTable(path, fields, m.CodePage()); err != nil {
			return m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		}
		m.UseArea(false, path, "", false)
		m.Con.Scroll(0, 0, 999, 999, 0, m.Colors().Std)
		m.Con.SetPos(0, 0)
		if gets.Alert(m, "Input data records now?", []string{"Yes", "No"}) == 1 {
			EditRecords(m, true)
		}
		return value.Nil
	})
	vm.Register("__DBMODSTRU", func(m *vm.VM, a []value.Value) value.Value {
		if !needInteractive(m, "MODIFY STRUCTURE") {
			return value.Nil
		}
		wa := m.NeedWA("MODIFY STRUCTURE")
		if wa == nil {
			return value.Nil
		}
		fields, ok := StructureEditor(m, "MODIFY STRUCTURE "+filepath.Base(wa.T.Path), vm.CloneDefs(wa.T.Fields))
		if !ok {
			return value.Nil
		}
		restructure(m, wa, fields)
		return value.Nil
	})
}

// ---------- BROWSE ----------

type bcol struct {
	name  string
	field int
	width int
}

// Browse is a spreadsheet style table editor for the current work area.
func Browse(m *vm.VM, t, l, b, r int, names []string) {
	wa := m.NeedWA("BROWSE")
	if wa == nil {
		return
	}
	var cols []bcol
	add := func(i int) {
		fd := wa.T.Fields[i]
		w := m.FieldWidth(fd)
		if len(fd.Name) > w {
			w = len(fd.Name)
		}
		if w > r-l-1 {
			w = r - l - 1
		}
		cols = append(cols, bcol{fd.Name, i, w})
	}
	if len(names) > 0 {
		for _, n := range names {
			if i := wa.T.FieldIndex(n); i >= 0 {
				add(i)
			}
		}
	} else {
		for i := range wa.T.Fields {
			add(i)
		}
	}
	if len(cols) == 0 {
		return
	}
	saveScr := m.Con.Save(t, l, b, r)
	defer m.Con.Restore(t, l, b, r, saveScr)
	std := m.Colors().Std
	hdr := m.Theme().Header
	cur := m.Colors().Enh
	dataTop := t + 2
	h := b - dataTop // last line is status
	if h < 1 {
		h = 1
	}
	if wa.EOF {
		wa.GoTop()
	}
	top := wa.Recno
	rowPos := 0
	colPos := 0
	leftCol := 0
	msg := ""
	for {
		// window of records
		var recs []int
		if wa.T.RecCount() > 0 {
			wa.GoTo(top)
			if wa.EOF || !wa.RecordVisible() {
				wa.GoTop()
				top = wa.Recno
			}
			for len(recs) < h && !wa.EOF {
				recs = append(recs, wa.Recno)
				wa.Skip(1)
			}
		}
		if rowPos >= len(recs) {
			rowPos = len(recs) - 1
		}
		if rowPos < 0 {
			rowPos = 0
		}
		// horizontal layout
		if colPos < leftCol {
			leftCol = colPos
		}
		var vis []int
		for {
			vis = vis[:0]
			x := l + 8
			for c := leftCol; c < len(cols); c++ {
				if x+cols[c].width > r+1 && len(vis) > 0 {
					break
				}
				vis = append(vis, c)
				x += cols[c].width + 1
			}
			if colPos <= vis[len(vis)-1] || leftCol >= colPos {
				break
			}
			leftCol++
		}
		m.Con.Scroll(t, l, b, r, 0, std)
		x := l + 8
		m.Con.Put(t, l, vm.PadTo(" Record", 7, 'R', " "), hdr)
		for _, c := range vis {
			m.Con.Put(t, x, vm.PadTo(cols[c].name, cols[c].width, 'R', " "), hdr)
			x += cols[c].width + 1
		}
		m.Con.Put(t+1, l, strings.Repeat("─", r-l+1), std)
		curX := l + 8
		for i, rec := range recs {
			wa.GoTo(rec)
			mark := " "
			if wa.Deleted() {
				mark = "*"
			}
			m.Con.Put(dataTop+i, l, fmt.Sprintf("%6d%s", rec, mark), std)
			x := l + 8
			for _, c := range vis {
				s := vm.PadTo(m.FieldDisplay(wa, cols[c].field, true), cols[c].width, 'R', " ")
				at := std
				if i == rowPos && c == colPos {
					at = cur
					curX = x
				}
				m.Con.Put(dataTop+i, x, s, at)
				x += cols[c].width + 1
			}
		}
		status := fmt.Sprintf(" Rec %d/%d  ^N Add  ^U Del  Enter Edit  Esc Exit  %s", 0, wa.T.RecCount(), msg)
		if len(recs) > 0 {
			wa.GoTo(recs[rowPos])
			status = fmt.Sprintf(" Rec %d/%d  ^N Add  ^U Del  Enter Edit  Esc Exit  %s", wa.Recno, wa.T.RecCount(), msg)
		} else {
			m.Con.Put(dataTop, l+8, "<no records>", std)
		}
		msg = ""
		m.Con.Put(b, l, vm.PadTo(status, r-l+1, 'R', " "), hdr)
		m.Con.SetPos(dataTop+rowPos, curX)
		k := m.Inkey(-1, true)
		switch k {
		case term.KEsc, term.KCtrlW:
			if len(recs) > 0 {
				wa.GoTo(recs[rowPos])
			}
			return
		case term.KDown:
			if rowPos < len(recs)-1 {
				rowPos++
			} else if len(recs) > 0 {
				wa.GoTo(recs[len(recs)-1])
				wa.Skip(1)
				if !wa.EOF && len(recs) > 1 {
					top = recs[1]
				} else if wa.EOF {
					msg = "End of file"
				}
			}
		case term.KUp:
			if rowPos > 0 {
				rowPos--
			} else if len(recs) > 0 {
				wa.GoTo(recs[0])
				wa.Skip(-1)
				if !wa.BOF {
					top = wa.Recno
				} else {
					msg = "Top of file"
				}
			}
		case term.KPgDn:
			if len(recs) > 0 {
				wa.GoTo(recs[len(recs)-1])
				wa.Skip(1)
				if !wa.EOF {
					top = wa.Recno
				} else {
					rowPos = len(recs) - 1
				}
			}
		case term.KPgUp:
			if len(recs) > 0 {
				wa.GoTo(recs[0])
				wa.Skip(-h)
				top = wa.Recno
				if wa.BOF {
					rowPos = 0
				}
			}
		case term.KCtrlPgUp, term.KCtrlHome:
			wa.GoTop()
			top, rowPos = wa.Recno, 0
		case term.KCtrlPgDn:
			wa.GoBottom()
			wa.Skip(-(h - 1))
			top = wa.Recno
			rowPos = h - 1
		case term.KRight, term.KTab:
			if colPos < len(cols)-1 {
				colPos++
			}
		case term.KLeft, term.KShTab:
			if colPos > 0 {
				colPos--
			}
		case term.KHome:
			colPos = 0
		case term.KEnd:
			colPos = len(cols) - 1
		case term.KCtrlU:
			if len(recs) > 0 {
				wa.GoTo(recs[rowPos])
				if !withRecordLock(m, wa, func() { wa.SetDeleted(!wa.Deleted()) }) {
					msg = inUseMsg
				}
			}
		case term.KCtrlN:
			m.DBErr(wa.Append(), "APPEND")
			wa.Commit()
			releaseAppendLock(m, wa)
			top = wa.Recno
			rowPos = 0
			// show the new record at the bottom of the window when possible
			wa.Skip(-(h - 1))
			if !wa.BOF {
				top = wa.Recno
				rowPos = h - 1
			}
		default:
			if len(recs) == 0 {
				continue
			}
			if k == term.KEnter || (k >= 32 && k < 256) {
				wa.GoTo(recs[rowPos])
				if k != term.KEnter {
					m.Con.PushKeys([]int{k})
				}
				fi := cols[colPos].field
				if !withRecordLock(m, wa, func() { editCell(m, wa, fi, dataTop+rowPos, curX) }) {
					m.Con.ClearKeys()
					msg = inUseMsg
				}
				if wa.Recno != recs[rowPos] {
					top = recs[0]
				}
			}
		}
	}
}

// inUseMsg is shown when BROWSE or EDIT can't lock a shared record.
const inUseMsg = "Record is in use by another user"

// withRecordLock runs fn with the current record locked when the table
// is shared and the record isn't locked already, committing and releasing
// the lock afterwards. It reports false, without running fn, when another
// session holds the lock. BROWSE and EDIT use it so that editing a shared
// table just works.
func withRecordLock(m *vm.VM, wa *db.WorkArea, fn func()) bool {
	if !wa.T.Shared() || wa.T.ReadOnly || wa.T.RecordLocked(wa.Recno) {
		fn()
		wa.Commit()
		return true
	}
	if err := wa.T.LockRecord(wa.Recno); err != nil {
		return false
	}
	fn()
	m.DBErr(wa.Commit(), "EDIT")
	wa.T.UnlockRecord(wa.Recno)
	return true
}

// releaseAppendLock drops the lock APPEND took on a new record in a
// shared table, once BROWSE or EDIT has written it.
func releaseAppendLock(m *vm.VM, wa *db.WorkArea) {
	if wa.T.Shared() && !wa.T.FileLocked() {
		wa.T.UnlockRecord(wa.Recno)
	}
}

// editCell edits one field in place.
func editCell(m *vm.VM, wa *db.WorkArea, fi int, row, col int) {
	fd := wa.T.Fields[fi]
	if fd.Type == 'M' {
		v := wa.GetField(fi)
		res, saved := editor.Edit(m.Con, editor.Options{Text: v.S, Status: true, Title: "Memo: " + fd.Name,
			Attr: m.Theme().Edit, StatusAttr: m.Theme().EditStatus, HelpAttr: m.Theme().EditHelp})
		if saved {
			m.PutField(wa, fd.Name, value.Str(strings.TrimSuffix(res, "\n")))
			wa.Commit()
		}
		return
	}
	name := fd.Name
	area := wa.Num
	g := &gets.Get{Row: row, Col: col, Name: name, Block: value.Value{K: value.BLOCK, P: &value.Block{Fn: vm.NativeBlock(func(m *vm.VM, args []value.Value) value.Value {
		w := m.Area(area)
		if len(args) > 0 && args[0].Deref().K != value.NIL {
			m.PutField(w, name, args[0])
			return args[0].Deref()
		}
		return m.GetField(w, name)
	})}}}
	if fd.Type == 'N' && fd.Dec > 0 {
		g.PicStr = strings.Repeat("9", fd.Len-fd.Dec-1) + "." + strings.Repeat("9", fd.Dec)
	} else if fd.Type == 'N' {
		g.PicStr = strings.Repeat("9", fd.Len)
	}
	g.Pic = vm.ParsePicture(g.PicStr)
	gets.PrepareGet(m, g)
	code := gets.EditGet(m, g)
	if code != gets.GexEscape && g.Changed() {
		if v, ok := gets.GetValue(m, g); ok {
			m.EvalBlock(g.Block, v)
			wa.Commit()
		}
	}
}

// ---------- EDIT / APPEND forms ----------

// EditRecords shows a full screen form for the current record (EDIT) or
// for new records (APPEND).
func EditRecords(m *vm.VM, appending bool) {
	wa := m.NeedWA("EDIT")
	if wa == nil {
		return
	}
	rows, _ := m.Con.Size()
	saveScr := m.Con.Save(0, 0, rows-1, 999)
	defer m.Con.Restore(0, 0, rows-1, 999, saveScr)
	if !appending && wa.EOF {
		wa.GoTop()
		if wa.EOF {
			return
		}
	}
	for {
		m.Con.Scroll(0, 0, rows-1, 999, 0, m.Colors().Std)
		var getList []*gets.Get
		vals := make([]*value.Ref, len(wa.T.Fields))
		title := ""
		if appending {
			title = fmt.Sprintf("APPEND  Record %d", wa.T.RecCount()+1)
		} else {
			del := ""
			if wa.Deleted() {
				del = "  <Deleted>"
			}
			title = fmt.Sprintf("EDIT  Record %d/%d%s   PgUp/PgDn move  ^U delete  Esc exit", wa.Recno, wa.T.RecCount(), del)
		}
		m.Con.Put(0, 0, title, m.Colors().Enh)
		maxRows := rows - 3
		for i, fd := range wa.T.Fields {
			if i >= maxRows {
				break
			}
			var v value.Value
			if appending {
				v = wa.T.EmptyValue(i)
			} else {
				v = wa.GetField(i)
			}
			vals[i] = &value.Ref{V: v}
			m.Con.Put(i+2, 1, vm.PadTo(fd.Name, 11, 'R', " "), m.Colors().Std)
			if fd.Type == 'M' {
				lbl := "memo"
				if v.S != "" {
					lbl = "Memo"
				}
				m.Con.Put(i+2, 13, lbl, m.Colors().Unsel)
				continue
			}
			ref := vals[i]
			g := &gets.Get{Row: i + 2, Col: 13, Name: fd.Name, Block: value.Value{K: value.BLOCK, P: &value.Block{Fn: vm.NativeBlock(func(m *vm.VM, args []value.Value) value.Value {
				if len(args) > 0 && args[0].Deref().K != value.NIL {
					ref.V = args[0].Deref()
				}
				return ref.V
			})}}}
			if fd.Type == 'N' {
				if fd.Dec > 0 {
					g.PicStr = strings.Repeat("9", fd.Len-fd.Dec-1) + "." + strings.Repeat("9", fd.Dec)
				} else {
					g.PicStr = strings.Repeat("9", fd.Len)
				}
			} else if fd.Type == 'C' {
				g.PicStr = "@S" + strconv.Itoa(min(fd.Len, 64))
			}
			g.Pic = vm.ParsePicture(g.PicStr)
			getList = append(getList, g)
		}
		m.SetLastKey(0)
		updated := gets.ReadModal(m, getList)
		k := m.LastKey()
		if appending {
			if k == term.KEsc || !updated {
				return
			}
			m.DBErr(wa.Append(), "APPEND")
			for i := range wa.T.Fields {
				if vals[i] != nil && wa.T.Fields[i].Type != 'M' {
					wa.PutField(i, vals[i].V)
				}
			}
			m.DBErr(wa.Commit(), "APPEND")
			releaseAppendLock(m, wa)
			continue
		}
		if updated && k != term.KEsc {
			ok := withRecordLock(m, wa, func() {
				for i := range wa.T.Fields {
					if vals[i] != nil && wa.T.Fields[i].Type != 'M' {
						wa.PutField(i, vals[i].V)
					}
				}
				m.DBErr(wa.Commit(), "EDIT")
			})
			if !ok {
				gets.Alert(m, "This record is in use by another user;Your changes were not saved", []string{"OK"})
			}
		}
		switch k {
		case term.KPgDn:
			wa.Skip(1)
			if wa.EOF {
				wa.GoBottom()
				return
			}
		case term.KPgUp:
			wa.Skip(-1)
		case term.KCtrlU:
			if !withRecordLock(m, wa, func() { wa.SetDeleted(!wa.Deleted()) }) {
				gets.Alert(m, "This record is in use by another user", []string{"OK"})
			}
		default:
			return
		}
	}
}

// ---------- structure designer ----------

// StructureEditor lets the user define fields. Returns false on cancel.
func StructureEditor(m *vm.VM, title string, fields []db.FieldDef) ([]db.FieldDef, bool) {
	rows, cols := m.Con.Size()
	saveScr := m.Con.Save(0, 0, rows-1, cols-1)
	defer m.Con.Restore(0, 0, rows-1, cols-1, saveScr)
	if len(fields) == 0 {
		fields = []db.FieldDef{{Type: 'C', Len: 10}}
	}
	std := m.Colors().Std
	hi := m.Colors().Enh
	hdr := m.Theme().Header
	top := 0
	cur, col := 0, 0
	h := rows - 6
	msg := ""
	colX := []int{6, 19, 32, 40}
	colW := []int{10, 9, 5, 3}
	typeName := map[byte]string{'C': "Character", 'N': "Numeric", 'D': "Date", 'L': "Logical", 'M': "Memo", 'F': "Float"}
	cell := func(f db.FieldDef, c int) string {
		switch c {
		case 0:
			return f.Name
		case 1:
			return typeName[f.Type]
		case 2:
			return strconv.Itoa(f.Len)
		case 3:
			if f.Type == 'N' || f.Type == 'F' {
				return strconv.Itoa(f.Dec)
			}
			return ""
		}
		return ""
	}
	total := func() int {
		n := 1
		for _, f := range fields {
			n += f.Len
		}
		return n
	}
	for {
		if cur < top {
			top = cur
		}
		if cur >= top+h {
			top = cur - h + 1
		}
		m.Con.Scroll(0, 0, rows-1, cols-1, 0, std)
		m.Con.Put(0, 0, vm.PadTo(" "+title, cols, 'R', " "), hdr)
		m.Con.Put(2, 0, " Num  Field Name   Type         Width   Dec", std)
		for i := 0; i < h && top+i < len(fields); i++ {
			r := top + i
			m.Con.Put(3+i, 0, fmt.Sprintf("%4d", r+1), std)
			for c := 0; c < 4; c++ {
				at := std
				if r == cur && c == col {
					at = hi
				}
				align := byte('R')
				if c >= 2 {
					align = 'L'
				}
				m.Con.Put(3+i, colX[c], vm.PadTo(cell(fields[r], c), colW[c], align, " "), at)
			}
		}
		m.Con.Put(rows-2, 0, fmt.Sprintf(" Fields: %d   Record length: %d   %s", len(fields), total(), msg), std)
		m.Con.Put(rows-1, 0, vm.PadTo(" Enter edit  ^N insert  ^U delete  ^W/^End save  Esc cancel   Type: C N D L M", cols, 'R', " "), hdr)
		msg = ""
		m.Con.SetPos(3+cur-top, colX[col])
		k := m.Inkey(-1, false)
		f := &fields[cur]
		switch k {
		case term.KEsc:
			if gets.Alert(m, "Abandon structure changes?", []string{"No", "Yes"}) == 2 {
				return nil, false
			}
		case term.KCtrlW:
			// validate
			var out []db.FieldDef
			seen := map[string]bool{}
			bad := ""
			for i, fd := range fields {
				fd.Name = strings.ToUpper(strings.TrimSpace(fd.Name))
				if fd.Name == "" {
					if i == len(fields)-1 && len(fields) > 1 {
						continue
					}
					bad = fmt.Sprintf("Field %d has no name", i+1)
					break
				}
				if seen[fd.Name] {
					bad = "Duplicate field name " + fd.Name
					break
				}
				seen[fd.Name] = true
				out = append(out, fd)
			}
			if bad == "" && len(out) == 0 {
				bad = "No fields defined"
			}
			if bad != "" {
				msg = bad
				m.Con.Bell()
				continue
			}
			return out, true
		case term.KUp:
			if cur > 0 {
				cur--
			}
		case term.KDown:
			if cur < len(fields)-1 {
				cur++
			} else if strings.TrimSpace(f.Name) != "" {
				fields = append(fields, db.FieldDef{Type: 'C', Len: 10})
				cur++
				col = 0
			}
		case term.KLeft, term.KShTab:
			if col > 0 {
				col--
			}
		case term.KRight, term.KTab:
			if col < 3 {
				col++
			}
		case term.KPgUp:
			cur -= h
			if cur < 0 {
				cur = 0
			}
		case term.KPgDn:
			cur += h
			if cur >= len(fields) {
				cur = len(fields) - 1
			}
		case term.KCtrlN:
			fields = append(fields, db.FieldDef{})
			copy(fields[cur+1:], fields[cur:])
			fields[cur] = db.FieldDef{Type: 'C', Len: 10}
			col = 0
		case term.KCtrlU:
			if len(fields) > 1 {
				fields = append(fields[:cur], fields[cur+1:]...)
				if cur >= len(fields) {
					cur = len(fields) - 1
				}
			} else {
				fields[0] = db.FieldDef{Type: 'C', Len: 10}
			}
		default:
			if col == 1 {
				// type column: letter or space cycles
				order := "CNDLM"
				var t byte
				switch {
				case k == ' ':
					i := strings.IndexByte(order, f.Type)
					t = order[(i+1)%len(order)]
				case k > 32 && k < 128:
					c := strings.ToUpper(string(rune(k)))[0]
					if strings.IndexByte(order+"F", c) >= 0 {
						t = c
					}
				case k == term.KEnter:
					col = 2
					if f.Type == 'D' || f.Type == 'L' || f.Type == 'M' {
						col = 0
						if cur == len(fields)-1 {
							fields = append(fields, db.FieldDef{Type: 'C', Len: 10})
						}
						cur++
					}
					continue
				}
				if t != 0 {
					f.Type = t
					switch t {
					case 'D':
						f.Len, f.Dec = 8, 0
					case 'L':
						f.Len, f.Dec = 1, 0
					case 'M':
						f.Len, f.Dec = 10, 0
					case 'N', 'F':
						if f.Len > 19 {
							f.Len = 10
						}
					}
				}
				continue
			}
			if k != term.KEnter && (k < 32 || k > 255) {
				continue
			}
			if col == 2 && (f.Type == 'D' || f.Type == 'L' || f.Type == 'M') {
				col = 0
				continue
			}
			if col == 3 && f.Type != 'N' && f.Type != 'F' {
				col = 0
				if cur == len(fields)-1 {
					fields = append(fields, db.FieldDef{Type: 'C', Len: 10})
				}
				cur++
				continue
			}
			initial := cell(*f, col)
			if k != term.KEnter {
				m.Con.PushKeys([]int{term.KCtrlY, k})
			}
			m.Con.SetPos(3+cur-top, colX[col])
			s, ok := term.ReadLine(m.Con, hi, initial, colW[col], nil)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			switch col {
			case 0:
				valid := s != ""
				for i := 0; i < len(s); i++ {
					c := s[i]
					if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
						valid = false
					}
				}
				if !valid {
					msg = "Invalid field name"
					m.Con.Bell()
					continue
				}
				f.Name = strings.ToUpper(s)
				col = 1
			case 2:
				n, err := strconv.Atoi(s)
				max := 254
				if f.Type == 'N' || f.Type == 'F' {
					max = 19
				}
				if err != nil || n < 1 || n > max {
					msg = fmt.Sprintf("Width must be 1 to %d", max)
					m.Con.Bell()
					continue
				}
				f.Len = n
				if f.Type == 'N' || f.Type == 'F' {
					col = 3
				} else {
					col = 0
					if cur == len(fields)-1 {
						fields = append(fields, db.FieldDef{Type: 'C', Len: 10})
					}
					cur++
				}
			case 3:
				n, err := strconv.Atoi(s)
				if s == "" {
					n, err = 0, nil
				}
				if err != nil || n < 0 || (n > 0 && n > f.Len-2) {
					msg = "Invalid decimals"
					m.Con.Bell()
					continue
				}
				f.Dec = n
				col = 0
				if cur == len(fields)-1 {
					fields = append(fields, db.FieldDef{Type: 'C', Len: 10})
				}
				cur++
			}
		}
	}
}

// restructure rebuilds the current table with a new structure, keeping data.
func restructure(m *vm.VM, wa *db.WorkArea, fields []db.FieldDef) {
	path := wa.T.Path
	alias := wa.Alias
	var ixPaths []string
	for _, ix := range wa.Indexes {
		ixPaths = append(ixPaths, ix.Path)
	}
	area := wa.Num
	cp := wa.T.CodePage()
	m.CloseArea(area)
	base := strings.TrimSuffix(path, filepath.Ext(path))
	bak := base + ".bak"
	os.Remove(bak)
	if err := os.Rename(path, bak); err != nil {
		m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Cannot rename: " + err.Error(), Filename: path})
		return
	}
	for _, mext := range []string{".dbt", ".DBT", ".fpt", ".FPT"} {
		if db.FileExists(base + mext) {
			os.Remove(base + ".tbk")
			os.Rename(base+mext, base+".tbk")
			break
		}
	}
	// open the backup under its original memo name so memos are readable
	tmpDbf := base + "_old.dbf"
	data, _ := os.ReadFile(bak)
	os.WriteFile(tmpDbf, data, 0644)
	if db.FileExists(base + ".tbk") {
		mdata, _ := os.ReadFile(base + ".tbk")
		ext := ".dbt"
		if len(data) > 0 && data[0] == 0xF5 {
			ext = ".fpt"
		}
		os.WriteFile(base+"_old"+ext, mdata, 0644)
		defer os.Remove(base + "_old" + ext)
	}
	defer os.Remove(tmpDbf)
	if err := db.CreateTable(path, fields, cp); err != nil {
		os.Rename(bak, path)
		m.Raise(&vm.RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		return
	}
	src := m.OpenTemp(tmpDbf)
	swa := m.Area(src)
	t, err := db.OpenTable(path, false)
	if err == nil {
		t.SetCodePage(m.CodePage())
		nwa := db.NewWorkArea(area, alias, t, &m.DBSet)
		m.SetArea(area, nwa)
		for r := 1; r <= swa.T.RecCount(); r++ {
			swa.GoTo(r)
			nwa.Append()
			for i, fd := range nwa.T.Fields {
				si := swa.T.FieldIndex(fd.Name)
				if si < 0 {
					continue
				}
				v := swa.GetField(si)
				nwa.PutField(i, convertValue(m, v, fd))
			}
			if swa.Deleted() {
				nwa.SetDeleted(true)
			}
		}
		nwa.Commit()
		m.CloseTemp(src)
		m.SelectArea(area)
		for _, p := range ixPaths {
			m.OpenIndex(nwa, p)
		}
		if len(nwa.Indexes) > 0 {
			nwa.Reindex()
		}
		nwa.GoTop()
	}
}

func convertValue(m *vm.VM, v value.Value, fd db.FieldDef) value.Value {
	switch fd.Type {
	case 'C', 'M':
		if v.K == value.STRING {
			return v
		}
		return value.Str(strings.TrimSpace(m.Display(v)))
	case 'N', 'F':
		if v.K == value.NUMBER {
			return v
		}
		if v.K == value.STRING {
			n, _ := strconv.ParseFloat(strings.TrimSpace(v.S), 64)
			return value.Num(n, fd.Dec)
		}
		return value.Int(0)
	case 'D':
		if v.K == value.DATE {
			return v
		}
		if v.K == value.STRING {
			return value.Date(value.STOD(v.S))
		}
		return value.Date(0)
	case 'L':
		if v.K == value.LOGICAL {
			return v
		}
		if v.K == value.STRING {
			s := strings.ToUpper(strings.TrimSpace(v.S))
			return value.Bool(s == "T" || s == "Y" || s == ".T.")
		}
		return value.False
	}
	return v
}

// ---------- help ----------

// helpText is shown by HELP, in a window at the dot prompt, so keep lines
// within 76 columns.
const helpText = `Files     USE <file> [ALIAS a] [NEW] [INDEX i,..]  CLOSE [ALL|DATABASES]
          CREATE <file>  MODIFY STRUCTURE  DISPLAY STRUCTURE  DIR  CD [dir]
Navigate  GO TOP|BOTTOM|<n>  SKIP [n]  SEEK <key>  FIND <text>
          LOCATE FOR <c>  CONTINUE
Edit      APPEND [BLANK]  EDIT  BROWSE  REPLACE <f> WITH <x> [scope]
          DELETE [scope]  RECALL [scope]  PACK  ZAP
Query     LIST|DISPLAY [fields] [FOR c] [WHILE c] [NEXT n] [ALL]
          COUNT|SUM|AVERAGE [TO var]  SET FILTER TO <c>
          SET RELATION TO <key> INTO <alias>
Indexes   INDEX ON <key> TO <file> [UNIQUE]  SET INDEX TO  SET ORDER TO
          REINDEX
Data      COPY TO  APPEND FROM  SORT TO  TOTAL ON  COPY STRUCTURE
          REPORT FORM <file>  LABEL FORM <file>
          CREATE|MODIFY REPORT <file>  CREATE|MODIFY LABEL <file>
Programs  MODIFY COMMAND <prog>  DO <prog> [WITH args]  SET PROCEDURE TO
Memory    STORE x TO v  ? / ??  DISPLAY MEMORY  RELEASE  SAVE TO  RESTORE
Screen    @ r,c SAY x [PICTURE p] GET v [VALID c] [WHEN c]  READ  CLEAR
Windows   WINDOW OPEN t,l TO b,r [TITLE c] [COLOR c] [FIXED] [TO var]
          SET WINDOW TO [n]  WINDOW CLOSE [n|ALL]  WINDOW MOVE n TO t,l
          WINDOW SIZE n TO rows,cols  WINDOW MINIMIZE|MAXIMIZE|RESTORE [n]
Settings  SET EXACT|DELETED|CENTURY|CONFIRM|SOFTSEEK ON|OFF  SET DATE
          SET COLOR TO [spec]  SET THEME TO [classic|color|mono]
          SET STATUS ON|OFF
Other     !<shell cmd>  RUN  VERSION [FULL]  HELP  QUIT
Keys      Up/Down recall history.  Ctrl-C interrupts a running program,
          Alt-D opens the debugger (or nantucket -debug <prog>).
          F1 shows or hides this help; Esc (on an empty line) closes it.
          F6 arranges windows: arrows move, Shift+arrows size, Tab next.
Mouse     Drag a title bar to move a window, its corner to resize it.
          [_] minimise  [↑] maximise  [x] close.`
