package symbolic

import (
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

// ---- Compare ----
// `Equal` compares
// UnaryOp/BinaryOp/uint256.Int/string/vm.OpCode
func Equal(n1, n2 any) bool {
	e1 := reflect.Indirect(reflect.ValueOf(n1))
	e2 := reflect.Indirect(reflect.ValueOf(n2))

	if e1.Type() != e2.Type() {
		return false
	}

	switch n1.(type) {
	case *UnaryOp:
		v1 := n1.(*UnaryOp)
		v2 := n2.(*UnaryOp)
		return Equal(v1.OpCode, v2.OpCode) && Equal(v1.X, v2.X)
	case *BinaryOp:
		v1 := n1.(*BinaryOp)
		v2 := n2.(*BinaryOp)
		return Equal(v1.OpCode, v2.OpCode) && Equal(v1.X, v2.X) && Equal(v1.Y, v2.Y)
	case *Const:
		v1 := n1.(*Const)
		v2 := n2.(*Const)
		return Equal(v1.Val, v2.Val)
	case *Label:
		v1 := n1.(*Label)
		v2 := n2.(*Label)
		return Equal(v1.Str, v2.Str)
	case uint256.Int:
		v1 := n1.(uint256.Int)
		v2 := n2.(uint256.Int)
		return v1.Eq(&v2)
	case string:
		v1 := n1.(string)
		v2 := n2.(string)
		return v1 == v2
	case vm.OpCode:
		v1 := n1.(vm.OpCode)
		v2 := n2.(vm.OpCode)
		return v1 == v2
	}

	return false
}

// ---- Walk ----
// code from Go astutil package
type iterator struct {
	index, step int
}

type Cursor struct {
	Parent Node
	Name   string    // the name of the parent Node field that contains the current Node.
	Iter   *iterator // valid if non-nil
	Node   Node
}

// Index reports the index >= 0 of the current Node in the slice of Nodes that
// contains it, or a value < 0 if the current Node is not part of a slice.
// The index of the current node changes if InsertBefore is called while
// processing the current node.
func (c *Cursor) Index() int {
	if c.Iter != nil {
		return c.Iter.index
	}
	return -1
}

// field returns the current node's parent field value.
func (c *Cursor) field() reflect.Value {
	return reflect.Indirect(reflect.ValueOf(c.Parent)).FieldByName(c.Name)
}

// Replace replaces the current Node with n.
// The replacement node is not walked by Walk().
func (c *Cursor) Replace(n Node) {
	v := c.field()
	if i := c.Index(); i >= 0 {
		v = v.Index(i)
	}
	v.Set(reflect.ValueOf(n))
}

// Delete deletes the current Node from its containing slice.
// If the current Node is not part of a slice, Delete panics.
// As a special case, if the current node is a package file,
// Delete removes it from the package's Files map.
func (c *Cursor) Delete() {
	i := c.Index()
	if i < 0 {
		panic("Delete node not contained in slice")
	}
	v := c.field()
	l := v.Len()
	reflect.Copy(v.Slice(i, l), v.Slice(i+1, l))
	v.Index(l - 1).Set(reflect.Zero(v.Type().Elem()))
	v.SetLen(l - 1)
	c.Iter.step--
}

// InsertAfter inserts n after the current Node in its containing slice.
// If the current Node is not part of a slice, InsertAfter panics.
// Walk does not walk n.
func (c *Cursor) InsertAfter(n Node) {
	i := c.Index()
	if i < 0 {
		panic("InsertAfter node not contained in slice")
	}
	v := c.field()
	v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
	l := v.Len()
	reflect.Copy(v.Slice(i+2, l), v.Slice(i+1, l))
	v.Index(i + 1).Set(reflect.ValueOf(n))
	c.Iter.step++
}

// InsertBefore inserts n before the current Node in its containing slice.
// If the current Node is not part of a slice, InsertBefore panics.
// Walk will not walk n.
func (c *Cursor) InsertBefore(n Node) {
	i := c.Index()
	if i < 0 {
		panic("InsertBefore node not contained in slice")
	}
	v := c.field()
	v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
	l := v.Len()
	reflect.Copy(v.Slice(i+1, l), v.Slice(i, l))
	v.Index(i).Set(reflect.ValueOf(n))
	c.Iter.index++
}

type WalkFunc func(*Cursor)

type walker struct {
	walkFn WalkFunc
	cursor Cursor
	iter   iterator
}

func (a *walker) walkList(parent Node, name string) {
	// avoid heap-allocating a new iterator for each walkList call; reuse a.iter instead
	saved := a.iter
	a.iter.index = 0
	for {
		// must reload parent.name each time, since cursor modifications might change it
		v := reflect.Indirect(reflect.ValueOf(parent)).FieldByName(name)
		if a.iter.index >= v.Len() {
			break
		}

		// element x may be nil
		var x Node
		if e := v.Index(a.iter.index); e.IsValid() {
			x = e.Interface().(Node)
		}

		a.iter.step = 1
		a.walk(parent, name, &a.iter, x)
		a.iter.index += a.iter.step
	}
	a.iter = saved
}

func (a *walker) walk(parent Node, name string, iter *iterator, n Node) {

	saved := a.cursor
	a.cursor.Parent = parent
	a.cursor.Name = name
	a.cursor.Iter = iter
	a.cursor.Node = n

	// walk children
	switch n := n.(type) {
	case nil, *Label, *MoneyTransfer, *ReturnValue:
		// nothing to do

	case *Memory:
		a.walk(n, "Offset", nil, n.Offset)
		a.walk(n, "Val", nil, n.Val)
	case *MemoryWrite:
		a.walk(n, "Memory", nil, n.Memory)
	case *Storage:
		a.walk(n, "Slot", nil, n.Slot)
		a.walk(n, "Val", nil, n.Val)
	case *StorageWrite:
		a.walk(n, "Storage", nil, n.Storage)
	case *Const, *NullaryOp:
		// do nothing
	case *UnaryOp:
		a.walk(n, "X", nil, n.X)
	case *BinaryOp:
		a.walk(n, "X", nil, n.X)
		a.walk(n, "Y", nil, n.Y)
	case *TernaryOp:
		a.walk(n, "X", nil, n.X)
		a.walk(n, "Y", nil, n.Y)
		a.walk(n, "Z", nil, n.Z)
	case *If:
		a.walk(n, "Cond", nil, n.Cond)
	case *Sha3:
		a.walkList(n, "Input")
	case *Log:
		a.walkList(n, "Topics")
		a.walkList(n, "Mem")
	case *Sha3Calc:
		a.walk(n, "Sha3", nil, n.Sha3)
	case *Return:
		a.walk(n, "ReturnValue", nil, n.ReturnValue)
		a.walk(n, "Memory", nil, n.Memory)
	case *Precompiled:
		a.walk(n, "Input", nil, n.Input)
	case *Block, *Call:
		a.walkList(n, "List")

	default:
		panic(fmt.Sprintf("Walk: unexpected node type %T", n))
	}

	a.walkFn(&a.cursor)

	a.cursor = saved
}

func Walk(
	root Node,
	fn func(*Cursor),
) Node {
	parent := &struct{ Node }{root}
	a := &walker{walkFn: fn}
	a.walk(parent, "Node", nil, root)
	return parent.Node
}
