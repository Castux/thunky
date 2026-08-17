package backend

import (
	"fmt"
	"sort"

	"github.com/Castux/thunky/internal/core"
	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/syntax"
	"github.com/Castux/thunky/internal/value"
)

// compile.go translates the Core IR (core.go) into the flat bytecode of
// bytecode.go. Every body — the program, each module binding, each lambda case,
// each thunk — is a contiguous span of the single Program.Code array, reached by
// its start PC. Out-of-line bodies (lambdas and thunks) are NOT emitted inline at
// the point they are referenced; instead a template is registered and the body is
// queued, then drained at the end. This keeps each span self-contained: a span is
// only ever reached by entering its PC, never by falling through from the code that
// built the closure or thunk that points at it.
//
// Codegen has two procedures, mirroring the lowerer's demanded/lazy split:
//
//   - compileBody(e): e is a body in demanded (tail) position. An application
//     pushes its arguments (rightmost first, so the leftmost is applied first) and
//     continues with the head; a let stores its bindings then continues with its
//     body; everything else builds a value. The caller terminates the span with
//     Enter.
//   - compileValue(e): leaves exactly one value on the operand stack and never
//     touches the reduction stack, so it is safe in any sub-expression position.
//
// Saturated builtin calls are lowered to Prim nodes (see lower.go) and compiled
// to the Prim opcode, which forces its operands directly rather than going through
// the spine. Forcing an operand that needs reduction suspends the activation onto
// the explicit stack (see runCode in machine.go); the live operands are snapshotted
// into the suspension, so the shared operand buffer is free for the forcing.

// compiler accumulates the single flat Program. emit* helpers append to the shared
// Code slice; intern* helpers deduplicate pool entries; pending holds out-of-line
// body compilations queued by declareLambda / declareThunk.
type compiler struct {
	prog *Program

	numIdx  map[float64]int32
	posIdx  map[source.SourcePos]int32
	nameIdx map[string]int32
	modIdx  map[string]int32

	pending []func()
}

func newCompiler() *compiler {
	return &compiler{
		prog:    &Program{Modules: make(map[string][]ModuleBinding)},
		numIdx:  make(map[float64]int32),
		posIdx:  make(map[source.SourcePos]int32),
		nameIdx: make(map[string]int32),
		modIdx:  make(map[string]int32),
	}
}

// --- pool interning ---

func (c *compiler) internConst(v value.Value) int32 {
	// Numbers are deduplicated; other constants (builtins, prebuilt string lists)
	// are rare and not safely hashable, so they are appended without dedup.
	if v.Tag == value.NumberTag {
		if idx, ok := c.numIdx[v.Num]; ok {
			return idx
		}
		idx := int32(len(c.prog.Consts))
		c.prog.Consts = append(c.prog.Consts, v)
		c.numIdx[v.Num] = idx
		return idx
	}
	idx := int32(len(c.prog.Consts))
	c.prog.Consts = append(c.prog.Consts, v)
	return idx
}

func (c *compiler) internPos(pos source.SourcePos) int32 {
	if idx, ok := c.posIdx[pos]; ok {
		return idx
	}
	idx := int32(len(c.prog.Posns))
	c.prog.Posns = append(c.prog.Posns, pos)
	c.posIdx[pos] = idx
	return idx
}

func (c *compiler) internName(name string) int32 {
	if idx, ok := c.nameIdx[name]; ok {
		return idx
	}
	idx := int32(len(c.prog.Names))
	c.prog.Names = append(c.prog.Names, name)
	c.nameIdx[name] = idx
	return idx
}

func (c *compiler) internModule(name string) int32 {
	if idx, ok := c.modIdx[name]; ok {
		return idx
	}
	idx := int32(len(c.prog.ModuleNames))
	c.prog.ModuleNames = append(c.prog.ModuleNames, name)
	c.modIdx[name] = idx
	return idx
}

// nameOrNone interns a binding name, returning -1 (→ "") for the anonymous case.
func (c *compiler) nameOrNone(name string) int32 {
	if name == "" {
		return -1
	}
	return c.internName(name)
}

// --- emit helpers ---

func (c *compiler) pc() PC { return int32(len(c.prog.Code)) }

func (c *compiler) emit(op Op, a int32) {
	c.prog.Code = append(c.prog.Code, Instr{Op: op, A: a})
}

func (c *compiler) emitAB(op Op, a, b int32) {
	c.prog.Code = append(c.prog.Code, Instr{Op: op, A: a, B: b})
}

