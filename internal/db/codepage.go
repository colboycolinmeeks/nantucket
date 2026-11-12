// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Code pages.
//
// DBF files store text as bytes in a code page: DOS programs (dBase,
// Clipper) used an OEM code page such as 437 or 850, Windows programs one
// such as 1252. Nantucket works in Unicode (UTF-8), so character and memo
// fields are converted when read and written, and index keys are kept in
// the table's code page so they match indexes other programs build.
//
// A table's code page comes from its header (byte 29, the language driver
// ID) when that is set, otherwise from SET CODEPAGE TO. The default,
// "auto", suits a mix of old files and files written by earlier versions
// of Nantucket: text that is valid UTF-8 is read as it is, anything else
// as code page 437, and text is written in code page 437. "UTF8" stores
// UTF-8 unchanged, which other programs won't read correctly.

// CodePage is a character set for a table's text.
type CodePage struct {
	Name string           // as SET CODEPAGE shows it: "437", "1252", "UTF8", "AUTO"
	LDID byte             // language driver ID written to new tables (0: none)
	cm   *charmap.Charmap // nil for UTF8 and AUTO
	auto bool             // AUTO: read UTF-8 as it is, else 437
}

var codePages = []*CodePage{
	{Name: "AUTO", LDID: 0x01, cm: charmap.CodePage437, auto: true},
	{Name: "437", LDID: 0x01, cm: charmap.CodePage437},
	{Name: "850", LDID: 0x02, cm: charmap.CodePage850},
	{Name: "852", LDID: 0x64, cm: charmap.CodePage852},
	{Name: "865", LDID: 0x66, cm: charmap.CodePage865},
	{Name: "866", LDID: 0x65, cm: charmap.CodePage866},
	{Name: "1250", LDID: 0xC8, cm: charmap.Windows1250},
	{Name: "1251", LDID: 0xC9, cm: charmap.Windows1251},
	{Name: "1252", LDID: 0x03, cm: charmap.Windows1252},
	{Name: "UTF8"},
}

// more language driver IDs, read only
var ldidAliases = map[byte]string{0x57: "1252", 0x58: "1252", 0x59: "1252", 0x1B: "437", 0x37: "850"}

// LookupCodePage finds a code page by name: "437", "CP850", "1252",
// "WIN1252", "UTF8", "UTF-8" or "AUTO".
func LookupCodePage(name string) (*CodePage, bool) {
	n := strings.ToUpper(strings.TrimSpace(name))
	n = strings.NewReplacer("-", "", "_", "", " ", "").Replace(n)
	n = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(n, "CP"), "WIN"), "WINDOWS")
	if n == "" {
		n = "AUTO"
	}
	for _, cp := range codePages {
		if cp.Name == n {
			return cp, true
		}
	}
	return nil, false
}

// CodePageNames lists the code pages SET CODEPAGE accepts.
func CodePageNames() []string {
	var out []string
	for _, cp := range codePages {
		out = append(out, cp.Name)
	}
	return out
}

func codePageForLDID(b byte) *CodePage {
	if b == 0 {
		return nil
	}
	if n, ok := ldidAliases[b]; ok {
		cp, _ := LookupCodePage(n)
		return cp
	}
	for _, cp := range codePages {
		if cp.LDID == b && !cp.auto {
			return cp
		}
	}
	return nil
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

// Decode turns bytes from a table into UTF-8 text.
func (cp *CodePage) Decode(b []byte) string {
	if cp == nil || cp.cm == nil || isASCII(b) {
		return string(b)
	}
	if cp.auto && utf8.Valid(b) {
		return string(b) // written by an earlier Nantucket
	}
	var sb strings.Builder
	sb.Grow(len(b) + 8)
	for _, c := range b {
		sb.WriteRune(cp.cm.DecodeByte(c))
	}
	return sb.String()
}

// Encode turns UTF-8 text into bytes for the table. Characters the code
// page doesn't have become "?".
func (cp *CodePage) Encode(s string) []byte {
	if cp == nil || cp.cm == nil || isASCII([]byte(s)) {
		return []byte(s)
	}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if b, ok := cp.cm.EncodeRune(r); ok {
			out = append(out, b)
		} else {
			out = append(out, '?')
		}
	}
	return out
}

// EncodeString is Encode returning a string (index keys).
func (cp *CodePage) EncodeString(s string) string { return string(cp.Encode(s)) }
