package value

import "github.com/Castux/thunky/internal/source"

// PC is a program counter: an index into the compiled instruction array. It lives
// here because Thunk and Closure store entry points; the backend re-exports it.
type PC = int32

// A Value is one runtime value, passed around by value (not by pointer). It is a
// small tagged union: a Number lives inline in Num, while every compound or
// heap-resident value keeps a pointer in Ref. This is the deliberate alternative
// to a Go interface for the runtime representation:
//
//   - A number never allocates. A float64 boxed into an interface{} heap-allocates
//     on every arithmetic result; Value{Tag: NumberTag, Num: x} does not. Numeric
//     programs allocate the most, so this is where the representation earns its
//     keep.
//   - Ref holds a *pointer* (to Cons, Tuple, …). A pointer stored in an interface
//     does NOT allocate in Go, so compound values pay only their own heap cost,
//     never an extra box.
//   - The Tag makes the variant explicit and cheap to switch on, instead of a
//     type switch on an interface's dynamic type.
type Value struct {
	Tag Tag
	Num float64
	Ref any
}

type Tag uint8

const (
	NumberTag      Tag = iota // Num holds the value; Ref is nil
	ConsTag                   // Ref = *Cons
	TupleTag                  // Ref = *Tuple   (arity 0, 1, 3, 4, …; never 2)
	ClosureTag                // Ref = *Closure
	BuiltinTag                // Ref = *Builtin
	CompositionTag            // Ref = *Composition
	ApplyTag                  // Ref = *Apply   (a runtime-synthesized application)
	ThunkTag                  // Ref = *Thunk   (not yet in weak head normal form)
)

// NoCode marks a thunk that has no compiled body: a graph-style indirection thunk
// (a pattern binding) whose deferred computation is simply "reduce Value". A real
// code thunk has Code >= 0.
const NoCode PC = -1

// A Thunk is a deferred computation that memoises its result. It comes in three
// forms, distinguished without a tag field:
//
//   - Read != nil — a stdin stream cell; forcing it calls Read to obtain its weak
//     head normal form (a cons cell or the empty list) and reads one input item.
//   - Code == NoCode — a graph indirection (a pattern binding); forcing it reduces
//     the value already in Value and memoises the result. This keeps the bound
//     name on the trace/show path without the name's owner having to re-evaluate.
//   - Code >= 0 — a compiled thunk; forcing it runs the body at Code over the
//     captured frames (whole-frame capture, see below).
//
// Code thunks capture the entire enclosing activation (Locals and Upvalues) by
// reference and address it with the enclosing slot numbers — no renumbering, and
// mutual recursion in a let group resolves because every binding thunk shares the
// frame, which is fully populated before any binding is forced.
//
// Every thunk memoises (call-by-need): the first force runs the computation and
// writes the weak-head-normal-form result back into Value (setting Forced), so it
// is never recomputed. This is why re-forcing a thunk that embeds an output
// builtin (peek/show/write/bwrite) does not repeat the effect — the effect runs
// exactly once, when the thunk is first forced.
type Thunk struct {
	Forced   bool
	Value    Value
	Name     string
	Code     PC
	Locals   []Value
	Upvalues []Value
	Read     func() Value
}

// A Cons is a head/tail pair. Thunky draws no distinction between a 2-tuple and
// a list cons cell (see LANGUAGE.md §11), so every arity-2 tuple — list literals,
// string code points, the stdin stream, a bare [a, b] pair — is a Cons. This packs
// the pair into one allocation and lets list code destructure cons cells directly.
type Cons struct {
	Head Value
	Tail Value
}

// A Tuple is an ordered sequence of any arity except 2 (arity 0, 1, 3, 4, …); the
// empty tuple is also the empty list. Arity-2 tuples are Cons cells instead.
type Tuple struct {
	Fields []Value
}

// A Closure is a lambda (one or more pattern cases) paired with the minimal set of
// free variables it captured. Frame is the largest of its cases' frame sizes,
// allocated once per application. NoMatch is the source span of the whole pattern
// set, used to locate a "no pattern matched" error.
type Closure struct {
	Code    PC
	Env     []Value
	Frame   int
	NoMatch source.SourcePos
}

// A Builtin is a primitive operation as a first-class value. It carries the
// operation, its arity, and the arguments gathered so far; applying it appends one
// argument, and when the count reaches Arity the machine runs the operation. This
// single representation is both the saturated and the partially-applied form:
// partial application is just a Builtin with fewer Args than Arity (see prims.go).
type Builtin struct {
	Prim  PrimOp
	Arity int
	Args  []Value
	Name  string
}

// A Composition is an unreduced function composition. Applying (First ∘ Second) to
// x reduces to First (Second x); see the CompositionTag case of reduce.
type Composition struct {
	First  Value
	Second Value
}

// An Apply is a runtime-synthesized application Fn Arg. The compiled bytecode never
// builds one — it pushes arguments and enters the head (the spine-less model). The
// only place an Apply is created is reducing a composition, which must turn
// (First ∘ Second) x into the application First (Second x). Pos locates a
// "cannot apply" failure.
type Apply struct {
	Fn  Value
	Arg Value
	Pos source.SourcePos
}

// Constructors for the common values, keeping the Tag / Ref pairing in one place.

func NumberValue(n float64) Value   { return Value{Tag: NumberTag, Num: n} }
func ConsValue(h, t Value) Value    { return Value{Tag: ConsTag, Ref: &Cons{Head: h, Tail: t}} }
func TupleValue(t *Tuple) Value     { return Value{Tag: TupleTag, Ref: t} }
func ThunkValue(t *Thunk) Value     { return Value{Tag: ThunkTag, Ref: t} }
func ClosureValue(c *Closure) Value { return Value{Tag: ClosureTag, Ref: c} }
func BuiltinValue(b *Builtin) Value { return Value{Tag: BuiltinTag, Ref: b} }
func ApplyValue(fn, arg Value, pos source.SourcePos) Value {
	return Value{Tag: ApplyTag, Ref: &Apply{Fn: fn, Arg: arg, Pos: pos}}
}

// EmptyTuple is the shared empty tuple / empty list. It is immutable, so a single
// instance is reused everywhere one is needed.
var EmptyTuple = Value{Tag: TupleTag, Ref: &Tuple{}}

// FoldStringValue decodes a string into the runtime representation of a string: a
// cons list of code points ending in the empty tuple. The lowerer calls it once
// per literal, building a shared immutable constant.
func FoldStringValue(s string) Value {
	current := EmptyTuple
	runes := []rune(s)
	for i := len(runes) - 1; i >= 0; i-- {
		current = ConsValue(NumberValue(float64(runes[i])), current)
	}
	return current
}

func (v Value) Thunk() *Thunk             { return v.Ref.(*Thunk) }
func (v Value) Cons() *Cons               { return v.Ref.(*Cons) }
func (v Value) Tuple() *Tuple             { return v.Ref.(*Tuple) }
func (v Value) Closure() *Closure         { return v.Ref.(*Closure) }
func (v Value) Builtin() *Builtin         { return v.Ref.(*Builtin) }
func (v Value) Composition() *Composition { return v.Ref.(*Composition) }
func (v Value) Apply() *Apply             { return v.Ref.(*Apply) }

// IsFunction reports whether v can be applied to an argument. Used only for
// display and error wording.
func (v Value) IsFunction() bool {
	return v.Tag == ClosureTag || v.Tag == BuiltinTag || v.Tag == CompositionTag
}
