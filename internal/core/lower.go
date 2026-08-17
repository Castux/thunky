package core

import (
	"fmt"

	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/syntax"
	"github.com/Castux/thunky/internal/value"
)

// lower.go translates the resolved AST into the Core IR (core.go). It is where
// laziness becomes explicit: every position that must stay unevaluated until
// demanded — a function argument, a data field, a let/module binding — becomes a
// Thunk, and every other position is built directly. Three lowering modes
// split demanded from lazy positions:
//
//   - lowerTail(e): e's value is demanded now. An application flattens its spine
//     (no intermediate node); a let stores its bindings then continues in tail
//     position; everything else is a value.
//   - lowerArg(e): e is an argument or a data field, so it must stay lazy. Anything
//     that could force when entered (an application, a pipe, a let) becomes a
//     thunk; atoms and pure constructors are built directly.
//   - lowerValue(e): builds a value (literal, constructor, name, closure,
//     composition). Its compound parts are lowered with lowerArg so they stay lazy.
//
// Name resolution facts come from resolve.go; this pass turns each use into an
// addressing mode (local slot, upvalue slot, or module slot) and assigns frame
// slots for pattern bindings and lets.

type Lowerer struct {
	res      *syntax.Resolution
	locals   map[syntax.Node]localDef // defining Node → its frame and slot
	modSlots map[*syntax.Binding]int  // module public binding → its environment slot
	current  *FrameBuilder
}

type localDef struct {
	frame *FrameBuilder
	slot  int
}

// FrameBuilder tracks slot allocation for one activation (the program body, a
// module binding, or a lambda) and the upvalues a nested lambda captures from it.
// freeNames and slotNames are debug-only; they are never read by the machine.
type FrameBuilder struct {
	parent    *FrameBuilder
	node      syntax.Node
	size      int
	free      []Addr   // captured free variables, in the parent's addressing
	freeNames []string // name of each entry in free, parallel
	slotNames []string // name of each local slot, indexed by slot number
}

func (fb *FrameBuilder) setSlotName(slot int, name string) {
	for len(fb.slotNames) <= slot {
		fb.slotNames = append(fb.slotNames, "")
	}
	fb.slotNames[slot] = name
}

// nameForAddr returns the debug name of a variable reached via addr in this frame.
func (fb *FrameBuilder) nameForAddr(addr Addr) string {
	switch addr.Kind {
	case AddrLocal:
		if addr.Slot < len(fb.slotNames) {
			return fb.slotNames[addr.Slot]
		}
	case AddrUpvalue:
		if addr.Slot < len(fb.freeNames) {
			return fb.freeNames[addr.Slot]
		}
	}
	return ""
}

func (fb *FrameBuilder) allocate() int {
	s := fb.size
	fb.size++
	return s
}

// resolveUpvalue returns how the current frame reaches a variable defined at
// (defFrame, defSlot). If it is this frame's own slot, that is a local; otherwise
// it is threaded down as an upvalue through every intervening frame's capture list.
func (fb *FrameBuilder) resolveUpvalue(defFrame *FrameBuilder, defSlot int) Addr {
	if fb == defFrame {
		return Addr{Kind: AddrLocal, Slot: defSlot}
	}

	parentAddr := fb.parent.resolveUpvalue(defFrame, defSlot)

	for i, freeAddr := range fb.free {
		if freeAddr == parentAddr {
			return Addr{Kind: AddrUpvalue, Slot: i}
		}
	}

	slot := len(fb.free)
	fb.free = append(fb.free, parentAddr)
	fb.freeNames = append(fb.freeNames, fb.parent.nameForAddr(parentAddr))
	return Addr{Kind: AddrUpvalue, Slot: slot}
}

