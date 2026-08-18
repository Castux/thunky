package types

import (
	"github.com/Castux/thunky/internal/syntax"
)

// Local bindings: the named values inside a `let`.
//
// A module's public bindings are its interface and are reported as such. Every
// other name a program gives a value lives in a `let`, and those are where most
// of the code actually is — a module binding is often one line delegating to a
// handful of locals, and a program is usually a single `let` with the whole
// program inside it.
//
// They are named by the path that reaches them, `magAdd.go` rather than `go`,
// because `go` and `step` and `loop` appear many times over in a library and a
// bare name would say nothing about which one is being reported.

// A local is one `let` binding together with the path that names it.
type local struct {
	mod  string
	path string
	b    *syntax.Binding
}

// collectLocals walks every unit and returns its `let` bindings in source order.
func collectLocals(program *syntax.Program, modules map[string]*syntax.Module) []local {
	var out []local
	c := localCollector{}

	for _, mod := range sortedModules(modules) {
		c.mod = mod.Name
		for _, b := range mod.PublicBindings {
			c.walk(b.Expression, b.Name.Value)
		}
		out = append(out, c.take()...)
	}

	c.mod = ""
	c.walk(program.Body, "")
	out = append(out, c.take()...)
	return out
}

type localCollector struct {
	mod   string
	found []local
}

func (c *localCollector) take() []local {
	out := c.found
	c.found = nil
	return out
}

// join builds the dotted path. A program's outermost `let` has no enclosing
// binding to prefix with, so its names stand alone.
func join(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func (c *localCollector) walk(e syntax.Expression, prefix string) {
	switch node := e.(type) {
	case *syntax.Let:
		for _, b := range node.Bindings {
			path := join(prefix, b.Name.Value)
			c.found = append(c.found, local{mod: c.mod, path: path, b: b})
			c.walk(b.Expression, path)
		}
		c.walk(node.Expression, prefix)
	case *syntax.Lambda:
		for _, cs := range node.Cases {
			c.walk(cs.Expression, prefix)
		}
	case *syntax.TupleExpr:
		for _, x := range node.SubExpressions {
			c.walk(x, prefix)
		}
	case *syntax.List:
		for _, x := range node.SubExpressions {
			c.walk(x, prefix)
		}
	case *syntax.Operation:
		for _, x := range node.Operands {
			c.walk(x, prefix)
		}
	}
}
