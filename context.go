package edb

import (
	"encoding/json"
	"io/ioutil"
	"math/big"

	"github.com/aj3423/edb/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/holiman/uint256"
)

type Msg struct {
	Data   util.ByteSlice
	Gas    uint64 // gasleft()
	Sender common.Address
	Value  *big.Int
}

type Tx struct {
	Hash     common.Hash // if provided, storage would be auto fetched online when SLOAD
	Origin   common.Address
	GasPrice uint64
}
type Block struct {
	Number     uint64         // block.number
	Timestamp  uint64         // block.timestamp
	Difficulty uint64         // block.difficulty
	Coinbase   common.Address // block.coinbase
	GasLimit   uint64         // block.gaslimit
	BaseFee    uint64
}
type Chain struct {
	Id      uint64
	NodeUrl string // should be archive node
}

type Code struct {
	Binary util.ByteSlice // 60806040...
	Asm    *Asm           `json:"-"`
}

func (c *Code) Disasm(code []byte) error {
	if c.Asm != nil { // already done disasm
		return nil
	}
	asm := NewAsm()
	e := asm.Disasm(code)
	if e != nil {
		return e
	}
	c.Asm = asm
	return nil
}

func (c *Code) Set(code []byte) error {
	e := c.Disasm(code)
	if e != nil {
		return e
	}
	c.Binary = code
	return nil
}

type Contract struct {
	Code    *Code
	Balance *big.Int
	Storage map[common.Hash]*uint256.Int
}

func NewContract() *Contract {
	return &Contract{
		Code:    &Code{},
		Storage: map[common.Hash]*uint256.Int{},
	}
}

// call context environment, for both:
// 1. the main contract execution
// 2. any inner CALL/DELEGATECALL
type Call struct {
	Msg Msg

	This common.Address // address(this)

	// The `CodePtr` only used for `delegatecall`.
	// In most cases, the `CodePtr` is nil and `This` is used for finding the code
	// But When A.delegatecall(B), in B, `This` is A and `CodePtr` is B
	CodePtr *common.Address

	Memory Memory
	Stack  Stack[uint256.Int]

	Pc uint64 // program counter

	// For return value of inner calls
	// Maybe these values should be defined in Context instead
	OuterReturnOffset uint64
	OuterReturnSize   uint64
	InnerReturnVal    util.ByteSlice // returned value from inner CALL/DELEGATECALL
}

// when in delegatecall, `ctx.This` references to the caller
// CodePtr points to the real code
func (c *Call) CodeAddress() common.Address {
	if c.CodePtr != nil { // only in delegatecall
		return *c.CodePtr
	}
	return c.This
}

type Context struct {
	IsDone    bool
	ethClient *ethclient.Client

	Chain Chain
	Tx    Tx
	Block Block

	Contracts map[common.Address]*Contract

	CallStack Stack[*Call]

	Hooks Hooks

	// `map[blockNum]blockHash`, replace with `map[blockNum]Block` if necessary
	BlockHashes map[uint64]common.Hash
}

/*
	steps n:
		> 0: run n lines
		==0: stop running
		< 0: run til death
*/
func (ctx *Context) Run(steps int) error {
	// first step always executed, no matter breakpoint or not
	is_first_step := true

	for steps != 0 && !ctx.IsDone {
		call := ctx.Call()

		line, e := ctx.Line()
		if e != nil {
			return e
		}
		opcode := line.Op.OpCode
		op := OpTable[opcode]

		// 1. run hooks before executing current line
		if e = ctx.Hooks.PreRunAll(call, line); e != nil && !is_first_step {
			return e
		}
		is_first_step = false

		e = op.Exec(ctx) // execute the asm line
		if e != nil {
			return e
		}

		// increase pc by 1
		// JUMPs handle pc themselves so ignore them
		if opcode != vm.JUMP && opcode != vm.JUMPI {
			call.Pc += 1 // +1 for 1-byte-opcode
		}

		// 2. run hooks after executing current line
		if e := ctx.Hooks.PostRunAll(call, line); e != nil {
			return e
		}

		steps--
	}
	return nil
}

// get current Call
func (ctx *Context) Call() *Call {
	return *ctx.CallStack.Peek()
}

// get current Code
func (ctx *Context) Code() *Code {
	addr := ctx.Call().CodeAddress()
	return ctx.Contracts[addr].Code
}

// get current asm line to execute
func (ctx *Context) Line() (*Line, error) {
	return ctx.Code().Asm.LineAtPc(ctx.Pc())
}

// get address(this) of current call
func (ctx *Context) This() common.Address {
	return ctx.Call().This
}

// get *Msg of current call
func (ctx *Context) Msg() *Msg {
	return &ctx.Call().Msg
}

// get *Stack of current call
func (ctx *Context) Stack() *Stack[uint256.Int] {
	return &ctx.Call().Stack
}

// get *Memory of current call
func (ctx *Context) Memory() *Memory {
	return &ctx.Call().Memory
}

// get current executing pc
func (ctx *Context) Pc() uint64 {
	return ctx.Call().Pc
}

// get current executing *Contract
func (ctx *Context) Contract() *Contract {
	return ctx.Contracts[ctx.This()]
}

func (ctx *Context) String() string {
	bs, _ := json.MarshalIndent(ctx, "", "  ")
	return string(bs)
}
func (ctx *Context) Save(fn string) error {
	return ioutil.WriteFile(fn, []byte(ctx.String()), 0666)
}
func (ctx *Context) Load(fn string) error {
	bs, e := ioutil.ReadFile(fn)
	if e != nil {
		return e
	}
	e = json.Unmarshal(bs, ctx)
	if e != nil {
		return e
	}

	// asm not saved in .json since it's too large
	// so disassemble all contracts after loaded
	for _, contract := range ctx.Contracts {
		e = contract.Code.Disasm(contract.Code.Binary)
		if e != nil {
			return e
		}
	}

	// ethClient
	if ctx.Chain.NodeUrl != "" {
		ctx.ethClient, e = ethclient.Dial(ctx.Chain.NodeUrl)
		if e != nil {
			return e
		}
	}
	return nil
}

func NewContext() *Context {
	ctx := &Context{
		BlockHashes: map[uint64]common.Hash{},
		Contracts:   map[common.Address]*Contract{},
	}
	ctx.CallStack.Push(&Call{Msg: Msg{Value: big.NewInt(0)}})
	return ctx
}
func NewSampleContext() *Context {
	ctx := NewContext()

	contract := NewContract()
	contract.Code.Set(util.HexDec("608060405234801561001057600080fd5b50600436106100365760003560e01c80633bc5de301461003b5780635b4b73a914610059575b600080fd5b610043610075565b60405161005091906100a1565b60405180910390f35b610073600480360381019061006e91906100ed565b61007e565b005b60008054905090565b8060008190555050565b6000819050919050565b61009b81610088565b82525050565b60006020820190506100b66000830184610092565b92915050565b600080fd5b6100ca81610088565b81146100d557600080fd5b50565b6000813590506100e7816100c1565b92915050565b600060208284031215610103576101026100bc565b5b6000610111848285016100d8565b9150509291505056fea2646970667358221220e5f07a97a4abeb88a5fcf07910fb20896f7f95326c9a7a8f1f2a2686532f5a3164736f6c634300080d0033"))
	contract.Storage[common.HexToHash("0x0")] = uint256.NewInt(0x1)
	contract.Storage[common.HexToHash("0xc0fee")] = uint256.NewInt(0xdead)
	ctx.Contracts[ctx.This()] = contract

	ctx.Msg().Data = util.HexDec("3bc5de30") // getData()
	return ctx
}
