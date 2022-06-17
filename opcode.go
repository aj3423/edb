package edb

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/aj3423/edb/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/fatih/color"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
)

type executionFunc func(*Context) error

type Operation struct {
	OpCode    vm.OpCode
	OpSize    uint64 // size of required data, eg: opSize for push3 == 3
	GasCost   gasFunc
	Exec      executionFunc
	NStackIn  uint8 // count of args that popup from stack
	NStackOut uint8 // count of new items pushed to stack
}

func make_op(
	opCode vm.OpCode,
	opSize uint64,
	gasCost gasFunc,
	nStackIn uint8,
	nStackOut uint8,
	exec executionFunc,
) *Operation {
	return &Operation{
		OpCode:    opCode,
		OpSize:    opSize,
		GasCost:   gasCost,
		NStackIn:  nStackIn,
		NStackOut: nStackOut,
		Exec:      exec,
	}
}

var OpTable map[vm.OpCode]*Operation

func init() {
	OpTable = map[vm.OpCode]*Operation{
		vm.STOP:           make_op(vm.STOP, 0, fixedGas(0), 0, 0, opStop),                           // 0x0
		vm.ADD:            make_op(vm.ADD, 0, fixedGas(3), 2, 1, opAdd),                             // 0x1
		vm.MUL:            make_op(vm.MUL, 0, fixedGas(5), 2, 1, opMul),                             // 0x2
		vm.SUB:            make_op(vm.SUB, 0, fixedGas(3), 2, 1, opSub),                             // 0x3
		vm.DIV:            make_op(vm.DIV, 0, fixedGas(5), 2, 1, opDiv),                             // 0x4
		vm.SDIV:           make_op(vm.SDIV, 0, fixedGas(5), 2, 1, opSdiv),                           // 0x5
		vm.MOD:            make_op(vm.MOD, 0, fixedGas(5), 2, 1, opMod),                             // 0x6
		vm.SMOD:           make_op(vm.SMOD, 0, fixedGas(5), 2, 1, opSmod),                           // 0x7
		vm.ADDMOD:         make_op(vm.ADDMOD, 0, fixedGas(8), 2, 1, opAddmod),                       // 0x8
		vm.MULMOD:         make_op(vm.MULMOD, 0, fixedGas(8), 2, 1, opMulmod),                       // 0x9
		vm.EXP:            make_op(vm.EXP, 0, gasExp, 2, 1, opExp),                                  // 0xa
		vm.SIGNEXTEND:     make_op(vm.SIGNEXTEND, 0, fixedGas(5), 2, 1, opSignExtend),               // 0xb
		vm.LT:             make_op(vm.LT, 0, fixedGas(3), 2, 1, opLt),                               // 0x10
		vm.GT:             make_op(vm.GT, 0, fixedGas(3), 2, 1, opGt),                               // 0x11
		vm.SLT:            make_op(vm.SLT, 0, fixedGas(3), 2, 1, opSlt),                             // 0x12
		vm.SGT:            make_op(vm.SGT, 0, fixedGas(3), 2, 1, opSgt),                             // 0x13
		vm.EQ:             make_op(vm.EQ, 0, fixedGas(3), 2, 1, opEq),                               // 0x14
		vm.ISZERO:         make_op(vm.ISZERO, 0, fixedGas(3), 1, 1, opIszero),                       // 0x15
		vm.AND:            make_op(vm.AND, 0, fixedGas(3), 2, 1, opAnd),                             // 0x16
		vm.OR:             make_op(vm.OR, 0, fixedGas(3), 2, 1, opOr),                               // 0x17
		vm.XOR:            make_op(vm.XOR, 0, fixedGas(3), 2, 1, opXor),                             // 0x18
		vm.NOT:            make_op(vm.NOT, 0, fixedGas(3), 1, 1, opNot),                             // 0x19
		vm.BYTE:           make_op(vm.BYTE, 0, fixedGas(3), 2, 1, opByte),                           // 0x1a
		vm.SHL:            make_op(vm.SHL, 0, fixedGas(3), 2, 1, opSHL),                             // 0x1b
		vm.SHR:            make_op(vm.SHR, 0, fixedGas(3), 2, 1, opSHR),                             // 0x1c
		vm.SAR:            make_op(vm.SAR, 0, fixedGas(3), 2, 1, opSAR),                             // 0x1d
		vm.SHA3:           make_op(vm.SHA3, 0, gasSha3, 2, 1, opSha3),                               // 0x20
		vm.ADDRESS:        make_op(vm.ADDRESS, 0, fixedGas(2), 0, 1, opAddress),                     // 0x30
		vm.BALANCE:        make_op(vm.BALANCE, 0, fixedGas(20), 1, 1, opBalance),                    // 0x31
		vm.ORIGIN:         make_op(vm.ORIGIN, 0, fixedGas(2), 0, 1, opOrigin),                       // 0x32
		vm.CALLER:         make_op(vm.CALLER, 0, fixedGas(2), 0, 1, opCaller),                       // 0x33
		vm.CALLVALUE:      make_op(vm.CALLVALUE, 0, fixedGas(2), 0, 1, opCallValue),                 // 0x34
		vm.CALLDATALOAD:   make_op(vm.CALLDATALOAD, 0, fixedGas(3), 0, 1, opCallDataLoad),           // 0x35
		vm.CALLDATASIZE:   make_op(vm.CALLDATASIZE, 0, fixedGas(2), 0, 1, opCallDataSize),           // 0x36
		vm.CALLDATACOPY:   make_op(vm.CALLDATACOPY, 0, gasCallDataCopy, 3, 0, opCallDataCopy),       // 0x37
		vm.CODESIZE:       make_op(vm.CODESIZE, 0, fixedGas(2), 0, 1, opCodeSize),                   // 0x38
		vm.CODECOPY:       make_op(vm.CODECOPY, 0, gasCodeCopy, 3, 0, opCodeCopy),                   // 0x39
		vm.GASPRICE:       make_op(vm.GASPRICE, 0, fixedGas(2), 0, 1, opGasprice),                   // 0x3a
		vm.EXTCODESIZE:    make_op(vm.EXTCODESIZE, 0, fixedGas(700), 0, 1, opExtCodeSize),           // 0x3b
		vm.EXTCODECOPY:    make_op(vm.EXTCODECOPY, 0, gasExtCodeCopy, 4, 0, opExtCodeCopy),          // 0x3c
		vm.RETURNDATASIZE: make_op(vm.RETURNDATASIZE, 0, fixedGas(2), 0, 1, opReturnDataSize),       // 0x3d
		vm.RETURNDATACOPY: make_op(vm.RETURNDATACOPY, 0, gasReturnDataCopy, 3, 0, opReturnDataCopy), // 0x3e
		vm.EXTCODEHASH:    make_op(vm.EXTCODEHASH, 0, fixedGas(700), 1, 1, opExtCodeHash),           // 0x3f
		vm.BLOCKHASH:      make_op(vm.BLOCKHASH, 0, fixedGas(20), 1, 1, opBlockhash),                // 0x40
		vm.COINBASE:       make_op(vm.COINBASE, 0, fixedGas(2), 0, 1, opCoinbase),                   // 0x41
		vm.TIMESTAMP:      make_op(vm.TIMESTAMP, 0, fixedGas(2), 0, 1, opTimestamp),                 // 0x42
		vm.NUMBER:         make_op(vm.NUMBER, 0, fixedGas(2), 0, 1, opNumber),                       // 0x43
		vm.DIFFICULTY:     make_op(vm.DIFFICULTY, 0, fixedGas(2), 0, 1, opDifficulty),               // 0x44
		vm.GASLIMIT:       make_op(vm.GASLIMIT, 0, fixedGas(2), 0, 1, opGasLimit),                   // 0x45
		vm.CHAINID:        make_op(vm.CHAINID, 0, fixedGas(2), 0, 1, opChainID),                     // 0x46
		vm.SELFBALANCE:    make_op(vm.SELFBALANCE, 0, fixedGas(5), 0, 1, opSelfBalance),             // 0x47
		vm.BASEFEE:        make_op(vm.BASEFEE, 0, fixedGas(2), 0, 1, opBaseFee),                     // 0x48
		vm.POP:            make_op(vm.POP, 0, fixedGas(2), 1, 0, opPop),                             // 0x50
		vm.MLOAD:          make_op(vm.MLOAD, 0, fixedGas(3), 1, 1, opMload),                         // 0x51
		vm.MSTORE:         make_op(vm.MSTORE, 0, fixedGas(3), 2, 0, opMstore),                       // 0x52
		vm.MSTORE8:        make_op(vm.MSTORE8, 0, fixedGas(3), 2, 0, opMstore8),                     // 0x53
		vm.SLOAD:          make_op(vm.SLOAD, 0, fixedGas(800), 1, 1, opSload),                       // 0x54
		vm.SSTORE:         make_op(vm.SSTORE, 0, gasSStore, 2, 0, opSstore),                         // 0x55
		vm.JUMP:           make_op(vm.JUMP, 0, fixedGas(8), 1, 0, opJump),                           // 0x56
		vm.JUMPI:          make_op(vm.JUMPI, 0, fixedGas(10), 2, 0, opJumpi),                        // 0x57
		vm.PC:             make_op(vm.PC, 0, fixedGas(2), 0, 1, opPc),                               // 0x58
		vm.MSIZE:          make_op(vm.MSIZE, 0, fixedGas(2), 0, 1, opMsize),                         // 0x59
		vm.GAS:            make_op(vm.GAS, 0, fixedGas(2), 0, 1, opGas),                             // 0x5a
		vm.JUMPDEST:       make_op(vm.JUMPDEST, 0, fixedGas(1), 0, 0, opJumpdest),                   // 0x5b
		vm.PUSH1:          make_op(vm.PUSH1, 1, fixedGas(3), 0, 1, makePush(1)),                     // 0x60
		vm.PUSH2:          make_op(vm.PUSH2, 2, fixedGas(3), 0, 1, makePush(2)),                     // 0x61
		vm.PUSH3:          make_op(vm.PUSH3, 3, fixedGas(3), 0, 1, makePush(3)),                     // 0x62
		vm.PUSH4:          make_op(vm.PUSH4, 4, fixedGas(3), 0, 1, makePush(4)),                     // 0x63
		vm.PUSH5:          make_op(vm.PUSH5, 5, fixedGas(3), 0, 1, makePush(5)),                     // 0x64
		vm.PUSH6:          make_op(vm.PUSH6, 6, fixedGas(3), 0, 1, makePush(6)),                     // 0x65
		vm.PUSH7:          make_op(vm.PUSH7, 7, fixedGas(3), 0, 1, makePush(7)),                     // 0x66
		vm.PUSH8:          make_op(vm.PUSH8, 8, fixedGas(3), 0, 1, makePush(8)),                     // 0x67
		vm.PUSH9:          make_op(vm.PUSH9, 9, fixedGas(3), 0, 1, makePush(9)),                     // 0x68
		vm.PUSH10:         make_op(vm.PUSH10, 10, fixedGas(3), 0, 1, makePush(10)),                  // 0x69
		vm.PUSH11:         make_op(vm.PUSH11, 11, fixedGas(3), 0, 1, makePush(11)),                  // 0x6a
		vm.PUSH12:         make_op(vm.PUSH12, 12, fixedGas(3), 0, 1, makePush(12)),                  // 0x6b
		vm.PUSH13:         make_op(vm.PUSH13, 13, fixedGas(3), 0, 1, makePush(13)),                  // 0x6c
		vm.PUSH14:         make_op(vm.PUSH14, 14, fixedGas(3), 0, 1, makePush(14)),                  // 0x6d
		vm.PUSH15:         make_op(vm.PUSH15, 15, fixedGas(3), 0, 1, makePush(15)),                  // 0x6e
		vm.PUSH16:         make_op(vm.PUSH16, 16, fixedGas(3), 0, 1, makePush(16)),                  // 0x6f
		vm.PUSH17:         make_op(vm.PUSH17, 17, fixedGas(3), 0, 1, makePush(17)),                  // 0x70
		vm.PUSH18:         make_op(vm.PUSH18, 18, fixedGas(3), 0, 1, makePush(18)),                  // 0x71
		vm.PUSH19:         make_op(vm.PUSH19, 19, fixedGas(3), 0, 1, makePush(19)),                  // 0x72
		vm.PUSH20:         make_op(vm.PUSH20, 20, fixedGas(3), 0, 1, makePush(20)),                  // 0x73
		vm.PUSH21:         make_op(vm.PUSH21, 21, fixedGas(3), 0, 1, makePush(21)),                  // 0x74
		vm.PUSH22:         make_op(vm.PUSH22, 22, fixedGas(3), 0, 1, makePush(22)),                  // 0x75
		vm.PUSH23:         make_op(vm.PUSH23, 23, fixedGas(3), 0, 1, makePush(23)),                  // 0x76
		vm.PUSH24:         make_op(vm.PUSH24, 24, fixedGas(3), 0, 1, makePush(24)),                  // 0x77
		vm.PUSH25:         make_op(vm.PUSH25, 25, fixedGas(3), 0, 1, makePush(25)),                  // 0x78
		vm.PUSH26:         make_op(vm.PUSH26, 26, fixedGas(3), 0, 1, makePush(26)),                  // 0x79
		vm.PUSH27:         make_op(vm.PUSH27, 27, fixedGas(3), 0, 1, makePush(27)),                  // 0x7a
		vm.PUSH28:         make_op(vm.PUSH28, 28, fixedGas(3), 0, 1, makePush(28)),                  // 0x7b
		vm.PUSH29:         make_op(vm.PUSH29, 29, fixedGas(3), 0, 1, makePush(29)),                  // 0x7c
		vm.PUSH30:         make_op(vm.PUSH30, 30, fixedGas(3), 0, 1, makePush(30)),                  // 0x7d
		vm.PUSH31:         make_op(vm.PUSH31, 31, fixedGas(3), 0, 1, makePush(31)),                  // 0x7e
		vm.PUSH32:         make_op(vm.PUSH32, 32, fixedGas(3), 0, 1, makePush(32)),                  // 0x7f
		vm.DUP1:           make_op(vm.DUP1, 0, fixedGas(3), 0, 1, makeDup(1)),                       // 0x80
		vm.DUP2:           make_op(vm.DUP2, 0, fixedGas(3), 0, 1, makeDup(2)),                       // 0x81
		vm.DUP3:           make_op(vm.DUP3, 0, fixedGas(3), 0, 1, makeDup(3)),                       // 0x82
		vm.DUP4:           make_op(vm.DUP4, 0, fixedGas(3), 0, 1, makeDup(4)),                       // 0x83
		vm.DUP5:           make_op(vm.DUP5, 0, fixedGas(3), 0, 1, makeDup(5)),                       // 0x84
		vm.DUP6:           make_op(vm.DUP6, 0, fixedGas(3), 0, 1, makeDup(6)),                       // 0x85
		vm.DUP7:           make_op(vm.DUP7, 0, fixedGas(3), 0, 1, makeDup(7)),                       // 0x86
		vm.DUP8:           make_op(vm.DUP8, 0, fixedGas(3), 0, 1, makeDup(8)),                       // 0x87
		vm.DUP9:           make_op(vm.DUP9, 0, fixedGas(3), 0, 1, makeDup(9)),                       // 0x88
		vm.DUP10:          make_op(vm.DUP10, 0, fixedGas(3), 0, 1, makeDup(10)),                     // 0x89
		vm.DUP11:          make_op(vm.DUP11, 0, fixedGas(3), 0, 1, makeDup(11)),                     // 0x8a
		vm.DUP12:          make_op(vm.DUP12, 0, fixedGas(3), 0, 1, makeDup(12)),                     // 0x8b
		vm.DUP13:          make_op(vm.DUP13, 0, fixedGas(3), 0, 1, makeDup(13)),                     // 0x8c
		vm.DUP14:          make_op(vm.DUP14, 0, fixedGas(3), 0, 1, makeDup(14)),                     // 0x8d
		vm.DUP15:          make_op(vm.DUP15, 0, fixedGas(3), 0, 1, makeDup(15)),                     // 0x8e
		vm.DUP16:          make_op(vm.DUP16, 0, fixedGas(3), 0, 1, makeDup(16)),                     // 0x8f
		vm.SWAP1:          make_op(vm.SWAP1, 0, fixedGas(3), 0, 0, makeSwap(1)),                     // 0x90
		vm.SWAP2:          make_op(vm.SWAP2, 0, fixedGas(3), 0, 0, makeSwap(2)),                     // 0x91
		vm.SWAP3:          make_op(vm.SWAP3, 0, fixedGas(3), 0, 0, makeSwap(3)),                     // 0x92
		vm.SWAP4:          make_op(vm.SWAP4, 0, fixedGas(3), 0, 0, makeSwap(4)),                     // 0x93
		vm.SWAP5:          make_op(vm.SWAP5, 0, fixedGas(3), 0, 0, makeSwap(5)),                     // 0x94
		vm.SWAP6:          make_op(vm.SWAP6, 0, fixedGas(3), 0, 0, makeSwap(6)),                     // 0x95
		vm.SWAP7:          make_op(vm.SWAP7, 0, fixedGas(3), 0, 0, makeSwap(7)),                     // 0x96
		vm.SWAP8:          make_op(vm.SWAP8, 0, fixedGas(3), 0, 0, makeSwap(8)),                     // 0x97
		vm.SWAP9:          make_op(vm.SWAP9, 0, fixedGas(3), 0, 0, makeSwap(9)),                     // 0x98
		vm.SWAP10:         make_op(vm.SWAP10, 0, fixedGas(3), 0, 0, makeSwap(10)),                   // 0x99
		vm.SWAP11:         make_op(vm.SWAP11, 0, fixedGas(3), 0, 0, makeSwap(11)),                   // 0x9a
		vm.SWAP12:         make_op(vm.SWAP12, 0, fixedGas(3), 0, 0, makeSwap(12)),                   // 0x9b
		vm.SWAP13:         make_op(vm.SWAP13, 0, fixedGas(3), 0, 0, makeSwap(13)),                   // 0x9c
		vm.SWAP14:         make_op(vm.SWAP14, 0, fixedGas(3), 0, 0, makeSwap(14)),                   // 0x9d
		vm.SWAP15:         make_op(vm.SWAP15, 0, fixedGas(3), 0, 0, makeSwap(15)),                   // 0x9e
		vm.SWAP16:         make_op(vm.SWAP16, 0, fixedGas(3), 0, 0, makeSwap(16)),                   // 0x9f
		vm.LOG0:           make_op(vm.LOG0, 0, makeGasLog(0), 2+0, 0, makeLog(0)),                   // 0xa0
		vm.LOG1:           make_op(vm.LOG1, 0, makeGasLog(1), 2+1, 0, makeLog(1)),                   // 0xa1
		vm.LOG2:           make_op(vm.LOG2, 0, makeGasLog(2), 2+2, 0, makeLog(2)),                   // 0xa2
		vm.LOG3:           make_op(vm.LOG3, 0, makeGasLog(3), 2+3, 0, makeLog(3)),                   // 0xa3
		vm.LOG4:           make_op(vm.LOG4, 0, makeGasLog(4), 2+4, 0, makeLog(4)),                   // 0xa4
		vm.CREATE:         make_op(vm.CREATE, 0, gasTodo, 0, 0, opCreate),                           // 0xf0
		vm.CALL:           make_op(vm.CALL, 0, gasTodo, 7, 0, opCall),                               // 0xf1
		vm.CALLCODE:       make_op(vm.CALLCODE, 0, gasTodo, 0, 0, opCallCode),                       // 0xf2
		vm.RETURN:         make_op(vm.RETURN, 0, fixedGas(0), 2, 0, opReturn),                       // 0xf3
		vm.DELEGATECALL:   make_op(vm.DELEGATECALL, 0, gasTodo, 6, 0, opDelegateCall),               // 0xf4
		vm.CREATE2:        make_op(vm.CREATE2, 0, gasTodo, 0, 0, opCreate2),                         // 0xf5
		vm.STATICCALL:     make_op(vm.STATICCALL, 0, gasTodo, 6, 0, opStaticCall),                   // 0xfa
		vm.REVERT:         make_op(vm.REVERT, 0, fixedGas(0), 2, 0, opRevert),                       // 0xfd
		vm.OpCode(0xfe):   make_op(vm.OpCode(0xfe), 0, gasTodo, 1, 0, opAssert),                     // 0xfe
		vm.SELFDESTRUCT:   make_op(vm.SELFDESTRUCT, 0, gasTodo, 1, 0, opSuicide),                    // 0xff
	}
}

