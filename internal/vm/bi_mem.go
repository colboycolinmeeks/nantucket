// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nantucket/internal/db"
	"nantucket/internal/value"
)

// wildMatch matches dBase skeletons with * and ?.
func wildMatch(pattern, name string) bool {
	ok, _ := filepath.Match(strings.ToUpper(pattern), strings.ToUpper(name))
	return ok
}

func (vm *VM) releaseVar(name string) {
	name = strings.ToUpper(strings.TrimSpace(name))
	st := vm.memvars[name]
	if len(st) == 0 {
		return
	}
	if name == "GETLIST" {
		st[len(st)-1].ref.V = value.ArrayOf(0)
		return
	}
	st = st[:len(st)-1]
	if len(st) == 0 {
		delete(vm.memvars, name)
	} else {
		vm.memvars[name] = st
	}
}

func (vm *VM) memvarNames() []string {
	var names []string
	for n, st := range vm.memvars {
		if len(st) > 0 {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func init() {
	reg("__MCLEAR", func(vm *VM, a []value.Value) value.Value {
		for _, n := range vm.memvarNames() {
			if n != "GETLIST" {
				delete(vm.memvars, n)
			}
		}
		return value.Nil
	})
	reg("__MXRELEASE", func(vm *VM, a []value.Value) value.Value {
		for _, x := range a {
			for _, n := range strings.Split(x.Deref().S, ",") {
				vm.releaseVar(n)
			}
		}
		return value.Nil
	})
	reg("__MRELEASE", func(vm *VM, a []value.Value) value.Value {
		skel, like := argStr(a, 0, "*"), argBool(a, 1, true)
		owner := vm.ownerFrame()
		atPrompt := vm.AtPrompt()
		for _, n := range vm.memvarNames() {
			st := vm.memvars[n]
			top := st[len(st)-1]
			if !atPrompt && top.owner != owner {
				continue
			}
			if n == "GETLIST" {
				continue
			}
			if wildMatch(skel, n) == like {
				vm.releaseVar(n)
			}
		}
		return value.Nil
	})
	reg("__MSAVE", func(vm *VM, a []value.Value) value.Value {
		path := vm.DefaultPath(argStr(a, 0, ""), ".mem")
		skel, like := argStr(a, 1, "*"), argBool(a, 2, true)
		f, err := os.Create(path)
		if err != nil {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 20, SubCode: 2006, Description: "Create error", Filename: path})
		}
		defer f.Close()
		for _, n := range vm.memvarNames() {
			if wildMatch(skel, n) != like || n == "GETLIST" {
				continue
			}
			v := vm.findMemvar(n).V
			hdr := make([]byte, 32)
			copy(hdr[0:10], n)
			var data []byte
			switch v.K {
			case value.STRING:
				hdr[11] = 'C' | 0x80
				data = append([]byte(v.S), 0)
				hdr[16] = byte(len(data) & 0xFF)
				hdr[17] = byte(len(data) >> 8)
			case value.NUMBER:
				hdr[11] = 'N' | 0x80
				hdr[16] = byte(v.DefaultWidth())
				hdr[17] = byte(v.Dec)
				data = make([]byte, 8)
				binary.LittleEndian.PutUint64(data, math.Float64bits(v.N))
			case value.DATE:
				hdr[11] = 'D' | 0x80
				hdr[16] = 1
				data = make([]byte, 8)
				binary.LittleEndian.PutUint64(data, math.Float64bits(float64(v.J)))
			case value.LOGICAL:
				hdr[11] = 'L' | 0x80
				hdr[16] = 1
				data = []byte{0}
				if v.B {
					data[0] = 1
				}
			default:
				continue
			}
			f.Write(hdr)
			f.Write(data)
		}
		f.Write([]byte{0x1A})
		return value.Nil
	})
	reg("__MRESTORE", func(vm *VM, a []value.Value) value.Value {
		path := vm.FindFile(argStr(a, 0, ""), ".mem")
		if path == "" {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2005, Description: "Open error", Filename: argStr(a, 0, "")})
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2005, Description: "Open error", Filename: path})
		}
		if !argBool(a, 1, false) {
			for _, n := range vm.memvarNames() {
				if n != "GETLIST" {
					vm.releaseVar(n)
				}
			}
		}
		vars, err := parseMem(data)
		if err != nil {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 32, SubCode: 2006, Description: "Corruption detected: " + err.Error(), Filename: path})
		}
		for _, mv := range vars {
			vm.privateVar(mv.name, mv.v)
		}
		return value.Nil
	})
	reg("__MDISP", func(vm *VM, a []value.Value) value.Value {
		names := vm.memvarNames()
		count := 0
		for _, n := range names {
			st := vm.memvars[n]
			m := st[len(st)-1]
			scope := "priv"
			if m.owner == nil {
				scope = "pub"
			}
			v := m.ref.V
			shown := vm.Display(v)
			if v.K == value.STRING {
				shown = `"` + v.S + `"`
			}
			if len(shown) > 50 {
				shown = shown[:50] + "..."
			}
			vm.Println(fmt.Sprintf("%-10s %-4s %s  %s", n, scope, v.TypeChar(), shown))
			count++
		}
		vm.Println(fmt.Sprintf("%5d variables defined", count))
		return value.Nil
	})

	// ---------- files ----------
	reg("FILE", func(vm *VM, a []value.Value) value.Value {
		spec := argStr(a, 0, "")
		if strings.ContainsAny(spec, "*?") {
			m, _ := filepath.Glob(spec)
			return value.Bool(len(m) > 0)
		}
		return value.Bool(vm.FindFile(spec, "") != "")
	})
	reg("HB_FILEEXISTS", func(vm *VM, a []value.Value) value.Value {
		_, err := os.Stat(argStr(a, 0, ""))
		return value.Bool(err == nil)
	})
	reg("FERASE", func(vm *VM, a []value.Value) value.Value {
		if err := os.Remove(vm.DefaultPath(argStr(a, 0, ""), "")); err != nil {
			vm.lastFError = 2
			return value.Int(-1)
		}
		return value.Int(0)
	})
	reg("FRENAME", func(vm *VM, a []value.Value) value.Value {
		if err := os.Rename(argStr(a, 0, ""), argStr(a, 1, "")); err != nil {
			vm.lastFError = 2
			return value.Int(-1)
		}
		return value.Int(0)
	})
	reg("__COPYFILE", func(vm *VM, a []value.Value) value.Value {
		data, err := os.ReadFile(argStr(a, 0, ""))
		if err != nil {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2012, Description: "Open error", Filename: argStr(a, 0, "")})
		}
		if err := os.WriteFile(argStr(a, 1, ""), data, 0644); err != nil {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 20, SubCode: 2012, Description: "Create error", Filename: argStr(a, 1, "")})
		}
		return value.Nil
	})
	reg("MEMOREAD", func(vm *VM, a []value.Value) value.Value {
		data, err := os.ReadFile(argStr(a, 0, ""))
		if err != nil {
			return value.Str("")
		}
		return value.Str(string(data))
	})
	reg("HB_MEMOREAD", builtinTable["MEMOREAD"].fn)
	reg("MEMOWRIT", func(vm *VM, a []value.Value) value.Value {
		return value.Bool(os.WriteFile(argStr(a, 0, ""), []byte(argStr(a, 1, "")), 0644) == nil)
	})
	reg("HB_MEMOWRIT", builtinTable["MEMOWRIT"].fn)
	reg("FOPEN", func(vm *VM, a []value.Value) value.Value {
		mode := argInt(a, 1, 0) & 3
		flag := os.O_RDONLY
		if mode == 1 {
			flag = os.O_WRONLY
		} else if mode == 2 {
			flag = os.O_RDWR
		}
		f, err := os.OpenFile(argStr(a, 0, ""), flag, 0)
		if err != nil {
			vm.lastFError = 2
			return value.Int(-1)
		}
		vm.lastFError = 0
		h := vm.nextHandle
		vm.nextHandle++
		vm.memFiles[h] = f
		return value.Int(h)
	})
	reg("FCREATE", func(vm *VM, a []value.Value) value.Value {
		f, err := os.Create(argStr(a, 0, ""))
		if err != nil {
			vm.lastFError = 5
			return value.Int(-1)
		}
		vm.lastFError = 0
		h := vm.nextHandle
		vm.nextHandle++
		vm.memFiles[h] = f
		return value.Int(h)
	})
	reg("FCLOSE", func(vm *VM, a []value.Value) value.Value {
		h := argInt(a, 0, -1)
		if f, ok := vm.memFiles[h]; ok {
			f.Close()
			delete(vm.memFiles, h)
			return value.True
		}
		return value.False
	})
	regRaw("FREAD", func(vm *VM, a []value.Value) value.Value {
		f, ok := vm.memFiles[argInt(a, 0, -1)]
		n := argInt(a, 2, 0)
		if !ok || n <= 0 {
			return value.Int(0)
		}
		buf := make([]byte, n)
		got, _ := io.ReadFull(f, buf)
		if len(a) > 1 && a[1].K == value.REF {
			r := a[1].Rf()
			old := r.V.S
			if len(old) > got {
				r.V = value.Str(string(buf[:got]) + old[got:])
			} else {
				r.V = value.Str(string(buf[:got]))
			}
		}
		return value.Int(got)
	})
	reg("FREADSTR", func(vm *VM, a []value.Value) value.Value {
		f, ok := vm.memFiles[argInt(a, 0, -1)]
		n := argInt(a, 1, 0)
		if !ok || n <= 0 {
			return value.Str("")
		}
		buf := make([]byte, n)
		got, _ := io.ReadFull(f, buf)
		s := string(buf[:got])
		if i := strings.IndexByte(s, 0); i >= 0 {
			s = s[:i]
		}
		return value.Str(s)
	})
	reg("FWRITE", func(vm *VM, a []value.Value) value.Value {
		f, ok := vm.memFiles[argInt(a, 0, -1)]
		if !ok {
			return value.Int(0)
		}
		s := argStr(a, 1, "")
		if has(a, 2) && argInt(a, 2, len(s)) < len(s) {
			s = s[:argInt(a, 2, len(s))]
		}
		n, _ := f.WriteString(s)
		return value.Int(n)
	})
	reg("FSEEK", func(vm *VM, a []value.Value) value.Value {
		f, ok := vm.memFiles[argInt(a, 0, -1)]
		if !ok {
			return value.Int(-1)
		}
		pos, err := f.Seek(int64(argInt(a, 1, 0)), argInt(a, 2, 0))
		if err != nil {
			return value.Int(-1)
		}
		return value.Int(int(pos))
	})
	reg("FERROR", func(vm *VM, a []value.Value) value.Value { return value.Int(vm.lastFError) })
	reg("DIRECTORY", func(vm *VM, a []value.Value) value.Value {
		spec := argStr(a, 0, "*")
		if spec == "" {
			spec = "*"
		}
		if st, err := os.Stat(spec); err == nil && st.IsDir() {
			spec = filepath.Join(spec, "*")
		}
		matches, _ := filepath.Glob(spec)
		attrs := strings.ToUpper(argStr(a, 1, ""))
		var items []value.Value
		for _, m := range matches {
			st, err := os.Stat(m)
			if err != nil {
				continue
			}
			attr := ""
			if st.IsDir() {
				if !strings.Contains(attrs, "D") {
					continue
				}
				attr = "D"
			}
			t := st.ModTime()
			items = append(items, value.NewArray([]value.Value{
				value.Str(filepath.Base(m)), value.Int(int(st.Size())),
				value.Date(value.ToJulian(t.Year(), int(t.Month()), t.Day())), value.Str(t.Format("15:04:05")), value.Str(attr)}))
		}
		return value.NewArray(items)
	})
	reg("__DIR", func(vm *VM, a []value.Value) value.Value {
		spec := strings.TrimSpace(argStr(a, 0, ""))
		if spec == "" {
			vm.Println("Database Files    # Records    Last Update     Size")
			matches, _ := filepath.Glob("*.[dD][bB][fF]")
			total := int64(0)
			for _, m := range matches {
				st, err := os.Stat(m)
				if err != nil {
					continue
				}
				recs, lu := 0, ""
				if t, err := db.OpenTableMode(m, true, db.OpenNoLock); err == nil {
					recs = t.RecCount()
					y, mo, d := t.LastUpdate()
					if value.ValidDate(y, mo, d) {
						lu = vm.DTOC(value.ToJulian(y, mo, d))
					}
					t.Close()
				}
				total += st.Size()
				vm.Println(fmt.Sprintf("%-16s %10d    %-12s %8d", filepath.Base(m), recs, lu, st.Size()))
			}
			vm.Println(fmt.Sprintf("\n%8d bytes in %5d files.", total, len(matches)))
			return value.Nil
		}
		if st, err := os.Stat(spec); err == nil && st.IsDir() {
			spec = filepath.Join(spec, "*")
		}
		matches, _ := filepath.Glob(spec)
		var line strings.Builder
		for i, m := range matches {
			line.WriteString(fmt.Sprintf("%-16s", filepath.Base(m)))
			if (i+1)%5 == 0 {
				vm.Println(line.String())
				line.Reset()
			}
		}
		if line.Len() > 0 {
			vm.Println(line.String())
		}
		vm.Println(fmt.Sprintf("%5d files", len(matches)))
		return value.Nil
	})
	reg("ADIR", func(vm *VM, a []value.Value) value.Value {
		matches, _ := filepath.Glob(argStr(a, 0, "*"))
		for k := 1; k <= 5; k++ {
			arr := arg(a, k)
			if arr.K != value.ARRAY {
				continue
			}
			it := arr.Arr().Items
			for i, m := range matches {
				if i >= len(it) {
					break
				}
				st, _ := os.Stat(m)
				if st == nil {
					continue
				}
				t := st.ModTime()
				switch k {
				case 1:
					it[i] = value.Str(filepath.Base(m))
				case 2:
					it[i] = value.Int(int(st.Size()))
				case 3:
					it[i] = value.Date(value.ToJulian(t.Year(), int(t.Month()), t.Day()))
				case 4:
					it[i] = value.Str(t.Format("15:04:05"))
				case 5:
					it[i] = value.Str("")
				}
			}
		}
		return value.Int(len(matches))
	})
	reg("__TYPEFILE", func(vm *VM, a []value.Value) value.Value {
		p := vm.FindFile(argStr(a, 0, ""), "")
		if p == "" {
			return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 21, SubCode: 2011, Description: "File does not exist", Filename: argStr(a, 0, "")})
		}
		data, _ := os.ReadFile(p)
		for _, ln := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			vm.Println(ln)
		}
		return value.Nil
	})
	reg("HB_DIREXISTS", func(vm *VM, a []value.Value) value.Value {
		st, err := os.Stat(argStr(a, 0, ""))
		return value.Bool(err == nil && st.IsDir())
	})
	reg("FILEDATE", func(vm *VM, a []value.Value) value.Value {
		st, err := os.Stat(argStr(a, 0, ""))
		if err != nil {
			return value.Date(0)
		}
		t := st.ModTime()
		return value.Date(value.ToJulian(t.Year(), int(t.Month()), t.Day()))
	})
	_ = time.Now
}

