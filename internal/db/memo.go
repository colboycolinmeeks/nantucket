// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
)

type MemoType int

const (
	MemoDBT3 MemoType = iota
	MemoDBT4
	MemoFPT
)

type Memo struct {
	f         *os.File
	typ       MemoType
	blockSize int
	nextFree  int
	readOnly  bool
	shared    bool // the table is shared: re-read nextFree under a lock
}

func CreateMemo(path string, typ MemoType) error {
	hdr := make([]byte, 512)
	switch typ {
	case MemoFPT:
		binary.BigEndian.PutUint32(hdr[0:], 8) // 512/64
		binary.BigEndian.PutUint16(hdr[6:], 64)
	default:
		binary.LittleEndian.PutUint32(hdr[0:], 1)
		hdr[16] = 0x03
	}
	return os.WriteFile(path, hdr, 0644)
}

func OpenMemo(path string, typ MemoType, readOnly bool) (*Memo, error) {
	flag := os.O_RDWR
	if readOnly {
		flag = os.O_RDONLY
	}
	f, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return nil, err
	}
	m := &Memo{f: f, typ: typ, blockSize: 512, readOnly: readOnly}
	hdr := make([]byte, 32)
	f.ReadAt(hdr, 0)
	switch typ {
	case MemoFPT:
		m.nextFree = int(binary.BigEndian.Uint32(hdr[0:]))
		m.blockSize = int(binary.BigEndian.Uint16(hdr[6:]))
		if m.blockSize == 0 {
			m.blockSize = 64
		}
	case MemoDBT4:
		m.nextFree = int(binary.LittleEndian.Uint32(hdr[0:]))
		bs := int(binary.LittleEndian.Uint16(hdr[20:]))
		if bs > 0 {
			m.blockSize = bs
		}
	default:
		m.nextFree = int(binary.LittleEndian.Uint32(hdr[0:]))
	}
	if m.nextFree < 1 {
		m.nextFree = 1
	}
	return m, nil
}

func (m *Memo) Close() { m.f.Close() }

func (m *Memo) Read(blk int) (string, error) {
	off := int64(blk) * int64(m.blockSize)
	switch m.typ {
	case MemoFPT:
		h := make([]byte, 8)
		if _, err := m.f.ReadAt(h, off); err != nil {
			return "", err
		}
		n, err := m.fits(int64(binary.BigEndian.Uint32(h[4:])), off+8)
		if err != nil {
			return "", err
		}
		data := make([]byte, n)
		m.f.ReadAt(data, off+8)
		return string(data), nil
	case MemoDBT4:
		h := make([]byte, 8)
		if _, err := m.f.ReadAt(h, off); err != nil {
			return "", err
		}
		if h[0] == 0xFF && h[1] == 0xFF {
			n, err := m.fits(int64(binary.LittleEndian.Uint32(h[4:]))-8, off+8)
			if err != nil {
				return "", err
			}
			data := make([]byte, n)
			m.f.ReadAt(data, off+8)
			return string(data), nil
		}
	}
	// dBase III: read until 0x1A
	var out []byte
	buf := make([]byte, 512)
	for {
		n, err := m.f.ReadAt(buf, off)
		if n == 0 {
			break
		}
		if i := bytes.IndexByte(buf[:n], 0x1A); i >= 0 {
			out = append(out, buf[:i]...)
			break
		}
		out = append(out, buf[:n]...)
		off += int64(n)
		if err != nil {
			break
		}
	}
	return string(out), nil
}

// ErrBadMemo means a memo's stored length doesn't fit in the file.
var ErrBadMemo = errors.New("memo file is damaged: a memo is longer than the file")

// fits checks that n bytes starting at off lie within the file, so that a
// damaged length can't make Nantucket allocate gigabytes.
func (m *Memo) fits(n, off int64) (int, error) {
	if n <= 0 {
		return 0, nil
	}
	st, err := m.f.Stat()
	if err != nil {
		return 0, err
	}
	if off+n > st.Size() {
		return 0, ErrBadMemo
	}
	return int(n), nil
}

func (m *Memo) encode(s string) []byte {
	switch m.typ {
	case MemoFPT:
		b := make([]byte, 8+len(s))
		binary.BigEndian.PutUint32(b[0:], 1)
		binary.BigEndian.PutUint32(b[4:], uint32(len(s)))
		copy(b[8:], s)
		return b
	case MemoDBT4:
		b := make([]byte, 8+len(s))
		b[0], b[1], b[2], b[3] = 0xFF, 0xFF, 0x08, 0x00
		binary.LittleEndian.PutUint32(b[4:], uint32(len(s)+8))
		copy(b[8:], s)
		return b
	}
	b := make([]byte, len(s)+2)
	copy(b, s)
	b[len(s)], b[len(s)+1] = 0x1A, 0x1A
	return b
}

// usedBlocks returns how many blocks the memo at blk occupies.
func (m *Memo) usedBlocks(blk int) int {
	s, err := m.Read(blk)
	if err != nil {
		return 0
	}
	n := len(m.encode(s))
	return (n + m.blockSize - 1) / m.blockSize
}

// Write stores s, reusing old's space when it fits. Returns the block.
func (m *Memo) Write(old int, s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	data := m.encode(s)
	need := (len(data) + m.blockSize - 1) / m.blockSize
	blk := 0
	if old > 0 && m.usedBlocks(old) >= need {
		blk = old
	} else {
		if m.shared {
			// another session may have added memos since we last looked
			if err := lockRange(m.f, indexLock, 1, true, true); err != nil {
				return 0, err
			}
			defer unlockRange(m.f, indexLock, 1)
			m.readNextFree()
		}
		blk = m.nextFree
		m.nextFree += need
		m.writeHeader()
	}
	padded := make([]byte, need*m.blockSize)
	copy(padded, data)
	_, err := m.f.WriteAt(padded, int64(blk)*int64(m.blockSize))
	return blk, err
}

// readNextFree re-reads the next free block from the header.
func (m *Memo) readNextFree() {
	var h [4]byte
	if _, err := m.f.ReadAt(h[:], 0); err != nil {
		return
	}
	n := int(binary.LittleEndian.Uint32(h[:]))
	if m.typ == MemoFPT {
		n = int(binary.BigEndian.Uint32(h[:]))
	}
	if n > m.nextFree {
		m.nextFree = n
	}
}

func (m *Memo) writeHeader() {
	h := make([]byte, 4)
	if m.typ == MemoFPT {
		binary.BigEndian.PutUint32(h, uint32(m.nextFree))
	} else {
		binary.LittleEndian.PutUint32(h, uint32(m.nextFree))
	}
	m.f.WriteAt(h, 0)
}

func (m *Memo) Zap() error {
	hdrBlocks := 1
	if m.typ == MemoFPT {
		hdrBlocks = 512 / m.blockSize
	}
	m.nextFree = hdrBlocks
	m.writeHeader()
	return m.f.Truncate(int64(hdrBlocks * m.blockSize))
}
