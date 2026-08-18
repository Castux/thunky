// Package types infers a structural type for every expression in a Thunky
// program. The language is dynamically typed and has exactly one primitive
// (the number) and one constructor (the tuple), so there is nothing to check
// against — but programs are written with a shape in mind, and that shape is
// recoverable: it is whatever the literals, the builtins and the patterns pin
// down, propagated through applications.
//
// The domain is a join lattice rather than a set of disjoint type constructors,
// because a Thunky value legitimately has several shapes at once. A multi-case
// lambda accepts the empty tuple *and* a pair; `case` returns whichever branch
// ran. So a type here is a set of alternatives:
//
//	num          — can be a number
//	tuples[n]    — can be a tuple of arity n, with these field types
//	fun          — can be a function, with this argument and result type
//	top          — could be anything (the analysis gave up here)
//
// A type with no alternatives at all is a variable: nothing has constrained it
// yet. Joining is therefore total — it never fails, it only widens — which is
// what makes the analysis usable on a language where `{ 0 -> 'a', n -> n }` is
// perfectly ordinary code.
//
// Types are graph nodes joined by union-find, and the graph is allowed to be
// cyclic. That is what expresses a list: the argument of `length` is the empty
// tuple joined with a pair whose second field is that same node again, which
// the printer renders as [a]. No separate recursive-type machinery is needed —
// the cycle is the recursion.
package types

// Type is one node of the type graph. Nodes are merged by union-find: `link`
// points at the node this one was merged into, and all reading goes through
// find().
type Type struct {
	link *Type

	top    bool // anything at all; absorbs every other alternative
	num    bool
	tuples map[int][]*Type
	fun    *arrow

	// level is the depth of the binding this node was created inside. A node
	// belonging to an enclosing binding must not be copied when a use of a
	// let-bound name is instantiated — it is shared with the outer scope, and
	// copying it would break the tie between, say, a helper's argument and the
	// parameter of the function the helper was defined in. This is the standard
	// rank-based generalisation, and without it `fix = f -> let x = f x in x`
	// comes out as (a -> a) -> b.
	level int

	id int
}

type arrow struct{ arg, res *Type }

// Analyzer-wide state: the node counter, the current binding depth, and the
// budget that stops a runaway join from building an unbounded type.
type builder struct {
	nextID int
	joins  int
	budget int
	level  int
}

// genericLevel marks a node that belongs to no enclosing binding: it is
// quantified, and every use of the name it came from gets its own copy.
const genericLevel = int(^uint(0) >> 1)

func newBuilder() *builder { return &builder{budget: 2000000} }

// generalize marks everything a finished binding invented as generic. A node
// still at or below the current level was borrowed from an enclosing scope and
// stays where it is, which is what keeps a helper tied to the parameter of the
// function it was defined in.
func (b *builder) generalize(t *Type) {
	seen := map[*Type]bool{}
	var walk func(*Type)
	walk = func(t *Type) {
		t = find(t)
		if seen[t] {
			return
		}
		seen[t] = true
		if t.level > b.level {
			t.level = genericLevel
		}
		for _, fields := range t.tuples {
			for _, f := range fields {
				walk(f)
			}
		}
		if t.fun != nil {
			walk(t.fun.arg)
			walk(t.fun.res)
		}
	}
	walk(t)
}

func (b *builder) fresh() *Type {
	b.nextID++
	return &Type{id: b.nextID, level: b.level}
}

func (b *builder) num() *Type {
	t := b.fresh()
	t.num = true
	return t
}

func (b *builder) tuple(fields ...*Type) *Type {
	t := b.fresh()
	t.tuples = map[int][]*Type{len(fields): fields}
	return t
}

func (b *builder) fn(arg, res *Type) *Type {
	t := b.fresh()
	t.fun = &arrow{arg: arg, res: res}
	return t
}

func (b *builder) any() *Type {
	t := b.fresh()
	t.top = true
	return t
}

// list builds the cyclic type of a list whose elements are `elem`: the empty
// tuple joined with a pair of an element and the list itself.
func (b *builder) list(elem *Type) *Type {
	l := b.fresh()
	l.tuples = map[int][]*Type{0: {}, 2: {elem, l}}
	return l
}

