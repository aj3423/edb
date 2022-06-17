package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aj3423/edb"
	"github.com/aj3423/edb/hooks"
	"github.com/aj3423/edb/hooks/symbolic"
	"github.com/aj3423/edb/util"
	"github.com/c-bata/go-prompt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/fatih/color"
)

// global value
var G = struct {
	JsonFile string
	ctx      *edb.Context
	HiTracer *symbolic.HighLevelTracer
}{
	JsonFile: "sample.json",
}

var suggestions = []prompt.Suggest{
	{Text: "help", Description: "Show all commands"},
	{Text: "mem [offset [size]]", Description: "Show memory"},
	{Text: "sto", Description: "Show Storage"},
	{Text: "s", Description: "Show Stack items"},
	{Text: "p [pc]", Description: "Show asm at current/target PC"},
	{Text: "load [.json]", Description: "Reload current .json file(default: sample.json)"},
	{Text: "save [.json]", Description: "Save context to current .json file(default: sample.json)"},
	{Text: "tx <tx_hash> <node_url>", Description: "Generate .json file from archive node"},
	{Text: "low", Description: "start low level trace"},
	{Text: "hi", Description: "start high level trace"},
	{Text: "op", Description: "Optimize and print result of high-level-trace"},
	{Text: "log", Description: "Log every executed EVM instruction to file"},
	{Text: "n", Description: "Single step"},
	{Text: "c", Description: "Continue"},
	{Text: "b", Description: "Breakpoint"},
}

func completer(in prompt.Document) []prompt.Suggest {
	if in.Text == "" {
		return nil
	}
	args := strings.Split(in.Text, " ")
	if len(args) == 1 {
		return prompt.FilterHasPrefix(
			suggestions, in.GetWordBeforeCursor(), true)
	} else {
		return nil
	}
}

func show_disasm(pc uint64) {

	addr := G.ctx.Call().CodeAddress()
	fmt.Println("---- " + addr.String())

	asm := G.ctx.Code().Asm

	line, e := asm.LineAtPc(pc)
	if e != nil {
		color.Red(e.Error())
		return
	}
	lineNum := line.LineNum

	beg := util.Max(int(lineNum)-4, 0)               // show 4 lines above pc
	end := util.Min(int(lineNum)+4, asm.LineCount()) // show 4 lines below pc

	for row := beg; row < end; row++ {
		line := asm.AtRow(row)
		if line.Pc == pc {
			color.Blue(line.String())
		} else {
			fmt.Println(line)
		}
	}
}

