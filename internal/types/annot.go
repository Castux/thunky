package types

import (
	"sort"

	"github.com/Castux/thunky/internal/source"
)

// Type annotations live in comments, so they stay optional and the language
// proper is untouched. A comment whose first token is `-->` is read by the
// analyser; every other comment is prose.
//
//	--> List a = [] | [a, List a]
//	--> Maybe a = [] | [a]
//	--> Point = [Num, Num]
//
// Two conventions do the work of a keyword, both already true of the codebase:
// a capitalised name is a type (no value binding in the standard library starts
// with a capital, and the printer already emits `Num`), and `=` means "is
// defined as" exactly as it does for a binding. The arrow points forward,
// which is also how a future annotation would attach to the binding below it.
//
// Deleting the dashes leaves a line that could become real syntax without
// reserving a keyword, which is the point of choosing this spelling.
//
// A declaration is a *name for a shape*. It changes no inference: the analysis
// runs exactly as before and the declaration only decides what the shape is
// called when printed. So an annotation cannot make a working program fail to
// analyse, and cannot narrow what a function accepts.

const annotMarker = "-->"

// A Decl is one parsed type declaration.
type Decl struct {
	// Mod is the module that declared it, "" for the program. Used to prefer a
	// module's own name when printing its bindings.
	Mod    string
	Name   string
	Params []string
	Body   string   // the source text, printed back verbatim
	Pat    *pattern // the shape to match nodes against
	Pos    source.SourcePos
}

// A pattern mirrors Type's alternative set, plus two extras: a hole standing for
// one of the declaration's parameters, and self, standing for the declaration
// applied to its own parameters. Mirroring Type is what makes matching a
// straightforward structural walk.
type pattern struct {
	hole int  // >= 0: this whole position is parameter #hole
	self bool // the declared type, recursively

	// asserted is set by a `!` suffix: the author claims this position beyond what
	// the analysis can confirm, so exhaustiveness is not required of it. It marks
	// an assumption rather than a fact, and the report counts them.
	asserted bool

	top    bool
	num    bool
	tuples map[int][]*pattern
	fun    *patArrow
}

type patArrow struct{ arg, res *pattern }

func newPattern() *pattern { return &pattern{hole: -1} }

// --- matching a node against a declaration -----------------------------------

// matchDecl tries to read node t as the declared type, returning the arguments
// its parameters stand for. Alternative sets must agree exactly: `[] | [a]` does
// not match a node that is also a number, because that node is a different type.
func matchDecl(t *Type, d Decl) ([]*Type, bool) {
	root := find(t)
	binds := make([]*Type, len(d.Params))
	seen := map[[2]any]bool{}
	if !matchPat(root, d.Pat, root, d, binds, seen) {
		return nil, false
	}
	for _, b := range binds {
		if b == nil {
			return nil, false // a parameter the shape never pinned down
		}
	}
	return binds, true
}

func matchPat(t *Type, p *pattern, root *Type, d Decl, binds []*Type, seen map[[2]any]bool) bool {
	t = find(t)

	if p.self {
		if t == root {
			return true
		}
		// A finite unrolling of a recursive type is the same regular tree, so a
		// self position also matches a *separate* node that reads as the
		// declaration in its own right. Inference produces such nodes routinely:
		// `tails` yields `List X` where X is one step of a list unrolled over the
		// tied one, and pointer identity alone would print that element as
		// `[] | [a, List a]` — spelled out beside the very name that means it.
		// Binds are shared with the outer match, so the arguments still have to
		// agree.
		return matchPat(t, d.Pat, t, d, binds, seen)
	}
	if p.hole >= 0 {
		if binds[p.hole] == nil {
			binds[p.hole] = t
			return true
		}
		return find(binds[p.hole]) == t
	}

	key := [2]any{t, p}
	if seen[key] {
		return true // already being checked further up; assume and let the rest decide
	}
	seen[key] = true

	if t.top != p.top || t.num != p.num || (t.fun != nil) != (p.fun != nil) {
		return false
	}
	if len(t.tuples) != len(p.tuples) {
		return false
	}
	for arity, fields := range p.tuples {
		got, ok := t.tuples[arity]
		if !ok || len(got) != len(fields) {
			return false
		}
		for i := range fields {
			if !matchPat(got[i], fields[i], root, d, binds, seen) {
				return false
			}
		}
	}
	if p.fun != nil {
		if !matchPat(t.fun.arg, p.fun.arg, root, d, binds, seen) {
			return false
		}
		if !matchPat(t.fun.res, p.fun.res, root, d, binds, seen) {
			return false
		}
	}
	return true
}

// sortDecls puts declarations in a deterministic order, and the more specific
// first so that a narrow shape is not shadowed by a broader one that happens to
// be declared earlier. "More specific" is approximated by pattern size.
func sortDecls(decls []Decl) {
	size := map[string]int{}
	for _, d := range decls {
		size[d.Name] = patSize(d.Pat)
	}
	sort.SliceStable(decls, func(i, j int) bool {
		if size[decls[i].Name] != size[decls[j].Name] {
			return size[decls[i].Name] > size[decls[j].Name]
		}
		return decls[i].Name < decls[j].Name
	})
}

func patSize(p *pattern) int { return patSizeSeen(p, map[*pattern]bool{}) }

// patSizeSeen needs the seen set because a pattern can be cyclic. Before a
// declaration could name another one, the only recursion was the `self` marker,
// which terminates by itself; expanding a reference ties the knot with a real
// pointer back to the root instead, and an unguarded walk runs forever.
func patSizeSeen(p *pattern, seen map[*pattern]bool) int {
	if p == nil || p.self || p.hole >= 0 || seen[p] {
		return 1
	}
	seen[p] = true
	n := 1
	for _, fields := range p.tuples {
		for _, f := range fields {
			n += patSizeSeen(f, seen)
		}
	}
	if p.fun != nil {
		n += patSizeSeen(p.fun.arg, seen) + patSizeSeen(p.fun.res, seen)
	}
	return n
}
