package edb

// IMPORTANT:
// Use Pointer as value type
// because slice allocates and copies when it excceeds the max capacity
type Stack[T any] struct {
	Data []T
}

func (st *Stack[T]) Push(d T) {
	st.Data = append(st.Data, d)
}
func (st *Stack[T]) PushN(ds ...T) {
	st.Data = append(st.Data, ds...)
}

func (st *Stack[T]) Pop() (ret T) {
	ret = st.Data[len(st.Data)-1]
	st.Data = st.Data[:len(st.Data)-1]
	return
}

func (st *Stack[T]) Len() int {
	return len(st.Data)
}

func (st *Stack[T]) Swap(n int) {
	st.Data[st.Len()-n], st.Data[st.Len()-1] = st.Data[st.Len()-1], st.Data[st.Len()-n]
}

func (st *Stack[T]) Dup(n int) {
	st.Push(st.Data[st.Len()-n])
}

func (st *Stack[T]) PeekI(n int) *T {
	return &st.Data[st.Len()-1-n]
}
func (st *Stack[T]) Peek() *T {
	return st.PeekI(0)
}

func (st *Stack[T]) Clear() {
	st.Data = nil
}