func opInvalid(ctx *Context) error {
	return errors.New("invalid op")
	//return nil
}

func opAdd(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.Add(&x, y)
	return nil
}

func opSub(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.Sub(&x, y)
	return nil
}

func opMul(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.Mul(&x, y)
	return nil
}

func opDiv(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.Div(&x, y)
	return nil
}

func opSdiv(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.SDiv(&x, y)
	return nil
}

func opMod(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.Mod(&x, y)
	return nil
}

func opSmod(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.SMod(&x, y)
	return nil
}

func opExp(ctx *Context) error {
	stack := ctx.Stack()
	base, exponent := stack.Pop(), stack.Peek()
	exponent.Exp(&base, exponent)
	return nil
}

// b, x ->  y = SIGNEXTEND(x, b)
//sign extends x from (b + 1) * 8 bits to 256 bits.
func opSignExtend(ctx *Context) error {
	stack := ctx.Stack()
	back, num := stack.Pop(), stack.Peek()
	num.ExtendSign(num, &back)
	return nil
}

func opNot(ctx *Context) error {
	x := ctx.Stack().Peek()
	x.Not(x)
	return nil
}

func opLt(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	if x.Lt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil
}

func opGt(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	if x.Gt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil
}

func opSlt(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	if x.Slt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil
}

