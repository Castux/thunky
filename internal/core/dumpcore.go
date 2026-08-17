package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Castux/thunky/internal/value"
)

// dumpcore.go renders the Core IR (core.go) as an indented tree, reached by the
// --dump-core flag (see main.go). The Core is where laziness and addressing are
// explicit, so this dump shows exactly what lowering decided: every name as an
// addressing mode (local/upvalue/module slot), every lazy position as a thunk with
// its update flag and frame size, every lambda with its frame size and free-
// variable capture list, and the desugaring of lists, strings, and list patterns
// into cons/tuple forms. It is the companion of the AST dump (dumpast.go): the same
// program, after resolution and lowering.

// DumpCore renders the program body thunk followed by every module's bindings,
// modules in sorted order for a stable result.
func DumpCore(mainCore Expr, modCores map[string][]Bind) string {
	var sb strings.Builder
	sb.WriteString("; thunky Core IR\n\n")

	main := mainCore.(Thunk)
	fmt.Fprintf(&sb, "program (frame %d)\n", main.Frame)
	writeCoreExpr(&sb, main.Body, "", true)

	var names []string
	for name := range modCores {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if len(modCores[name]) == 0 {
			continue // every binding of this module was pruned as unreachable
		}
		fmt.Fprintf(&sb, "\nmodule %s\n", name)
		for i, bind := range modCores[name] {
			isLast := i == len(modCores[name])-1
			thunk := bind.Body.(Thunk)
			coreLine(&sb, "", isLast, "binding %d %s (frame %d)", bind.Slot, bind.Name, thunk.Frame)
			nextPrefix := "    "
			if !isLast {
				nextPrefix = "│   "
			}
			writeCoreExpr(&sb, thunk.Body, nextPrefix, true)
		}
	}
	return sb.String()
}

// formatNumber renders a float literal the same way the value printer does.
func formatNumber(num float64) string {
	return strconv.FormatFloat(num, 'g', -1, 64)
}

func coreLine(sb *strings.Builder, prefix string, isLast bool, format string, args ...any) {
	sb.WriteString(prefix)
	if isLast {
		sb.WriteString("└── ")
	} else {
		sb.WriteString("├── ")
	}
	fmt.Fprintf(sb, format, args...)
	sb.WriteByte('\n')
}