func find(t *Type) *Type {
	if t == nil {
		return nil
	}
	root := t
	for root.link != nil {
		root = root.link
	}
	for t.link != nil {
		t.link, t = root, t.link
	}
	return root
}

// isVar reports whether nothing is known about t yet.
func isVar(t *Type) bool {
	t = find(t)
	return !t.top && !t.num && t.fun == nil && len(t.tuples) == 0
}

// join merges b into a, destructively, and returns the merged node. It is the
// lattice's least upper bound: the result admits every shape either side
// admitted. Merging happens before the fields are joined, so a cycle in the
// graph terminates instead of recursing forever.
func (bld *builder) join(a, b *Type) *Type {
	a, b = find(a), find(b)
	if a == b {
		return a
	}

	bld.joins++
	if bld.joins > bld.budget {
		a.top = true
		a.num, a.tuples, a.fun = false, nil, nil
		b.link = a
		return a
	}

	b.link = a
	if b.level < a.level {
		a.level = b.level
	}

	if b.top {
		a.top = true
	}
	if a.top {
		a.num, a.tuples, a.fun = false, nil, nil
		return a
	}

	a.num = a.num || b.num

	for arity, bFields := range b.tuples {
		aFields, ok := a.tuples[arity]
		if !ok {
			if a.tuples == nil {
				a.tuples = map[int][]*Type{}
			}
			a.tuples[arity] = bFields
			continue
		}
		for i := range aFields {
			aFields[i] = bld.join(aFields[i], bFields[i])
		}
	}

	if b.fun != nil {
		if a.fun == nil {
			a.fun = b.fun
		} else {
			// Domains are joined rather than intersected: this is an
			// approximation, and it is the one that keeps the result readable
			// when a name is bound to two different functions.
			a.fun.arg = bld.join(a.fun.arg, b.fun.arg)
			a.fun.res = bld.join(a.fun.res, b.fun.res)
		}
	}

	// Anything reachable from a node belongs to that node's binding at the
	// latest, so a merge that lowered the level has to push it down. Without
	// this, unifying an outer variable with a structure built here leaves the
	// structure's insides looking local, and they get quantified away: `fix`
	// comes out as (a -> a) -> b instead of (a -> a) -> a.
	seen := map[*Type]bool{a: true}
	for _, fields := range a.tuples {
		for _, f := range fields {
			clampLevel(f, a.level, seen)
		}
	}
	if a.fun != nil {
		clampLevel(a.fun.arg, a.level, seen)
		clampLevel(a.fun.res, a.level, seen)
	}

	return a
}

func clampLevel(t *Type, level int, seen map[*Type]bool) {
	t = find(t)
	if t.level <= level || seen[t] {
		return
	}
	seen[t] = true
	t.level = level
	for _, fields := range t.tuples {
		for _, f := range fields {
			clampLevel(f, level, seen)
		}
	}
	if t.fun != nil {
		clampLevel(t.fun.arg, level, seen)
		clampLevel(t.fun.res, level, seen)
	}
}

// instantiate copies a type graph so that a use of a let-bound name is
// independent of every other use. Nodes belonging to an enclosing binding are
// shared rather than copied — those are the monomorphic parts, tied to
// something still in scope. Cycles are preserved via the memo table, which is
// what keeps a list type a list.
func (bld *builder) instantiate(t *Type, memo map[*Type]*Type) *Type {
	t = find(t)
	if t.level != genericLevel {
		return t
	}
	if seen, ok := memo[t]; ok {
		return seen
	}

	fresh := bld.fresh()
	memo[t] = fresh
	fresh.top, fresh.num = t.top, t.num
	if t.tuples != nil {
		fresh.tuples = make(map[int][]*Type, len(t.tuples))
		for arity, fields := range t.tuples {
			copied := make([]*Type, len(fields))
			for i, f := range fields {
				copied[i] = bld.instantiate(f, memo)
			}
			fresh.tuples[arity] = copied
		}
	}
	if t.fun != nil {
		fresh.fun = &arrow{
			arg: bld.instantiate(t.fun.arg, memo),
			res: bld.instantiate(t.fun.res, memo),
		}
	}
	return fresh
}

// ---------------------------------------------------------------- printing

// String renders a type. Cyclic nodes get a mu binder, and the list shape —
// the empty tuple joined with a pair whose tail is the same type — is printed
// as [element] rather than spelled out.