func opSgt(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	if x.Sgt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil
}

func opEq(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	if x.Eq(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil
}

func opIszero(ctx *Context) error {
	x := ctx.Stack().Peek()
	if x.IsZero() {
		x.SetOne()
	} else {
		x.Clear()
	}
	return nil
}

func opAnd(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.And(&x, y)
	return nil
}

func opOr(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.Or(&x, y)
	return nil
}

func opXor(ctx *Context) error {
	stack := ctx.Stack()
	x, y := stack.Pop(), stack.Peek()
	y.Xor(&x, y)
	return nil
}

// i'th byte of (u)int256 x, counting from most significant byte
func opByte(ctx *Context) error {
	stack := ctx.Stack()
	th, val := stack.Pop(), stack.Peek()
	val.Byte(&th)
	return nil
}

func opAddmod(ctx *Context) error {
	stack := ctx.Stack()
	x, y, z := stack.Pop(), stack.Pop(), stack.Peek()
	if z.IsZero() {
		z.Clear()
	} else {
		z.AddMod(&x, &y, z)
	}
	return nil
}

func opMulmod(ctx *Context) error {
	stack := ctx.Stack()
	x, y, z := stack.Pop(), stack.Pop(), stack.Peek()
	z.MulMod(&x, &y, z)
	return nil
}

// opSHL implements Shift Left
// The SHL instruction (shift left) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the left by arg1 number of bits.
func opSHL(ctx *Context) error {
	stack := ctx.Stack()
	// Note, second operand is left in the stack; accumulate result into it, and no need to push it afterwards
	shift, value := stack.Pop(), stack.Peek()
	if shift.LtUint64(256) {
		value.Lsh(value, uint(shift.Uint64()))
	} else {
		value.Clear()
	}
	return nil
}

// opSHR implements Logical Shift Right
// The SHR instruction (logical shift right) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the right by arg1 number of bits with zero fill.
func opSHR(ctx *Context) error {
	stack := ctx.Stack()
	// Note, second operand is left in the stack; accumulate result into it, and no need to push it afterwards
	shift, value := stack.Pop(), stack.Peek()
	if shift.LtUint64(256) {
		value.Rsh(value, uint(shift.Uint64()))
	} else {
		value.Clear()
	}
	return nil
}

// opSAR implements Arithmetic Shift Right
// The SAR instruction (arithmetic shift right) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the right by arg1 number of bits with sign extension.
func opSAR(ctx *Context) error {
	stack := ctx.Stack()
	shift, value := stack.Pop(), stack.Peek()
	if shift.GtUint64(256) {
		if value.Sign() >= 0 {
			value.Clear()
		} else {
			// Max negative shift: all bits set
			value.SetAllOne()
		}
		return nil
	}
	n := uint(shift.Uint64())
	value.SRsh(value, n)
	return nil
}

func opSha3(ctx *Context) error {
	stack := ctx.Stack()
	offset, size := stack.Pop(), stack.Peek()
	data := ctx.Memory().GetPtr(int64(offset.Uint64()), int64(size.Uint64()))

	bs := util.Sha3(data)

	size.SetBytes(bs)
	return nil
}
func opAddress(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetBytes(ctx.This().Bytes()))
	return nil
}

