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

	// Declared is true when the name came from a `-->` annotation rather than
	// being generated, so a report can say which names are the author's.
	Declared bool
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

	// decls are the `-->` declarations in scope. A node matching one is printed
	// with the declared name instead of a generated T<n>, which is the whole
	// effect of a declaration: it names a shape and changes nothing else.
	decls []Decl
	used  map[string]bool // declared names already listed in the preamble

	// notes are remarks about the declarations themselves: two names for one
	// shape, or one name declared twice. Neither is an error — structurally
	// identical types *are* the same type here — but choosing silently would hide
	// that the other name applies just as well.
	notes    []Warning
	seenPair map[string]bool
}

func NewNamer() *Namer {
	return &Namer{byKey: map[string]int{}, used: map[string]bool{}, seenPair: map[string]bool{}}
}

// Declare puts type declarations in scope for every type this Namer renders. A
// name already declared is kept as it was; a second declaration of the same name
// with a different body is reported, since one of the two is not taking effect.
func (n *Namer) Declare(decls []Decl) {
	for _, d := range decls {
		if prev, dup := n.byName(d.Name); dup {
			if prev.Body != d.Body {
				n.notes = append(n.notes, Warning{
					Message: d.Name + " is declared twice with different bodies (" +
						prev.Body + " and " + d.Body + "); the first is used.",
					Pos: d.Pos,
				})
			}
			continue
		}
		n.decls = append(n.decls, d)
	}
	sortDecls(n.decls)
}

func (n *Namer) byName(name string) (Decl, bool) {
	for _, d := range n.decls {
		if d.Name == name {
			return d, true
		}
	}
	return Decl{}, false
}

// definedInTermsOf reports whether a's body names b, which makes any shape
// agreement between them deliberate rather than a coincidence worth flagging.
func definedInTermsOf(a, b Decl) bool {
	for _, ref := range referencedTypes(a.Body) {
		if ref == b.Name {
			return true
		}
		if mod, name, ok := splitQualified(ref); ok && name == b.Name && mod == b.Mod {
			return true
		}
	}
	return false
}

// preferred returns the declarations in match order, with those declared in mod
// first so a module's own name wins for a shape two declarations both describe.
func (n *Namer) preferred(mod string) []Decl {
	if mod == "" {
		return n.decls
	}
	out := make([]Decl, 0, len(n.decls))
	for _, d := range n.decls {
		if d.Mod == mod {
			out = append(out, d)
		}
	}
	for _, d := range n.decls {
		if d.Mod != mod {
			out = append(out, d)
		}
	}
	return out
}

// Notes reports remarks about the declarations: shapes that more than one name
// matched, and names declared more than once. In a structural system two
// declarations of one shape denote one type, so a name is a view rather than a
// distinction, and neither case is an error.
func (n *Namer) Notes() []Warning { return n.notes }

// Equations returns the equations every type rendered so far depends on, in the
// order they were first needed.
func (n *Namer) Equations() []Equation { return n.eqs }

// String renders one type with no module preference.
func (n *Namer) String(t *Type) string { return n.StringIn("", t) }

// StringIn renders one type as it should read inside module `mod`. Variable names
// restart at `a` for each call — they are local to a signature — while equation
// names are shared.
//
// The preference matters because two declarations can name one shape:
// `Table k v = List [k, v]` *is* a list of pairs, so without a preference every
// list of pairs in the library reads as `Table`, including list's own `zip` and
// `lookup`. A module's own name wins inside that module.
//
// Preference only breaks ties; it is deliberately not a visibility rule. A name
// is used wherever its shape matches, even in a module that does not import its
// home — because `<program> : List Num` is true and useful for a program that
// never imports `list`, and scoping the name would push most reports back to
// generated `T<n>`. The cost is that a declaration is effectively report-wide,
// so only distinctive shapes are worth naming: `Cmp a = (a -> a -> Num)` looks
// reasonable in heap and renders `math.max` as `Cmp Num`, since with no Bool
// type that shape is everywhere. Name data, not arities.
func (n *Namer) StringIn(mod string, t *Type) string {
	p := &printer{
		namer:   n,
		prefer:  mod,
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
	prefer  string // module whose own declarations win a tie
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

// declared checks a node against the `-->` declarations in scope and, on a
// match, returns the declared name applied to whatever its parameters stood for.
// The declaration is added to the preamble the first time it is used, so a report
// lists only the names it actually mentions.
func (p *printer) declared(t *Type) (string, bool) {
	if p.namer == nil {
		return "", false
	}
	for i, d := range p.namer.preferred(p.prefer) {
		args, ok := matchDecl(t, d)
		if !ok {
			continue
		}
		// Another declaration of the same shape is not wrong, but the reader
		// should know their name applies here too.
		for _, other := range p.namer.preferred(p.prefer)[i+1:] {
			if other.Name == d.Name || definedInTermsOf(d, other) || definedInTermsOf(other, d) {
				// An alias by construction — `Table k v = List [k, v]` — is the same
				// shape on purpose. Saying so would be noise on every use.
				continue
			}
			if _, also := matchDecl(t, other); also {
				key := d.Name + "/" + other.Name
				if !p.namer.seenPair[key] {
					p.namer.seenPair[key] = true
					p.namer.notes = append(p.namer.notes, Warning{
						Message: d.Name + " and " + other.Name + " name the same shape; " +
							d.Name + " is used. Structurally they are one type — if they " +
							"should differ, one of them needs a tag in the value.",
						Pos: other.Pos,
					})
				}
			}
		}
		if !p.namer.used[d.Name] {
			p.namer.used[d.Name] = true
			p.namer.eqs = append(p.namer.eqs, Equation{
				Name:     d.Name,
				Params:   d.Params,
				Body:     d.Body,
				Declared: true,
			})
		}
		// Mark the node so a recursive reference inside the arguments prints the
		// name rather than unrolling the shape again.
		p.names[t] = d.Name
		p.onPath[t] = true
		ref := d.Name
		for _, arg := range args {
			s := p.print(find(arg))
			if !atomic(s) {
				s = "(" + s + ")"
			}
			ref += " " + s
		}
		p.onPath[t] = false
		return ref, true
	}
	return "", false
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

	// A declared name wins over a generated one, and applies to any shape — not
	// only a recursive one, since `Maybe a = [] | [a]` is not recursive.
	if !p.onPath[t] {
		if ref, ok := p.declared(t); ok {
			return ref
		}
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

// UseNamed lists these declarations in the preamble. Needed because a report
// that displays an author's signature never *renders* the shapes it names, so
// nothing else would pull their equations in.
func (n *Namer) UseNamed(decls []Decl) {
	for _, d := range decls {
		if n.used[d.Name] {
			continue
		}
		n.used[d.Name] = true
		n.eqs = append(n.eqs, Equation{Name: d.Name, Params: d.Params, Body: d.Body, Declared: true})
	}
}
