package symbolic

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/aj3423/edb"
	"github.com/aj3423/edb/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

// ---- interfaces ----
type Node interface {
	// for printing node formular
	String() string
}

// node with value
type valueNode interface {
	Node
	Value() *uint256.Int
}
type opNode interface { // for opcode with operands, eg: BinaryOp
	Node
	Op() vm.OpCode
}
type pcNode interface { // pc is used to optimize "for loop"
	Node
	Pc() uint64
}

// ---- basic types ----

// Node that have an output value on stack
type ValueNode struct {
	Val uint256.Int
}

func (n *ValueNode) Value() *uint256.Int {
	return &n.Val
}

type PcNode struct {
	Pc uint64
}

func (n *PcNode) PC() uint64 {
	return n.Pc
}

// Node like `ADD` would use OpCode for printing
type OpNode struct {
	OpCode vm.OpCode
}

func (n *OpNode) Op() vm.OpCode {
	return n.OpCode
}

// ---- concrete nodes ----

// for printing simple string like "func_sig"
type Label struct {
	Str string
}

func (n *Label) String() string {
	return n.Str
}

// For PUSHed values
type Const struct { // 0xe0, 0xff00, ...
	ValueNode
}

func NewConst(v *uint256.Int) *Const {
	c := &Const{}
	c.Val = *v
	return c
}

func (n *Const) String() string {
	return n.Val.String()
}

// These op codes have no input param from stack
// and output 1 value to stack
// eg: vm.ADDRESS, vm.BALANCE
type NullaryOp struct {
	OpNode
	ValueNode
}

func (n *NullaryOp) String() string {
	return PrettifyOp(n.OpCode)
}

// op with 1 input and 1 output on stack
type UnaryOp struct {
	OpNode
	X Node
	ValueNode
}

func (n *UnaryOp) String() string {
	return fmt.Sprintf("%s(%s)",
		PrettifyOp(n.OpCode), n.X.String())
}

// op with 2 input and 1 output on stack
type BinaryOp struct {
	OpNode
	X Node
	Y Node // Y is first operand, eg: Y-X
	ValueNode
}

func (n *BinaryOp) String() string {
	return fmt.Sprintf("(%s %s %s)",
		n.X.String(), PrettifyOp(n.OpCode), n.Y.String())
}

// op with 3 input and 1 output on stack
type TernaryOp struct {
	OpNode

	X Node
	Y Node
	Z Node
	ValueNode
}

func (n *TernaryOp) String() string {
	return fmt.Sprintf("%s(%s, %s, %s)",
		PrettifyOp(n.OpCode), n.X.String(), n.Y.String(), n.Z.String())
}

// `If` is simply `JUMPI`
type If struct {
	PcNode
	Cond  Node
	Taken bool
}

func (n *If) String() string {
	if n.Taken {
		return "if " + n.Cond.String() + " <yes>"
	} else {
		return "if " + n.Cond.String() + " <no>"
	}
}

// A block of lines, currently only used for `Call`,
// will be used for "for loop"(TODO)
type Block struct {
	List []Node
}