func opBalance(ctx *Context) error {
	// TODO get online if not set in json
	return errors.New("TODO opBalance")
	// slot := ctx.Stack().Peek()
	// address := common.Address(slot.Bytes20())
	// bal := ensure_contract_at(ctx, address).Balance
	// slot.SetFromBig(bal)
	// return nil
}

func opOrigin(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetBytes(ctx.Tx.Origin.Bytes()))
	return nil
}
func opCaller(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetBytes(ctx.Msg().Sender.Bytes()))
	return nil
}

func opCallValue(ctx *Context) error {
	v, _ := uint256.FromBig(ctx.Msg().Value)
	ctx.Stack().Push(*v)
	return nil
}

// getData returns a slice from the data based on the start and size and pads
// up to size with zero's. This function is overflow safe.
func getData(data []byte, start uint64, size uint64) []byte {
	length := uint64(len(data))
	if start > length {
		start = length
	}
	end := start + size
	if end > length {
		end = length
	}
	return common.RightPadBytes(data[start:end], int(size))
}

// reads a (u)int256 from message data
//   msg.data[i:i+32]
func opCallDataLoad(ctx *Context) error {
	off := ctx.Stack().Peek()
	if offset, overflow := off.Uint64WithOverflow(); !overflow {
		data := getData(ctx.Msg().Data, offset, 32)
		off.SetBytes(data)
	} else {
		off.Clear()
	}
	return nil
}