// Lower produces the Core for the program body (wrapped in a memoising thunk that
// carries the body's frame size) and for every module's public bindings.
func Lower(program *syntax.Program, modules map[string]*syntax.Module, res *syntax.Resolution) (Expr, map[string][]Bind) {
	l := &Lowerer{
		res:      res,
		locals:   make(map[syntax.Node]localDef),
		modSlots: make(map[*syntax.Binding]int),
	}

	// Only module bindings reachable from the program body are compiled. The rest
	// are dead code: laziness means an unreferenced binding never runs, and the
	// resolver has already checked every module independently, so dropping them
	// changes nothing observable — it only spares them a compiled span and a startup
	// thunk. Module binding slots are needed before any body is lowered, since
	// modules (and the program) may reference each other in any order; they are
	// assigned densely over the surviving bindings, per module in declaration order,
	// so the module environment a slot indexes stays gap-free.
	reachable := reachableModuleBindings(program, res)
	for _, mod := range modules {
		slot := 0
		for _, pb := range mod.PublicBindings {
			if reachable[pb] {
				l.modSlots[pb] = slot
				slot++
			}
		}
	}

	modBinds := make(map[string][]Bind)
	for modName, mod := range modules {
		var binds []Bind
		for _, pb := range mod.PublicBindings {
			if !reachable[pb] {
				continue
			}
			fb := &FrameBuilder{node: mod}
			l.current = fb
			body := l.lowerTail(pb.Expression)
			binds = append(binds, Bind{
				Slot: l.modSlots[pb],
				Name: pb.Name.Value,
				Body: Thunk{Body: body, Frame: fb.size, Name: pb.Name.Value, Pos: syntax.NodePos(pb.Expression)},
			})
		}
		modBinds[modName] = binds
	}

	fb := &FrameBuilder{node: program}
	l.current = fb
	mainBody := l.lowerTail(program.Body)
	// The program body has no binding name, so it never appears in a trace and its
	// thunk is left anonymous.
	mainThunk := Thunk{Body: mainBody, Frame: fb.size, Name: ""}

	return mainThunk, modBinds
}

// reachableModuleBindings computes the set of module public bindings transitively
// referenced from the program body: a binding is live if the body uses it, or a
// live binding's right-hand side uses it. It reads the resolver's facts — a plain
// name resolving to a module binding, or any qualified name — so it needs no extra
// analysis, just a worklist closure over the "references" relation.
func reachableModuleBindings(program *syntax.Program, res *syntax.Resolution) map[*syntax.Binding]bool {
	reachable := make(map[*syntax.Binding]bool)
	var worklist []*syntax.Binding

	collectRefs := func(expr syntax.Expression) {
		syntax.Traverse(expr, func(n syntax.Node) {
			switch x := n.(type) {
			case *syntax.Name:
				if fact, ok := res.Uses[x]; ok && fact.Kind == syntax.ResolveModule {
					worklist = append(worklist, fact.Def.(*syntax.Binding))
				}
			case *syntax.QualifiedName:
				if b, ok := res.Quals[x]; ok {
					worklist = append(worklist, b)
				}
			}
		}, nil)
	}

	collectRefs(program.Body)
	for len(worklist) > 0 {
		b := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		if reachable[b] {
			continue
		}
		reachable[b] = true
		collectRefs(b.Expression)
	}
	return reachable
}

// lowerTail lowers an expression in demanded position.
func (l *Lowerer) lowerTail(expr syntax.Expression) Expr {
	switch e := expr.(type) {
	case *syntax.Operation:
		switch e.Operator {
		case "":
			return l.lowerApp(e)
		case ">", "<":
			return l.lowerPipe(e.Operator, e.Operands, syntax.NodePos(e))
		default: // "*>", "<*": a composition is a value
			return l.lowerValue(e)
		}
	case *syntax.Let:
		return l.lowerLet(e)
	default:
		return l.lowerValue(expr)
	}
}

// lowerArg lowers an expression in a lazy position (argument or data field).
func (l *Lowerer) lowerArg(expr syntax.Expression) Expr {
	switch e := expr.(type) {
	case *syntax.Operation:
		switch e.Operator {
		case "", ">", "<": // an application/pipe may force when entered → defer it
			return Thunk{Body: l.lowerTail(e), Pos: syntax.NodePos(e)}
		default: // a composition only builds a value → build it directly
			return l.lowerValue(e)
		}
	case *syntax.Let:
		return Thunk{Body: l.lowerTail(e), Pos: syntax.NodePos(e)}
	default:
		return l.lowerValue(expr)
	}
}

