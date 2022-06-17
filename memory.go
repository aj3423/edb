package edb

import (
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/aj3423/edb/util"
	"github.com/holiman/uint256"
)

type Memory struct {
	store []byte
}

func (m *Memory) MarshalJSON() ([]byte, error) {
	ss := []string{}
	p := 0
	for p < len(m.store) {
		chunkLen := util.Min(32, len(m.store)-p)

		chunk := m.store[p : p+chunkLen]
		ss = append(ss, util.HexEnc(chunk))
		p += chunkLen
	}

	return json.MarshalIndent(ss, "", "  ")
}
func (m *Memory) UnmarshalJSON(bs []byte) error {
	ss := []string{}
	e := json.Unmarshal(bs, &ss)
	if e != nil {
		return e
	}
	all := strings.Join(ss, "")
	m.store, e = hex.DecodeString(all)
	return e
}

// Set sets offset + size to value
func (m *Memory) Set(offset, size uint64, value []byte) {
	// It's possible the offset is greater than 0 and size equals 0. This is because
	// the calcMemSize (common.go) could potentially return 0 when size is zero (NO-OP)
	if size > 0 {
		// length of store may never be less than offset + size.
		// The store should be resized PRIOR to setting the memory
		if offset+size > uint64(len(m.store)) {
			m.Resize(offset + size)
		}
		copy(m.store[offset:offset+size], value)
	}
}

// Set32 sets the 32 bytes starting at offset to the value of val, left-padded with zeroes to
// 32 bytes.
func (m *Memory) Set32(offset uint64, val *uint256.Int) {
	// length of store may never be less than offset + size.
	// The store should be resized PRIOR to setting the memory
	if offset+32 > uint64(len(m.store)) {
		m.Resize(offset + 32)
	}
	// Zero the memory area
	copy(m.store[offset:offset+32], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	// Fill in relevant bits
	val.WriteToSlice(m.store[offset:])
}

// Resize resizes the memory to size
func (m *Memory) Resize(size uint64) {
	if m.Len() < size {
		m.store = append(m.store, make([]byte, size-m.Len())...)
	}
}

// Get returns offset + size as a new slice
func (m *Memory) GetCopy(offset, size int64) (cpy []byte) {
	if size == 0 {
		return nil
	}

	if len(m.store) > int(offset) {
		cpy = make([]byte, size)
		copy(cpy, m.store[offset:offset+size])

		return
	}

	return
}

// GetPtr returns the offset + size
func (m *Memory) GetPtr(offset, size int64) []byte {
	if size == 0 {
		return nil
	}

	if len(m.store) > int(offset) {
		return m.store[offset : offset+size]
	}

	return nil
}

// Len returns the length of the backing slice
func (m *Memory) Len() uint64 {
	return uint64(len(m.store))
}

// Data returns the backing slice
func (m *Memory) Data() []byte {
	return m.store
}