func opCallDataSize(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetUint64(uint64(len(ctx.Msg().Data))))
	return nil
}

func opCallDataCopy(ctx *Context) error {
	stack := ctx.Stack()
	var (
		memOffset  = stack.Pop()
		dataOffset = stack.Pop()
		length     = stack.Pop()
	)
	dataOffset64, overflow := dataOffset.Uint64WithOverflow()
	if overflow {
		dataOffset64 = 0xffffffffffffffff
	}
	// These values are checked for overflow during gas cost calculation
	memOffset64 := memOffset.Uint64()
	length64 := length.Uint64()
	ctx.Memory().Set(memOffset64, length64, getData(ctx.Msg().Data, dataOffset64, length64))

	return nil
}

func opReturnDataSize(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetUint64(
		uint64(len(ctx.Call().InnerReturnVal))))
	return nil
}

// memory[memOffset : memOffset+length] =
//     RETURNDATA[dataOffset : dataOffset+length]
func opReturnDataCopy(ctx *Context) error {
	stack := ctx.Stack()
	var (
		memOffset  = stack.Pop()
		dataOffset = stack.Pop()
		length     = stack.Pop()
	)

	offset64, overflow := dataOffset.Uint64WithOverflow()
	if overflow {
		return vm.ErrReturnDataOutOfBounds
	}
	// we can reuse dataOffset now (aliasing it for clarity)
	var end = dataOffset
	end.Add(&dataOffset, &length)
	end64, overflow := end.Uint64WithOverflow()
	if overflow || uint64(len(ctx.Call().InnerReturnVal)) < end64 {
		return vm.ErrReturnDataOutOfBounds
	}
	ctx.Memory().Set(memOffset.Uint64(), length.Uint64(), ctx.Call().InnerReturnVal[offset64:end64])
	return nil
}

