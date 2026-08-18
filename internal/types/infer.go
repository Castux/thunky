package types

import (
	"fmt"
	"sort"

	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/syntax"
)

// The inference walk. Every expression gets a type node; every binder — a
// let/module binding, or a name inside a pattern — gets one too, kept in `env`
// under the AST node that defines it, which is exactly what the resolver hands
// back for a use.
//
// Bindings are inferred on demand, the first time something refers to them, so
// definitions are visited in dependency order without anyone having to sort
// them. A reference that arrives while a binding is still being inferred is a
// recursive one: it gets the binding's own node, un-copied, so that the
// recursion ties a knot in the graph — that is what turns `length`'s argument
// into a list rather than an ever-deeper pile of pairs. Once a binding is
// finished, later uses get a fresh copy, which is what lets `id` be used at one
// type here and another there.

type bindState uint8

const (
	unvisited bindState = iota
	inProgress
	finished
)

// A Warning is a place where the shapes did not line up. None of them stop the
// analysis: it widens and carries on.
type Warning struct {
	Message string
	Pos     source.SourcePos
}

// An Entry is one reported binding: its name, where it is, and what it is.
type Entry struct {
	Name string
	Pos  source.SourcePos
	Type string

	// Given is true when Type is the author's signature rather than the inferred
	// shape. Inferred is filled in only when a given signature is contradicted, so
	// the report can show the disagreement in place. Asserted counts the `!` marks
	// in it: assumptions the analysis was told to take on trust.
	Given    bool
	Inferred string
	Asserted int
}

// ExprType is one expression's inferred type, for the full dump.
type ExprType struct {
	Kind string
	Pos  source.SourcePos
	Type string
}

type Analysis struct {
	// Equations are the named recursive types the rendered types refer to.
	Equations []Equation

	Program  string
	Modules  []ModuleEntry
	Exprs    []ExprType
	Warnings []Warning
}

type ModuleEntry struct {
	Name    string
	Entries []Entry
}

type inferrer struct {
	b       *builder
	res     *syntax.Resolution
	modules map[string]*syntax.Module

	env   map[syntax.Node]*Type
	state map[*syntax.Binding]bindState
	types map[syntax.Node]*Type

	// The declared type of every binding that has a signature, built once and
	// instantiated per use. Its presence is also what makes a callee trusted:
	// its parameter types are the author's, not a join of whatever reached them.
	sigTypes map[*syntax.Binding]*Type

	warnings []Warning
	seenWarn map[string]bool

	// settled is set for the refinement pass over the program body. Types are
	// wider by then — every use of a name has been joined into the node the
	// report will print — so a second look would describe the same call site
	// with a different, vaguer type. Checking belongs to the first walk.
	settled bool

	// namer renders types for diagnostics, so a warning and the report agree.
	namer *Namer
}