func writeCoreExpr(sb *strings.Builder, expr Expr, prefix string, isLast bool) {
	switch e := expr.(type) {
	case Num:
		coreLine(sb, prefix, isLast, "num %s", formatNumber(e.Val))

	case Const:
		coreLine(sb, prefix, isLast, "const %s", value.ShowConst(e.Val))

	case Stdin:
		if e.Bytes {
			coreLine(sb, prefix, isLast, "bstdin")
		} else {
			coreLine(sb, prefix, isLast, "stdin")
		}

	case Var:
		coreLine(sb, prefix, isLast, "var %s", showAddr(e.Addr))

	case App:
		coreLine(sb, prefix, isLast, "app")
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		coreLine(sb, nextPrefix, false, "head")
		writeCoreExpr(sb, e.Head, nextPrefix+"│   ", true)
		for i, arg := range e.Args {
			argIsLast := i == len(e.Args)-1
			coreLine(sb, nextPrefix, argIsLast, "arg %d", i)
			writeCoreExpr(sb, arg, nextPrefix+pick(argIsLast, "    ", "│   "), true)
		}

	case Prim:
		coreLine(sb, prefix, isLast, "prim %s", value.PrimName(e.Op))
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		for i, arg := range e.Args {
			argIsLast := i == len(e.Args)-1
			coreLine(sb, nextPrefix, argIsLast, "arg %d", i)
			writeCoreExpr(sb, arg, nextPrefix+pick(argIsLast, "    ", "│   "), true)
		}

	case Compose:
		coreLine(sb, prefix, isLast, "compose (forward %t)", e.Forward)
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		for i, fn := range e.Fns {
			writeCoreExpr(sb, fn, nextPrefix, i == len(e.Fns)-1)
		}

	case Cons:
		coreLine(sb, prefix, isLast, "cons")
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		coreLine(sb, nextPrefix, false, "head")
		writeCoreExpr(sb, e.Head, nextPrefix+"│   ", true)
		coreLine(sb, nextPrefix, true, "tail")
		writeCoreExpr(sb, e.Tail, nextPrefix+"    ", true)

	case Tuple:
		coreLine(sb, prefix, isLast, "tuple (arity %d)", len(e.Fields))
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		for i, field := range e.Fields {
			writeCoreExpr(sb, field, nextPrefix, i == len(e.Fields)-1)
		}

	case Let:
		coreLine(sb, prefix, isLast, "let (%d binding(s))", len(e.Binds))
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		for _, bind := range e.Binds {
			coreLine(sb, nextPrefix, false, "bind slot %d %s", bind.Slot, bind.Name)
			writeCoreExpr(sb, bind.Body, nextPrefix+"│   ", true)
		}
		coreLine(sb, nextPrefix, true, "in")
		writeCoreExpr(sb, e.Body, nextPrefix+"    ", true)

	case Lambda:
		coreLine(sb, prefix, isLast, "lambda (frame %d, free %s)", e.Frame, showAddrs(e.Free))
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		for i, c := range e.Cases {
			caseIsLast := i == len(e.Cases)-1
			coreLine(sb, nextPrefix, caseIsLast, "case #%d (frame %d)", i, c.Frame)
			casePrefix := nextPrefix
			if caseIsLast {
				casePrefix += "    "
			} else {
				casePrefix += "│   "
			}
			coreLine(sb, casePrefix, false, "pattern")
			writeCorePattern(sb, c.Pattern, casePrefix+"│   ", true)
			coreLine(sb, casePrefix, true, "body")
			writeCoreExpr(sb, c.Body, casePrefix+"    ", true)
		}

	case Thunk:
		coreLine(sb, prefix, isLast, "thunk (frame %d%s)", e.Frame, thunkName(e.Name))
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		writeCoreExpr(sb, e.Body, nextPrefix, true)

	default:
		coreLine(sb, prefix, isLast, "<unknown core expression %T>", expr)
	}
}

func writeCorePattern(sb *strings.Builder, pattern Pattern, prefix string, isLast bool) {
	switch p := pattern.(type) {
	case PatternVar:
		coreLine(sb, prefix, isLast, "var slot %d %s", p.Slot, p.Name)

	case PatternConst:
		coreLine(sb, prefix, isLast, "const %s", value.ShowConst(p.Val))

	case PatternTuple:
		coreLine(sb, prefix, isLast, "tuple-pattern (arity %d)", len(p.Fields))
		nextPrefix := prefix
		if isLast {
			nextPrefix += "    "
		} else {
			nextPrefix += "│   "
		}
		for i, field := range p.Fields {
			writeCorePattern(sb, field, nextPrefix, i == len(p.Fields)-1)
		}

	default:
		coreLine(sb, prefix, isLast, "<unknown core pattern %T>", pattern)
	}
}

func pick(cond bool, t, f string) string {
	if cond {
		return t
	}
	return f
}

func thunkName(name string) string {
	if name == "" {
		return ""
	}
	return ", name " + name
}

// showAddr renders one addressing mode as lowering computed it.
func showAddr(addr Addr) string {
	switch addr.Kind {
	case AddrLocal:
		return fmt.Sprintf("local[%d]", addr.Slot)
	case AddrUpvalue:
		return fmt.Sprintf("upvalue[%d]", addr.Slot)
	case AddrModule:
		return fmt.Sprintf("module %s[%d]", addr.Module, addr.Slot)
	default:
		return fmt.Sprintf("<addr %d>", addr.Kind)
	}
}

func showAddrs(addrs []Addr) string {
	if len(addrs) == 0 {
		return "[]"
	}
	parts := make([]string, len(addrs))
	for i, addr := range addrs {
		parts[i] = showAddr(addr)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
