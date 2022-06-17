package hooks

import (
	"fmt"

	"github.com/aj3423/edb"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/pkg/errors"
)

var ErrBreakpoint = errors.New("breakpoint")

func init() {
	edb.Register((*BpPc)(nil))
	edb.Register((*BpOpCode)(nil))
}

// break at Pc of target cotract
type BpPc struct {
	edb.EmptyHook
	Contract *common.Address
	Pc       uint64
}

func (bp *BpPc) String() string {
	if bp.Contract == nil {
		return fmt.Sprintf("@ Pc: %x", bp.Pc)
	} else {
		return fmt.Sprintf("@ Pc: %x of %s",
			bp.Pc, bp.Contract.Hex())
	}
}

func (bp *BpPc) PreRun(call *edb.Call, line *edb.Line) error {
	if bp.Contract != nil && *bp.Contract != call.CodeAddress() {
		return nil
	}
	if line.Pc != bp.Pc {
		return nil
	}
	return errors.Wrap(ErrBreakpoint, bp.String())
}

// break at Op code of target contract
// eg: break at `SHA3`
type BpOpCode struct {
	edb.EmptyHook
	Contract *common.Address
	OpCode   vm.OpCode
}

func (bp *BpOpCode) String() string {
	if bp.Contract == nil {
		return fmt.Sprintf(
			"@ OpCode: %s", bp.OpCode.String())
	} else {
		return fmt.Sprintf(
			"@ OpCode: %s of %s",
			bp.OpCode.String(), bp.Contract.Hex())
	}
}

func (bp *BpOpCode) PreRun(call *edb.Call, line *edb.Line) error {
	if bp.Contract != nil && *bp.Contract != call.CodeAddress() {
		return nil
	}
	if line.Op.OpCode != bp.OpCode {
		return nil
	}
	return errors.Wrap(ErrBreakpoint, bp.String())
}
