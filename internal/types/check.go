package types

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/syntax"
)

// Signature checking. A `-->` comment naming a binding claims a shape for it:
//
//	--> length : List a -> Num
//	length = { [] -> 0, [h, t] -> add 1 (length t) },
//
// The claim is compared against what inference found. It is never fed *into*
// inference, which is what keeps a signature from narrowing the language: adding
// one cannot change a program's analysis, only report on it.
//
// The comparison is deliberately conservative. It reports a *contradiction* —
// a position where the claim and the finding are both concrete and cannot both
// be true — and stays quiet about imprecision. Two reasons:
//
//   - The join lattice widens rather than failing, so an inferred type is
//     sometimes broader than the truth. Treating "broader than my claim" as an
//     error would flag correct signatures.
//   - A signature that is *more specific* than the finding is usually right and
//     useful: `sum : List Num -> Num` on a function inference typed
//     `List a -> Num` is a true statement about how it is meant to be used.
//
// So `length : Num -> Num` against `List a -> Num` is reported (a number is not
// a list), while `length : List Num -> Num` is not.

// A Signature is one `--> name : Type` annotation.
type Signature struct {
	Name string
	Text string
	Pat  *pattern
	Pos  source.SourcePos
}

// A unit is a program or a module: a source file, the type names visible in it,
// and the bindings a signature can attach to.
type unit struct {
	mod     string // "" for the program
	file    *source.Source
	imports []string
}

// --- scope -------------------------------------------------------------------

// typeScope is the set of type names a signature may use: the unit's own
// declarations, plus those of every module it imports, plus qualified
// `module.Name` for any module in its import clause. That is exactly the rule
// the language already uses for values, so there is nothing new to learn.
type typeScope struct {
	byName map[string]Decl            // unqualified, later import shadowing earlier
	byMod  map[string]map[string]Decl // qualified
}

func buildScope(u unit, perModule map[string][]Decl) typeScope {
	s := typeScope{byName: map[string]Decl{}, byMod: map[string]map[string]Decl{}}
	// Imports first, in order, so a later one shadows an earlier one; the unit's
	// own declarations win over all of them.
	for _, imp := range u.imports {
		tbl := map[string]Decl{}
		for _, d := range perModule[imp] {
			s.byName[d.Name] = d
			tbl[d.Name] = d
		}
		s.byMod[imp] = tbl
	}
	for _, d := range perModule[u.mod] {
		s.byName[d.Name] = d
	}
	return s
}

// --- collecting signatures ---------------------------------------------------

// CollectSignatures reads the `--> name : Type` annotations from a unit's
// comments, resolving type names in the given scope.
func CollectSignatures(file *source.Source, scope typeScope) ([]Signature, []Warning) {
	var sigs []Signature
	var warns []Warning

	for _, pos := range file.Comments {
		text := file.Text[pos.Start : pos.Start+pos.Length]
		if !strings.HasPrefix(text, annotMarker) {
			continue
		}
		body := strings.TrimSpace(text[len(annotMarker):])
		i := strings.Index(body, ":")
		if i < 0 {
			continue // a declaration, or malformed; CollectDecls already ruled on it
		}
		if eq := strings.Index(body, "="); eq >= 0 && eq < i {
			continue // a declaration whose body contains a colon
		}
		name := strings.TrimSpace(body[:i])
		rest := strings.TrimSpace(body[i+1:])
		if !isVarName(name) || rest == "" {
			continue // CollectDecls reported it
		}

		// A signature's lowercase names are free variables, not parameters, and it
		// may not claim `?`.
		pat, err := parseTypeExpr(rest, typeExprOpts{scope: scope})
		if err != "" {
			warns = append(warns, Warning{Message: "signature: " + err, Pos: pos})
			continue
		}
		sigs = append(sigs, Signature{Name: name, Text: rest, Pat: pat, Pos: pos})
	}
	return sigs, warns
}

// --- the check ---------------------------------------------------------------

