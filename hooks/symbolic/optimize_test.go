package symbolic

import (
	"testing"

	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func TestFuncSig(t *testing.T) {
	n := &BinaryOp{
		OpNode: OpNode{vm.EQ},
		X:      NewConst(uint256.NewInt(0x11223344)),
		Y: &BinaryOp{
			OpNode: OpNode{vm.SHR},
			X: &UnaryOp{
				OpNode: OpNode{vm.CALLDATALOAD},
				X:      NewConst(uint256.NewInt(0)),
			},
			Y: NewConst(uint256.NewInt(0xe0)),
		},
	}

	n2, _ := Optimize(n, Optimizers{&FunSig{}})

	assert.Equal(t, "(0x11223344 == func_sig)", n2.String())
}

// 	 `ADD(0x4, SUB(x, 0x4))`
func TestConterBinaryOp(t *testing.T) {
	n := &BinaryOp{
		OpNode: OpNode{vm.ADD},
		X:      NewConst(uint256.NewInt(4)),
		Y: &BinaryOp{
			OpNode: OpNode{vm.SUB},
			X:      &Label{Str: "x"},
			Y:      NewConst(uint256.NewInt(4)),
		},
	}

	n2, _ := Optimize(n, Optimizers{&CounterBinaryOp{}})

	assert.Equal(t, "x", n2.String())
}

// `!(!(x))`
func TestConterUnaryOp(t *testing.T) {
	n := &UnaryOp{
		OpNode: OpNode{vm.ISZERO},
		X: &UnaryOp{
			OpNode: OpNode{vm.ISZERO},
			X:      &Label{Str: "x"},
		},
	}

	n2, _ := Optimize(n, Optimizers{&CounterUnaryOp{}})

	assert.Equal(t, "x", n2.String())
}

// `((CALLER << 0x60) & NOT(((0x1 << 0x60) - 0x1)))`
// ->
// (CALLER << 0x60)
func TestSignCast(t *testing.T) {
	n := &BinaryOp{
		OpNode: OpNode{vm.AND},
		X: &BinaryOp{
			OpNode: OpNode{vm.SHL},
			X:      &NullaryOp{OpNode: OpNode{vm.CALLER}},
			Y:      NewConst(uint256.NewInt(0x60)),
		},
		Y: &UnaryOp{
			OpNode: OpNode{vm.NOT},
			X: &BinaryOp{
				OpNode: OpNode{vm.SUB},
				Y:      NewConst(uint256.NewInt(1)),
				X: &BinaryOp{
					OpNode: OpNode{vm.SHL},
					X:      NewConst(uint256.NewInt(1)),
					Y:      NewConst(uint256.NewInt(0x60)),
				},
			},
		},
	}

	n2, _ := Optimize(n, Optimizers{&SignCast{}})

	assert.Equal(t, "(CALLER << 0x60)", n2.String())
}
