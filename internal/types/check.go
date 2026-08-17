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

		pat, err := parseSigPattern(rest, scope)
		if err != "" {
			warns = append(warns, Warning{Message: "signature: " + err, Pos: pos})
			continue
		}
		sigs = append(sigs, Signature{Name: name, Text: rest, Pat: pat, Pos: pos})
	}
	return sigs, warns
}

// --- parsing a signature's type ----------------------------------------------

// parseSigPattern parses a signature's type. It differs from a declaration body
// in two ways: a lowercase name is a free variable rather than a declared
// parameter, and a capitalised name is looked up in scope and expanded, so a
// signature may name List, Maybe, or list.List.
func parseSigPattern(text string, scope typeScope) (*pattern, string) {
	p := &sigParser{toks: tokenizeType(text), scope: scope, vars: map[string]bool{}}
	t := p.parseType()
	if p.err != "" {
		return nil, p.err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Sprintf("unexpected %q", p.toks[p.pos])
	}
	return t, ""
}

type sigParser struct {
	toks  []string
	pos   int
	scope typeScope
	vars  map[string]bool
	err   string
}

func (p *sigParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func (p *sigParser) fail(format string, args ...any) *pattern {
	if p.err == "" {
		p.err = fmt.Sprintf(format, args...)
	}
	return wildcard()
}

// wildcard is a position a signature says nothing concrete about: a free type
// variable. Checking treats it as "no claim", which is why variables never
// produce a contradiction.
func wildcard() *pattern { p := newPattern(); p.hole = 0; return p }

func (p *sigParser) parseType() *pattern {
	left := p.parseUnion()
	if p.peek() == "->" {
		p.pos++
		right := p.parseType()
		t := newPattern()
		t.fun = &patArrow{arg: left, res: right}
		return t
	}
	return left
}

func (p *sigParser) parseUnion() *pattern {
	alts := []*pattern{p.parseApp()}
	for p.peek() == "|" {
		p.pos++
		alts = append(alts, p.parseApp())
	}
	if len(alts) == 1 {
		return alts[0]
	}
	out := newPattern()
	for _, a := range alts {
		if a.hole >= 0 {
			return p.fail("a type variable cannot be one alternative of a union")
		}
		out.top = out.top || a.top
		out.num = out.num || a.num
		if a.fun != nil {
			out.fun = a.fun
		}
		for arity, fields := range a.tuples {
			if out.tuples == nil {
				out.tuples = map[int][]*pattern{}
			}
			if _, dup := out.tuples[arity]; dup {
				return p.fail("two alternatives are both tuples of arity %d", arity)
			}
			out.tuples[arity] = fields
		}
	}
	return out
}

// parseApp reads a type application: a declared name applied to arguments,
// which is expanded on the spot by substituting the arguments for the
// declaration's parameters.
func (p *sigParser) parseApp() *pattern {
	tok := p.peek()
	// Num is built in, not declared, so it must not go through the scope lookup.
	if tok != "Num" && (isTypeName(tok) || p.qualifiedAhead()) {
		return p.parseNamed()
	}
	return p.parseAtom()
}

// qualifiedAhead reports whether the next tokens are `module . Name`.
func (p *sigParser) qualifiedAhead() bool {
	return p.pos+2 < len(p.toks) && isVarName(p.peek()) && p.toks[p.pos+1] == "."
}

func (p *sigParser) parseNamed() *pattern {
	// `list.List` or `List`
	var d Decl
	var ok bool
	head := p.peek()
	if p.pos+2 < len(p.toks) && p.toks[p.pos+1] == "." {
		mod, member := head, p.toks[p.pos+2]
		p.pos += 3
		tbl, seen := p.scope.byMod[mod]
		if !seen {
			return p.fail("module %q is not imported here, so %s.%s is not in scope", mod, mod, member)
		}
		d, ok = tbl[member]
		if !ok {
			return p.fail("module %q declares no type %q", mod, member)
		}
	} else {
		p.pos++
		d, ok = p.scope.byName[head]
		if !ok {
			return p.fail("unknown type %q — declare it with `--> %s … = …`", head, head)
		}
	}

	args := make([]*pattern, 0, len(d.Params))
	for len(args) < len(d.Params) {
		switch p.peek() {
		case "", "->", "|", ",", ";", ")", "]":
			return p.fail("%s takes %d parameter(s), given %d", d.Name, len(d.Params), len(args))
		}
		args = append(args, p.parseAtom())
		if p.err != "" {
			return wildcard()
		}
	}
	return substitute(d.Pat, args, d)
}

func (p *sigParser) parseAtom() *pattern {
	tok := p.peek()
	switch {
	case tok == "":
		return p.fail("unexpected end of type")

	case tok == "?":
		return p.fail("`?` claims nothing and cannot be checked; leave the position a variable instead")

	case tok == "Num":
		p.pos++
		t := newPattern()
		t.num = true
		return t

	case tok == "(":
		p.pos++
		inner := p.parseType()
		if p.peek() != ")" {
			return p.fail("expected ')'")
		}
		p.pos++
		return inner

	case tok == "[":
		p.pos++
		return p.parseBrackets()

	case isTypeName(tok) || p.qualifiedAhead():
		return p.parseNamed()

	case isVarName(tok):
		p.pos++
		p.vars[tok] = true
		return wildcard()

	default:
		return p.fail("unexpected %q", tok)
	}
}

func (p *sigParser) parseBrackets() *pattern {
	if p.peek() == "]" {
		p.pos++
		t := newPattern()
		t.tuples = map[int][]*pattern{0: {}}
		return t
	}
	first := p.parseType()
	if p.err != "" {
		return first
	}
	switch p.peek() {
	case "]":
		p.pos++
		t := newPattern()
		t.tuples = map[int][]*pattern{1: {first}}
		return t
	case ",":
		fields := []*pattern{first}
		for p.peek() == "," {
			p.pos++
			fields = append(fields, p.parseType())
			if p.err != "" {
				return first
			}
		}
		if p.peek() != "]" {
			return p.fail("expected ']'")
		}
		p.pos++
		t := newPattern()
		t.tuples = map[int][]*pattern{len(fields): fields}
		return t
	case ";":
		elems := []*pattern{first}
		for p.peek() == ";" {
			p.pos++
			if p.peek() == "]" {
				break
			}
			elems = append(elems, p.parseType())
			if p.err != "" {
				return first
			}
		}
		if p.peek() != "]" {
			return p.fail("expected ']'")
		}
		p.pos++
		empty := newPattern()
		empty.tuples = map[int][]*pattern{0: {}}
		out := empty
		for i := len(elems) - 1; i >= 0; i-- {
			cell := newPattern()
			cell.tuples = map[int][]*pattern{2: {elems[i], out}}
			out = cell
		}
		return out
	default:
		return p.fail("expected ',', ';' or ']' inside brackets")
	}
}

// substitute expands a declaration: parameters become the given arguments, and
// the declaration's self-reference becomes a cyclic pattern so that a signature
// mentioning `List a` describes the same infinite shape the declaration does.
func substitute(p *pattern, args []*pattern, d Decl) *pattern {
	memo := map[*pattern]*pattern{}
	var walk func(*pattern) *pattern
	walk = func(src *pattern) *pattern {
		if src == nil {
			return nil
		}
		if src.hole >= 0 {
			if src.hole < len(args) {
				return args[src.hole]
			}
			return wildcard()
		}
		if out, done := memo[src]; done {
			return out
		}
		out := newPattern()
		memo[src] = out
		if src.self {
			// Tie the knot: the expansion of the whole declaration, which is the
			// pattern being built at the root of this walk.
			out.self = true
			return out
		}
		out.top, out.num = src.top, src.num
		if src.tuples != nil {
			out.tuples = map[int][]*pattern{}
			for arity, fields := range src.tuples {
				cp := make([]*pattern, len(fields))
				for i, f := range fields {
					cp[i] = walk(f)
				}
				out.tuples[arity] = cp
			}
		}
		if src.fun != nil {
			out.fun = &patArrow{arg: walk(src.fun.arg), res: walk(src.fun.res)}
		}
		return out
	}
	root := walk(p)
	// Replace every self marker with the root, making the pattern cyclic.
	tieSelf(root, root, map[*pattern]bool{})
	return root
}

func tieSelf(p, root *pattern, seen map[*pattern]bool) {
	if p == nil || seen[p] {
		return
	}
	seen[p] = true
	for arity, fields := range p.tuples {
		for i, f := range fields {
			if f != nil && f.self {
				p.tuples[arity][i] = root
			} else {
				tieSelf(f, root, seen)
			}
		}
	}
	if p.fun != nil {
		if p.fun.arg != nil && p.fun.arg.self {
			p.fun.arg = root
		} else {
			tieSelf(p.fun.arg, root, seen)
		}
		if p.fun.res != nil && p.fun.res.self {
			p.fun.res = root
		} else {
			tieSelf(p.fun.res, root, seen)
		}
	}
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
