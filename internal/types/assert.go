package types

import (
	"fmt"

	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/syntax"
)

// Propagating an assertion from a call site to the caller.
//
// A `!` in a signature says the author is assuming something the analysis cannot
// confirm. That assumption does not stop at the function that declares it: a
// caller who passes an argument the assumption might not hold for is making the
// same assumption, at one remove, and its own signature should say so.
//
//	--> head : List a! -> a
//	nth = n -> drop n *> head        `drop n l` is a List: it may be empty
//	--> nth : Num -> List a! -> a    so nth inherits the assumption
//
// Without this, a library could launder every assumption it makes by wrapping
// it in one more function, and the marks would only ever appear on the handful
// of definitions that pattern match incompletely.
//
// # Discharging
//
// Most call sites are safe for a reason the analysis *can* see, and demanding a
// mark for them would make the mark meaningless. A call discharges the
// assumption when the argument is *built right there* — a tuple, a list literal,
// a string — in a shape the callee matches:
//
//	lastSafe = { [] -> none, [h, t] -> some (last [h, t]) }
//
// `last` matches a pair and nothing else; `[h, t]` is written as a pair; so the
// call is safe, and the reason the reader believes it is the reason the analysis
// accepts it. The same rule covers `char '0'`, since a non-empty string literal
// is a pair and never the empty list.
//
// The shape is read off the *source*, not off the argument's inferred type,
// because by the end of the walk that type no longer says anything. Passing
// `[h, t]` to `last` joins the two, so the argument node ends up carrying last's
// `List a` — empty case and all. The narrower fact exists only at the syntax.
//
// The callee's *patterns* are what license this, not its signature: they are the
// evidence for what it really handles. A binding that is an alias inherits them,
// so `char = head` discharges exactly as head does.
//
// # What cannot be discharged
//
// A binding whose `!` is not backed by patterns has an assumption the analysis
// cannot characterise, so nothing discharges it and every caller inherits it.
// `nth` is the example, and it is not a limitation to fix: its assumption is a
// relation between the index and the length, and `nth 5 [1; 2; 3]` fails with an
// argument type that is a perfectly good non-empty list. A rule keyed on the
// argument's shape would call that call site safe. Refusing to discharge is the
// honest answer, and it is why nth's callers must mark.

// assertion is one asserted argument position of a callee.
type assertion struct {
	name     string
	position int          // 1-based, for the message
	matched  map[int]bool // arities the callee's patterns match; nil if none
	hasPats  bool         // whether patterns were found at all
}

// checkAssertions reports call sites that inherit an assertion without saying so.
func (in *inferrer) checkAssertions(program *syntax.Program, modules map[string]*syntax.Module,
	given map[*syntax.Binding]givenSig) {

	c := &assertChecker{in: in, given: given, cache: map[*syntax.Binding][]assertion{}}

	// Every top-level binding is walked with itself as the responsible party: a
	// nested `let` has no signature to mark, so an assumption made inside one
	// belongs to the binding that contains it.
	for _, mod := range sortedModules(modules) {
		for _, b := range mod.PublicBindings {
			c.owner = b
			c.alias(b)
			c.walk(b.Expression)
		}
	}
	c.owner = nil
	c.walk(program.Body)
}

type assertChecker struct {
	in    *inferrer
	given map[*syntax.Binding]givenSig
	cache map[*syntax.Binding][]assertion
	owner *syntax.Binding
}

// alias handles a binding that is nothing but another name — `char = head`. There
// is no call site to check, and without this the alias would launder the
// assumption: every caller would see char's signature and none would see head's.
func (c *assertChecker) alias(b *syntax.Binding) {
	name, ok := b.Expression.(*syntax.Name)
	if !ok {
		return
	}
	fact, ok := c.in.res.Uses[name]
	if !ok {
		return
	}
	target, ok := fact.Def.(*syntax.Binding)
	if !ok || target == b {
		return
	}
	for _, a := range c.assertions(target) {
		c.demand(a, syntax.NodePos(name))
	}
}

func (c *assertChecker) walk(e syntax.Expression) {
	switch node := e.(type) {
	case *syntax.Let:
		for _, b := range node.Bindings {
			// A nested binding answers for itself only if it has a signature to
			// answer with; otherwise the assumption belongs to whatever encloses it.
			outer := c.owner
			if _, annotated := c.given[b]; annotated {
				c.owner = b
			}
			c.walk(b.Expression)
			c.owner = outer
		}
		c.walk(node.Expression)
	case *syntax.Lambda:
		for _, cs := range node.Cases {
			c.walk(cs.Expression)
		}
	case *syntax.TupleExpr:
		for _, x := range node.SubExpressions {
			c.walk(x)
		}
	case *syntax.List:
		for _, x := range node.SubExpressions {
			c.walk(x)
		}
	case *syntax.Operation:
		c.operation(node)
		for _, x := range node.Operands {
			c.walk(x)
		}
	}
}

// operation checks the call sites in one operation. Each form applies a function
// to arguments in a different order, so each says which operand is the callee and
// which types flow into it.
func (c *assertChecker) operation(op *syntax.Operation) {
	switch op.Operator {
	case "": // f a b c
		for i, arg := range op.Operands[1:] {
			c.callSite(op.Operands[0], i, arg, syntax.NodePos(arg))
		}
	case ">": // a > f > g
		for _, fn := range op.Operands[1:] {
			c.callSite(fn, 0, op.Operands[0], syntax.NodePos(fn))
		}
	case "<": // f < g < x
		last := len(op.Operands) - 1
		for i := last - 1; i >= 0; i-- {
			c.callSite(op.Operands[i], 0, op.Operands[last], syntax.NodePos(op.Operands[i]))
		}
	case "*>", "<*":
		// Composition: the argument is whatever the neighbouring function
		// produced, and there is no expression for it at all. Nothing here can be
		// discharged, so the assumption stands and the caller inherits it.
		for _, fn := range op.Operands {
			c.callSite(fn, 0, nil, syntax.NodePos(fn))
		}
	}
}