func (b *Block) String() string {
	sb := strings.Builder{}
	sb.WriteString("{\n")
	for _, n := range b.List {
		sb.WriteString("\t" + n.String() + "\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

// A CALL/DELEGATECALL/STATICCALL
type Call struct {
	OpNode
	*Block
	Stack edb.Stack[Node] // symbolic stack

	// keep track of all *Memory/*Storage
	MemMap     map[uint64]*Memory       // no need to iterate through when `Apply`
	StorageMap map[uint256.Int]*Storage // no need to iterate through when `Apply`

	// for printing target/func_sig
	Target *common.Address
	Input  []byte
}

func NewCall(
	op vm.OpCode,
	target *common.Address,
	input []byte,
) *Call {
	return &Call{
		Target:     target,
		Input:      util.CloneSlice(input),
		Block:      &Block{},
		OpNode:     OpNode{op},
		MemMap:     make(map[uint64]*Memory),
		StorageMap: make(map[uint256.Int]*Storage),
	}
}

// Logs a line, these lines will be optimized and printed
func (c *Call) AddTrace(n Node) {
	c.List = append(c.List, n)
}
func (c *Call) String() string {
	return "use `printer` to print *Call"
}

// TODO decode func name using 4bytes database:
//  https://github.com/ethereum-lists/4bytes
func (c *Call) FuncSig() string {
	if len(c.Input) >= 4 {
		return fmt.Sprintf("%x", c.Input[0:4])
	}
	return ""
}

type Precompiled struct {
	To    common.Address
	Input *Memory
}

func (n *Precompiled) String() string {
	return fmt.Sprintf(
		"Call Precompiled %s, input: [\n%s]",
		n.To.String(),
		// The hex dump here is more readable than symbolic value
		hex.Dump(n.Input.VmBytes),
	)
}

type MoneyTransfer struct {
	To     common.Address
	Amount uint256.Int
}

func (n *MoneyTransfer) String() string {
	return fmt.Sprintf(
		"Transfer %s(%s) -> %s\n",
		n.Amount.ToBig().String(), // dec format
		n.Amount.String(),         // hex format
		n.To.String(),
	)
}

type ReturnValue struct{}

func (n *ReturnValue) String() string {
	return fmt.Sprintf("ReturnVal_%p", n)
}

type Return struct {
	*ReturnValue
	*Memory
}

func (n *Return) String() string {
	return fmt.Sprintf("%s: [\n%s]",
		n.ReturnValue.String(),
		hex.Dump(n.VmBytes),
	)
}

// Represents a Storage read(SLOAD)
type Storage struct {
	Slot Node
	Val  Node // result of get, or value to set
}

func (s *Storage) String() string {
	return fmt.Sprintf("Storage[%s]", s.Slot.String())
}

// Normally it represents a Storage write access(SSTORE)
// But it's also considered as write access
// for SLOAD when value not found locally and fetched from remote server,
type StorageWrite struct {
	IsGetOnline bool
	*Storage
}

func (n *StorageWrite) String() string {
	if n.IsGetOnline {
		return fmt.Sprintf("%s = online %s", n.Val.String(), n.Storage.String())
	} else {
		s := fmt.Sprintf("%s = %s", n.Storage.String(), n.Val.String())
		// if it can be evaluated, also show the value number
		if eval, ok := EvaluateConst(n.Val); ok {
			if eval.String() != n.Val.String() { // don't show if they are same
				s += fmt.Sprintf(" (%s)", eval.String())
			}
		}
		return s
	}
}

type Log struct {
	Topics []Node
	Mem    []*Memory
}

func (n *Log) String() string {
	sb := &strings.Builder{}

	arr := []string{}
	for _, top := range n.Topics {
		arr = append(arr, top.String())
	}
	fmt.Fprintf(sb, "Log (%s)\n", strings.Join(arr, ", "))
	// Ascending sort "Input" by offset
	// so it can be printed in order
	sort.Slice(n.Mem, func(i, j int) bool {
		return n.Mem[i].VmOffset < n.Mem[j].VmOffset
	})

	sb.WriteString("Memory: [\n")
	for _, mem := range n.Mem {
		sb.WriteString(mem.Val.String() + ":\n")
		sb.WriteString(util.HexDumpEx(
			mem.VmBytes, uint(mem.VmOffset)))
	}
	sb.WriteString("]\n")

	return sb.String()
}

// A block of memory, not the whole memory
type Memory struct {
	Offset   Node // the symbolic offset of total memory
	Val      Node
	VmOffset uint64 // the offset of total memory
	VmBytes  []byte
}

func (n *Memory) String() string {
	return fmt.Sprintf("Memory[%s]", n.Offset.String())
}

// for op codes that writes memory, eg: MSTORE, RETURN, ...
type MemoryWrite struct {
	*Memory
	// show memory dump or not, useful for ReturnVal
	dump bool
}

func (n *MemoryWrite) String() string {
	s := fmt.Sprintf("Memory[%s] = %s",
		n.Offset.String(), n.Val.String())

	// Sometimes the value is string like: "SafeMath: subtraction overflow"
	// show it as string.
	if hs, e := uint256.FromHex(n.Val.String()); e == nil {
		if utf8.Valid(hs.Bytes()) { // is utf8 string
			s += ` ("` + string(hs.Bytes()) + `")`
		}
	}
	if n.dump {
		s += "\n"
		s += hex.Dump(n.VmBytes)
	}
	return s
}

// Conatains all input/output of an SHA3 operation
type Sha3 struct {
	Input []*Memory

	Offset, Size uint64 // raw memory offset/size

	ValueNode
}

func (n *Sha3) String() string {
	// Show as variable like "sha3_0x11223344"
	// use pointer address as variable name
	return fmt.Sprintf("Sha3_%p", n)
}

// For printing `SHA3` calculation in detail
type Sha3Calc struct {
	*Sha3
}

func (n *Sha3Calc) String() string {
	// Ascending sort "Input" by memory offset
	// so it can be printed in order
	sort.Slice(n.Input, func(i, j int) bool {
		return n.Input[i].VmOffset < n.Input[j].VmOffset
	})

	sb := &strings.Builder{}
	sb.WriteString("\n")
	sb.WriteString(n.Sha3.String() + " = [")
	sb.WriteString("\n")

	// build the input memory regions
	for _, mem := range n.Input {
		sb.WriteString(mem.Val.String() + ":\n")
		sb.WriteString(util.HexDumpEx(
			mem.VmBytes, uint(mem.VmOffset)))
	}

	sb.WriteString("] -> " + n.Value().String())
	sb.WriteString("\n")
	return sb.String()
}