func opExtCodeSize(ctx *Context) error {
	slot := ctx.Stack().Peek()
	addr := common.Address(slot.Bytes20())

	code, e := ensure_code(ctx, addr)
	if e != nil {
		return e
	}

	slot.SetUint64(uint64(len(code)))
	return nil
}

// address(this).code.size
func opCodeSize(ctx *Context) error {
	addr := ctx.This()

	code, e := ensure_code(ctx, addr)
	if e != nil {
		return e
	}

	l := new(uint256.Int)
	l.SetUint64(uint64(len(code)))
	ctx.Stack().Push(*l)
	return nil
}

func opCodeCopy(ctx *Context) error {
	stack := ctx.Stack()
	var (
		memOffset  = stack.Pop()
		codeOffset = stack.Pop()
		length     = stack.Pop()
	)
	uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
	if overflow {
		uint64CodeOffset = 0xffffffffffffffff
	}

	addr := ctx.This()
	code, e := ensure_code(ctx, addr)
	if e != nil {
		return e
	}

	codeToCopy := getData(code, uint64CodeOffset, length.Uint64())
	ctx.Memory().Set(memOffset.Uint64(), length.Uint64(), codeToCopy)

	return nil
}

func opExtCodeCopy(ctx *Context) error {
	var (
		stack = ctx.Stack()

		a          = stack.Pop()
		memOffset  = stack.Pop()
		codeOffset = stack.Pop()
		length     = stack.Pop()
	)
	uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
	if overflow {
		uint64CodeOffset = 0xffffffffffffffff
	}
	addr := common.Address(a.Bytes20())

	code, e := ensure_code(ctx, addr)
	if e != nil {
		return e
	}

	codeToCopy := getData(code, uint64CodeOffset, length.Uint64())
	ctx.Memory().Set(memOffset.Uint64(), length.Uint64(), codeToCopy)

	return nil
}

// hash = address(addr).exists ? keccak256(address(addr).code) : 0
func opExtCodeHash(ctx *Context) error {
	slot := ctx.Stack().Peek()
	addr := common.Address(slot.Bytes20())

	code, e := ensure_code(ctx, addr)
	if e != nil {
		return e
	}

	slot.SetBytes(util.Sha3(code))
	return nil
}

func opGasprice(ctx *Context) error {
	v := uint256.NewInt(ctx.Tx.GasPrice)
	ctx.Stack().Push(*v)
	return nil
}

// 	hash = block.blockHash(blockNumber)
func opBlockhash(ctx *Context) error {
	num := ctx.Stack().Peek()
	num64, overflow := num.Uint64WithOverflow()
	if overflow {
		return errors.New("block hash overflow: " + num.String())
	}

	var upper, lower uint64
	upper = ctx.Block.Number
	if upper < 257 {
		lower = 0
	} else {
		lower = upper - 256
	}
	if num64 >= lower && num64 < upper {
		hash, e := ensure_block_hash(ctx, num64)
		if e != nil {
			return e
		}
		num.SetBytes(hash.Bytes())
	} else {
		num.Clear()
	}
	return nil
}

func opCoinbase(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetBytes(ctx.Block.Coinbase.Bytes()))
	return nil
}

// block.timestamp
func opTimestamp(ctx *Context) error {
	v, _ := uint256.FromBig(big.NewInt(int64(ctx.Block.Timestamp)))
	ctx.Stack().Push(*v)
	return nil
}

// block.number
func opNumber(ctx *Context) error {
	v, _ := uint256.FromBig(big.NewInt(int64(ctx.Block.Number)))
	ctx.Stack().Push(*v)
	return nil
}

