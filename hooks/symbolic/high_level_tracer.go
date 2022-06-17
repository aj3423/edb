package symbolic

import (
	"fmt"

	"github.com/aj3423/edb"
	"github.com/aj3423/edb/hooks"
	"github.com/aj3423/edb/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
)

var TODO = errors.New("HighLevelTracer TODO")

func init() {
	edb.Register((*HighLevelTracer)(nil))
}

type HighLevelTracer struct {
	*hooks.ParamTracer

	ctx       *edb.Context
	CallStack edb.Stack[*Call]
}

func NewHighLevelTracer(ctx *edb.Context) *HighLevelTracer {
	t := &HighLevelTracer{
		ctx:         ctx,
		ParamTracer: &hooks.ParamTracer{},
	}
	vmCall := ctx.Call()
	t.CallStack.Push(
		NewCall(vm.CALL, &vmCall.This, vmCall.Msg.Data))
	return t
}

// Record every asm param to its symbolic form
// Here just record, no optimize, optimization will be done later
func (t *HighLevelTracer) PostRun(vmCall *edb.Call, line *edb.Line) error {
	t.ParamTracer.PostRun(vmCall, line)

	call := *t.CallStack.Peek()
	stack := &call.Stack

	defer func() { // for debugging only
		if vmCall.Stack.Len() != stack.Len() {
			panic(fmt.Sprintf(
				"HiLevelTracer: symbolic stack is different from vmStack @pc: %d",
				t.PcPre))
		}
	}()

	opcode := line.Op.OpCode

	switch opcode {

	// Nullary operations, no stack input, 1 output
	case vm.ADDRESS, vm.BALANCE, vm.ORIGIN, vm.CALLER, vm.CALLVALUE, vm.CALLDATASIZE, vm.CODESIZE, vm.GASPRICE, vm.COINBASE, vm.TIMESTAMP, vm.NUMBER, vm.DIFFICULTY, vm.GASLIMIT, vm.CHAINID, vm.SELFBALANCE, vm.BASEFEE, vm.GAS, vm.PC, vm.MSIZE:

		n := &NullaryOp{}
		n.OpCode = opcode
		n.Val = t.StackPost.Pop()

		stack.Push(n)
		return nil

	// unary operation, 1 stack input, 1 output
	case vm.ISZERO, vm.NOT, vm.CALLDATALOAD, vm.EXTCODESIZE, vm.EXTCODEHASH, vm.BLOCKHASH:

		sym := stack.Pop()

		n := &UnaryOp{X: sym}
		n.OpCode = opcode
		n.Val = t.StackPost.Pop()

		stack.Push(n)
		return nil

	// binary operation, 2 stack input, 1 output
	case vm.ADD, vm.MUL, vm.SUB, vm.DIV, vm.SDIV, vm.MOD, vm.SMOD, vm.EXP, vm.SIGNEXTEND, vm.LT, vm.GT, vm.SLT, vm.SGT, vm.EQ, vm.AND, vm.OR, vm.XOR, vm.SHL, vm.SHR, vm.SAR, vm.BYTE:
		// sub: x-y, shl: y<<x
		x, y := stack.Pop(), stack.Pop()

		// SHL, SHR, SAR in reverse order
		switch opcode {
		case vm.SHL, vm.SHR, vm.SAR, vm.SIGNEXTEND, vm.BYTE:
			tmp := x
			x = y
			y = tmp
		}

		n := &BinaryOp{X: x, Y: y}
		n.OpCode = opcode
		n.Val = t.StackPost.Pop()

		stack.Push(n)
		return nil

	// ternary operation, 3 stack input, 1 output
	case vm.ADDMOD, vm.MULMOD:
		x, y, z := stack.Pop(), stack.Pop(), stack.Pop()

		n := &TernaryOp{X: x, Y: y, Z: z}
		n.OpCode = opcode
		n.Val = t.StackPost.Pop()

		stack.Push(n)
		return nil

	case vm.CALLDATACOPY:
		memOffset, dataOffset, length := stack.Pop(), stack.Pop(), stack.Pop()
		_, _, _ = memOffset, dataOffset, length

		return nil

	case vm.POP:
		stack.Pop()
		return nil

	case vm.SHA3:
		_, _ = stack.Pop(), stack.Pop()

		vmOffset, vmSize := t.StackPre.PeekI(0).Uint64(), t.StackPre.PeekI(1).Uint64()

		sha3 := &Sha3{
			Offset:    vmOffset,
			Size:      vmSize,
			ValueNode: ValueNode{t.StackPost.Pop()},
		}
		// All MemoryWrite that inside region
		//   [vmOffset : vmOffset+vmSize]
		// are considered as input to the sha3 calculation
		for ofst, mem := range call.MemMap {
			if ofst >= vmOffset && ofst < (vmOffset+vmSize) {
				sha3.Input = append(sha3.Input, mem)
			}
		}

		stack.Push(sha3)
		call.AddTrace(&Sha3Calc{
			Sha3: sha3,
		})

		return nil
	case vm.MLOAD:
		_ = stack.Pop()

		offset := t.StackPre.Peek().Uint64()
		mem, ok := call.MemMap[offset]
		if !ok {
			// Op code "RETURN" copies a large chunk of memory
			// Maybe this `offset` is not the begining of it, but inside the region
			// So find which block contains this offset
			for _, m := range call.MemMap {
				if offset >= m.VmOffset && (offset-m.VmOffset+32 <= uint64(len(m.VmBytes))) {
					// Build a new *Memory, increase the "offset"
					// eg: for offset==0x10
					//   old: Memory[0x20]
					//   new: Memory[0x20 + 0x10]
					mem = &Memory{
						Offset: &BinaryOp{ // old offset + new offet
							OpNode: OpNode{vm.ADD},
							X:      m.Offset,
							Y:      NewConst(uint256.NewInt(offset - m.VmOffset)),
						},
						Val:      m.Val,
						VmOffset: offset,
						VmBytes:  m.VmBytes[offset-m.VmOffset:],
					}
					call.MemMap[offset] = mem
				}
			}
		}
		// if still not found, create a new *Memory
		if mem == nil {
			mem = &Memory{
				Offset:   NewConst(uint256.NewInt(offset)),
				Val:      &Label{"Unknown_Memory"},
				VmOffset: offset,
			}
			if offset < uint64(len(t.MemPre)) {
				mem.VmBytes = t.MemPre[offset : offset+32]
			}
		}

		stack.Push(mem)

		return nil
	case vm.MSTORE, vm.MSTORE8:
		offset, val := stack.Pop(), stack.Pop()
		vmOffset := t.StackPre.Peek().Uint64()
		vmSize := 32
		if opcode == vm.MSTORE8 {
			vmSize = 1
		}

		mem := &Memory{
			Offset:   offset,
			Val:      val,
			VmOffset: vmOffset,
			VmBytes: vmCall.Memory.GetCopy(
				int64(vmOffset),
				int64(vmSize),
			),
		}
		call.MemMap[vmOffset] = mem
		call.AddTrace(&MemoryWrite{
			Memory: mem,
		})
		return nil

	case vm.SLOAD:
		slot := stack.Pop()

		vmSlot := t.StackPre.Pop()

		sto, ok := call.StorageMap[vmSlot]
		if ok {
			stack.Push(sto)
		} else {
			// If it not exists in cache,
			// then the value is got from online server
			// so it's actually a StorageWrite
			sto := &Storage{
				Slot: slot,
				Val:  NewConst(t.StackPost.Peek()),
			}
			call.StorageMap[vmSlot] = sto
			stack.Push(sto)
			call.AddTrace(&StorageWrite{
				IsGetOnline: true,
				Storage:     sto,
			})
		}

		return nil
	case vm.SSTORE:
		slot, val := stack.Pop(), stack.Pop()
		vmSlot := t.StackPre.Pop()

		sto := &Storage{Slot: slot, Val: val}

		call.StorageMap[vmSlot] = sto
		call.AddTrace(&StorageWrite{
			Storage: sto,
		})
		return nil

	case vm.JUMP:
		stack.Pop()
		return nil
	case vm.JUMPI:
		_, cond := stack.Pop(), stack.Pop()
		vmCond := t.StackPre.PeekI(1)

		n := &If{Cond: cond, Taken: !vmCond.IsZero()}
		call.AddTrace(n)
		return nil

	case vm.JUMPDEST:
		return nil

	case vm.PUSH1, vm.PUSH2, vm.PUSH3, vm.PUSH4, vm.PUSH5, vm.PUSH6, vm.PUSH7, vm.PUSH8, vm.PUSH9, vm.PUSH10, vm.PUSH11, vm.PUSH12, vm.PUSH13, vm.PUSH14, vm.PUSH15, vm.PUSH16, vm.PUSH17, vm.PUSH18, vm.PUSH19, vm.PUSH20, vm.PUSH21, vm.PUSH22, vm.PUSH23, vm.PUSH24, vm.PUSH25, vm.PUSH26, vm.PUSH27, vm.PUSH28, vm.PUSH29, vm.PUSH30, vm.PUSH31, vm.PUSH32:
		n := &Const{}
		n.Val = t.StackPost.Pop()
		stack.Push(n)
		return nil

	case vm.DUP1, vm.DUP2, vm.DUP3, vm.DUP4, vm.DUP5, vm.DUP6, vm.DUP7, vm.DUP8, vm.DUP9, vm.DUP10, vm.DUP11, vm.DUP12, vm.DUP13, vm.DUP14, vm.DUP15, vm.DUP16:
		size := line.Op.OpCode - vm.DUP1 + 1
		stack.Dup(int(size))
		return nil
	case vm.SWAP1, vm.SWAP2, vm.SWAP3, vm.SWAP4, vm.SWAP5, vm.SWAP6, vm.SWAP7, vm.SWAP8, vm.SWAP9, vm.SWAP10, vm.SWAP11, vm.SWAP12, vm.SWAP13, vm.SWAP14, vm.SWAP15, vm.SWAP16:
		size := opcode - vm.SWAP1 + 1
		size++
		stack.Swap(int(size))
		return nil

	case vm.LOG0, vm.LOG1, vm.LOG2, vm.LOG3, vm.LOG4:
		vmStart, vmSize := t.StackPre.PeekI(0).Uint64(), t.StackPre.PeekI(1).Uint64()
		_, _ = stack.Pop(), stack.Pop()
		topicCount := opcode - vm.LOG0

		n := &Log{}
		for i := 0; i < int(topicCount); i++ {
			n.Topics = append(n.Topics, stack.Pop())
		}
		// all MemoryWrite inside [vmStart: vmStart+vmSize]
		for ofst, mem := range call.MemMap {
			if ofst >= vmStart && ofst <= (vmStart+vmSize) {
				n.Mem = append(n.Mem, mem)
			}
		}
		call.AddTrace(n)
		return nil

	case vm.CALL, vm.DELEGATECALL, vm.STATICCALL:
		_, _, _, _, _, _ = stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop()

		newVmCall := t.ctx.Call()

		addr := common.Address(t.StackPre.PeekI(1).Bytes20())
		if opcode == vm.CALL {
			stack.Pop() // one more param for CALL

			// check if it's transfer by checking "inSize == 0"
			if inSize := t.StackPre.PeekI(4); inSize.IsZero() { //
				call.AddTrace(&MoneyTransfer{
					To:     addr,
					Amount: *t.StackPre.PeekI(2),
				})
				// just assume it succeeded
				stack.Push(NewConst(uint256.NewInt(1)))
				return nil
			}
		}

		// check if it's precompiled
		if _, ok := vm.PrecompiledContractsBerlin[addr]; ok {

			inOffset := t.StackPre.PeekI(2).Uint64()
			inMem := call.MemMap[inOffset]
			call.AddTrace(&Precompiled{To: addr, Input: inMem})

			retOffset, retSize := t.StackPre.PeekI(4).Uint64(), t.StackPre.PeekI(5).Uint64()

			retVal := &ReturnValue{}
			mem := &Memory{
				Offset:   NewConst(uint256.NewInt(retOffset)),
				Val:      retVal,
				VmOffset: retOffset,
				VmBytes:  util.CloneSlice(t.MemPost[retOffset : retOffset+retSize]),
			}
			call.MemMap[retOffset] = mem
			call.AddTrace(&Return{
				ReturnValue: retVal,
				Memory:      mem,
			})
			stack.Push(NewConst(uint256.NewInt(1)))
			return nil
		}

		toAddr := newVmCall.CodeAddress()
		newCall := NewCall(opcode, &toAddr, newVmCall.Msg.Data)
		t.CallStack.Push(newCall)
		call.AddTrace(newCall)
		return nil

	case vm.STOP:
		if t.CallStack.Len() > 1 {
			t.CallStack.Pop()

			// Get it again becauses above `CallStack.Pop()`
			call = *t.CallStack.Peek()
			stack_ := &call.Stack
			stack_.Push(NewConst(uint256.NewInt(1)))
		}
		return nil

	case vm.RETURN:
		_, _ = stack.Pop(), stack.Pop()
		if t.CallStack.Len() == 1 { // return from main call
			return nil
		}
		t.CallStack.Pop()

		// Get it again becauses above `CallStack.Pop()`
		call = *t.CallStack.Peek()
		stack_ := &call.Stack

		// copy return value to outer Call.Memory
		offset, size := vmCall.OuterReturnOffset, vmCall.OuterReturnSize

		newVmCall := t.ctx.Call()

		mem := &Memory{
			Offset:   NewConst(uint256.NewInt(offset)),
			Val:      &ReturnValue{},
			VmOffset: offset,
			VmBytes: newVmCall.Memory.GetCopy(
				int64(offset),
				int64(size),
			),
		}
		call.MemMap[offset] = mem
		call.AddTrace(&MemoryWrite{
			Memory: mem,
		})

		stack_.Push(NewConst(uint256.NewInt(1)))
		return nil

	case vm.RETURNDATASIZE:
		stack.Push(&Label{"ReturnDataSize"})
		return nil
	case vm.RETURNDATACOPY, vm.CODECOPY:
		_, _, _ = stack.Pop(), stack.Pop(), stack.Pop()

		memOffset, dataOffset, length :=
			t.StackPre.PeekI(0).Uint64(), t.StackPre.PeekI(1).Uint64(), t.StackPre.PeekI(2).Uint64()
		mem := &Memory{
			Offset:   NewConst(uint256.NewInt(memOffset)),
			VmOffset: memOffset,
			VmBytes:  t.MemPost[memOffset : memOffset+length],
		}

		switch opcode {
		case vm.RETURNDATACOPY:
			mem.Val = &Label{"ReturnValue"}
		case vm.CODECOPY:
			mem.Val = &Label{"CodeCopy"}
		}

		call.MemMap[dataOffset] = mem

		call.AddTrace(&MemoryWrite{
			Memory: mem,
		})

		return nil
	case vm.EXTCODECOPY:

	case vm.REVERT:
		call.AddTrace(&Label{"Reverted"})
		return nil
	case vm.SELFDESTRUCT:
	case vm.CREATE:
	case vm.CREATE2:
	case vm.CALLCODE:
	}

	return errors.Wrap(TODO, opcode.String())
}
