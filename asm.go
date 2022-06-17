package edb

import (
	"encoding/binary"
	"fmt"

	"github.com/aj3423/edb/util"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/fatih/color"
)

var ShowHexPC = false

// structs for display only
type Line struct {
	Pc      uint64
	LineNum uint64
	Op      *Operation
	Data    []byte // additional bytes for OpCode, eg: 4 bytes for PUSH4
}

func (l *Line) String() string {
	if ShowHexPC {
		return fmt.Sprintf("%8x %12s  %s", l.Pc, l.Op.OpCode.String(), util.HexEnc(l.Data))
	} else {
		return fmt.Sprintf("% 8d %12s  %s", l.Pc, l.Op.OpCode.String(), util.HexEnc(l.Data))
	}
}

// TODO replace with IndexedArray
type Asm struct {
	// for finding *Line by lineNum
	sequence []*Line

	// for finding *Line by pc
	mapPc map[uint64]*Line // map[pc]*Line
}

func NewAsm() *Asm {
	a := &Asm{}
	return a.Reset()
}

func (a *Asm) Reset() *Asm {
	a.sequence = nil
	a.mapPc = map[uint64]*Line{}
	return a
}
func (a *Asm) LineCount() int {
	return len(a.sequence)
}
func (a *Asm) LineAtPc(pc uint64) (*Line, error) {
	line, ok := a.mapPc[pc]
	if !ok {
		fmt.Println("mapPc len", len(a.mapPc))
		return nil, fmt.Errorf("invalid pc: %d", pc)
	}
	return line, nil
}
func (a *Asm) AtRow(row int) *Line {
	return a.sequence[row]
}

func (a *Asm) Disasm(
	code []byte,
) error {
	a.Reset()

	if len(code) > 2 { // remove trailing CBOR encoded metadata:
		last_2 := code[len(code)-2:]
		meta_len := binary.BigEndian.Uint16(last_2)
		code = code[0 : len(code)-int(meta_len)-2]
	}

	var codeLen = uint64(len(code))

	var pc uint64 = 0

	var line *Line

	for pc < codeLen {
		opCode := code[pc]
		op, ok := OpTable[vm.OpCode(opCode)]
		if !ok {
			// invalid op occurs at the end of code,
			// don't know how to parse the code+metadata perfectly,
			// stop disassembling with a warning

			color.Yellow("Warning: Disasm: invalid opcode: %s @pc: %d/%d", vm.OpCode(opCode).String(), pc, codeLen)
			return nil
		}

		if pc+1+op.OpSize > codeLen {
			/*
				In most cases this error is caused by trailing CBOR metadata, see:
					https://docs.soliditylang.org/en/v0.8.13/metadata.html#contract-metadata
			*/
			// TODO skip metadata
			color.Yellow("Warning: Disasm: not enough data for opcode: %s at pc: %d/%d, require %d bytes",
				op.OpCode.String(), pc, codeLen, op.OpSize)
			return nil
		}
		line = &Line{
			Pc:      pc,
			LineNum: uint64(len(a.sequence)),
			Op:      op,
			Data:    code[pc+1 : pc+1+op.OpSize],
		}
		a.sequence = append(a.sequence, line)
		a.mapPc[uint64(pc)] = line

		pc += 1 + op.OpSize
	}
	return nil
}
