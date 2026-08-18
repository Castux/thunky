package types

import (
	"fmt"
	"strings"
)

// One parser for both kinds of annotation. A declaration body and a signature
// use the same notation and differ in exactly one place: what a lowercase name
// means.
//
//	--> List a = [] | [a, List a]      `a` is a parameter of the declaration
//	--> length : List a -> Num         `a` is a free type variable
//
// Everything else — Num, brackets, unions, arrows, and naming another declared
// type — is shared, so it lives here rather than being written twice.
//
//	Type   := Union ('->' Type)?          right associative, loosest
//	Union  := App ('|' App)*
//	App    := Named | Atom
//	Named  := (name '.')? Name Atom*      a declared type applied to arguments
//	Atom   := 'Num' | '?' | name | '[' Fields ']' | '(' Type ')'
//	Fields := empty | Type (',' Type)* | Type (';' Type)* ';'?

type typeExprOpts struct {
	// self is the name of the declaration being defined, so its body can refer
	// to itself. Empty for a signature.
	self string
	// params maps a declaration's parameter names to their positions. Nil for a
	// signature, where a lowercase name is instead a free variable.
	params map[string]int
	// scope is the other declared types that may be named, resolved already.
	scope typeScope
	// allowTop permits `?`. A declaration may describe it; a signature may not
	// claim it, since claiming "anything" always checks out.
	allowTop bool
}

func parseTypeExpr(text string, o typeExprOpts) (*pattern, string) {
	p := &exprParser{toks: tokenizeType(text), o: o}
	t := p.parseType()
	if p.err != "" {
		return nil, p.err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Sprintf("unexpected %q", p.toks[p.pos])
	}
	return t, ""
}

type exprParser struct {
	toks []string
	pos  int
	o    typeExprOpts
	err  string
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
		case strings.ContainsRune("[](),;|?.!", rune(s[i])):
			out = append(out, string(s[i]))
			i++
		default:
			j := i
			for j < len(s) && isIdent(string(s[j])) {
				j++
			}
			if j == i {
				out = append(out, string(s[i])) // unknown character; let the parser complain
				i++
				continue
			}
			out = append(out, s[i:j])
			i = j
		}
	}
	return out
}