// Infer walks a resolved program and its modules and returns the inferred type
// of every expression.
func Infer(program *syntax.Program, modules map[string]*syntax.Module, res *syntax.Resolution) *Analysis {
	in := &inferrer{
		b:        newBuilder(),
		res:      res,
		modules:  modules,
		env:      map[syntax.Node]*Type{},
		state:    map[*syntax.Binding]bindState{},
		types:    map[syntax.Node]*Type{},
		sigTypes: map[*syntax.Binding]*Type{},
		seenWarn: map[string]bool{},
	}

	// One namer for the whole report: a recursive shape earns its equation once,
	// however many signatures mention it. It is built before the walk so that a
	// diagnostic raised during inference names types the same way the report does.
	//
	// `-->` declarations are read out of the comments of every source loaded, and
	// gathered report-wide rather than per module: a declaration only decides what
	// a shape is *called*, so sharing them costs nothing and keeps one name for
	// one shape. (Checking a signature will need the module's own import scope.)
	in.namer = NewNamer()
	units := unitsOf(program, modules)

	// Two passes, because a declaration body may name another declared type:
	// heads first so arities are known, then bodies in dependency order.
	var raws []rawDecl
	for _, u := range units {
		rs, warns := CollectRawDecls(u)
		raws = append(raws, rs...)
		in.warnings = append(in.warnings, warns...)
	}
	perModule, declWarns := resolveDecls(units, raws)
	in.warnings = append(in.warnings, declWarns...)
	for _, u := range units {
		in.namer.Declare(perModule[u.mod])
	}

	// Signatures are resolved *before* the walk, because a declared type is now
	// what the analysis uses: a use of an annotated binding gets a fresh copy of
	// its signature rather than of whatever its body inferred to. That is the one
	// place this differs from a report — a claim participates, so it has to be
	// checked, which is what checkGiven and exhaustiveness are for.
	given := in.collectSignatures(units, perModule, program, modules)

	// The program body pulls in whatever it needs; a second pass over every
	// module binding then reaches the ones nothing referred to, and refines the
	// ones whose recursive group was entered at an awkward point. Joining is
	// monotone, so a second look can only add information.
	bodyType := in.expr(program.Body)
	for _, mod := range sortedModules(modules) {
		for _, b := range mod.PublicBindings {
			in.bindingType(b)
		}
	}
	in.settled = true
	in.expr(program.Body)

	namer := in.namer
	// Now that every body has been walked, each claim is compared with what its
	// own definition inferred to. An annotated binding is *displayed* as its
	// signature, so the report is the documented interface, verified, rather than
	// a restatement of what inference happened to find.
	in.checkGiven(given)

	// Assertions propagate once every signature is known: a call site that does
	// not rule out a callee's assumption passes it to the caller.
	in.checkAssertions(program, modules, given)

	analysis := &Analysis{Program: namer.String(bodyType), Warnings: in.warnings}
	for _, mod := range sortedModules(modules) {
		entry := ModuleEntry{Name: mod.Name}
		for _, b := range mod.PublicBindings {
			t, ok := in.env[b]
			if !ok {
				continue
			}
			e := Entry{Name: b.Name.Value, Pos: b.Name.Pos, Type: namer.StringIn(mod.Name, t)}
			if g, ok := given[b]; ok {
				// The author's words win. Any declared type the signature names has
				// to reach the preamble, which rendering the inferred shape would
				// otherwise have been what did it.
				e.Given = true
				e.Asserted = g.asserted
				if g.conflicted {
					e.Inferred = e.Type
				}
				e.Type = g.text
				namer.UseNamed(g.names)
			}
			entry.Entries = append(entry.Entries, e)
		}
		analysis.Modules = append(analysis.Modules, entry)
	}

	modOfFile := map[*source.Source]string{}
	for _, u := range units {
		modOfFile[u.file] = u.mod
	}
	for node, t := range in.types {
		pos := syntax.NodePos(node)
		analysis.Exprs = append(analysis.Exprs, ExprType{
			Kind: syntax.NodeType(node),
			Pos:  pos,
			Type: namer.StringIn(modOfFile[pos.File], t),
		})
	}
	// Signatures are checked after the walk, against the finished types. Type
	// names resolve in the unit's own scope — its declarations plus those of the
	// modules it imports, qualified or not — which is the rule the language
	// already uses for values.
	analysis.Warnings = in.warnings
	analysis.Equations = namer.Equations()
	analysis.Warnings = append(analysis.Warnings, namer.Notes()...)

	sort.Slice(analysis.Exprs, func(i, j int) bool {
		a, b := analysis.Exprs[i].Pos, analysis.Exprs[j].Pos
		if a.File != b.File {
			return fileName(a) < fileName(b)
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Length > b.Length
	})

	return analysis
}

func fileName(p source.SourcePos) string {
	if p.File == nil {
		return ""
	}
	return p.File.Path
}

func sortedModules(modules map[string]*syntax.Module) []*syntax.Module {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]*syntax.Module, 0, len(names))
	for _, name := range names {
		out = append(out, modules[name])
	}
	return out
}