// lowerValue builds a value: a literal, a constructor, a name, a closure, or a
// composition. Compound parts are lowered with lowerArg so they stay lazy.
func (l *Lowerer) lowerValue(expr syntax.Expression) Expr {
	switch e := expr.(type) {
	case *syntax.NumberLiteral:
		return Num{Val: e.Value}

	case *syntax.StringLiteral:
		// Decode the literal once into a shared immutable cons list of code points.
		return Const{Val: value.FoldStringValue(e.Value)}

	case *syntax.TupleExpr:
		switch len(e.SubExpressions) {
		case 0:
			return Const{Val: value.EmptyTuple}
		case 2:
			return Cons{Head: l.lowerArg(e.SubExpressions[0]), Tail: l.lowerArg(e.SubExpressions[1])}
		default:
			var fields []Expr
			for _, sub := range e.SubExpressions {
				fields = append(fields, l.lowerArg(sub))
			}
			return Tuple{Fields: fields}
		}

	case *syntax.List:
		var current Expr = Const{Val: value.EmptyTuple}
		for i := len(e.SubExpressions) - 1; i >= 0; i-- {
			current = Cons{Head: l.lowerArg(e.SubExpressions[i]), Tail: current}
		}
		return current

	case *syntax.Name:
		return l.lowerName(e)

	case *syntax.QualifiedName:
		binding := l.res.Quals[e]
		return Var{Addr: Addr{Kind: AddrModule, Module: e.Module, Slot: l.modSlots[binding]}}

	case *syntax.Lambda:
		return l.lowerLambda(e)

	case *syntax.Operation:
		// Only compositions reach here; application and pipe are handled by callers.
		return l.lowerCompose(e)
	}

	panic(fmt.Sprintf("lowerValue: unexpected expression %T", expr))
}

// lowerApp flattens an application spine `f a1 … an` in tail position.
// When the head is a builtin name applied to exactly its arity of arguments,
// it emits Prim directly — no Builtin value is built, no spine saturation in
// the reducer. lowerArg still wraps any forcing sub-expression in a thunk;
// the machine forces those when executing the Prim opcode.
func (l *Lowerer) lowerApp(op *syntax.Operation) Expr {
	pos := syntax.NodePos(op)

	if name, ok := op.Operands[0].(*syntax.Name); ok {
		if fact := l.res.Uses[name]; fact.Kind == syntax.ResolveBuiltin {
			if b := value.InitialBuiltins[name.Value]; b != nil && len(op.Operands)-1 == b.Arity {
				args := make([]Expr, b.Arity)
				for k := range args {
					args[k] = l.lowerArg(op.Operands[k+1])
				}
				return Prim{Op: b.Prim, Args: args, Pos: pos}
			}
		}
	}

	head := l.lowerTail(op.Operands[0])
	var args []Expr
	for k := 1; k < len(op.Operands); k++ {
		args = append(args, l.lowerArg(op.Operands[k]))
	}
	return App{Head: head, Args: args, Pos: pos}
}

// lowerPipe lowers `>` / `<` chains to nested single-argument applications:
// `a > b > c` is `c (b a)` and `a < b < c` is `a (b c)`. The folded remainder is
// carried in a lazy thunk; the same whole-chain position is threaded through every
// level so error locations line up.
func (l *Lowerer) lowerPipe(operator string, operands []syntax.Expression, pos source.SourcePos) Expr {
	if len(operands) == 1 {
		return l.lowerTail(operands[0])
	}

	var head syntax.Expression
	var rest []syntax.Expression
	if operator == ">" {
		head, rest = operands[len(operands)-1], operands[:len(operands)-1]
	} else { // "<"
		head, rest = operands[0], operands[1:]
	}

	var arg Expr
	if len(rest) == 1 {
		arg = l.lowerArg(rest[0])
	} else {
		arg = Thunk{Body: l.lowerPipe(operator, rest, pos)}
	}
	return App{Head: l.lowerTail(head), Args: []Expr{arg}, Pos: pos}
}

// lowerCompose builds a Compose. Operands are functions applied lazily when
// the composition is, so each is lowered with lowerArg. Fns stays in source order;
// codegen reverses it for forward composition (see compile.go).
func (l *Lowerer) lowerCompose(op *syntax.Operation) Expr {
	var fns []Expr
	for _, operand := range op.Operands {
		fns = append(fns, l.lowerArg(operand))
	}
	return Compose{Forward: op.Operator == "*>", Fns: fns}
}

// lowerLet lowers a let in tail position. Slots for all bindings are allocated
// first so a right-hand side may refer to a later binding (or itself); the binding
// thunks share the frame by reference, which is what makes recursion work.
func (l *Lowerer) lowerLet(let *syntax.Let) Expr {
	slots := make([]int, len(let.Bindings))
	for i, b := range let.Bindings {
		slots[i] = l.current.allocate()
		l.locals[b] = localDef{frame: l.current, slot: slots[i]}
		l.current.setSlotName(slots[i], b.Name.Value)
	}

	binds := make([]Bind, len(let.Bindings))
	for i, b := range let.Bindings {
		body := l.lowerTail(b.Expression)
		binds[i] = Bind{
			Slot: slots[i],
			Name: b.Name.Value,
			Body: Thunk{Body: body, Name: b.Name.Value, Pos: syntax.NodePos(b.Expression)},
		}
	}
	return Let{Binds: binds, Body: l.lowerTail(let.Expression)}
}