// conflict walks a claim against a finding and returns the first position where
// the two are both concrete and cannot both hold, described in the notation the
// report uses.
func conflict(claim *pattern, found *Type, n *Namer, path string, seen map[[2]any]bool) (string, bool) {
	found = find(found)

	// A variable in the claim says nothing; top or a variable in the finding
	// means inference learned nothing. Neither can contradict.
	if claim == nil || claim.hole >= 0 {
		return "", false
	}
	if found.top || isVar(found) {
		return "", false
	}

	key := [2]any{found, claim}
	if seen[key] {
		return "", false
	}
	seen[key] = true

	claimKinds := kindsOfPattern(claim)
	foundKinds := kindsOfType(found)
	if len(claimKinds) == 0 || len(foundKinds) == 0 {
		return "", false
	}
	if !overlaps(claimKinds, foundKinds) {
		return fmt.Sprintf("%s: claimed %s, inferred %s",
			where2(path), describeKinds(claimKinds), describeKinds(foundKinds)), true
	}

	// The kinds overlap; recurse into the parts they share.
	for arity, fields := range claim.tuples {
		got, ok := found.tuples[arity]
		if !ok || len(got) != len(fields) {
			continue
		}
		for i := range fields {
			if msg, bad := conflict(fields[i], got[i], n, joinPath(path, fmt.Sprintf("field %d of the %s", i+1, tupleWord(arity))), seen); bad {
				return msg, true
			}
		}
	}
	if claim.fun != nil && found.fun != nil {
		if msg, bad := conflict(claim.fun.arg, found.fun.arg, n, joinPath(path, "the argument"), seen); bad {
			return msg, true
		}
		if msg, bad := conflict(claim.fun.res, found.fun.res, n, joinPath(path, "the result"), seen); bad {
			return msg, true
		}
	}
	return "", false
}

func joinPath(path, step string) string {
	if path == "" {
		return step
	}
	return step + " of " + path
}

func where2(path string) string {
	if path == "" {
		return "the type"
	}
	return path
}

func tupleWord(arity int) string {
	if arity == 2 {
		return "pair"
	}
	return fmt.Sprintf("%d-tuple", arity)
}

func kindsOfPattern(p *pattern) []string {
	var ks []string
	if p.num {
		ks = append(ks, "Num")
	}
	arities := make([]int, 0, len(p.tuples))
	for a := range p.tuples {
		arities = append(arities, a)
	}
	sort.Ints(arities)
	for _, a := range arities {
		ks = append(ks, fmt.Sprintf("[%d]", a))
	}
	if p.fun != nil {
		ks = append(ks, "->")
	}
	return ks
}

func kindsOfType(t *Type) []string {
	var ks []string
	if t.num {
		ks = append(ks, "Num")
	}
	arities := make([]int, 0, len(t.tuples))
	for a := range t.tuples {
		arities = append(arities, a)
	}
	sort.Ints(arities)
	for _, a := range arities {
		ks = append(ks, fmt.Sprintf("[%d]", a))
	}
	if t.fun != nil {
		ks = append(ks, "->")
	}
	return ks
}

func overlaps(a, b []string) bool {
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, y := range b {
		if set[y] {
			return true
		}
	}
	return false
}

func describeKinds(ks []string) string {
	out := make([]string, len(ks))
	for i, k := range ks {
		switch {
		case k == "Num":
			out[i] = "a number"
		case k == "->":
			out[i] = "a function"
		case k == "[0]":
			out[i] = "the empty tuple"
		default:
			var arity int
			fmt.Sscanf(k, "[%d]", &arity)
			out[i] = "a " + tupleWord(arity)
		}
	}
	return strings.Join(out, " or ")
}

// --- attaching signatures to bindings ----------------------------------------

// attach pairs each signature with the binding it precedes. A signature applies
// to the first binding whose name begins on a later line, which is the
// doc-comment convention: adjacency, not scope.
func attach(sigs []Signature, bindings []*syntax.Binding) (map[*syntax.Binding]Signature, []Warning) {
	out := map[*syntax.Binding]Signature{}
	var warns []Warning

	type entry struct {
		line int
		b    *syntax.Binding
	}
	var byLine []entry
	for _, b := range bindings {
		line, _, _ := b.Name.Pos.LineCol()
		byLine = append(byLine, entry{line, b})
	}
	sort.Slice(byLine, func(i, j int) bool { return byLine[i].line < byLine[j].line })

	for _, s := range sigs {
		sline, _, _ := s.Pos.LineCol()
		var target *syntax.Binding
		for _, e := range byLine {
			if e.line > sline {
				target = e.b
				break
			}
		}
		if target == nil {
			warns = append(warns, Warning{
				Message: fmt.Sprintf("signature for %q has no binding after it", s.Name),
				Pos:     s.Pos,
			})
			continue
		}
		if target.Name.Value != s.Name {
			warns = append(warns, Warning{
				Message: fmt.Sprintf("signature names %q but the binding below it is %q",
					s.Name, target.Name.Value),
				Pos: s.Pos,
			})
			continue
		}
		if prev, dup := out[target]; dup {
			warns = append(warns, Warning{
				Message: fmt.Sprintf("%q already has a signature (%s)", s.Name, prev.Text),
				Pos:     s.Pos,
			})
			continue
		}
		out[target] = s
	}
	return out, warns
}