func (p *exprParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func (p *exprParser) fail(format string, args ...any) *pattern {
	if p.err == "" {
		p.err = fmt.Sprintf(format, args...)
	}
	return wildcard()
}

// wildcard is a position the annotation says nothing concrete about.
func wildcard() *pattern { p := newPattern(); p.hole = 0; return p }

func (p *exprParser) parseType() *pattern {
	left := p.parseAsserted()
	if p.peek() == "->" {
		p.pos++
		right := p.parseType()
		t := newPattern()
		t.fun = &patArrow{arg: left, res: right}
		return t
	}
	return left
}

// parseAsserted reads a type and an optional `!` suffix.
//
// Precedence: `!` binds to the *whole* type at this position, so `List a!` means
// `(List a)!` — an assertion about the argument, not about its element type.
// Parentheses are accepted for readers who want it spelled out, and `List (a!)`
// is the way to mark the element instead. A `!` on a bare type variable is an
// error: a variable claims nothing, so there is nothing to over-claim.
func (p *exprParser) parseAsserted() *pattern {
	t := p.parseUnion()
	if p.peek() != "!" {
		return t
	}
	p.pos++
	if t.hole >= 0 {
		return p.fail("`!` on a type variable claims nothing; put it on a concrete type")
	}
	t.asserted = true
	return t
}

func (p *exprParser) parseUnion() *pattern {
	alts := []*pattern{p.parseApp()}
	for p.peek() == "|" {
		p.pos++
		alts = append(alts, p.parseApp())
	}
	if len(alts) == 1 {
		return alts[0]
	}
	// A union is one node carrying every alternative, which is how the analysis
	// represents it.
	out := newPattern()
	for _, a := range alts {
		if a.hole >= 0 || a.self {
			return p.fail("a type variable or a recursive reference cannot be one alternative of a union")
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

// qualifiedAhead reports whether the next tokens are `module . Name`.
func (p *exprParser) qualifiedAhead() bool {
	return p.pos+2 < len(p.toks) && isVarName(p.peek()) && p.toks[p.pos+1] == "."
}

func (p *exprParser) parseApp() *pattern {
	tok := p.peek()
	if tok != "Num" && (isTypeName(tok) || p.qualifiedAhead()) {
		return p.parseNamed()
	}
	return p.parseAtom()
}

// parseNamed reads a declared type, applied to as many arguments as it takes,
// and expands it on the spot. The declaration being defined refers to itself
// instead, which is what ties the recursive knot.
func (p *exprParser) parseNamed() *pattern {
	head := p.peek()
	qualified := ""
	if p.qualifiedAhead() {
		qualified = head
		head = p.toks[p.pos+2]
		p.pos += 3
	} else {
		p.pos++
	}

	// A reference to the declaration currently being defined.
	if qualified == "" && head == p.o.self {
		nparams := len(p.o.params)
		for i := 0; i < nparams; i++ {
			switch p.peek() {
			case "", "->", "|", ",", ";", ")", "]":
				return p.fail("%s takes %d parameter(s), given %d", head, nparams, i)
			}
			p.parseAtom() // consumed: a recursive use is at the declaration's own parameters
			if p.err != "" {
				return wildcard()
			}
		}
		t := newPattern()
		t.self = true
		return t
	}

	var d Decl
	var ok bool
	if qualified != "" {
		tbl, seen := p.o.scope.byMod[qualified]
		if !seen {
			return p.fail("module %q is not imported here, so %s.%s is not in scope", qualified, qualified, head)
		}
		if d, ok = tbl[head]; !ok {
			return p.fail("module %q declares no type %q", qualified, head)
		}
	} else {
		if d, ok = p.o.scope.byName[head]; !ok {
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
	return substitute(d.Pat, args)
}

func (p *exprParser) parseAtom() *pattern {
	tok := p.peek()
	switch {
	case tok == "":
		return p.fail("unexpected end of type")

	case tok == "?":
		p.pos++
		if !p.o.allowTop {
			return p.fail("`?` claims nothing and cannot be checked; leave the position a variable instead")
		}
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

	case isTypeName(tok) || p.qualifiedAhead():
		return p.parseNamed()

	case isVarName(tok):
		p.pos++
		if p.o.params == nil {
			// A signature's free type variable. It claims nothing on its own, so
			// it stays a hole; the name is kept so that every occurrence of `a`
			// becomes one node when the signature is instantiated at a call site.
			t := wildcard()
			t.varName = tok
			return t
		}
		i, ok := p.o.params[tok]
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
func (p *exprParser) parseBrackets() *pattern {
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

// substitute expands a declaration's pattern: parameters become the given
// arguments, and its self-reference becomes a cycle, so a use of `List a`
// describes the same infinite shape the declaration does.
func substitute(p *pattern, args []*pattern) *pattern {
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
	tieSelf(root, root, map[*pattern]bool{})
	return root
}

// tieSelf replaces every self marker with the root, making the pattern cyclic.
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

// referencedTypes lists the declared types an annotation body names, so that
// declarations can be resolved in dependency order. Qualified references come
// back as "module.Name".
func referencedTypes(text string) []string {
	toks := tokenizeType(text)
	var out []string
	for i := 0; i < len(toks); i++ {
		if toks[i] == "Num" {
			continue
		}
		if i+2 < len(toks) && isVarName(toks[i]) && toks[i+1] == "." && isTypeName(toks[i+2]) {
			out = append(out, toks[i]+"."+toks[i+2])
			i += 2
			continue
		}
		if isTypeName(toks[i]) {
			out = append(out, toks[i])
		}
	}
	return out
}

// --- name predicates ---------------------------------------------------------
//
// The two conventions that stand in for a keyword: a capitalised name is a type,
// a lowercase one is a value or a type variable.

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