func opDifficulty(ctx *Context) error {
	v, _ := uint256.FromBig(big.NewInt(int64(ctx.Block.Difficulty)))
	ctx.Stack().Push(*v)
	return nil
}

// block.gaslimit, current block's gaslimit
func opGasLimit(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetUint64(ctx.Block.GasLimit))
	return nil
}

func opPop(ctx *Context) error {
	ctx.Stack().Pop()
	return nil
}

func opMload(ctx *Context) error {
	v := ctx.Stack().Peek()
	offset := int64(v.Uint64())
	v.SetBytes(ctx.Memory().GetPtr(offset, 32))
	return nil
}

func opMstore(ctx *Context) error {
	stack := ctx.Stack()
	// pop value of the stack
	ptr, val := stack.Pop(), stack.Pop()
	ctx.Memory().Set32(ptr.Uint64(), &val)
	return nil
}

// mstore(offset value) -> memory[offset] = value & 0xFF
func opMstore8(ctx *Context) error {
	stack := ctx.Stack()
	ptr, val := stack.Pop(), stack.Pop()
	ctx.Memory().Set(ptr.Uint64(), 1, []byte{byte(val.Uint64())})
	return nil
}

func opSload(ctx *Context) error {
	slot := ctx.Stack().Peek()

	this := ctx.Call().This

	val, e := ensure_storage(ctx, this, slot)
	if e != nil {
		return e
	}

	slot.SetBytes(val.Bytes())
	return nil
}

// sstore(key, value)
func opSstore(ctx *Context) error {
	stack := ctx.Stack()
	slot, val := stack.Pop(), stack.Pop()
	ctx.Contract().Storage[common.BigToHash(slot.ToBig())] = &val
	return nil
}

func opJump(ctx *Context) error {
	pos := ctx.Stack().Pop()
	code := ctx.Code().Binary
	codeLen := uint64(len(code))
	if pos.Uint64() >= codeLen {
		return vm.ErrInvalidJump
	}
	ctx.Call().Pc = pos.Uint64()
	return nil
}

func opJumpi(ctx *Context) error {
	stack := ctx.Stack()
	pos, cond := stack.Pop(), stack.Pop()
	if !cond.IsZero() {
		code := ctx.Code().Binary
		codeLen := uint64(len(code))
		if pos.Uint64() >= codeLen {
			return vm.ErrInvalidJump
		}
		ctx.Call().Pc = pos.Uint64()
	} else {
		ctx.Call().Pc++
	}
	return nil
}

func opJumpdest(ctx *Context) error {
	return nil
}

func opPc(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetUint64(ctx.Pc()))
	return nil
}

func opMsize(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetUint64(uint64(ctx.Memory().Len())))
	return nil
}

// gas remaining
func opGas(ctx *Context) error {
	ctx.Stack().Push(*new(uint256.Int).SetUint64(ctx.Msg().Gas))
	return nil
}

func opCreate(ctx *Context) error {
	return errors.New("TODO opCreate")
}

func opCreate2(ctx *Context) error {
	return errors.New("TODO opCreate2")
}

/*
A send tx -> B -> call(C), in C:
	msg.sender inside C is B. msg.value is passed by argument
	C.storage == C.storage, C.address(this) == C
*/

func do_opcall(
	ctx *Context,
	gas, addr, value, inOffset, inSize, retOffset, retSize uint256.Int,
) error {
	_ = gas

	toAddr := common.Address(addr.Bytes20())

	input := ctx.Memory().GetPtr(int64(inOffset.Uint64()), int64(inSize.Uint64()))

	if precompiled, ok := vm.PrecompiledContractsBerlin[toAddr]; ok {
		output, e := precompiled.Run(input)
		if e != nil {
			return errors.Wrap(e, "Precompiled")
		}

		ctx.Call().InnerReturnVal = output

		ctx.Memory().Set(retOffset.Uint64(), retSize.Uint64(), output)

		ctx.Stack().Push(*uint256.NewInt(1))
		return nil
	}

	_, e := ensure_code(ctx, toAddr) // fetch code + disasm for new Contract
	if e != nil {
		color.Red("ensure code fail")
		return e
	}

	var bigVal = big.NewInt(0)
	if !value.IsZero() {
		//gas += params.CallStipend
		bigVal = value.ToBig()
	}

	currCall := ctx.Call()

	// add a newCall to CallStack
	newCall := &Call{
		Msg: Msg{
			Data:   input,
			Sender: currCall.This,
			Value:  bigVal,
			Gas:    currCall.Msg.Gas,
		},
		This:              toAddr,
		OuterReturnOffset: retOffset.Uint64(),
		OuterReturnSize:   retSize.Uint64(),
	}
	ctx.CallStack.Push(newCall)
	return nil
}
func opCall(ctx *Context) error {
	stack := ctx.Stack()

	gas, addr, value, inOffset, inSize, retOffset, retSize :=
		stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop()

	// when input is empty, it's transfer: `addr.call{value:xxx}("")`
	if inSize.IsZero() {
		// just assume it succeeded
		stack.Push(*uint256.NewInt(1))

		return nil
	}

	return do_opcall(ctx,
		gas, addr, value, inOffset, inSize, retOffset, retSize)
}

func opCallCode(ctx *Context) error {
	return errors.New("TODO opCallCode")
}

