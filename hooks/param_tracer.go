package hooks

import (
	"github.com/aj3423/edb"
	"github.com/aj3423/edb/util"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

func init() {

	edb.Register((*ParamTracer)(nil))
}

// Get stack input/outpus when executing op code
type ParamTracer struct {
	StackPre  edb.Stack[uint256.Int] // stack args before exec
	StackPost edb.Stack[uint256.Int] // result pushed to stack after exec

	PcPre  uint64 // pc before exec
	PcPost uint64 // pc after exec

	MemPre  []byte // full memory copy(before exec)
	MemPost []byte // full memory copy(after exec)
}

func (t *ParamTracer) PreRun(call *edb.Call, line *edb.Line) error {
	stack := &call.Stack
	size := stack.Len()

	t.PcPre = call.Pc
	// Stack:
	//   last n items in stack, n == param count for this OpCode
	t.StackPre.Data = util.CloneSlice(stack.Data[size-int(line.Op.NStackIn) : size]) // clone stack params

	// Memory:
	//   memory is only used for some op code
	switch line.Op.OpCode {
	case vm.SHA3, vm.MLOAD, vm.MSTORE, vm.MSTORE8, vm.CALL, vm.DELEGATECALL, vm.STATICCALL:
		m := call.Memory.Data()
		t.MemPre = append(m[:0:0], m...) // clone byte slice
	}

	return nil
}
func (t *ParamTracer) PostRun(call *edb.Call, line *edb.Line) error {
	stack := &call.Stack
	size := stack.Len()

	t.PcPost = call.Pc

	// Stack:
	//   return values on stack after executing this op code
	t.StackPost.Data = util.CloneSlice(stack.Data[size-int(line.Op.NStackOut) : size]) // clone stack params

	// Memory:
	//   Memory is only used for opcodes below.
	//   Not copy memory for other opcodes to improve performance.
	switch line.Op.OpCode {
	case vm.SHA3, vm.MLOAD, vm.MSTORE, vm.MSTORE8, vm.CALL,
		vm.DELEGATECALL, vm.STATICCALL, vm.CODECOPY, vm.CALLDATACOPY,
		vm.RETURNDATACOPY, vm.EXTCODECOPY, vm.RETURN, vm.REVERT:

		m := call.Memory.Data()
		t.MemPost = append(m[:0:0], m...) // clone byte slice
	}

	return nil
}
