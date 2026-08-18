package types

import (
	"fmt"
	"sort"
)

// Containment: the relation a real check needs, and the one the signature-driven
// analysis is built on.
//
// The older question was *disjointness* — do these two types have no shape in
// common? That reports a number meeting a tuple and nothing else, which is the
// right question for a report that only wants to name outright contradictions,
// and the wrong one for a checker. `Maybe a` and `List a` overlap, on `[]`, so
// disjointness is silent about `reverse (some 5)`; and a 2-tuple of pairs
// overlaps a list at the top level, so it is silent about `length [[1,2],[3,4]]`
// too — which crashes with `no pattern matched value 4`.
//
// The question here is instead: does `got` admit any shape `want` does not? That
// is a subtyping test, and it has two properties disjointness does not.
//
//   - It is recursive by necessity, not by choice. `[[1,2],[3,4]]` is only wrong
//     two levels down, where a number stands where a list has to be.
//   - It has variance. `got <: want` for functions means the supplied function
//     must accept *at least* what the declaration promises to pass it, so the
//     argument position recurses with the two sides swapped. Disjointness is
//     symmetric and could ignore this; containment cannot, and getting it wrong
//     would reject correct higher-order code, which in `list` is most of it.
//
// What keeps it quiet is knownness, not depth: a variable or `?` on either side
// ends the descent, because neither side is then claiming anything that could be
// violated. That is the same discipline the old check used, applied at every
// level rather than only at the top.

type containKey struct{ want, got *Type }

// violation is a position where got admits something want does not.
type violation struct {
	path  string   // "" for the type as a whole
	extra []string // the kinds got has and want lacks, in the report's notation
}

// admits reports the first position at which `got` admits a shape that `want`
// does not. It errs toward silence: an unknown on either side ends the descent,
// since nothing there can be violated.
func admits(want, got *Type, seen map[containKey]bool) (violation, bool) {
	want, got = find(want), find(got)

	// `want` admitting anything cannot be violated; nothing known about `want`
	// claims nothing; nothing known about `got` cannot violate a claim. A
	// position the signature wrote as a variable claims nothing either, however
	// concrete this call site has since made it — on either side, since the two
	// swap under an arrow.
	if want == got || want.top || got.top || want.fromVar || got.fromVar ||
		want.asserted || isVar(want) || isVar(got) {
		return violation{}, true
	}

	// Coinduction: both graphs may be cyclic, and a list is a cycle. Assuming the
	// pair holds while checking it is what makes `List Num <: List a` terminate.
	key := containKey{want, got}
	if seen[key] {
		return violation{}, true
	}
	seen[key] = true

	// A consumed variable is concrete only because an earlier argument made it
	// so, and it goes on collecting from every argument that mentions it. What
	// can be said is that the alternatives must not be disjoint — a callback
	// taking numbers cannot be fed lists — and no more: descending would report
	// `map ([a; b; c] -> …) rows` for matching a narrower shape than `List a`
	// admits, which is how the library is written.
	if want.weak {
		if conflicts(want, got) {
			return violation{extra: extraKinds(want, got)}, false
		}
		return violation{}, true
	}

	if extra := extraKinds(want, got); len(extra) > 0 {
		return violation{extra: extra}, false
	}

	// Same kinds: descend. Fields are covariant.
	for _, arity := range sortedArities(got.tuples) {
		wf, gf := want.tuples[arity], got.tuples[arity]
		if len(wf) != len(gf) {
			continue
		}
		for i := range gf {
			if v, ok := admits(wf[i], gf[i], seen); !ok {
				return v.under(fmt.Sprintf("field %d of the %s", i+1, tupleWord(arity))), false
			}
		}
	}

	if want.fun != nil && got.fun != nil {
		// Contravariance. The supplied function is used at the declared domain, so
		// it has to accept everything the declaration says will arrive: the roles
		// swap, and `want` becomes what must be admitted.
		if v, ok := admits(got.fun.arg, want.fun.arg, seen); !ok {
			return v.under("the argument"), false
		}
		if v, ok := admits(want.fun.res, got.fun.res, seen); !ok {
			return v.under("the result"), false
		}
	}

	return violation{}, true
}

func (v violation) under(step string) violation {
	return violation{path: joinPath(v.path, step), extra: v.extra}
}

// describe renders the violation as a clause to be appended to a message that
// has already named the two types.
func (v violation) describe() string {
	if v.path == "" {
		return ""
	}
	return fmt.Sprintf(" — %s: %s is not admitted", v.path, describeKinds(v.extra))
}

// extraKinds lists the alternatives got has and want lacks.
func extraKinds(want, got *Type) []string {
	var extra []string
	if got.num && !want.num {
		extra = append(extra, "Num")
	}
	for _, arity := range sortedArities(got.tuples) {
		if _, ok := want.tuples[arity]; !ok {
			extra = append(extra, fmt.Sprintf("[%d]", arity))
		}
	}
	if got.fun != nil && want.fun == nil {
		extra = append(extra, "->")
	}
	return extra
}