// placeholder emits an instruction whose B (jump target) is patched later.
func (c *compiler) placeholder(op Op, a int32) int {
	idx := len(c.prog.Code)
	c.prog.Code = append(c.prog.Code, Instr{Op: op, A: a})
	return idx
}

func (c *compiler) patchB(idx int, b int32) {
	c.prog.Code[idx].B = b
}

// --- top-level entry ---

// Compile translates the Core IR (program body + module bindings) into a flat
// Program. Bodies referenced out of line (lambdas, thunks) are queued and drained
// after the top-level bodies, so the layout never lets one span fall into another.
func Compile(mainCore core.Expr, modCores map[string][]core.Bind, sourceProgram *syntax.Program, modules map[string]*syntax.Module) *Program {
	c := newCompiler()

	// Program body.
	mainThunk := mainCore.(core.Thunk)
	c.prog.Entry = c.pc()
	c.prog.EntryFrame = mainThunk.Frame
	c.compileBody(mainThunk.Body)
	c.emit(Enter, 0)

	// Module bindings, in a deterministic order. Each binding's right-hand side is
	// its own activation (its own frame), memoised in the module environment.
	var modNames []string
	for name := range modules {
		modNames = append(modNames, name)
	}
	sort.Strings(modNames)
	for _, modName := range modNames {
		var bindings []ModuleBinding
		for _, cb := range modCores[modName] {
			thunk := cb.Body.(core.Thunk)
			entry := c.pc()
			c.compileBody(thunk.Body)
			c.emit(Enter, 0)
			bindings = append(bindings, ModuleBinding{Code: entry, Frame: thunk.Frame, Name: cb.Name})
		}
		c.prog.Modules[modName] = bindings
		c.prog.ModuleOrder = append(c.prog.ModuleOrder, modName)
	}

	// Drain queued out-of-line bodies. Compiling one may queue more.
	for len(c.pending) > 0 {
		job := c.pending[len(c.pending)-1]
		c.pending = c.pending[:len(c.pending)-1]
		job()
	}

	return c.prog
}

// --- body compilation (demanded / tail position) ---

func (c *compiler) compileBody(expr core.Expr) {
	switch e := expr.(type) {
	case core.App:
		posIdx := c.internPos(e.Pos)
		// Push rightmost first so the leftmost argument ends up on top of the
		// reduction stack and is applied first.
		for k := len(e.Args) - 1; k >= 0; k-- {
			c.compileValue(e.Args[k])
			c.emit(PushArg, posIdx)
		}
		c.compileBody(e.Head) // the head extends the same spine

	case core.Prim:
		posIdx := c.internPos(e.Pos)
		for _, arg := range e.Args {
			c.compileValue(arg)
		}
		c.emitAB(Prim, int32(e.Op), posIdx)

	case core.Let:
		c.compileLetBindings(e.Binds)
		c.compileBody(e.Body)

	default:
		c.compileValue(expr)
	}
}

// --- value compilation (leaves one operand, never forces) ---

func (c *compiler) compileValue(expr core.Expr) {
	switch e := expr.(type) {
	case core.Num:
		c.emit(PushConst, c.internConst(value.NumberValue(e.Val)))

	case core.Const:
		c.emit(PushConst, c.internConst(e.Val))

	case core.Stdin:
		// Its own opcode rather than a pooled constant: the stream is created on
		// demand by the machine (and memoised there, so every reference shares
		// one stream), which keeps a mutable value out of the constant pool and
		// makes the two streams distinguishable in a disassembly.
		if e.Bytes {
			c.emit(PushBstdin, 0)
		} else {
			c.emit(PushStdin, 0)
		}

	case core.Var:
		c.compileAddr(e.Addr)

	case core.Cons:
		c.compileValue(e.Head)
		c.compileValue(e.Tail)
		c.emit(MakeCons, 0)

	case core.Tuple:
		if len(e.Fields) == 0 {
			c.emit(PushConst, c.internConst(value.EmptyTuple))
			return
		}
		for _, f := range e.Fields {
			c.compileValue(f)
		}
		c.emit(MakeTuple, int32(len(e.Fields)))

	case core.Compose:
		// MakeCompose pops second, first → Composition{first, second}, applied as
		// first(second(x)). Push so the pairwise folds compose in the right order.
		if e.Forward {
			for i := len(e.Fns) - 1; i >= 0; i-- {
				c.compileValue(e.Fns[i])
			}
		} else {
			for _, fn := range e.Fns {
				c.compileValue(fn)
			}
		}
		for i := 0; i < len(e.Fns)-1; i++ {
			c.emit(MakeCompose, 0)
		}

	case core.Lambda:
		c.emit(MakeClosure, c.declareLambda(e))

	case core.Thunk:
		// A thunk in value position is a lazy argument or field; like every thunk
		// it memoises, so the deferred computation runs at most once.
		c.emit(MakeThunk, c.declareThunk(e))

	case core.Let:
		// A let in value position: store its bindings, then build the body value.
		c.compileLetBindings(e.Binds)
		c.compileValue(e.Body)

	default:
		panic(fmt.Sprintf("compileValue: unexpected Core expression %T", expr))
	}
}

