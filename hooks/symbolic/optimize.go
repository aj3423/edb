package symbolic

import (
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

type optimizer interface {
	// return true if the node is modified
	Do(c *Cursor) (modified bool)
}

type Optimizers []optimizer

var DefaultOptimizers = []optimizer{
	&FunSig{},
	&CounterUnaryOp{},
	&CounterBinaryOp{},
	&ProxyEIP1967{},
	&SignCast{},
}

// replace
// 	 `Const(0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc)`
// ->
//   `PROXY_EIP1967_SLOT`
var PROXY_EIP1967_SLOT, _ = uint256.FromHex("0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc")
var PROXY_EIP1967_ADMIN, _ = uint256.FromHex("0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103")

type ProxyEIP1967 struct {
	sample Node
}

func (op *ProxyEIP1967) Do(c *Cursor) (modified bool) {
	if n, ok := c.Node.(*Const); ok {
		if n.Value().Eq(PROXY_EIP1967_SLOT) {
			c.Replace(&Label{"PROXY_EIP1967_SLOT"})
			modified = true
		} else if n.Value().Eq(PROXY_EIP1967_ADMIN) {
			c.Replace(&Label{"PROXY_EIP1967_ADMIN"})
			modified = true
		}
	}
	return
}

// Evaluate the Node value
func EvaluateConst(n Node) (uint256.Int, bool) {
	switch n := n.(type) {
	case *Const:
		return *n.Value(), true
	case *UnaryOp:
		switch n.OpCode {
		case vm.NOT:
			v, ok := EvaluateConst(n.X)
			if !ok {
				return uint256.Int{}, false
			}
			v.Not(&v)
			return v, true
		}
	case *BinaryOp: // y-x, y<<x
		x, okx := EvaluateConst(n.X)
		y, oky := EvaluateConst(n.Y)
		if !okx || !oky {
			return uint256.Int{}, false
		}
		switch n.OpCode {
		case vm.ADD:
			x.Add(&x, &y)
			return x, true
		case vm.SUB:
			x.Sub(&x, &y)
			return x, true
		case vm.SHL:
			x.Lsh(&x, uint(y.Uint64()))
			return x, true
		case vm.SHR:
			x.Rsh(&x, uint(y.Uint64()))
			return x, true
		case vm.SAR:
			x.SRsh(&x, uint(y.Uint64()))
			return x, true
		}
	}
	return uint256.Int{}, false
}

// replace
// 	 address  `(0xffffffffffffffffffffffffffffffffffffffff & x)`
// 	 u32      `(0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff & x)`
// 	 u16      `(0xffffffffffffffffffffffffffffff & x)`
// ->
//   `x`
var (
	U20, _     = uint256.FromHex("0xffffffffffffffffffffffffffffffffffffffff")
	U20High, _ = uint256.FromHex("0xffffffffffffffffffffffffffffffffffffffff000000000000000000000000")
	U16, _     = uint256.FromHex("0xffffffffffffffffffffffffffffffff")
	U16High, _ = uint256.FromHex("0xffffffffffffffffffffffffffffffff00000000000000000000000000000000")
	U32, _     = uint256.FromHex("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
)

type SignCast struct{}

func (op *SignCast) match(x_or_y Node) bool {
	// Sometimes it's not simply 0xffffffffffffffffffffffffffffffffffffffff,
	// it may also be `((0x1 << 0xa0) - 0x1)`
	// So evaluate it
	if val, ok := EvaluateConst(x_or_y); ok {
		if val.Eq(U20) || val.Eq(U20High) ||
			val.Eq(U32) ||
			val.Eq(U16) || val.Eq(U16High) {
			return true
		}
	}
	return false
}

func (op *SignCast) Do(c *Cursor) (modified bool) {
	if bn, ok := c.Node.(*BinaryOp); ok {
		if bn.OpCode == vm.AND {
			if op.match(bn.X) {
				c.Replace(bn.Y)
				modified = true
			}
			if op.match(bn.Y) {
				c.Replace(bn.X)
				modified = true
			}
		}
	}
	return
}

// replace
// 	 `(CALLDATALOAD(0x0) >> 0xe0)`
// ->
//   `func_sig`
type FunSig struct {
}

var funSigSample = &BinaryOp{
	OpNode: OpNode{OpCode: vm.SHR},
	X: &UnaryOp{
		OpNode: OpNode{OpCode: vm.CALLDATALOAD},
		X:      NewConst(uint256.NewInt(0)),
	},
	Y: NewConst(uint256.NewInt(0xe0)),
}

func (op *FunSig) Do(c *Cursor) (modified bool) {
	if Equal(funSigSample, c.Node) {
		c.Replace(&Label{Str: "func_sig"})

		modified = true
	}
	return
}

func is_counter_op(op1, op2 vm.OpCode) bool {
	if op1 == vm.ADD && op2 == vm.SUB {
		return true
	}
	if op1 == vm.SUB && op2 == vm.ADD {
		return true
	}

	return false
}

// replace
// 	 `ADD(0x4, SUB(x, 0x4))`
// ->
//   `x`
type CounterBinaryOp struct{}

func (op *CounterBinaryOp) Do(c *Cursor) (modified bool) {
	n1, ok := c.Node.(*BinaryOp)
	if !ok { // the `ADD`
		return
	}
	n2, ok := n1.Y.(*BinaryOp)
	if !ok { // the `SUB`
		return
	}
	if !is_counter_op(n1.OpCode, n2.OpCode) {
		return
	}
	if _, ok = n1.X.(*Const); ok { // it's const number
		if Equal(n1.X, n2.Y) {
			c.Replace(n2.X)
			modified = true
		}
	}

	return
}

// replace
// 	 `!(!(x))`
// ->
//   `x`
type CounterUnaryOp struct{}

func (op *CounterUnaryOp) Do(c *Cursor) (modified bool) {
	outer, ok := c.Node.(*UnaryOp)
	if !ok { // the outer
		return
	}
	inner, ok := outer.X.(*UnaryOp)
	if !ok { // the inner
		return
	}
	if outer.OpCode == inner.OpCode {
		switch outer.OpCode {
		case vm.ISZERO, vm.NOT:
			c.Replace(inner.X)
			modified = true
		}
	}
	return
}

// Returns the optimized node
// and if it's modified
func Optimize(
	root Node,
	opts Optimizers,
) (newNode Node, modified bool) {

	newNode = Walk(root, func(c *Cursor) {

		for _, o := range opts {
			if o.Do(c) {
				modified = true
			}
		}
	})

	return
}
