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

	warnings []Warning
	seenWarn map[string]bool

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
	in.expr(program.Body)

	namer := in.namer
	analysis := &Analysis{Program: namer.String(bodyType), Warnings: in.warnings}
	for _, mod := range sortedModules(modules) {
		entry := ModuleEntry{Name: mod.Name}
		for _, b := range mod.PublicBindings {
			t, ok := in.env[b]
			if !ok {
				continue
			}
			entry.Entries = append(entry.Entries, Entry{
				Name: b.Name.Value,
				Pos:  b.Name.Pos,
				Type: namer.StringIn(mod.Name, t),
			})
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
	in.checkSignatures(units, perModule)

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
func (in *inferrer) apply(fn, arg *Type, pos source.SourcePos, what string) *Type {
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
	if conflicts(f.fun.arg, arg) {
		in.warn("passed "+in.namer.String(find(arg))+" to "+what+", which takes "+in.namer.String(find(f.fun.arg)), pos)
	}
	in.b.join(f.fun.arg, arg)
	return f.fun.res
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
	// A parameter whose type is a function that contains itself is a chain like
	// `core.case`, which takes a value one moment and another (condition, value)
	// pair the next. The analysis folds the two into one node, so everything
	// passed to it looks wrong. Nothing said about such a parameter is worth
	// saying. (A list is cyclic too, but has no function in the cycle.)
	if isRecursiveFunction(w) {
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
		for _, arg := range op.Operands[1:] {
			t = in.apply(t, in.expr(arg), pos, describe(op.Operands[0]))
		}
		return t

	case ">": // a > f > g  =  g (f a)
		t := in.expr(op.Operands[0])
		for _, fn := range op.Operands[1:] {
			t = in.apply(in.expr(fn), t, pos, describe(fn))
		}
		return t

	case "<": // f < g < x  =  f (g x)
		last := len(op.Operands) - 1
		t := in.expr(op.Operands[last])
		for i := last - 1; i >= 0; i-- {
			t = in.apply(in.expr(op.Operands[i]), t, pos, describe(op.Operands[i]))
		}
		return t

	case "*>": // (a *> b) x = b (a x)
		in0 := in.b.fresh()
		t := in0
		for _, fn := range op.Operands {
			t = in.apply(in.expr(fn), t, pos, describe(fn))
		}
		return in.b.fn(in0, t)

	case "<*": // (a <* b) x = a (b x)
		in0 := in.b.fresh()
		t := in0
		for i := len(op.Operands) - 1; i >= 0; i-- {
			t = in.apply(in.expr(op.Operands[i]), t, pos, describe(op.Operands[i]))
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

// checkSignatures resolves every `--> name : Type` annotation in its own unit's
// scope, attaches it to the binding below it, and reports contradictions.
func (in *inferrer) checkSignatures(units []unit, perModule map[string][]Decl) {
	// Every binding the walk gave a type to, so a signature can be matched to one.
	var bindings []*syntax.Binding
	for node := range in.env {
		if b, ok := node.(*syntax.Binding); ok {
			bindings = append(bindings, b)
		}
	}

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
			t, ok := in.env[b]
			if !ok {
				continue
			}
			if msg, bad := conflict(sig.Pat, t, in.namer, "", map[[2]any]bool{}); bad {
				in.warnings = append(in.warnings, Warning{
					Message: fmt.Sprintf("%s : %s does not hold — %s (inferred %s)",
						sig.Name, sig.Text, msg, in.namer.String(t)),
					Pos: sig.Pos,
				})
			}
		}
	}
}
