package edb

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/aj3423/edb/util"
)

type hook interface {
	// called before executing current instruction, return error to stop running
	PreRun(call *Call, line *Line) error
	// called after executing current instruction, return error to stop running
	PostRun(call *Call, line *Line) error
}

type EmptyHook struct{}

func (h *EmptyHook) PreRun(call *Call, line *Line) error  { return nil }
func (h *EmptyHook) PostRun(call *Call, line *Line) error { return nil }

var map_hook_types = make(map[string]reflect.Type) // eg: map["BpPc"]*main.BpPc

// To support save/load of Hooks, they should be Registered first
func Register(c hook) {
	name := reflect.TypeOf(c).Elem().Name()
	t := reflect.TypeOf(c).Elem()
	map_hook_types[name] = t
}
func makeInstance(name string) interface{} {
	return reflect.New(map_hook_types[name]).Interface()
}

type Hooks struct {
	arr []hook
}

/*
result:
[
	{ "Type": "BpPc", "Value": {Pc:3} },
	{ "Type": "BpOp", "Value": {Op:4} },
	...
]
*/
func (hks *Hooks) MarshalJSON() ([]byte, error) {
	arr := []any{}
	for _, hk := range hks.arr {
		arr = append(arr, map[string]any{
			"Type":  reflect.TypeOf(hk).Elem().Name(),
			"Value": hk,
		})
	}
	return json.MarshalIndent(arr, "", "  ")
}
func (hks *Hooks) UnmarshalJSON(bs []byte) error {
	arr := []any{}
	e := json.Unmarshal(bs, &arr)
	if e != nil {
		return e
	}
	for i := 0; i < len(arr); i++ {
		x, ok := arr[i].(map[string]any)
		if !ok {
			return fmt.Errorf("invalid hook format: %v", arr[i])
		}
		type_, ok := x["Type"].(string)
		if !ok {
			return fmt.Errorf("invalid 'Name' field: %v", x)
		}

		hk := makeInstance(type_)

		if e := util.MapToStruct(x["Value"], hk); e != nil {
			return e
		}
		hks.Attach(hk.(hook))
	}

	return nil
}

func (hks *Hooks) PreRunAll(
	call *Call, line *Line,
) error {
	var err error
	for _, h := range hks.arr {
		if e := h.PreRun(call, line); e != nil {
			err = e
		}
	}
	return err
}
func (hks *Hooks) PostRunAll(
	call *Call, line *Line,
) error {
	var err error
	for _, h := range hks.arr {
		if e := h.PostRun(call, line); e != nil {
			err = e
		}
	}
	return err
}

func (hks *Hooks) Attach(h hook) {
	hks.arr = append(hks.arr, h)
}
func (hks *Hooks) Detach(i int) {
	if i >= 0 && i < len(hks.arr) {
		hks.arr = append((hks.arr)[0:i], (hks.arr)[i+1:]...)
	}
}
func (hks *Hooks) List() []hook {
	return hks.arr
}
