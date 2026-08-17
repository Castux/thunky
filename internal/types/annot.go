package types

import (
	"fmt"
	"sort"
	"strings"

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

	top    bool
	num    bool
	tuples map[int][]*pattern
	fun    *patArrow
}

type patArrow struct{ arg, res *pattern }

func newPattern() *pattern { return &pattern{hole: -1} }

// --- collecting declarations from comments -----------------------------------

// CollectDecls reads the `-->` annotations out of a source file's comments. It
// returns the declarations and one message per annotation it could not read —
// a malformed annotation is reported rather than silently ignored, because an
// annotation that quietly does not apply is worse than none.
func CollectDecls(file *source.Source) ([]Decl, []Warning) {
	var decls []Decl
	var warns []Warning

	for _, pos := range file.Comments {
		// The marker is the comment's own dashes plus '>', so it is matched against
		// the whole comment text rather than against what follows the dashes.
		text := file.Text[pos.Start : pos.Start+pos.Length]
		if !strings.HasPrefix(text, annotMarker) {
			continue
		}
		body := strings.TrimSpace(text[len(annotMarker):])
		if body == "" {
			continue
		}

		d, err := parseDecl(body)
		if err != "" {
			warns = append(warns, Warning{Message: "annotation: " + err, Pos: pos})
			continue
		}
		if d.Name == "" {
			continue // recognised, but nothing to record yet — a signature
		}
		d.Pos = pos
		decls = append(decls, d)
	}
	return decls, warns
}

// parseDecl reads `Name params = TypeExpr`, returning a Decl with an empty Name
// when the line is recognised but records nothing.
//
// A lowercase head followed by `:` is a *signature*. Signatures are recognised
// and then ignored: this stage names shapes, it does not check claims. They are
// deliberately not validated either, because a signature may legitimately name
// types declared in another file, and resolving that needs the per-module import
// scope that checking will bring with it. Validating them now would mean
// rejecting correct annotations.
func parseDecl(text string) (Decl, string) {
	if i := strings.Index(text, "="); i >= 0 && !strings.Contains(text[:i], ":") {
		head, rest := strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:])
		words := strings.Fields(head)
		if len(words) == 0 {
			return Decl{}, "no name before '='"
		}
		name, params := words[0], words[1:]
		if !isTypeName(name) {
			return Decl{}, fmt.Sprintf("type name %q must start with a capital letter", name)
		}
		for _, p := range params {
			if !isVarName(p) {
				return Decl{}, fmt.Sprintf("parameter %q must be a lowercase name", p)
			}
		}
		if dup := firstDuplicate(params); dup != "" {
			return Decl{}, fmt.Sprintf("parameter %q appears twice", dup)
		}
		pat, err := parsePattern(rest, name, params)
		if err != "" {
			return Decl{}, err
		}
		return Decl{Name: name, Params: params, Body: rest, Pat: pat}, ""
	}

	if i := strings.Index(text, ":"); i >= 0 {
		head, rest := strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:])
		if !isVarName(head) {
			return Decl{}, fmt.Sprintf("%q is not a binding name; a signature is `name : Type`", head)
		}
		if rest == "" {
			return Decl{}, "a signature needs a type after ':'"
		}
		return Decl{}, "" // recognised; nothing to record at this stage
	}

	return Decl{}, "expected `Name params = Type` or `name : Type`"
}

func isTypeName(s string) bool {
	return s != "" && s[0] >= 'A' && s[0] <= 'Z' && isIdent(s)
}

func isVarName(s string) bool {
	return s != "" && s[0] >= 'a' && s[0] <= 'z' && isIdent(s)
}