func (in *inferrer) warn(msg string, pos source.SourcePos) {
	key := msg + "@" + fileName(pos) + string(rune(pos.Start))
	if in.seenWarn[key] {
		return
	}
	in.seenWarn[key] = true
	in.warnings = append(in.warnings, Warning{Message: msg, Pos: pos})
}

func (in *inferrer) record(node syntax.Node, t *Type) *Type {
	if existing, ok := in.types[node]; ok {
		return in.b.join(existing, t)
	}
	in.types[node] = t
	return t
}

// ------------------------------------------------------------- expressions

func (in *inferrer) expr(e syntax.Expression) *Type {
	switch node := e.(type) {
	case *syntax.NumberLiteral:
		return in.record(node, in.b.num())

	case *syntax.StringLiteral:
		return in.record(node, in.b.list(in.b.num()))

	case *syntax.TupleExpr:
		fields := make([]*Type, len(node.SubExpressions))
		for i, sub := range node.SubExpressions {
			fields[i] = in.expr(sub)
		}
		return in.record(node, in.b.tuple(fields...))

	case *syntax.List:
		t := in.b.tuple()
		for i := len(node.SubExpressions) - 1; i >= 0; i-- {
			t = in.b.tuple(in.expr(node.SubExpressions[i]), t)
		}
		return in.record(node, t)

	case *syntax.Name:
		return in.record(node, in.name(node))

	case *syntax.QualifiedName:
		if b, ok := in.res.Quals[node]; ok {
			return in.record(node, in.bindingType(b))
		}
		return in.record(node, in.b.any())

	case *syntax.Lambda:
		return in.record(node, in.lambda(node))

	case *syntax.Let:
		for _, b := range node.Bindings {
			in.bindingType(b)
		}
		return in.record(node, in.expr(node.Expression))

	case *syntax.Operation:
		return in.record(node, in.operation(node))
	}

	return in.b.any()
}

func (in *inferrer) name(node *syntax.Name) *Type {
	fact, ok := in.res.Uses[node]
	if !ok {
		// A binder's own name, or something the resolver could not place.
		if t, ok := in.env[node]; ok {
			return t
		}
		return in.b.any()
	}

	switch fact.Kind {
	case syntax.ResolveBuiltin:
		return in.builtin(node.Value)
	default:
		switch def := fact.Def.(type) {
		case *syntax.Binding:
			return in.bindingType(def)
		case *syntax.Name:
			if t, ok := in.env[def]; ok {
				return t
			}
			t := in.b.fresh()
			in.env[def] = t
			return t
		}
	}
	return in.b.any()
}

// bindingType returns the type of a let or module binding, inferring it the
// first time it is asked for. A request that arrives while the binding is still
// being inferred is a recursive reference and gets the binding's own node.
func (in *inferrer) bindingType(b *syntax.Binding) *Type {
	// A signature is authoritative. Every use — including a recursive one — gets
	// a fresh copy of the declared type, so nothing has to be known about the
	// body first. That is what removes the knot-tying: a recursive call no longer
	// has to share the node it is still building, and an annotated module can be
	// checked in any order.
	if declared, signed := in.sigTypes[b]; signed {
		in.inferBody(b, declared)
		return in.b.instantiate(declared, map[*Type]*Type{})
	}

	switch in.state[b] {
	case inProgress:
		return in.env[b]
	case finished:
		return in.b.instantiate(in.env[b], map[*Type]*Type{})
	}

	// The body is inferred one level in, so that whatever it invents is a
	// variable of this binding and gets copied per use, while anything it
	// touches from an enclosing scope keeps its own, shallower level and stays
	// shared.
	in.b.level++
	node := in.b.fresh()
	in.env[b] = node
	in.state[b] = inProgress
	in.b.join(node, in.expr(b.Expression))
	in.state[b] = finished
	in.b.level--
	in.b.generalize(node)
	in.record(b.Name, node)

	return in.b.instantiate(node, map[*Type]*Type{})
}

