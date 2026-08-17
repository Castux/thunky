package core

import (
	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/value"
)

type Expr interface{ coreExpr() }

func (Num) coreExpr()     {}
func (Const) coreExpr()   {}
func (Stdin) coreExpr()   {}
func (Var) coreExpr()     {}
func (App) coreExpr()     {}
func (Compose) coreExpr() {}
func (Cons) coreExpr()    {}
func (Tuple) coreExpr()   {}
func (Prim) coreExpr()    {}
func (Let) coreExpr()     {}
func (Lambda) coreExpr()  {}
func (Thunk) coreExpr()   {}

type Num struct{ Val float64 }
type Const struct{ Val value.Value }

// Stdin is a reference to one of the two standard-input streams: the code-point
// stream, or the byte stream when Bytes is set. It is a node of its own rather
// than a Const holding the stream value, because the stream is a stateful thunk
// that is created on demand at run time — a constant pool is for immutable
// values, and one holding a live stream has nothing sensible to render in the
// AST or bytecode dumps.
type Stdin struct{ Bytes bool }

type Var struct{ Addr Addr }

type App struct {
	Head Expr
	Args []Expr
	Pos  source.SourcePos
}

type Compose struct {
	Forward bool
	Fns     []Expr
}

type Cons struct{ Head, Tail Expr }

type Tuple struct{ Fields []Expr }

type Prim struct {
	Op   value.PrimOp
	Args []Expr
	Pos  source.SourcePos
}

type Let struct {
	Binds []Bind
	Body  Expr
}

type Lambda struct {
	Cases     []Case
	Free      []Addr
	FreeNames []string // debug: name of each captured variable, parallel to Free
	Frame     int
	NoMatch   source.SourcePos // span of the whole pattern set, for the "no pattern matched" error
}

type Thunk struct {
	Body  Expr
	Frame int
	Name  string
	Pos   source.SourcePos // debug: definition site, for the bytecode dump
}

type AddrKind uint8

const (
	AddrLocal AddrKind = iota
	AddrUpvalue
	AddrModule
)

type Addr struct {
	Kind   AddrKind
	Slot   int
	Module string
}

type Bind struct {
	Slot int
	Name string
	Body Expr
}

type Case struct {
	Pattern Pattern
	Body    Expr
	Frame   int
}

type Pattern interface{ corePattern() }

func (PatternTuple) corePattern() {}
func (PatternVar) corePattern()   {}
func (PatternConst) corePattern() {}

type PatternTuple struct {
	Fields []Pattern // arity 2 is cons
}

type PatternVar struct {
	Slot int
	Name string // the bound variable's name, kept for traces and show
}

type PatternConst struct {
	Val value.Value
}
