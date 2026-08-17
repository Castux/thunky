package syntax

import (
	"reflect"
	"regexp"

	"github.com/Castux/thunky/internal/source"
)

type Node interface {
	FirstPos() source.SourcePos
	LastPos() source.SourcePos
}

func NodePos(node Node) source.SourcePos {
	return node.FirstPos().To(node.LastPos())
}

var reName = regexp.MustCompile(`\w+$`)

func NodeType(node Node) string {
	return reName.FindString(reflect.TypeOf(node).String())
}

func (x *Program) FirstPos() source.SourcePos { return x.Start }
func (x *Program) LastPos() source.SourcePos  { return x.Body.LastPos() }

func (x *Module) FirstPos() source.SourcePos { return x.Start }
func (x *Module) LastPos() source.SourcePos {
	return x.PublicBindings[len(x.PublicBindings)-1].LastPos()
}

func (x *Binding) FirstPos() source.SourcePos { return x.Name.FirstPos() }
func (x *Binding) LastPos() source.SourcePos  { return x.Expression.LastPos() }

func (x *Let) FirstPos() source.SourcePos { return x.Start }
func (x *Let) LastPos() source.SourcePos  { return x.Expression.LastPos() }

func (x *Lambda) FirstPos() source.SourcePos { return x.Start }
func (x *Lambda) LastPos() source.SourcePos  { return x.End }

func (x *LambdaCase) FirstPos() source.SourcePos { return x.Pattern.FirstPos() }
func (x *LambdaCase) LastPos() source.SourcePos  { return x.Expression.LastPos() }

func (x *TuplePattern) FirstPos() source.SourcePos { return x.Start }
func (x *TuplePattern) LastPos() source.SourcePos  { return x.End }

func (x *ListPattern) FirstPos() source.SourcePos { return x.Start }
func (x *ListPattern) LastPos() source.SourcePos  { return x.End }

func (x *Operation) FirstPos() source.SourcePos { return x.Operands[0].FirstPos() }
func (x *Operation) LastPos() source.SourcePos  { return x.Operands[len(x.Operands)-1].LastPos() }

func (x *Name) FirstPos() source.SourcePos { return x.Pos }
func (x *Name) LastPos() source.SourcePos  { return x.Pos }

func (x *QualifiedName) FirstPos() source.SourcePos { return x.Start }
func (x *QualifiedName) LastPos() source.SourcePos  { return x.End }

func (x *NumberLiteral) FirstPos() source.SourcePos { return x.Pos }
func (x *NumberLiteral) LastPos() source.SourcePos  { return x.Pos }

func (x *StringLiteral) FirstPos() source.SourcePos { return x.Pos }
func (x *StringLiteral) LastPos() source.SourcePos  { return x.Pos }

func (x *TupleExpr) FirstPos() source.SourcePos { return x.Start }
func (x *TupleExpr) LastPos() source.SourcePos  { return x.End }

func (x *List) FirstPos() source.SourcePos { return x.Start }
func (x *List) LastPos() source.SourcePos  { return x.End }

func TraverseList[N Node](list []N, pre, post func(n Node)) {
	for _, node := range list {
		Traverse(node, pre, post)
	}
}

func Traverse(node Node, pre, post func(n Node)) {

	if pre != nil {
		pre(node)
	}

	switch n := node.(type) {
	case *Program:
		TraverseList(n.Imports, pre, post)
		Traverse(n.Body, pre, post)

	case *Module:
		TraverseList(n.Imports, pre, post)
		TraverseList(n.PublicBindings, pre, post)

	case *Binding:
		Traverse(n.Name, pre, post)
		Traverse(n.Expression, pre, post)

	case *Let:
		TraverseList(n.Bindings, pre, post)
		Traverse(n.Expression, pre, post)

	case *Lambda:
		TraverseList(n.Cases, pre, post)

	case *LambdaCase:
		Traverse(n.Pattern, pre, post)
		Traverse(n.Expression, pre, post)

	case *TuplePattern:
		TraverseList(n.SubPatterns, pre, post)

	case *ListPattern:
		TraverseList(n.SubPatterns, pre, post)

	case *Operation:
		TraverseList(n.Operands, pre, post)

	case *TupleExpr:
		TraverseList(n.SubExpressions, pre, post)

	case *List:
		TraverseList(n.SubExpressions, pre, post)
	}

	if post != nil {
		post(node)
	}
}