// inferBody walks an annotated binding's definition once, so that the claim can
// be compared with it afterwards. The body is inferred *against* the signature:
// the declared argument types are pushed into the patterns, which is what gives
// a binder in `[h, t]` the field types the declaration promises instead of a
// fresh variable.
func (in *inferrer) inferBody(b *syntax.Binding, declared *Type) {
	if in.state[b] != unvisited {
		return
	}
	in.state[b] = inProgress

	in.b.level++
	node := in.b.fresh()
	in.env[b] = node
	// A copy, so that seeding the patterns cannot write anything back into the
	// declared type every other use is served from.
	in.b.join(node, in.exprAgainst(b.Expression, in.b.instantiate(declared, map[*Type]*Type{})))
	in.b.level--

	in.state[b] = finished
	in.b.generalize(node)
	in.record(b.Name, node)
}

// exprAgainst infers an expression with an expected type in hand. Only a lambda
// can use it — everything else is inferred bottom-up and compared afterwards —
// but that is the case that matters, since a lambda is where names get bound.
func (in *inferrer) exprAgainst(e syntax.Expression, want *Type) *Type {
	if lam, ok := e.(*syntax.Lambda); ok {
		return in.record(lam, in.lambdaAgainst(lam, want))
	}
	return in.expr(e)
}

// lambdaAgainst is the checking half of the walk: the domain is seeded from the
// declaration before the patterns are read, so each case's binders come out at
// the declared field types. `[h, t]` against `List a` gives `h : a` and
// `t : List a` — refinement by the shape of the pattern, which needs no flow
// analysis and no knowledge of what the other cases matched.
//
// The codomain is deliberately *not* seeded: joining it with the declared result
// would merge the claim into the finding and leave nothing to check.
func (in *inferrer) lambdaAgainst(l *syntax.Lambda, want *Type) *Type {
	w := find(want)
	if w.fun == nil {
		return in.lambda(l)
	}
	dom := in.b.fresh()
	cod := in.b.fresh()
	in.b.join(dom, w.fun.arg)
	for _, c := range l.Cases {
		in.b.join(dom, in.pattern(c.Pattern))
		in.b.join(cod, in.exprAgainst(c.Expression, w.fun.res))
	}
	return in.b.fn(dom, cod)
}

func (in *inferrer) lambda(l *syntax.Lambda) *Type {
	dom := in.b.fresh()
	cod := in.b.fresh()
	for _, c := range l.Cases {
		in.b.join(dom, in.pattern(c.Pattern))
		in.b.join(cod, in.expr(c.Expression))
	}
	return in.b.fn(dom, cod)
}

func (in *inferrer) pattern(p syntax.Pattern) *Type {
	switch node := p.(type) {
	case *syntax.Name:
		t := in.b.fresh()
		in.env[node] = t
		return in.record(node, t)

	case *syntax.NumberLiteral:
		return in.record(node, in.b.num())

	case *syntax.StringLiteral:
		return in.record(node, in.b.list(in.b.num()))

	case *syntax.TuplePattern:
		fields := make([]*Type, len(node.SubPatterns))
		for i, sub := range node.SubPatterns {
			fields[i] = in.pattern(sub)
		}
		return in.record(node, in.b.tuple(fields...))

	case *syntax.ListPattern:
		t := in.b.tuple()
		for i := len(node.SubPatterns) - 1; i >= 0; i-- {
			t = in.b.tuple(in.pattern(node.SubPatterns[i]), t)
		}
		return in.record(node, t)
	}
	return in.b.any()
}

