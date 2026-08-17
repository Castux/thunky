package types

import (
	"fmt"
	"sort"
	"strings"
)

// Rendering a type uses the language's own notation, and nothing else:
//
//	[]              the empty tuple, which is also the empty list
//	[a, b]          a tuple
//	[a; b]          a list of known length — nested pairs ending in []
//	a -> b          a function
//	a | b           a value that can be either (the join lattice's whole point)
//	Num             a number
//	?               anything (the analysis gave up here)
//
// There is deliberately no notation that *asserts* a list. A list is a cycle in
// the graph — the empty tuple joined with a pair whose tail is the same node —
// and printing that as `[a]` claims something stronger than the structure says,
// silently turning "terminated by []" and "goes on forever" into the same text.
// The two are different types and now read differently.
//
// Cycles are lifted out into named equations instead, abstracted over the parts
// that are not the recursion:
//
//	T1 a : [] | [a, T1 a]     a list
//	T2 a : [a, T2 a]          an infinite list, which is NOT the same type
//
// A Namer is shared across a whole report, so one shape gets one equation
// however many signatures mention it, and `map : (a -> b) -> T1 a -> T1 b` reads
// about as well as the old sugar while still being exactly true.

// An Equation is a named recursive type. Params are its formal parameters, so
// the whole thing reads `Name params : Body`.
type Equation struct {
	Name   string
	Params []string
	Body   string
}

// Header renders the left-hand side, `T1 a b`.
func (e Equation) Header() string {
	if len(e.Params) == 0 {
		return e.Name
	}
	return e.Name + " " + strings.Join(e.Params, " ")
}

// A Namer renders types and accumulates the recursive-type equations they need.
type Namer struct {
	eqs   []Equation
	byKey map[string]int // canonical shape -> index into eqs
}

func NewNamer() *Namer { return &Namer{byKey: map[string]int{}} }

// Equations returns the equations every type rendered so far depends on, in the
// order they were first needed.
func (n *Namer) Equations() []Equation { return n.eqs }

// String renders one type. Variable names restart at `a` for each call — they
// are local to a signature — while equation names are shared.
func (n *Namer) String(t *Type) string {
	p := &printer{
		namer:   n,
		names:   map[*Type]string{},
		onPath:  map[*Type]bool{},
		recNeed: map[*Type]bool{},
	}
	p.mark(find(t), map[*Type]bool{})
	return p.print(find(t))
}

// String renders a type on its own, with any equations it needs inlined as `mu`
// binders instead of hoisted. Used where there is nowhere to put a preamble.
func String(t *Type) string {
	p := &printer{
		names:   map[*Type]string{},
		onPath:  map[*Type]bool{},
		recNeed: map[*Type]bool{},
	}
	p.mark(find(t), map[*Type]bool{})
	return p.print(find(t))
}

type printer struct {
	namer   *Namer // nil to inline recursion as mu instead of naming it
	names   map[*Type]string
	onPath  map[*Type]bool
	recNeed map[*Type]bool
	nextVar int
	nextRec int
}

// mark finds the nodes that are re-entered while printing, i.e. the ones that
// are recursive and so need either an equation or a mu binder.
func (p *printer) mark(t *Type, path map[*Type]bool) {
	t = find(t)
	if path[t] {
		p.recNeed[t] = true
		return
	}
	path[t] = true
	for _, fields := range t.tuples {
		for _, f := range fields {
			p.mark(f, path)
		}
	}
	if t.fun != nil {
		p.mark(t.fun.arg, path)
		p.mark(t.fun.res, path)
	}
	delete(path, t)
}

var varNames = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m"}

func varName(i int) string {
	if i < len(varNames) {
		return varNames[i]
	}
	return fmt.Sprintf("t%d", i)
}

func (p *printer) varName(t *Type) string {
	if n, ok := p.names[t]; ok {
		return n
	}
	n := varName(p.nextVar)
	p.nextVar++
	p.names[t] = n
	return n
}