// lowerLambda lowers a lambda into a Lambda node. Each case shares the closure
// frame (the cases reset the slot counter so they overlap, and Frame is the
// largest); the upvalue capture list accumulates across all cases.
func (l *Lowerer) lowerLambda(lam *syntax.Lambda) Expr {
	fb := &FrameBuilder{parent: l.current, node: lam}
	l.current = fb

	var cases []Case
	maxSize := 0
	for _, c := range lam.Cases {
		saved := fb.size
		pat := l.lowerPattern(c.Pattern)
		body := l.lowerTail(c.Expression)
		if fb.size > maxSize {
			maxSize = fb.size
		}
		cases = append(cases, Case{Pattern: pat, Body: body, Frame: fb.size - saved})
		fb.size = saved // the next case reuses the same slots
	}

	free := fb.free
	freeNames := fb.freeNames
	l.current = fb.parent
	return Lambda{Cases: cases, Free: free, FreeNames: freeNames, Frame: maxSize, NoMatch: noMatchPos(lam)}
}

// noMatchPos is the source span covering a lambda's whole pattern set, used to
// locate a non-exhaustive match.
func noMatchPos(lam *syntax.Lambda) source.SourcePos {
	if lam == nil || len(lam.Cases) == 0 {
		return source.SourcePos{}
	}
	cases := lam.Cases
	return cases[0].Pattern.FirstPos().To(cases[len(cases)-1].Pattern.LastPos())
}

func (l *Lowerer) lowerName(e *syntax.Name) Expr {
	fact := l.res.Uses[e]
	switch fact.Kind {
	case syntax.ResolveBuiltin:
		switch e.Value {
		case "stdin":
			return Stdin{}
		case "bstdin":
			return Stdin{Bytes: true}
		}
		return Const{Val: value.BuiltinValue(value.InitialBuiltins[e.Value])}

	case syntax.ResolveModule:
		return Var{Addr: Addr{Kind: AddrModule, Module: fact.Module.Name, Slot: l.modSlots[fact.Def.(*syntax.Binding)]}}

	case syntax.ResolveLocal:
		ld := l.locals[fact.Def]
		return Var{Addr: l.current.resolveUpvalue(ld.frame, ld.slot)}
	}
	panic("lowerName: unknown resolution kind")
}

// lowerPattern normalises a pattern: list patterns and string literals become
// nested arity-2 tuple (cons) patterns, so codegen only ever sees tuple/var/const
// patterns. Pattern variables allocate a frame slot here and keep their name for
// traces and show.
func (l *Lowerer) lowerPattern(p syntax.Pattern) Pattern {
	switch pat := p.(type) {
	case *syntax.Name:
		slot := l.current.allocate()
		l.locals[pat] = localDef{frame: l.current, slot: slot}
		l.current.setSlotName(slot, pat.Value)
		return PatternVar{Slot: slot, Name: pat.Value}

	case *syntax.NumberLiteral:
		return PatternConst{Val: value.NumberValue(pat.Value)}

	case *syntax.StringLiteral:
		var current Pattern = PatternTuple{Fields: nil}
		runes := []rune(pat.Value)
		for i := len(runes) - 1; i >= 0; i-- {
			current = PatternTuple{Fields: []Pattern{
				PatternConst{Val: value.NumberValue(float64(runes[i]))}, current,
			}}
		}
		return current

	case *syntax.TuplePattern:
		var fields []Pattern
		for _, sub := range pat.SubPatterns {
			fields = append(fields, l.lowerPattern(sub))
		}
		return PatternTuple{Fields: fields}

	case *syntax.ListPattern:
		var current Pattern = PatternTuple{Fields: nil}
		for i := len(pat.SubPatterns) - 1; i >= 0; i-- {
			current = PatternTuple{Fields: []Pattern{l.lowerPattern(pat.SubPatterns[i]), current}}
		}
		return current
	}
	panic(fmt.Sprintf("lowerPattern: unexpected pattern %T", p))
}