// apply is where most of the information comes from: the argument's shape flows
// into the function's parameter, and the function's result becomes the type of
// the application.
//
// It is also where the argument is checked, and how strictly depends on where
// the parameter's type came from. A *trusted* callee — one with a signature, or
// a builtin — has a parameter that is exact and freshly instantiated for this
// call, so the argument can be required to be contained in it. Anything else has
// a parameter inference arrived at by joining whatever reached it, which is both
// narrower than intended (`isEmpty` only ever matches `[]`) and polluted by other
// call sites, so only outright disjointness is reported there.
func (in *inferrer) apply(fn, arg *Type, pos source.SourcePos, what string, trusted bool) *Type {
	f := find(fn)
	if f.top {
		return in.b.any()
	}
	if f.fun == nil {
		if !isVar(f) {
			in.warn("applied "+what+", which is "+in.namer.String(f)+", to an argument", pos)
			return in.b.any()
		}
		res := in.b.fresh()
		in.b.join(f, in.b.fn(arg, res))
		return res
	}

	if in.settled {
		// The refinement pass: nothing here is news.
	} else if trusted {
		if v, ok := admits(f.fun.arg, arg, map[containKey]bool{}); !ok {
			in.warn("passed "+in.namer.String(find(arg))+" to "+what+
				", which takes "+in.namer.String(find(f.fun.arg))+v.describe(), pos)
		}
	} else if conflicts(f.fun.arg, arg) {
		in.warn("passed "+in.namer.String(find(arg))+" to "+what+", which takes "+in.namer.String(find(f.fun.arg)), pos)
	}

	// The join is what instantiates the signature's variables: passing `List Num`
	// to `reverse : List a -> List a` puts Num into `a`, and the result node,
	// which is the same `a`, carries it back out.
	in.b.join(f.fun.arg, arg)
	return f.fun.res
}

// trusted reports whether a callee's parameter types are the author's rather
// than inference's: a builtin, or a binding with a signature. A partial
// application is still the same instantiated signature, so it stays trusted.
func (in *inferrer) trusted(e syntax.Expression) bool {
	switch node := e.(type) {
	case *syntax.Name:
		fact, ok := in.res.Uses[node]
		if !ok {
			return false
		}
		if fact.Kind == syntax.ResolveBuiltin {
			return true
		}
		if b, ok := fact.Def.(*syntax.Binding); ok {
			_, signed := in.sigTypes[b]
			return signed
		}
	case *syntax.QualifiedName:
		if b, ok := in.res.Quals[node]; ok {
			_, signed := in.sigTypes[b]
			return signed
		}
	case *syntax.Operation:
		if node.Operator == "" && len(node.Operands) > 0 {
			return in.trusted(node.Operands[0])
		}
	}
	return false
}

// conflicts reports whether two types have no shape in common at all — a number
// meeting a tuple, say. Joining them would widen to a union that no value can
// have, which in a language with no type declarations is the closest thing to
// a type error there is. Anything unknown on either side is not a conflict:
// silence is the right answer when the analysis does not know.
func conflicts(want, got *Type) bool {
	w, g := find(want), find(got)
	if w.top || g.top || isVar(w) || isVar(g) {
		return false
	}
	if w.num && g.num {
		return false
	}
	if w.fun != nil && g.fun != nil {
		return false
	}
	for arity := range w.tuples {
		if _, ok := g.tuples[arity]; ok {
			return false
		}
	}
	// Tuples of different arities alone are not a conflict: that is how a list
	// is written, the empty one and the pair being the same type.
	if len(w.tuples) > 0 && len(g.tuples) > 0 {
		return false
	}
	return true
}

func (in *inferrer) operation(op *syntax.Operation) *Type {
	pos := syntax.NodePos(op)

	switch op.Operator {
	case "": // application: f a b c
		t := in.expr(op.Operands[0])
		trusted := in.trusted(op.Operands[0])
		for _, arg := range op.Operands[1:] {
			t = in.apply(t, in.expr(arg), pos, describe(op.Operands[0]), trusted)
		}
		return t

	case ">": // a > f > g  =  g (f a)
		t := in.expr(op.Operands[0])
		for _, fn := range op.Operands[1:] {
			t = in.apply(in.expr(fn), t, pos, describe(fn), in.trusted(fn))
		}
		return t

	case "<": // f < g < x  =  f (g x)
		last := len(op.Operands) - 1
		t := in.expr(op.Operands[last])
		for i := last - 1; i >= 0; i-- {
			t = in.apply(in.expr(op.Operands[i]), t, pos, describe(op.Operands[i]), in.trusted(op.Operands[i]))
		}
		return t

	case "*>": // (a *> b) x = b (a x)
		in0 := in.b.fresh()
		t := in0
		for _, fn := range op.Operands {
			t = in.apply(in.expr(fn), t, pos, describe(fn), in.trusted(fn))
		}
		return in.b.fn(in0, t)

	case "<*": // (a <* b) x = a (b x)
		in0 := in.b.fresh()
		t := in0
		for i := len(op.Operands) - 1; i >= 0; i-- {
			t = in.apply(in.expr(op.Operands[i]), t, pos, describe(op.Operands[i]), in.trusted(op.Operands[i]))
		}
		return in.b.fn(in0, t)
	}

	return in.b.any()
}