func sortedArities(m map[int][]*Type) []int {
	out := make([]int, 0, len(m))
	for a := range m {
		out = append(out, a)
	}
	sort.Ints(out)
	return out
}

// --- a signature as a type ---------------------------------------------------

// sigType builds the type graph a signature describes, so that a declared type
// can be used the way an inferred one is: instantiated at each call site,
// unified with the argument, and read back for the result.
//
// Every node it makes is generic, so each use gets its own copy and one
// signature can serve `map` at two element types in the same expression. The
// variable *names* matter here and nowhere else: both `a`s in `List a -> List a`
// become one node, which is what carries `Num` from the argument to the result.
// An unnamed position — a hole from a declaration's parameter that the signature
// did not name — gets a fresh variable each time, since there is nothing to tie
// it to.
func (b *builder) sigType(p *pattern) *Type {
	t := b.sigNode(p, consumedVars(p), map[string]*Type{}, map[*pattern]*Type{})
	markGeneric(t, map[*Type]bool{})
	return t
}

func (b *builder) sigNode(p *pattern, consumed map[string]bool,
	vars map[string]*Type, memo map[*pattern]*Type) *Type {

	if p == nil || p.self {
		return b.fresh()
	}
	if p.hole >= 0 {
		if p.varName == "" {
			t := b.fresh()
			t.fromVar = true
			return t
		}
		if t, ok := vars[p.varName]; ok {
			return t
		}
		t := b.fresh()
		t.fromVar = !consumed[p.varName]
		t.weak = !t.fromVar
		vars[p.varName] = t
		return t
	}
	if t, ok := memo[p]; ok {
		return t
	}

	t := b.fresh()
	memo[p] = t // before the fields, so a cyclic declaration ties its own knot
	t.top, t.num, t.asserted = p.top, p.num, p.asserted
	if p.tuples != nil {
		t.tuples = make(map[int][]*Type, len(p.tuples))
		for arity, fields := range p.tuples {
			fs := make([]*Type, len(fields))
			for i, f := range fields {
				fs[i] = b.sigNode(f, consumed, vars, memo)
			}
			t.tuples[arity] = fs
		}
	}
	if p.fun != nil {
		t.fun = &arrow{
			arg: b.sigNode(p.fun.arg, consumed, vars, memo),
			res: b.sigNode(p.fun.res, consumed, vars, memo),
		}
	}
	return t
}

// consumedVars names the type variables a signature does not merely carry.
//
// Two arguments meeting at the same variable are joined, not required to agree,
// and for most signatures that is right: `if : Num -> a -> a -> a` takes `a`
// from whichever branch comes first, and `if c (some x) none` is ordinary code
// whose result is the union. Both branches are *sources*; the callee hands the
// value straight back and never looks at it.
//
// `map : (a -> b) -> List a -> List b` is the other case. `a` is supplied by the
// caller in `List a`, and handed back to the caller's own function in `a -> b`,
// before the call returns. Those two have to agree, or the callback is applied
// to something it does not match — and joining them would let
// `map (n -> add n 1) [[1; 2]; [3; 4]]` through.
//
// The two are told apart by polarity. A variable is *consumed* when it occurs
// positively somewhere other than the result spine: positive means the callee
// produces a value there, and off the spine means it does so before returning.
// `a` in `if` occurs positively only as the result; `a` in `map` occurs
// positively as the callback's domain, which is inside an argument.
func consumedVars(p *pattern) map[string]bool {
	type key struct {
		p              *pattern
		positive, tail bool
	}
	out := map[string]bool{}
	seen := map[key]bool{}

	var walk func(p *pattern, positive, tail bool)
	walk = func(p *pattern, positive, tail bool) {
		if p == nil || p.self {
			return
		}
		if p.hole >= 0 {
			if p.varName != "" && positive && !tail {
				out[p.varName] = true
			}
			return
		}
		k := key{p, positive, tail}
		if seen[k] {
			return
		}
		seen[k] = true

		for _, fields := range p.tuples {
			for _, f := range fields {
				walk(f, positive, tail)
			}
		}
		if p.fun != nil {
			// The domain flips polarity and leaves the result spine for good.
			walk(p.fun.arg, !positive, false)
			walk(p.fun.res, positive, tail)
		}
	}
	walk(p, true, true)
	return out
}

func markGeneric(t *Type, seen map[*Type]bool) {
	t = find(t)
	if seen[t] {
		return
	}
	seen[t] = true
	t.level = genericLevel
	for _, fields := range t.tuples {
		for _, f := range fields {
			markGeneric(f, seen)
		}
	}
	if t.fun != nil {
		markGeneric(t.fun.arg, seen)
		markGeneric(t.fun.res, seen)
	}
}

// explain renders the violation for a message that has not named the two types,
// so it has to say where and what by itself.
func (v violation) explain() string {
	return fmt.Sprintf("%s: %s is not admitted", where2(v.path), describeKinds(v.extra))
}