// fixedList reports whether t is a list of known length — a chain of pairs
// ending in the empty tuple — and returns its element types. This is the one
// abbreviation kept, because it is exact: [a; b] and [a, [b, []]] are the same
// type, written the two ways the language writes the value.
func fixedList(t *Type) ([]*Type, bool) {
	var elems []*Type
	for depth := 0; depth < 24; depth++ {
		t = find(t)
		if t.top || t.num || t.fun != nil || len(t.tuples) != 1 {
			return nil, false
		}
		if _, ok := t.tuples[0]; ok {
			return elems, len(elems) > 0
		}
		pair, ok := t.tuples[2]
		if !ok {
			return nil, false
		}
		elems = append(elems, pair[0])
		t = pair[1]
	}
	return nil, false
}

func isArrow(t *Type) bool {
	t = find(t)
	return t.fun != nil && !t.top && !t.num && len(t.tuples) == 0
}

// atomic reports whether a rendered type can stand as the argument of a type
// application without brackets. Variables, Num, ? and a single self-delimiting
// [...] group can; arrows, unions and applications cannot, since application
// binds tighter than either and `T [a] ` must not read as two arguments.
func atomic(s string) bool {
	if !strings.ContainsAny(s, " |") {
		return true
	}
	if strings.HasPrefix(s, "[") {
		depth := 0
		for i, r := range s {
			switch r {
			case '[':
				depth++
			case ']':
				depth--
			}
			if depth == 0 {
				return i == len(s)-1
			}
		}
	}
	return false
}