func describe(e syntax.Expression) string {
	switch node := e.(type) {
	case *syntax.Name:
		return node.Value
	case *syntax.QualifiedName:
		return node.Module + "." + node.Value
	}
	return "this expression"
}

// sourcesOf collects the distinct source files a program and its modules came
// from, in a deterministic order, so their comments can be scanned once each.
func sourcesOf(program *syntax.Program, modules map[string]*syntax.Module) []*source.Source {
	var out []*source.Source
	seen := map[*source.Source]bool{}
	add := func(pos source.SourcePos) {
		if pos.File != nil && !seen[pos.File] {
			seen[pos.File] = true
			out = append(out, pos.File)
		}
	}
	add(program.Start)
	for _, mod := range sortedModules(modules) {
		add(mod.Start)
	}
	return out
}

// unitsOf describes each source file the analysis saw: which module it is (the
// empty string for the program), the file itself, and what it imports.
func unitsOf(program *syntax.Program, modules map[string]*syntax.Module) []unit {
	var out []unit
	seen := map[*source.Source]bool{}

	names := func(ns []*syntax.Name) []string {
		out := make([]string, len(ns))
		for i, n := range ns {
			out[i] = n.Value
		}
		return out
	}

	if program.Start.File != nil {
		seen[program.Start.File] = true
		out = append(out, unit{mod: "", file: program.Start.File, imports: names(program.Imports)})
	}
	for _, mod := range sortedModules(modules) {
		if mod.Start.File == nil || seen[mod.Start.File] {
			continue
		}
		seen[mod.Start.File] = true
		out = append(out, unit{mod: mod.Name, file: mod.Start.File, imports: names(mod.Imports)})
	}
	return out
}

// A givenSig is a signature that attached to a binding: the text to display, the
// declared type names it mentions, and whether the check contradicted it.
type givenSig struct {
	text       string
	names      []Decl
	conflicted bool
	asserted   int // `!` marks, counted so the report can total them

	// sig is the signature as parsed. Propagating an assertion needs the shape,
	// not just the text: which argument position carries the mark.
	sig Signature
}

// collectSignatures resolves every `--> name : Type` annotation in its own unit's
// scope, attaches it to the binding below it, and builds the type each one
// describes. It runs before the walk, because those types are what the walk uses.
//
// The bindings come from the AST rather than from the environment: nothing has
// been inferred yet, and a signature has to be in hand before the binding it
// names is first referred to.
func (in *inferrer) collectSignatures(units []unit, perModule map[string][]Decl,
	program *syntax.Program, modules map[string]*syntax.Module) map[*syntax.Binding]givenSig {

	out := map[*syntax.Binding]givenSig{}
	bindings := allBindings(program, modules)

	for _, u := range units {
		scope := buildScope(u, perModule)
		sigs, warns := CollectSignatures(u.file, scope)
		in.warnings = append(in.warnings, warns...)

		// Only the bindings in this file can be the target of its signatures.
		var local []*syntax.Binding
		for _, b := range bindings {
			if b.Name.Pos.File == u.file {
				local = append(local, b)
			}
		}
		bound, warns := attach(sigs, local)
		in.warnings = append(in.warnings, warns...)

		for b, sig := range bound {
			out[b] = givenSig{text: sig.Text, names: namedIn(sig.Text, scope), sig: sig,
				asserted: countAsserted(sig.Pat, map[*pattern]bool{})}
			in.sigTypes[b] = in.b.sigType(sig.Pat)
		}
	}
	return out
}

