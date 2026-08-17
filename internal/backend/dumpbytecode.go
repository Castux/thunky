package backend

import (
	"fmt"
	"strings"

	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/value"
)

// dumpbytecode.go disassembles the flat bytecode (bytecode.go) to text, reached by
// the --dump-bytecode flag (see main.go). It is the last of the three inspection
// dumps (after the AST and Core IR) and shows the final form the machine runs: one
// dense []Instr array whose every body — the program, each module binding, each
// lambda case, each thunk — is a PC-addressed span of that one array.
//
// The disassembly walks the array in PC order and prints a span header whenever a
// known entry point (the program entry, a module binding, a closure template, a
// thunk template) begins. Operands are decoded against the pools (constants, names,
// modules, positions) the instructions index, so no separate pool listing is needed
// to read the program.
//
// Names are surfaced for locals and upvalues:
//   - PushLocal/PushUpvalue: annotated with the variable name as a semicolon comment.
//     PushLocal names are inferred by scanning each case block for the Bind/StoreLet
//     instructions that write to each slot. PushUpvalue names come from the closure's
//     Capture list (populated with the source name at compile time).
//   - PushArg and Prim show the source position rather than an opaque pool index.
//   - MakeThunk, StoreLet, thunk span headers, and closure span headers all show
//     the binding name and (where available) the definition site position.

// DumpBytecode renders the whole Program.
func DumpBytecode(p *Program) string {
	var sb strings.Builder
	sb.WriteString("; thunky bytecode\n")
	fmt.Fprintf(&sb, "; %d instructions, %d consts, %d closures, %d thunks\n",
		len(p.Code), len(p.Consts), len(p.Closures), len(p.Thunks))

	labels := spanLabels(p)
	closureByPC := closureIdxByPC(p)

	// State updated as we walk the instructions linearly.
	slotNames := make(map[int]string) // current case/body: slot → bound name
	closureIdx := -1                  // index of the closure span we're in, or -1
	caseNum := 0                      // which case we're on within a closure, or 0

	for pc := 0; pc < len(p.Code); pc++ {
		if label, ok := labels[PC(pc)]; ok {
			fmt.Fprintf(&sb, "\n%s\n", label)
			slotNames = make(map[int]string)
			closureIdx = closureByPC[PC(pc)]
			caseNum = 0
		}

		in := p.Code[pc]

		// Update slot→name table before printing so StoreLet's own line shows the name.
		switch in.Op {
		case Case:
			slotNames = make(map[int]string)
			if closureIdx >= 0 {
				caseNum++ // increment case counter within closures
			}
		case Bind:
			if in.B >= 0 {
				slotNames[int(in.A)] = p.Names[in.B]
			}
		case StoreLet:
			if tmpl := p.Thunks[in.B]; tmpl.Name >= 0 {
				slotNames[int(in.A)] = p.Names[tmpl.Name]
			}
		}

		fmt.Fprintf(&sb, "│ %5d  %-12s  %s\n", pc, opName(in.Op), operandText(p, in, slotNames, closureIdx, caseNum))
	}
	return sb.String()
}

// closureIdxByPC maps each span-entry PC to its closure index, or -1 for
// non-closure spans (program body, module bindings, thunk bodies).
func closureIdxByPC(p *Program) map[PC]int {
	m := make(map[PC]int)
	m[p.Entry] = -1
	for _, mbs := range p.Modules {
		for _, mb := range mbs {
			m[mb.Code] = -1
		}
	}
	for i, tmpl := range p.Closures {
		m[tmpl.Code] = i
	}
	for _, tmpl := range p.Thunks {
		m[tmpl.Code] = -1
	}
	return m
}

// spanLabels maps each span-entry PC to its header line.
func spanLabels(p *Program) map[PC]string {
	labels := make(map[PC]string)
	labels[p.Entry] = fmt.Sprintf("[program body]  frame=%d", p.EntryFrame)

	for _, name := range p.ModuleOrder {
		for i, mb := range p.Modules[name] {
			labels[mb.Code] = fmt.Sprintf("[module %s binding %d %q]  frame=%d", name, i, mb.Name, mb.Frame)
		}
	}
	for i, tmpl := range p.Closures {
		hdr := fmt.Sprintf("[closure #%d]  frame=%d capture=%s", i, tmpl.Frame, showCaptures(tmpl.Capture))
		if pos := posText(tmpl.NoMatch); pos != "" {
			hdr += "  ; " + pos
		}
		labels[tmpl.Code] = hdr
	}
	for i, tmpl := range p.Thunks {
		hdr := fmt.Sprintf("[thunk #%d %s]", i, quotedName(p, int32(tmpl.Name)))
		if pos := posText(tmpl.Pos); pos != "" {
			hdr += "  ; " + pos
		}
		labels[tmpl.Code] = hdr
	}
	return labels
}

