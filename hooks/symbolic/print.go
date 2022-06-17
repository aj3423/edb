package symbolic

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/core/vm"
)

// Low performance, maybe replace
//   `Node.String` -> `Node.Print`
type printer struct {
	indentLevel  int
	sb           strings.Builder
	indentString string
}

// Add "\t" before string
func (p *printer) line(s string) {
	p.sb.WriteString(
		strings.Repeat(p.indentString, p.indentLevel) +
			s + "\n")
}

// For printing node with indention
func (p *printer) print(n Node) {
	switch n := n.(type) {

	case *Call: // increase indent level by 1
		p.line("")
		p.line(fmt.Sprintf("%s -> %s, func: %s {",
			n.OpCode.String(), n.Target.String(), n.FuncSig(),
		))
		p.indentLevel++

		for _, ch := range n.List {
			p.print(ch)
		}

		p.indentLevel--
		p.line("}") // last line "}"
	case *Sha3Calc, *Log, *Return, *Precompiled:
		// add indent to all output lines of `Sha3Calc.String()`
		ss := strings.Split(n.String(), "\n")
		for _, s := range ss {
			p.line(s)
		}

	default: // for all other Node, just print their `String()`
		p.line(n.String())
	}

}
func PrintNode(n Node) string {
	p := &printer{indentString: "    "}
	p.print(n)
	return p.sb.String()
}

func PrettifyOp(op vm.OpCode) string {
	switch op {
	case vm.ISZERO:
		return "!"
	case vm.LT, vm.SLT:
		return "<"
	case vm.GT, vm.SGT:
		return ">"
	case vm.EQ:
		return "=="
	case vm.ADD:
		return "+"
	case vm.SUB:
		return "-"
	case vm.MUL:
		return "*"
	case vm.DIV:
		return "/"
	case vm.MOD:
		return "%"
	case vm.AND:
		return "&"
	case vm.OR:
		return "|"
	case vm.SHR:
		return ">>"
	case vm.SHL:
		return "<<"

	default:
		return op.String()
	}
}