func isIdent(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

func firstDuplicate(xs []string) string {
	seen := map[string]bool{}
	for _, x := range xs {
		if seen[x] {
			return x
		}
		seen[x] = true
	}
	return ""
}

// --- the type-expression parser ----------------------------------------------
//
// The grammar is exactly what the printer emits, so an annotation and a report
// use one notation:
//
//	Type   := Union ('->' Type)?          right associative, loosest
//	Union  := App ('|' App)*
//	App    := Atom+                       application binds tighter than both
//	Atom   := 'Num' | '?' | name | '[' Fields ']' | '(' Type ')'
//	Fields := empty | Type (',' Type)* | Type (';' Type)* ';'?

type patParser struct {
	toks   []string
	pos    int
	self   string
	params map[string]int
	err    string
}

func parsePattern(text, self string, params []string) (*pattern, string) {
	idx := map[string]int{}
	for i, p := range params {
		idx[p] = i
	}
	p := &patParser{toks: tokenizeType(text), self: self, params: idx}
	t := p.parseType()
	if p.err != "" {
		return nil, p.err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Sprintf("unexpected %q", p.toks[p.pos])
	}
	return t, ""
}

func tokenizeType(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		switch {
		case s[i] == ' ' || s[i] == '\t':
			i++
		case strings.HasPrefix(s[i:], "->"):
			out = append(out, "->")
			i += 2
		case strings.ContainsRune("[](),;|?", rune(s[i])):
			out = append(out, string(s[i]))
			i++
		default:
			j := i
			for j < len(s) && isIdent(string(s[j])) {
				j++
			}
			if j == i { // an unknown character: emit it so the parser can complain
				out = append(out, string(s[i]))
				i++
				continue
			}
			out = append(out, s[i:j])
			i = j
		}
	}
	return out
}

func (p *patParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func (p *patParser) fail(format string, args ...any) *pattern {
	if p.err == "" {
		p.err = fmt.Sprintf(format, args...)
	}
	return newPattern()
}

func (p *patParser) parseType() *pattern {
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

func (p *patParser) parseUnion() *pattern {
	alts := []*pattern{p.parseApp()}
	for p.peek() == "|" {
		p.pos++
		alts = append(alts, p.parseApp())
	}
	if len(alts) == 1 {
		return alts[0]
	}
	// A union of alternatives is one node carrying all of them, which is exactly
	// how the analysis represents it.
	out := newPattern()
	for _, a := range alts {
		if a.hole >= 0 || a.self {
			return p.fail("a parameter or a recursive reference cannot be one alternative of a union")
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

func (p *patParser) parseApp() *pattern {
	head := p.parseAtom()
	// The only application this stage understands is the declaration applied to
	// its own parameters, which is how a recursive body refers to itself.
	var args []*pattern
	for {
		switch p.peek() {
		case "", "->", "|", ",", ";", ")", "]":
			if len(args) == 0 {
				return head
			}
			if !head.self {
				return p.fail("only the type being declared may be applied to arguments here")
			}
			return head
		}
		args = append(args, p.parseAtom())
		if p.err != "" {
			return head
		}
	}
}

func (p *patParser) parseAtom() *pattern {
	tok := p.peek()
	switch {
	case tok == "":
		return p.fail("unexpected end of type")

	case tok == "?":
		p.pos++
		t := newPattern()
		t.top = true
		return t

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

	case isTypeName(tok):
		p.pos++
		if tok != p.self {
			return p.fail("unknown type %q — only the type being declared may be named in its own body", tok)
		}
		t := newPattern()
		t.self = true
		return t

	case isVarName(tok):
		p.pos++
		i, ok := p.params[tok]
		if !ok {
			return p.fail("unbound type variable %q", tok)
		}
		t := newPattern()
		t.hole = i
		return t

	default:
		return p.fail("unexpected %q", tok)
	}
}

// parseBrackets reads what follows a '['. The separator decides the shape,
// exactly as it does in the language: commas make a tuple, semicolons make a
// list of nested pairs ending in the empty tuple.
func (p *patParser) parseBrackets() *pattern {
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
		// [a; b] is [a, [b, []]] — nested pairs ending in the empty tuple.
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
		return t == root
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

func patSize(p *pattern) int {
	if p == nil || p.self || p.hole >= 0 {
		return 1
	}
	n := 1
	for _, fields := range p.tuples {
		for _, f := range fields {
			n += patSize(f)
		}
	}
	if p.fun != nil {
		n += patSize(p.fun.arg) + patSize(p.fun.res)
	}
	return n
}