func executor(in string) {
	in = strings.TrimSpace(in)

	if in == "" {
		in = "n" // press enter -> single step
	}

	arg := strings.Split(in, " ")
	argc := len(arg)

	cmd := arg[0]

	if G.ctx == nil &&
		(cmd != "load" && cmd != "tx" && cmd != "help") {

		color.Red("'load' first")
		return
	}

	switch cmd {
	case "help":
		for _, s := range suggestions {
			color.HiBlue("%s \t %s", s.Text, color.WhiteString(s.Description))
		}
		return

	case "ctx", "context":
		fmt.Println(to_pretty_json(G.ctx))
		return

	case "m", "mem", "memory":
		switch argc {
		case 1:
			fmt.Println(hex.Dump(G.ctx.Memory().Data()))
			return
		case 3:
			offset, e2 := parse_any_int(arg[1])
			len_, e3 := parse_any_int(arg[2])

			if e2 != nil || e3 != nil {
				color.Red("wrong format, usage: memory <offset> <len>")
				return
			}

			mem := G.ctx.Memory().Data()

			if offset+len_ > uint64(len(mem)) {
				color.Red("invalid memory region, %d > %d", offset+len_, len(mem))
				return
			}

			chunk := mem[offset : offset+len_]
			fmt.Println(hex.Dump(chunk))
			return
		}

	case "storage":
		fmt.Println(to_pretty_json(G.ctx.Contract().Storage))
		return
	case "s", "stack":
		fmt.Println(to_pretty_json(G.ctx.Stack()))
		return

	case "p", "print": // show disasm
		var pc = G.ctx.Pc()

		if argc == 2 { // show disasm at target pc
			pc_, e := parse_any_int(arg[1])
			if e != nil {
				color.Red(e.Error())
				return
			}
			pc = uint64(pc_)
		}

		show_disasm(pc)
		return

	case "save":
		var fn = G.JsonFile
		if argc == 2 {
			fn = arg[1]
		}
		if e := G.ctx.Save(fn); e != nil {
			color.Red("fail save json: " + e.Error())
			return
		}
		color.Green("saved to '%s' ", fn)
		return
	case "load", "reload":
		if argc > 1 {
			G.JsonFile = arg[1]
		}

		ctx := &edb.Context{}
		if e := ctx.Load(G.JsonFile); e != nil {
			color.Red(e.Error())
			return
		}
		color.Green("loaded: %s", G.JsonFile)
		G.ctx = ctx

		show_disasm(G.ctx.Pc())

		return

	case "tx":
		var node_url string
		var tx_hash string

		_, e := fmt.Sscanf(in,
			"tx %s %s",
			&tx_hash, &node_url)

		if e != nil {
			color.Red("usage: tx <tx_hash> <node_url>")
			return
		}

		ctx, e := edb.ContextFromTx(node_url, tx_hash)
		if e != nil {
			color.Red("fail: " + e.Error())
			return
		}
		fn := tx_hash + ".json"
		e = ctx.Save(fn)
		if e != nil {
			color.Red("fail save json: " + e.Error())
			return
		}
		G.ctx = ctx
		color.Green("saved to '%s' ", fn)
		return

	case "low", "lowleveltrace": // trace input/output data for all algorithms
		G.ctx.Hooks.Attach(hooks.NewLowLevelTracer())
		color.Yellow("tracing low-level operations")
		return
	case "hi", "high", "highleveltrace":
		if G.HiTracer != nil {
			color.Yellow("already tracing high-level operations")
			return
		}
		G.HiTracer = symbolic.NewHighLevelTracer(G.ctx)
		G.ctx.Hooks.Attach(G.HiTracer)
		color.Yellow("tracing high-level operations")
		return
	case "hic", "high-level-callstack": // for debugging only
		fmt.Println(*G.HiTracer.CallStack.Peek())
		return
	case "his", "high-level-stack": // for debugging only
		for _, s := range (*G.HiTracer.CallStack.Peek()).Stack.Data {
			fmt.Println(s)
		}
		return
	case "op", "optimize": // show optimized result of high-level-tracer
		if G.HiTracer == nil {
			color.Red("no HighLevelTracer, restart with command 'hi' to enable high level ")
			return
		}
		rootCall := G.HiTracer.CallStack.Data[0]
		symbolic.Optimize(rootCall, symbolic.DefaultOptimizers)

		// print result
		x := symbolic.PrintNode(rootCall)
		fmt.Println(x)
		fn := strings.ReplaceAll(G.JsonFile, ".json", "") + ".high"
		util.FileWriteStr(fn, x)
		color.Yellow("Written to file '%s'", fn)
		return

	case "log", "evm_log":
		fn := strings.Replace(G.JsonFile, ".json", ".log", 1)

		fd, e := os.OpenFile(
			fn, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if e != nil {
			color.Red(e.Error())
			return
		}
		G.ctx.Hooks.Attach(&hooks.EvmLog{Fd: fd})
		color.Yellow("logging to '%s'", fn)
		return

	case "n", "next":
		e := G.ctx.Run(1)
		if e != nil {
			color.Red(e.Error())
		}
		show_disasm(G.ctx.Pc())
		return

	case "c", "continue", "r", "run":
		e := G.ctx.Run(-1)
		if e != nil {
			if errors.Is(e, hooks.ErrBreakpoint) {
				color.Yellow("interrupted: %s", e.Error())
			} else {
				color.Red(e.Error())
			}
		} else {
			color.Green("\nall done.\n\n")
		}
		show_disasm(G.ctx.Pc())
		return

	case "b", "bp", "breakpoint":
		if argc == 2 {
			if arg[1] == "l" { // list all breakpoints/tracers
				for i, cp := range G.ctx.Hooks.List() {
					fmt.Printf("%d: %v\n", i, cp)
				}
				return
			}
		}
		if argc == 3 {
			if arg[1] == "d" { // del n'th
				if i, e := strconv.Atoi(arg[2]); e == nil {
					G.ctx.Hooks.Detach(i)
					return
				}
			}
		}

		if argc >= 3 { // eg: b op SHA3 0x1122334455...
			var contract *common.Address = nil
			if argc == 4 {
				x := common.HexToAddress(arg[3])
				contract = &x
			}

			switch arg[1] {
			case "op": // break by op code

				opStr := strings.ToUpper(arg[2])
				op := vm.StringToOp(opStr)

				// the above StringToOp returns STOP for any invalid input
				if op == vm.STOP && opStr != "STOP" {
					color.Red("wrong op string")
					return
				}
				bp := &hooks.BpOpCode{
					Contract: contract,
					OpCode:   op,
				}
				G.ctx.Hooks.Attach(bp)
				color.Yellow("bp added: %v", bp)
				return

			case "pc": // break by pc
				pc, e := parse_any_int(arg[2])
				if e != nil {
					color.Red("wrong pc format")
					return
				}
				bp := &hooks.BpPc{
					Contract: contract,
					Pc:       uint64(pc),
				}
				G.ctx.Hooks.Attach(bp)
				color.Yellow("bp added: %v", bp)
				return
			}
		}
	}
	color.Red("unknown command")
}

func main() {
	if !util.FileExist(G.JsonFile) {
		edb.NewSampleContext().Save(G.JsonFile)
		fmt.Printf(
			"A sample config '%s' generated, load it with '%s'\n",
			color.MagentaString(G.JsonFile), color.CyanString("load"))
	}

	p := prompt.New(
		executor,
		completer,
		prompt.OptionPrefix(">>> "),
	)
	p.Run()
}

func main2() { // only for quick testing
	G.JsonFile = "1.json"

	var e error
	G.ctx = &edb.Context{}
	if e = G.ctx.Load(G.JsonFile); e != nil {
		color.Red(e.Error())
		return
	}
	color.Green("loaded: %s", G.JsonFile)

	{
		G.ctx.Hooks.Attach(&hooks.BpPc{Pc: 11257})
		color.Yellow("tracing low-level operations")
	}

	e = G.ctx.Run(-1)
	if e != nil {
		if errors.Is(e, hooks.ErrBreakpoint) {
			color.Yellow("interrupted: %s", e.Error())
		} else {
			color.Red(e.Error())
		}
	} else {
		color.Green("\nall done.\n\n")
	}
	G.ctx.Run(-1)
	// show_disasm(G.ctx.Pc())

}
