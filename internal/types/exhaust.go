package types

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Castux/thunky/internal/syntax"
)

// Exhaustiveness, checked against the *signature* rather than against inference.
//
// A signature is a claim about the domain, so it is also the specification the
// patterns have to cover:
//
//	--> last : List a -> a          List a = [] | [a, List a]
//	last = { [x, []] -> x, [h, t] -> last t }
//	                                the [] alternative is never matched
//
// This is what inference cannot tell you. Inference reports `last : List a -> a`
// all by itself, because the recursive call ties the tail's type to the argument
// and the nested `[]` pattern pulls the empty alternative in — so comparing a
// claim against inference would find nothing wrong. Comparing the *patterns*
// against the claim finds it immediately.
//
// A `!` suffix says the author is asserting that position anyway:
//
//	--> last : List a! -> a
//
// which is the trade the language already lives on: `head` on a list is more
// useful than `headSafe` alone, and the marker says out loud that the caller is
// responsible for the assumption.
//
// Two limits worth knowing. The check sees only a binding's *own* patterns, so a
// function that is partial by delegation — `nth = n -> drop n *> head`, which
// never matches on the list at all — is not flagged, and its `!` is documentation
// the checker can neither demand nor verify. And exhaustiveness is relative to
// the declared domain, so silence means "the patterns cover what you claimed",
// not "this function is total".

// checkExhaustive reports positions where a signature's claimed domain is not
// covered by the binding's patterns.
func (in *inferrer) checkExhaustive(b *syntax.Binding, sig Signature) {
	claim, body := sig.Pat, b.Expression
	depth := 0

	for {
		if claim == nil || claim.fun == nil {
			return
		}
		lam, ok := body.(*syntax.Lambda)
		if !ok {
			return // nothing matches here; a delegating body is not our business
		}
		depth++

		arg := claim.fun.arg
		if arg != nil && !arg.asserted && arg.hole < 0 {
			pats := make([]syntax.Pattern, 0, len(lam.Cases))
			for _, c := range lam.Cases {
				pats = append(pats, c.Pattern)
			}
			if missing, ok := uncovered(arg, pats); !ok {
				in.warnings = append(in.warnings, Warning{
					Message: fmt.Sprintf("%s : %s — argument %d does not cover %s of what it claims; "+
						"handle it, or write `!` after that type to assert it anyway",
						sig.Name, sig.Text, depth, missing),
					Pos: sig.Pos,
				})
			}
		}

		// Continue into the result only when there is one case, so that "the body"
		// is unambiguous. `map = f -> { … }` is exactly this shape.
		if len(lam.Cases) != 1 {
			return
		}
		claim, body = claim.fun.res, lam.Cases[0].Expression
	}
}

// uncovered reports which alternative of the claimed type no pattern matches.
// It errs toward silence: it only reports an alternative it is sure is missing,
// because a false alarm would have to be silenced with a `!` that then means
// nothing.
func uncovered(claim *pattern, pats []syntax.Pattern) (string, bool) {
	// A variable pattern matches anything, so nothing is left over.
	for _, p := range pats {
		if _, isVar := p.(*syntax.Name); isVar {
			return "", true
		}
	}

	// Group the constructor patterns by the arity they match. A list pattern is
	// nested pairs ending in the empty tuple, exactly as in the language.
	byArity := map[int][][]syntax.Pattern{}
	literals := false
	for _, p := range pats {
		collectArities(p, byArity, &literals)
	}

	var missing []string
	if claim.num {
		// Number literals can never exhaust the numbers; only a variable could,
		// and there is none.
		missing = append(missing, "Num")
	}
	if claim.fun != nil {
		missing = append(missing, "a function")
	}

	arities := make([]int, 0, len(claim.tuples))
	for a := range claim.tuples {
		arities = append(arities, a)
	}
	sort.Ints(arities)
	for _, arity := range arities {
		groups, present := byArity[arity]
		if !present {
			missing = append(missing, describeArity(arity))
			continue
		}
		// The arity is matched. Its fields are fully covered when some one pattern
		// binds every field to a variable; with several partial patterns per arity a
		// sound answer needs the full usefulness algorithm, so this stays quiet
		// rather than guessing.
		fieldsCovered := false
		for _, g := range groups {
			all := true
			for _, sub := range g {
				if _, isVar := sub.(*syntax.Name); !isVar {
					all = false
					break
				}
			}
			if all {
				fieldsCovered = true
				break
			}
		}
		if fieldsCovered {
			continue
		}
		// Otherwise check each field against the patterns in that position. This is
		// an approximation, so it only reports when a field is missing an
		// alternative outright.
		fields := claim.tuples[arity]
		for i := range fields {
			var subs []syntax.Pattern
			for _, g := range groups {
				if i < len(g) {
					subs = append(subs, g[i])
				}
			}
			if sub, ok := uncovered(fields[i], subs); !ok {
				missing = append(missing, fmt.Sprintf("%s in field %d of %s",
					sub, i+1, describeArity(arity)))
				break
			}
		}
	}

	if len(missing) == 0 {
		return "", true
	}
	return strings.Join(missing, " and "), false
}

// collectArities records what a pattern matches, desugaring a list pattern into
// the nested pairs it stands for.
func collectArities(p syntax.Pattern, byArity map[int][][]syntax.Pattern, literals *bool) {
	switch pat := p.(type) {
	case *syntax.TuplePattern:
		byArity[len(pat.SubPatterns)] = append(byArity[len(pat.SubPatterns)], pat.SubPatterns)
	case *syntax.ListPattern:
		if len(pat.SubPatterns) == 0 {
			byArity[0] = append(byArity[0], nil)
			return
		}
		// [a; b] is [a, [b, []]]: this level matches a pair whose second field is
		// the rest of the list.
		rest := &syntax.ListPattern{SubPatterns: pat.SubPatterns[1:], Start: pat.Start, End: pat.End}
		byArity[2] = append(byArity[2], []syntax.Pattern{pat.SubPatterns[0], rest})
	case *syntax.NumberLiteral, *syntax.StringLiteral:
		*literals = true
	}
}

func describeArity(arity int) string {
	switch arity {
	case 0:
		return "the empty tuple"
	case 2:
		return "a pair"
	default:
		return fmt.Sprintf("a %d-tuple", arity)
	}
}

// countAsserted counts the `!` marks in a signature, so a report can say how many
// assumptions it is resting on.
func countAsserted(p *pattern, seen map[*pattern]bool) int {
	if p == nil || seen[p] {
		return 0
	}
	seen[p] = true
	n := 0
	if p.asserted {
		n = 1
	}
	for _, fields := range p.tuples {
		for _, f := range fields {
			n += countAsserted(f, seen)
		}
	}
	if p.fun != nil {
		n += countAsserted(p.fun.arg, seen) + countAsserted(p.fun.res, seen)
	}
	return n
}