func showCaptures(captures []Capture) string {
	if len(captures) == 0 {
		return "[]"
	}
	parts := make([]string, len(captures))
	for i, c := range captures {
		addr := fmt.Sprintf("local[%d]", c.Slot)
		if c.FromUpvalue {
			addr = fmt.Sprintf("upvalue[%d]", c.Slot)
		}
		if c.Name != "" {
			parts[i] = fmt.Sprintf("%q=%s", c.Name, addr)
		} else {
			parts[i] = addr
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// operandText decodes an instruction's operands against the program's pools.
// slotNames is the current case/body's slot→name table; closureIdx is the index
// of the enclosing closure (for PushUpvalue names), or -1; caseNum is which case
// we're on within a closure (1-indexed), or 0 for non-closure spans.
func operandText(p *Program, in Instr, slotNames map[int]string, closureIdx int, caseNum int) string {
	switch in.Op {
	case PushConst:
		return value.ShowConst(p.Consts[in.A])

	case PushLocal:
		if name := slotNames[int(in.A)]; name != "" {
			return fmt.Sprintf("slot=%d  ; %q", in.A, name)
		}
		return fmt.Sprintf("slot=%d", in.A)

	case PushUpvalue:
		name := ""
		if closureIdx >= 0 && int(in.A) < len(p.Closures[closureIdx].Capture) {
			name = p.Closures[closureIdx].Capture[in.A].Name
		}
		if name != "" {
			return fmt.Sprintf("slot=%d  ; %q", in.A, name)
		}
		return fmt.Sprintf("slot=%d", in.A)

	case PushModule:
		modName := p.ModuleNames[in.A]
		if mbs, ok := p.Modules[modName]; ok && int(in.B) < len(mbs) {
			return fmt.Sprintf("%s[%d]  ; %q", modName, in.B, mbs[in.B].Name)
		}
		return fmt.Sprintf("%s[%d]", modName, in.B)

	case MakeTuple:
		return fmt.Sprintf("arity=%d", in.A)

	case MakeClosure:
		return fmt.Sprintf("closure #%d", in.A)

	case MakeThunk:
		tmpl := p.Thunks[in.A]
		name := quotedName(p, int32(tmpl.Name))
		if pos := posText(tmpl.Pos); pos != "" {
			return fmt.Sprintf("%s thunk #%d  ; %s", name, in.A, pos)
		}
		return fmt.Sprintf("%s thunk #%d", name, in.A)

	case StoreLet:
		tmpl := p.Thunks[in.B]
		name := quotedName(p, int32(tmpl.Name))
		if pos := posText(tmpl.Pos); pos != "" {
			return fmt.Sprintf("slot=%d %s thunk #%d  ; %s", in.A, name, in.B, pos)
		}
		return fmt.Sprintf("slot=%d %s thunk #%d", in.A, name, in.B)

	case PushArg:
		if pos := posText(p.Posns[in.A]); pos != "" {
			return fmt.Sprintf("pos #%d  ; %s", in.A, pos)
		}
		return fmt.Sprintf("pos #%d", in.A)

	case MatchNumber, MatchString:
		return fmt.Sprintf("%s -> %d", value.ShowConst(p.Consts[in.A]), in.B)

	case MatchTuple:
		return fmt.Sprintf("arity=%d -> %d", in.A, in.B)

	case Bind:
		return fmt.Sprintf("slot=%d name=%s", in.A, quotedName(p, in.B))

	case Case:
		if closureIdx >= 0 {
			return fmt.Sprintf("case %d", caseNum)
		}
		return ""

	case NoMatch:
		if closureIdx >= 0 {
			if pos := posText(p.Closures[closureIdx].NoMatch); pos != "" {
				return fmt.Sprintf("; %s", pos)
			}
		}
		return ""

	case Prim:
		name := value.PrimName(value.PrimOp(in.A))
		if pos := posText(p.Posns[in.B]); pos != "" {
			return fmt.Sprintf("%s  ; %s", name, pos)
		}
		return name

	default:
		return ""
	}
}

// posText formats a source position as "file:line:col", or "" for a zero pos.
func posText(pos source.SourcePos) string {
	if pos.File == nil {
		return ""
	}
	text := pos.File.Text[:pos.Start]
	line := 1 + strings.Count(text, "\n")
	col := pos.Start - strings.LastIndex(text, "\n")
	return fmt.Sprintf("%s:%d:%d", pos.File.Path, line, col)
}

// quotedName renders a Names-pool index, with -1 meaning the anonymous name.
func quotedName(p *Program, idx int32) string {
	if idx < 0 {
		return `""`
	}
	return fmt.Sprintf("%q", p.Names[idx])
}

var opNames = [...]string{
	PushConst:   "PushConst",
	PushLocal:   "PushLocal",
	PushUpvalue: "PushUpvalue",
	PushModule:  "PushModule",
	PushStdin:   "PushStdin",
	PushBstdin:  "PushBstdin",
	MakeCons:    "MakeCons",
	MakeTuple:   "MakeTuple",
	MakeCompose: "MakeCompose",
	MakeClosure: "MakeClosure",
	MakeThunk:   "MakeThunk",
	StoreLet:    "StoreLet",
	PushArg:     "PushArg",
	Enter:       "Enter",
	Case:        "Case",
	MatchNumber: "MatchNumber",
	MatchTuple:  "MatchTuple",
	MatchString: "MatchString",
	Bind:        "Bind",
	NoMatch:     "NoMatch",
	Prim:        "Prim",
}

func opName(op Op) string {
	if int(op) < len(opNames) && opNames[op] != "" {
		return opNames[op]
	}
	return fmt.Sprintf("Op(%d)", op)
}
