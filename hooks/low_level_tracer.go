package hooks

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/aj3423/edb"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/fatih/color"
	"github.com/holiman/uint256"
)

type LowLevelTracer struct {
	*ParamTracer
}

func NewLowLevelTracer() *LowLevelTracer {
	return &LowLevelTracer{
		ParamTracer: &ParamTracer{},
	}
}
func (t *LowLevelTracer) PreRun(call *edb.Call, line *edb.Line) error {

	t.ParamTracer.PreRun(call, line) // save stack/mem params for PostRun
	return nil
}

func (t *LowLevelTracer) PostRun(call *edb.Call, line *edb.Line) error {
	opcode := line.Op.OpCode

	t.ParamTracer.PostRun(call, line)

	switch opcode {
	case vm.SHA3:
		// get result from stack
		color.Magenta("\nSHA3 memory (")
		// mem :=
		offset, size := t.StackPre.PeekI(0).ToBig().Uint64(), t.StackPre.PeekI(1).ToBig().Uint64()
		mem := t.MemPre[offset : offset+size]

		// pretty print
		switch len(mem) {
		case 0x20: // some index
			fmt.Println("    " + new(uint256.Int).SetBytes(mem).String())
		case 0x40: // some nested index
			fmt.Println("    " + new(uint256.Int).SetBytes(mem[0:0x20]).String())
			fmt.Println("    " + new(uint256.Int).SetBytes(mem[0x20:]).String())
		default:
			fmt.Println(hex.Dump(mem))
		}
		color.Magenta(")  ->  %s\n\n", t.StackPost.PeekI(0).String())
	case vm.MLOAD:
		color.White("  %s = mem[%s]", t.StackPost.PeekI(0).String(), t.StackPre.PeekI(0).String())
	case vm.MSTORE, vm.MSTORE8:
		color.White("  mem[%s] = %s", t.StackPre.PeekI(0).String(), t.StackPre.PeekI(1).String())
	case vm.SLOAD:
		color.White("    %s = storage[%s]", t.StackPost.PeekI(0).String(), t.StackPre.PeekI(0).String())
	case vm.SSTORE:
		color.White("    storage[%s] = %s", t.StackPre.PeekI(0).String(), t.StackPre.PeekI(1).String())
	case vm.CALL, vm.DELEGATECALL, vm.STATICCALL:
		var callee, value, inOffset, inSize *uint256.Int
		if opcode == vm.CALL {
			callee, value, inOffset, inSize =
				t.StackPre.PeekI(1), t.StackPre.PeekI(2), t.StackPre.PeekI(3), t.StackPre.PeekI(4)

			if inSize.IsZero() { // must be CALL, normal tranfer, addr.call{value:...}("")
				color.HiCyan("transfer value: %s -> %s",
					value.ToBig().String(), callee.String())
				return nil
			}
		} else {
			callee, inOffset, inSize =
				t.StackPre.PeekI(1), t.StackPre.PeekI(2), t.StackPre.PeekI(3)
		}
		data := t.MemPre[inOffset.ToBig().Uint64() : inOffset.ToBig().Uint64()+inSize.ToBig().Uint64()]

		fn := data[0:4]
		color.HiCyan("%s -> %s, fn: %x",
			opcode.String(), callee.String(), fn)

	case vm.LOG0, vm.LOG1, vm.LOG2, vm.LOG3, vm.LOG4:
		_, _ = t.StackPre.Pop(), t.StackPre.Pop()
		n := opcode - vm.LOG0
		topics := []string{}
		for i := 0; i < int(n); i++ {
			topic := t.StackPre.Pop()
			topics = append(topics, topic.String())
		}
		color.Magenta("%s (%s)",
			opcode.String(), strings.Join(topics, ","))

	// 0 arg
	case vm.TIMESTAMP, vm.NUMBER, vm.ADDRESS, vm.ORIGIN, vm.CALLER, vm.CALLVALUE,
		vm.GASPRICE, vm.COINBASE, vm.DIFFICULTY, vm.GASLIMIT, vm.CHAINID,
		vm.SELFBALANCE, vm.BASEFEE, vm.PC, vm.MSIZE, vm.GAS:
		color.White("  %s = %s", t.StackPost.Peek().String(), opcode.String())

	// 1 arg
	case vm.ISZERO, vm.NOT, vm.EXTCODEHASH, vm.BLOCKHASH:
		color.White(
			"%s (%s) -> %s\n",
			opcode.String(), t.StackPre.PeekI(0).String(), t.StackPost.Peek().String())
	// 2 arg
	case vm.ADD, vm.MUL, vm.SUB, vm.DIV, vm.SDIV, vm.MOD, vm.SMOD, vm.EXP,
		vm.SHL, vm.SHR, vm.SAR, vm.LT, vm.GT, vm.SLT, vm.SGT, vm.EQ,
		vm.SIGNEXTEND, vm.AND, vm.OR, vm.XOR, vm.BYTE:
		color.White(
			"%s (%s, %s) -> %s\n",
			opcode.String(), t.StackPre.PeekI(0).String(), t.StackPre.PeekI(1).String(), t.StackPost.Peek().String())
	// 3 arg
	case vm.ADDMOD, vm.MULMOD:
		color.White(
			"%s (%s, %s, %s) -> %s\n",
			opcode.String(), t.StackPre.PeekI(0).String(), t.StackPre.PeekI(1).String(), t.StackPre.PeekI(2).String(), t.StackPost.Peek().String())
	}
	return nil
}

// full evm trace log, eg:
//   0  PUSH 80
//   2  PUSH 40
//   4  MSTORE
//   ...
type EvmLog struct {
	edb.EmptyHook
	Fd *os.File
}

func (t *EvmLog) PreRun(call *edb.Call, line *edb.Line) error {
	fmt.Fprintf(t.Fd, "%d\t %s\n", line.Pc, line.Op.OpCode.String())
	return nil
}