// callSite reports if `fn` asserts the position this argument fills and the
// argument does not discharge it.
func (c *assertChecker) callSite(fn syntax.Expression, argIndex int, arg syntax.Expression, pos source.SourcePos) {
	callee, applied, ok := c.resolve(fn)
	if !ok {
		return
	}
	position := applied + argIndex
	for _, a := range c.assertions(callee) {
		if a.position != position+1 {
			continue
		}
		if discharged(a, arg) {
			continue
		}
		c.demand(a, pos)
	}
}

// resolve reads a callee expression as a binding plus the number of arguments
// already applied to it, so that `l > nth 2` is understood as nth's second
// argument rather than its first.
func (c *assertChecker) resolve(e syntax.Expression) (*syntax.Binding, int, bool) {
	switch node := e.(type) {
	case *syntax.Name:
		if fact, ok := c.in.res.Uses[node]; ok {
			if b, ok := fact.Def.(*syntax.Binding); ok {
				return b, 0, true
			}
		}
	case *syntax.Operation:
		if node.Operator == "" && len(node.Operands) > 0 {
			if b, applied, ok := c.resolve(node.Operands[0]); ok {
				return b, applied + len(node.Operands) - 1, true
			}
		}
	}
	return nil, 0, false
}

// assertions lists the asserted argument positions of a binding, with the
// evidence for what it really accepts.
func (c *assertChecker) assertions(b *syntax.Binding) []assertion {
	if as, ok := c.cache[b]; ok {
		return as
	}
	c.cache[b] = nil // guard against an alias cycle

	sig, ok := c.given[b]
	if !ok {
		return nil
	}
	var as []assertion
	claim, position := sig.sig.Pat, 0
	for claim != nil && claim.fun != nil {
		position++
		if arg := claim.fun.arg; arg != nil && arg.asserted {
			matched, hasPats := c.matchedAt(b, position-1)
			as = append(as, assertion{
				name:     sig.sig.Name,
				position: position,
				matched:  matched,
				hasPats:  hasPats,
			})
		}
		claim = claim.fun.res
	}
	c.cache[b] = as
	return as
}

// matchedAt reports the arities the binding's own patterns match at an argument
// position. An alias — a body that is just another binding's name — inherits
// that binding's patterns, which is what makes `char = head` behave like head.
func (c *assertChecker) matchedAt(b *syntax.Binding, position int) (map[int]bool, bool) {
	body := b.Expression
	for i := 0; ; i++ {
		if name, ok := body.(*syntax.Name); ok && i == 0 {
			if fact, ok := c.in.res.Uses[name]; ok {
				if alias, ok := fact.Def.(*syntax.Binding); ok && alias != b {
					return c.matchedAt(alias, position)
				}
			}
			return nil, false
		}
		lam, ok := body.(*syntax.Lambda)
		if !ok {
			return nil, false
		}
		if i == position {
			byArity := map[int][][]syntax.Pattern{}
			literals := false
			for _, cs := range lam.Cases {
				if _, isVar := cs.Pattern.(*syntax.Name); isVar {
					return nil, false // matches anything: no evidence of a restriction
				}
				collectArities(cs.Pattern, byArity, &literals)
			}
			matched := map[int]bool{}
			for arity := range byArity {
				matched[arity] = true
			}
			return matched, true
		}
		if len(lam.Cases) != 1 {
			return nil, false
		}
		body = lam.Cases[0].Expression
	}
}

// discharged reports whether the argument is written in a shape the callee is
// known to match. Anything but a literal or a constructed cell is unknown, and
// unknown does not discharge.
func discharged(a assertion, arg syntax.Expression) bool {
	if !a.hasPats || arg == nil {
		return false
	}
	arity, ok := writtenArity(arg)
	return ok && a.matched[arity]
}

// writtenArity reads the arity an expression is written as. `[a, b]` is a pair;
// `[a; b]` is a list, which is a pair at this level; a non-empty string is a list
// of code points, so also a pair. Empty ones are the empty tuple.
func writtenArity(e syntax.Expression) (int, bool) {
	switch node := e.(type) {
	case *syntax.TupleExpr:
		return len(node.SubExpressions), true
	case *syntax.List:
		if len(node.SubExpressions) == 0 {
			return 0, true
		}
		return 2, true
	case *syntax.StringLiteral:
		if node.Value == "" {
			return 0, true
		}
		return 2, true
	}
	return 0, false
}

// demand reports a call site whose caller does not acknowledge the assumption it
// inherits. A caller that already carries a mark has said so, wherever it chose
// to put it: the analysis cannot attribute an inherited assumption to one of the
// caller's own argument positions, so it asks only that the signature admit it.
func (c *assertChecker) demand(a assertion, pos source.SourcePos) {
	if c.owner != nil {
		if sig, ok := c.given[c.owner]; ok && countAsserted(sig.sig.Pat, map[*pattern]bool{}) > 0 {
			return
		}
	}
	what := fmt.Sprintf("argument %d of %s is asserted with `!`", a.position, a.name)
	if c.owner == nil {
		c.in.warnings = append(c.in.warnings, Warning{
			Message: what + ", and nothing here rules the assumption out",
			Pos:     pos,
		})
		return
	}
	c.in.warnings = append(c.in.warnings, Warning{
		Message: fmt.Sprintf("%s, and nothing here rules the assumption out, so %s inherits it; "+
			"mark %s with `!` too, or call something total",
			what, c.owner.Name.Value, c.owner.Name.Value),
		Pos: pos,
	})
}