type memVar struct {
	name string
	v    value.Value
}

// parseMem reads the variables in a .mem file (SAVE TO), checking every
// length against the data so that a damaged file is reported rather than
// crashing.
func parseMem(data []byte) ([]memVar, error) {
	var out []memVar
	for p := 0; p < len(data); {
		if data[p] == 0x1A {
			break
		}
		if p+32 > len(data) {
			return out, fmt.Errorf("variable %d is cut short", len(out)+1)
		}
		hdr := data[p : p+32]
		name := strings.TrimRight(string(hdr[0:11]), "\x00 ")
		if i := strings.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		if name == "" {
			return out, fmt.Errorf("variable %d has no name", len(out)+1)
		}
		p += 32
		need := 0
		switch hdr[11] &^ 0x80 {
		case 'C':
			need = int(hdr[16]) | int(hdr[17])<<8
		case 'N', 'D':
			need = 8
		case 'L':
			need = 1
		default:
			return out, fmt.Errorf("variable %s has unknown type %q", name, hdr[11]&^0x80)
		}
		if p+need > len(data) {
			return out, fmt.Errorf("variable %s is cut short", name)
		}
		var v value.Value
		switch hdr[11] &^ 0x80 {
		case 'C':
			v = value.Str(strings.TrimRight(string(data[p:p+need]), "\x00"))
		case 'N':
			f := math.Float64frombits(binary.LittleEndian.Uint64(data[p:]))
			if math.IsNaN(f) || math.IsInf(f, 0) {
				f = 0
			}
			v = value.NumW(f, 0, int(hdr[17]))
		case 'D':
			f := math.Float64frombits(binary.LittleEndian.Uint64(data[p:]))
			if math.IsNaN(f) || f < 0 || f > 1e8 {
				f = 0
			}
			v = value.Date(int32(f))
		case 'L':
			v = value.Bool(data[p] != 0)
		}
		p += need
		out = append(out, memVar{name, v})
	}
	return out, nil
}
