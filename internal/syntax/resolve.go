package syntax

import "github.com/Castux/thunky/internal/source"

type ResolveKind uint8

const (
	ResolveLocal ResolveKind = iota
	ResolveModule
	ResolveBuiltin
)

type ResolutionFact struct {
	Kind   ResolveKind
	Def    Node    // The defining *Binding or *Name; nil for builtins
	Module *Module // Set if Kind == ResolveModule
}

type Resolution struct {
	Errors int
	Uses   map[*Name]ResolutionFact
	Quals  map[*QualifiedName]*Binding
}

type scope struct {
	node  Node
	binds map[string]Node
}

type Resolver struct {
	program *Program
	modules map[string]*Module

	res      *Resolution
	imported map[string]bool
	stack    []*scope
	defining map[*Name]bool
}

func Resolve(program *Program, modules map[string]*Module) *Resolution {
	r := &Resolver{
		program:  program,
		modules:  modules,
		res:      &Resolution{Uses: make(map[*Name]ResolutionFact), Quals: make(map[*QualifiedName]*Binding)},
		imported: make(map[string]bool),
		defining: make(map[*Name]bool),
	}
	r.run()
	return r.res
}

var knownBuiltins = []string{
	"add", "sub", "mul", "div", "fdiv", "mod", "fmod", "pow", "sqrt",
	"eq", "lt", "lte", "gte", "gt", "neq",
	"eval", "peek", "show", "write", "bwrite", "equal", "string", "hash", "stdin", "bstdin",
	"seq",
}

func (r *Resolver) pushScope(node Node) {
	r.stack = append(r.stack, &scope{node: node, binds: make(map[string]Node)})
}

func (r *Resolver) popScope() {
	r.stack = r.stack[:len(r.stack)-1]
}

func (r *Resolver) currentScope() *scope {
	return r.stack[len(r.stack)-1]
}

func (r *Resolver) define(name string, def Node, pos source.SourcePos) {
	s := r.currentScope()
	if existing, ok := s.binds[name]; ok {
		source.Log(name+" was already defined", pos, source.SeverityInfo)
		var existPos source.SourcePos
		if existing != nil {
			existPos = existing.FirstPos()
		}
		source.Log("here", existPos, source.SeverityError)
		r.res.Errors++
		return
	}
	s.binds[name] = def
}

func (r *Resolver) handleImports(imports []*Name) {
	for _, imp := range imports {
		r.imported[imp.Value] = true
		r.defining[imp] = true
		mod := r.modules[imp.Value]
		r.pushScope(mod)
		for _, b := range mod.PublicBindings {
			r.define(b.Name.Value, b, b.Name.Pos)
			r.defining[b.Name] = true
		}
	}
}

func (r *Resolver) resolveName(name *Name) {
	for i := len(r.stack) - 1; i >= 0; i-- {
		s := r.stack[i]
		if def, ok := s.binds[name.Value]; ok {
			kind := ResolveLocal
			var mod *Module
			if s.node == nil {
				kind = ResolveBuiltin
			} else if m, isMod := s.node.(*Module); isMod {
				kind = ResolveModule
				mod = m
			}
			r.res.Uses[name] = ResolutionFact{Kind: kind, Def: def, Module: mod}
			return
		}
	}
	source.Log("no definition for "+name.Value, name.Pos, source.SeverityError)
	r.res.Errors++
}

func (r *Resolver) resolveQualified(name *QualifiedName) {
	if !r.imported[name.Module] {
		source.Log("module "+name.Module+" was not imported", name.Start, source.SeverityError)
		r.res.Errors++
		return
	}
	mod := r.modules[name.Module]
	for _, b := range mod.PublicBindings {
		if b.Name.Value == name.Value {
			r.res.Quals[name] = b
			return
		}
	}
	source.Log("no definition for "+name.Value+" in module "+name.Module, name.Start, source.SeverityError)
	r.res.Errors++
}

func (r *Resolver) getNamesInPattern(patt Pattern) []*Name {
	var names []*Name
	Traverse(patt, func(n Node) {
		if nm, ok := n.(*Name); ok {
			names = append(names, nm)
		}
	}, nil)
	return names
}

func (r *Resolver) run() {
	r.resetToBuiltins()
	r.analyze(r.program)
	for _, mod := range r.modules {
		r.resetToBuiltins()
		r.analyze(mod)
	}
}

func (r *Resolver) resetToBuiltins() {
	r.stack = nil
	r.imported = make(map[string]bool)
	r.pushScope(nil) // builtin scope
	for _, b := range knownBuiltins {
		r.define(b, nil, source.SourcePos{})
	}
}

func (r *Resolver) analyze(root Node) {
	Traverse(root, func(n Node) {
		switch node := n.(type) {
		case *Program:
			r.handleImports(node.Imports)
		case *Module:
			r.handleImports(node.Imports)
			r.pushScope(node)
			for _, b := range node.PublicBindings {
				r.define(b.Name.Value, b, b.Name.Pos)
			}
		case *Let:
			r.pushScope(node)
			for _, b := range node.Bindings {
				r.define(b.Name.Value, b, b.Name.Pos)
				r.defining[b.Name] = true
			}
		case *Lambda:
			r.pushScope(node)
		case *LambdaCase:
			r.pushScope(node)
			for _, name := range r.getNamesInPattern(node.Pattern) {
				r.define(name.Value, name, name.Pos)
				r.defining[name] = true
			}
		case *Name:
			if !r.defining[node] {
				r.resolveName(node)
			}
		case *QualifiedName:
			r.resolveQualified(node)
		}
	}, func(n Node) {
		for len(r.stack) > 0 && r.currentScope().node == n {
			r.popScope()
		}
	})
}