// checkGiven compares each claim with what its own definition inferred to. This
// is the half of the contract that keeps a declared type honest on the way *out*
// — the result a body actually produces has to be one the signature allows —
// while exhaustiveness keeps it honest on the way *in*, by demanding that the
// patterns cover the domain it claims. Neither means much without the other:
// call sites are checked against the declaration, so a declaration wider than
// the code behind it is a lie every caller inherits.
func (in *inferrer) checkGiven(given map[*syntax.Binding]givenSig) {
	for b, g := range given {
		found, ok := in.env[b]
		if !ok {
			continue // nothing referred to it, so there is nothing to compare
		}
		if v, ok := admits(in.sigTypes[b], found, map[containKey]bool{}); !ok {
			g.conflicted = true
			given[b] = g
			in.warnings = append(in.warnings, Warning{
				Message: fmt.Sprintf("%s : %s does not hold — %s (inferred %s)",
					g.sig.Name, g.text, v.explain(), in.namer.String(found)),
				Pos: g.sig.Pos,
			})
			continue
		}
		// Only when the claim is not simply wrong: the patterns cannot cover a
		// domain the code does not accept, and suggesting `!` for a plainly wrong
		// type is how the mark would become a rubber stamp.
		in.checkExhaustive(b, g.sig)
	}
}

// allBindings lists every binding in the program and its modules, nested lets
// included, in a deterministic order.
func allBindings(program *syntax.Program, modules map[string]*syntax.Module) []*syntax.Binding {
	var out []*syntax.Binding
	var walk func(syntax.Expression)
	add := func(b *syntax.Binding) {
		out = append(out, b)
		walk(b.Expression)
	}
	walk = func(e syntax.Expression) {
		switch node := e.(type) {
		case *syntax.Let:
			for _, b := range node.Bindings {
				add(b)
			}
			walk(node.Expression)
		case *syntax.Lambda:
			for _, c := range node.Cases {
				walk(c.Expression)
			}
		case *syntax.TupleExpr:
			for _, x := range node.SubExpressions {
				walk(x)
			}
		case *syntax.List:
			for _, x := range node.SubExpressions {
				walk(x)
			}
		case *syntax.Operation:
			for _, x := range node.Operands {
				walk(x)
			}
		}
	}
	for _, mod := range sortedModules(modules) {
		for _, b := range mod.PublicBindings {
			add(b)
		}
	}
	walk(program.Body)
	return out
}

// namedIn resolves the declared types a signature mentions, so that displaying
// the signature still puts their equations in the preamble.
func namedIn(text string, scope typeScope) []Decl {
	var out []Decl
	seen := map[string]bool{}
	for _, ref := range referencedTypes(text) {
		if mod, name, ok := splitQualified(ref); ok {
			if tbl, exists := scope.byMod[mod]; exists {
				if d, found := tbl[name]; found && !seen[d.Name] {
					seen[d.Name] = true
					out = append(out, d)
				}
			}
			continue
		}
		if d, found := scope.byName[ref]; found && !seen[d.Name] {
			seen[d.Name] = true
			out = append(out, d)
		}
	}
	return out
}

// Coverage counts how many reported module bindings show an author's signature
// rather than an inferred shape.
func (a *Analysis) Coverage() (given, total int) {
	for _, m := range a.Modules {
		for _, e := range m.Entries {
			total++
			if e.Given {
				given++
			}
		}
	}
	return given, total
}

// Assertions counts the `!` marks across the report, and how many signatures
// carry at least one. They are assumptions the analysis could not verify, so
// they are worth being able to count.
func (a *Analysis) Assertions() (marks, sigs int) {
	for _, m := range a.Modules {
		for _, e := range m.Entries {
			if e.Asserted > 0 {
				marks += e.Asserted
				sigs++
			}
		}
	}
	return marks, sigs
}