/*
A send tx -> B -> delegatecall(C), in C:
	C.msg == B.msg (msg.sender inside C is A, C has same msg.sender/msg.value as B)
	C.storage == B.storage
*/
func opDelegateCall(ctx *Context) error {
	stack := ctx.Stack()
	gas, addr, inOffset, inSize, retOffset, retSize :=
		stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop()

	_ = gas

	toAddr := common.Address(addr.Bytes20())

	_, e := ensure_code(ctx, toAddr) // fetch code + disasm for new Contract
	if e != nil {
		return e
	}

	currCall := ctx.Call()

	args := ctx.Memory().GetPtr(int64(inOffset.Uint64()), int64(inSize.Uint64()))
	newCall := &Call{
		Msg: Msg{
			Data:   args,
			Sender: currCall.Msg.Sender,
			Value:  currCall.Msg.Value,
			Gas:    currCall.Msg.Gas,
		},
		This:    currCall.This, // address(this) doesn't change in delegatecall
		CodePtr: &toAddr,       // `Contract` is required for delegatecall, which used to find the correct disasm code

		OuterReturnOffset: retOffset.Uint64(),
		OuterReturnSize:   retSize.Uint64(),
	}
	ctx.CallStack.Push(newCall)
	return nil
}

/*
	STATICCALL functions equivalently to a CALL,
	except it takes only 6 arguments
	(the “value” argument is not included and taken to be zero).
*/
func opStaticCall(ctx *Context) error {
	stack := ctx.Stack()

	gas, addr, inOffset, inSize, retOffset, retSize :=
		stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop()

	var value uint256.Int // 0

	return do_opcall(ctx,
		gas, addr, value, inOffset, inSize, retOffset, retSize)
}

func opReturn(ctx *Context) error {
	stack := ctx.Stack()
	offset, size := stack.Pop(), stack.Pop()
	output := ctx.Memory().GetPtr(int64(offset.Uint64()), int64(size.Uint64()))

	call := ctx.Call()

	if ctx.CallStack.Len() > 1 { // returning from inner call, not main call
		// popup current call from CallStack
		ctx.CallStack.Pop()

		// after pop(), now it points to the outer call
		ctx.Call().InnerReturnVal = output

		// copy memory[retOffset:retSize] to the outer Call.Memory
		// Note: This copying is supposed to be done in xxCALL operations,
		// but we can't do that when single step, so copy it in RETURN
		ctx.Memory().Set(
			call.OuterReturnOffset, call.OuterReturnSize, output)

		ctx.Stack().Push(*uint256.NewInt(1)) // outerStack
	} else {
		ctx.IsDone = true
	}

	return nil
}

func opRevert(ctx *Context) error {
	stack := ctx.Stack()
	offset, size := stack.Pop(), stack.Pop()
	ret := ctx.Memory().GetPtr(int64(offset.Uint64()), int64(size.Uint64()))
	_ = ret

	//ctx.CurrentCall().ReturnVal = ret
	color.Red(hex.Dump(ret))
	return errors.New("Reverted")
}
func opAssert(ctx *Context) error {
	// stack := ctx.Stack()
	// stack.Pop()
	return errors.New("TODO: opcode Assert")
}

// return from function with no return value
func opStop(ctx *Context) error {
	if ctx.CallStack.Len() > 1 { // return from inner call, not main call
		// popup current call from CallStack
		ctx.CallStack.Pop()

		ctx.Stack().Push(*uint256.NewInt(1))
	} else {
		ctx.IsDone = true
	}
	return nil
}

// SELFDESTRUCT
func opSuicide(ctx *Context) error {
	beneficiary := ctx.Stack().Pop()
	return fmt.Errorf("opSuicide -> %s", beneficiary.ToBig().String())
}

// following functions are used by the instruction jump  table

func opChainID(ctx *Context) error {
	ctx.Stack().Push(*uint256.NewInt(ctx.Chain.Id))
	return nil
}

// address(this).balance
func opSelfBalance(ctx *Context) error {
	bal, e := ensure_balance(ctx, ctx.Call().This)
	if e != nil {
		return e
	}
	balance, _ := uint256.FromBig(bal)

	ctx.Stack().Push(*balance)
	return nil
}

func opBaseFee(ctx *Context) error {
	ctx.Stack().Push(*uint256.NewInt(ctx.Block.BaseFee))
	return nil
}

// make push instruction function
func makePush(n uint64) executionFunc {
	return func(ctx *Context) error {
		code := ctx.Code().Binary
		codeLen := uint64(len(code))

		pc := &ctx.Call().Pc

		if *pc+1+n > codeLen {
			return errors.New("opPushN not enough data")
		}

		integer := new(uint256.Int)
		ctx.Stack().Push(*integer.SetBytes(
			code[*pc+1 : *pc+1+n]))

		*pc += n
		return nil
	}
}

// make dup instruction function
func makeDup(size int64) executionFunc {
	return func(ctx *Context) error {
		ctx.Stack().Dup(int(size))
		return nil
	}
}

// make swap instruction function
func makeSwap(size int64) executionFunc {
	// switch n + 1 otherwise n would be swapped with n
	size++
	return func(ctx *Context) error {
		ctx.Stack().Swap(int(size))
		return nil
	}
}

func makeLog(size int) executionFunc {
	return func(ctx *Context) error {
		stack := ctx.Stack()
		stack.Pop()
		stack.Pop()

		for i := 0; i < size; i++ {
			ctx.Stack().Pop()
		}

		return nil
	}
}