func (c *compiler) compileLetBindings(binds []core.Bind) {
	for _, b := range binds {
		tmpl := c.declareThunk(b.Body.(core.Thunk))
		c.emitAB(StoreLet, int32(b.Slot), tmpl)
	}
}

func (c *compiler) compileAddr(addr core.Addr) {
	switch addr.Kind {
	case core.AddrLocal:
		c.emit(PushLocal, int32(addr.Slot))
	case core.AddrUpvalue:
		c.emit(PushUpvalue, int32(addr.Slot))
	case core.AddrModule:
		c.emitAB(PushModule, c.internModule(addr.Module), int32(addr.Slot))
	}
}

// --- out-of-line bodies ---

// declareThunk registers a thunk template and queues its body for compilation,
// returning the template index. The body runs over the enclosing activation's
// frames (whole-frame capture), so no capture list is needed.
func (c *compiler) declareThunk(t core.Thunk) int32 {
	idx := int32(len(c.prog.Thunks))
	c.prog.Thunks = append(c.prog.Thunks, ThunkTemplate{Code: -1, Name: int(c.nameOrNone(t.Name)), Pos: t.Pos})
	c.pending = append(c.pending, func() {
		c.prog.Thunks[idx].Code = c.pc()
		c.compileBody(t.Body)
		c.emit(Enter, 0)
	})
	return idx
}

// declareLambda registers a closure template (with its minimal capture list) and
// queues its cases for compilation, returning the template index.
func (c *compiler) declareLambda(lam core.Lambda) int32 {
	idx := int32(len(c.prog.Closures))

	var captures []Capture
	for i, addr := range lam.Free {
		name := ""
		if i < len(lam.FreeNames) {
			name = lam.FreeNames[i]
		}
		captures = append(captures, Capture{FromUpvalue: addr.Kind == core.AddrUpvalue, Slot: addr.Slot, Name: name})
	}
	c.prog.Closures = append(c.prog.Closures, ClosureTemplate{Frame: lam.Frame, Capture: captures, NoMatch: lam.NoMatch})

	c.pending = append(c.pending, func() {
		c.prog.Closures[idx].Code = c.pc()
		// Each case is [Case, match instrs (failing to the next case), body, Enter].
		// The last case's failure path is NoMatch.
		for i, cs := range lam.Cases {
			c.emit(Case, 0)
			failSlots := c.compilePattern(cs.Pattern)
			c.compileBody(cs.Body)
			c.emit(Enter, 0)

			var failTarget PC
			if i < len(lam.Cases)-1 {
				failTarget = c.pc()
			} else {
				failTarget = c.pc()
				c.emit(NoMatch, 0)
			}
			for _, slot := range failSlots {
				c.patchB(slot, failTarget)
			}
		}
	})
	return idx
}

// --- pattern compilation ---

// compilePattern emits a pattern's match instructions and returns the indices of
// the instructions whose jump target must be patched to the case's failure PC.
func (c *compiler) compilePattern(pat core.Pattern) []int {
	switch p := pat.(type) {
	case core.PatternVar:
		c.emitAB(Bind, int32(p.Slot), c.nameOrNone(p.Name))
		return nil // Bind never fails.

	case core.PatternConst:
		// Lowering reduces every constant pattern to a number (strings become nested
		// cons patterns), so MatchNumber covers them all.
		return []int{c.placeholder(MatchNumber, c.internConst(p.Val))}

	case core.PatternTuple:
		arity := len(p.Fields)
		fails := []int{c.placeholder(MatchTuple, int32(arity))}
		// On success MatchTuple pushes the fields; sub-patterns consume them
		// left-to-right.
		for _, sub := range p.Fields {
			fails = append(fails, c.compilePattern(sub)...)
		}
		return fails

	default:
		panic(fmt.Sprintf("compilePattern: unexpected Core pattern %T", pat))
	}
}
