package types

// The builtins are where all the concrete information enters the analysis:
// every other type is a consequence of these signatures meeting the literals
// and the patterns. `a` and `b` below are fresh variables per use, so `show`
// staying polymorphic does not force its two call sites to agree.

func (in *inferrer) builtin(name string) *Type {
	b := in.b
	num := func() *Type { return b.num() }
	str := func() *Type { return b.list(b.num()) }

	switch name {
	// Arithmetic and comparison: numbers in, a number out. The comparisons
	// return 0 or 1, which in this language is a number like any other.
	case "add", "sub", "mul", "div", "fdiv", "mod", "fmod", "pow",
		"eq", "lt", "lte", "gte", "gt", "neq":
		return b.fn(num(), b.fn(num(), num()))

	case "sqrt":
		return b.fn(num(), num())

	// equal and hash look at the structure of any value.
	case "equal":
		return b.fn(b.fresh(), b.fn(b.fresh(), num()))
	case "hash":
		return b.fn(b.fresh(), num())

	// The output builtins return their argument, so they can be dropped into a
	// pipe without changing its type.
	case "eval", "peek", "show":
		t := b.fresh()
		return b.fn(t, t)

	// write and bwrite take a list of code points (or bytes) and hand it back.
	case "write", "bwrite":
		t := str()
		return b.fn(t, t)

	case "string":
		return b.fn(b.fresh(), str())

	// seq forces its first argument and returns the second unforced.
	case "seq":
		a, c := b.fresh(), b.fresh()
		return b.fn(a, b.fn(c, c))

	// Standard input is a lazy list: of code points, or of raw bytes.
	case "stdin", "bstdin":
		return str()
	}

	return b.any()
}
