package edb

import (
	"github.com/ethereum/go-ethereum/params"
)

type gasFunc func(*Context) uint64

func fixedGas(n uint64) gasFunc {
	return func(*Context) uint64 { return n }
}

/*
	(exp == 0) ? 10 : (10 + 10 * (1 + log256(exp)))
	If exponent is 0, gas used is 10
	If exponent is greater than 0, gas used is 10 plus 10 times a factor related to how large the log of the exponent is.
*/
func gasExp(ctx *Context) uint64 {
	stack := ctx.Stack()
	exponent := stack.PeekI(1)

	expByteLen := uint64((exponent.BitLen() + 7) / 8)

	return expByteLen*params.ExpByteEIP158 + 10
}

/*
	30 + 6 * (size of input in words)
		30 is the paid for the operation plus
		6 paid for each word (rounded up) for the input data.
*/
func gasSha3(ctx *Context) uint64 {
	stack := ctx.Stack()

	length := stack.PeekI(1).Uint64() // stack: [ data, length
	return 30 + (toWordSize(length) * 6)
}

func toWordSize(bytesLen uint64) uint64 {
	return (bytesLen + 31) / 32
}

/*
	((value != 0) && (storage_location == 0)) ? 20000 : 5000
	20000 is paid when storage value is set to non-zero from zero.
	5000 is paid when the storage value's zeroness remains unchanged or is set to zero.
*/
func gasSStore(ctx *Context) uint64 {
	// gasCost is different when value changes or not
	stack := ctx.Stack()
	this := ctx.Call().This
	slot, new_val := stack.PeekI(0), stack.PeekI(1)

	old_val, e := ensure_storage(ctx, this, slot)
	if e != nil {
		// TODO find a better way to stop running
		return 0xffffffffffffffff
	}

	if !old_val.IsZero() && new_val.IsZero() {
		return 20000
	}
	return 5000
}

// calculates gas for memory expansion.
// only calculates the memory region that is expanded, not the total memory.
func memoryGasCost(currMemSize, newMemSize uint64) uint64 {
	if newMemSize == 0 {
		return 0
	}
	newMemSizeWords := toWordSize(newMemSize)
	newMemSize = newMemSizeWords * 32

	if newMemSize > currMemSize {
		square := newMemSizeWords * newMemSizeWords
		linCoef := newMemSizeWords * params.MemoryGas
		quadCoef := square / params.QuadCoeffDiv
		newTotalFee := linCoef + quadCoef
		return newTotalFee

	}
	return 0
}

func memoryCopierGas(baseGas uint64, offsetPos, lenPos int) gasFunc {
	return func(ctx *Context) uint64 {
		stack := ctx.Stack()
		offset, length := stack.PeekI(offsetPos).Uint64(), stack.PeekI(lenPos).Uint64()

		newMemSize := offset + length
		currMemSize := ctx.Memory().Len()

		// Gas for expanding the memory
		gasExpand := memoryGasCost(currMemSize, newMemSize)

		// And gas for copying data, charged per word at param.CopyGas
		gasCopy := toWordSize(length) * params.CopyGas

		fee := gasExpand + gasCopy
		return baseGas + fee
	}
}

// baseGas + 3 * (number of words copied, rounded up)
// baseGas is paid for the operation, plus 3 for each word copied (rounded up).
var (
	gasCallDataCopy   = memoryCopierGas(3, 0, 2)
	gasCodeCopy       = memoryCopierGas(3, 0, 2)
	gasExtCodeCopy    = memoryCopierGas(700, 1, 3)
	gasReturnDataCopy = memoryCopierGas(3, 0, 2)
)

/*
	375 + 8 * (number of bytes in log data) + n * 375
		375 is paid for operation plus 8 for each byte in data to be logged
		plus n * 375 for the n topics to be logged.
*/
func makeGasLog(n uint64) gasFunc {
	return func(ctx *Context) uint64 {
		size := ctx.Stack().PeekI(1).Uint64()
		return 375 + 8*size + n*375
	}
}

func gasTodo(ctx *Context) uint64 {
	return 0
}