// reaches reports whether target is reachable from t, following fields and
// arrows. Used to decide which parts of a recursive type are the recursion and
// which are parameters.
func reaches(t, target *Type, seen map[*Type]bool) bool {
	t = find(t)
	if t == target {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	for _, fields := range t.tuples {
		for _, f := range fields {
			if reaches(f, target, seen) {
				return true
			}
		}
	}
	if t.fun != nil {
		return reaches(t.fun.arg, target, seen) || reaches(t.fun.res, target, seen)
	}
	return false
}

const shapeDepthLimit = 12

// shape builds a canonical skeleton of a recursive node: the recursion itself
// becomes "@", and every maximal part that cannot reach the recursion becomes a
// numbered hole. Two recursive types with the same skeleton are the same shape
// and share an equation; the holes are what the equation abstracts over.
func shape(r *Type) (skel string, params []*Type, ok bool) {
	idx := map[*Type]int{}
	var render func(t *Type, depth int) (string, bool)
	render = func(t *Type, depth int) (string, bool) {
		t = find(t)
		if depth > shapeDepthLimit {
			return "", false
		}
		if depth > 0 {
			if t == r {
				return "@", true
			}
			if !reaches(t, r, map[*Type]bool{}) {
				if i, seen := idx[t]; seen {
					return fmt.Sprintf("#%d", i), true
				}
				i := len(params)
				idx[t] = i
				params = append(params, t)
				return fmt.Sprintf("#%d", i), true
			}
		}

		var parts []string
		if t.top {
			parts = append(parts, "?")
		}
		if t.num {
			parts = append(parts, "Num")
		}
		arities := make([]int, 0, len(t.tuples))
		for arity := range t.tuples {
			arities = append(arities, arity)
		}
		sort.Ints(arities)
		for _, arity := range arities {
			if arity == 0 {
				parts = append(parts, "[]")
				continue
			}
			pieces := make([]string, arity)
			for i, f := range t.tuples[arity] {
				s, k := render(f, depth+1)
				if !k {
					return "", false
				}
				pieces[i] = s
			}
			parts = append(parts, "["+strings.Join(pieces, ", ")+"]")
		}
		if t.fun != nil {
			arg, k1 := render(t.fun.arg, depth+1)
			res, k2 := render(t.fun.res, depth+1)
			if !k1 || !k2 {
				return "", false
			}
			if strings.Contains(arg, " -> ") || strings.Contains(arg, " | ") {
				arg = "(" + arg + ")"
			}
			parts = append(parts, arg+" -> "+res)
		}
		if len(parts) == 0 {
			// A bare variable inside the recursion that cannot reach it was
			// already turned into a hole above; reaching here means the node is
			// the recursion's own empty shell, which says nothing.
			return "", false
		}
		return strings.Join(parts, " | "), true
	}

	skel, ok = render(r, 0)
	if !ok || !strings.Contains(skel, "@") {
		return "", nil, false
	}
	return skel, params, true
}

// equation interns a recursive node's shape and returns a reference to it,
// rendering the arguments in the caller's own variable namespace.
func (p *printer) equation(t *Type) (string, bool) {
	if p.namer == nil {
		return "", false
	}
	skel, params, ok := shape(t)
	if !ok {
		return "", false
	}

	n := p.namer
	i, seen := n.byKey[skel]
	if !seen {
		formals := make([]string, len(params))
		for j := range params {
			formals[j] = varName(j)
		}
		body := skel
		// "@" is the equation referring to itself, so it is the whole left-hand
		// side applied to its own parameters. It needs no brackets: it only ever
		// appears inside a [...] group, beside a `|`, or as an arrow operand, and
		// application binds tighter than all three.
		self := strings.Join(append([]string{fmt.Sprintf("T%d", len(n.eqs)+1)}, formals...), " ")
		for j, f := range formals {
			body = strings.ReplaceAll(body, fmt.Sprintf("#%d", j), f)
		}
		body = strings.ReplaceAll(body, "@", self)
		i = len(n.eqs)
		n.eqs = append(n.eqs, Equation{
			Name:   fmt.Sprintf("T%d", i+1),
			Params: formals,
			Body:   body,
		})
		n.byKey[skel] = i
	}

	eq := n.eqs[i]
	ref := eq.Name
	for _, arg := range params {
		s := p.print(find(arg))
		if !atomic(s) {
			s = "(" + s + ")"
		}
		ref += " " + s
	}
	return ref, true
}

func (p *printer) print(t *Type) string {
	t = find(t)

	if name, ok := p.names[t]; ok && p.onPath[t] {
		return name
	}

	if p.recNeed[t] && !p.onPath[t] {
		if ref, ok := p.equation(t); ok {
			return ref
		}
	}

	var recName string
	if p.recNeed[t] {
		p.nextRec++
		recName = fmt.Sprintf("r%d", p.nextRec)
		p.names[t] = recName
	}
	p.onPath[t] = true
	defer func() { p.onPath[t] = false }()

	body := p.body(t)
	if recName != "" && strings.Contains(body, recName) {
		return "mu " + recName + "." + body
	}
	return body
}

func (p *printer) body(t *Type) string {
	if t.top {
		return "?"
	}

	if elems, ok := fixedList(t); ok {
		pieces := make([]string, len(elems))
		for i, e := range elems {
			pieces[i] = p.print(e)
		}
		if len(pieces) == 1 {
			return "[" + pieces[0] + ";]"
		}
		return "[" + strings.Join(pieces, "; ") + "]"
	}

	var parts []string
	if t.num {
		parts = append(parts, "Num")
	}

	arities := make([]int, 0, len(t.tuples))
	for arity := range t.tuples {
		arities = append(arities, arity)
	}
	sort.Ints(arities)
	for _, arity := range arities {
		fields := t.tuples[arity]
		if arity == 0 {
			parts = append(parts, "[]")
			continue
		}
		pieces := make([]string, len(fields))
		for i, f := range fields {
			pieces[i] = p.print(f)
		}
		parts = append(parts, "["+strings.Join(pieces, ", ")+"]")
	}

	if t.fun != nil {
		arg := p.print(t.fun.arg)
		// An argument that is itself a function needs bracketing, and so does a
		// union, which would otherwise read as an alternative of the whole
		// arrow rather than of its argument.
		if isArrow(t.fun.arg) || strings.Contains(arg, " | ") {
			arg = "(" + arg + ")"
		}
		parts = append(parts, arg+" -> "+p.print(t.fun.res))
	}

	if len(parts) == 0 {
		return p.varName(t)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts, " | ")
}
