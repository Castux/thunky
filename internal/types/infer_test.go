package types_test

import (
	"strings"
	"testing"

	"github.com/Castux/thunky/internal/syntax"
	"github.com/Castux/thunky/internal/types"
)

// analyze runs the whole front end over one source string and returns the
// inferred type of the program body. No modules are loaded, so a case can only
// use the builtins — which is the point: everything else is a consequence of
// them.
func analyze(t *testing.T, src string) (string, string) {
	t.Helper()

	tokens := syntax.LexContent("test.th", src)
	if tokens == nil {
		t.Fatalf("lexing failed:\n%s", src)
	}
	program := syntax.ParseProgram(tokens)
	if program == nil {
		t.Fatalf("parsing failed:\n%s", src)
	}
	modules := map[string]*syntax.Module{}
	resolution := syntax.Resolve(program, modules)
	if resolution.Errors > 0 {
		t.Fatalf("resolution failed:\n%s", src)
	}

	a := types.Infer(program, modules, resolution)
	// The equations are part of the answer: without them "T1 a -> Num" does not
	// say whether T1 is a list, an infinite list, or something else entirely.
	// They cover every type the analysis rendered, sub-expressions included, so a
	// case whose own type mentions none of them can still produce one.
	eqs := make([]string, len(a.Equations))
	for i, e := range a.Equations {
		eqs[i] = e.Header() + " : " + e.Body
	}
	return a.Program, strings.Join(eqs, "; ")
}

// list is the equation a terminated list produces, and infinite is the one an
// unterminated one produces. They are different types; the whole point of the
// notation is that they now read differently.
const (
	list     = "T1 a : [] | [a, T1 a]"
	infinite = "T1 a : [a, T1 a]"
)

// TestBuiltins pins the types the analysis reads straight off the builtins.
func TestBuiltins(t *testing.T) {
	cases := []struct{ src, want, eqs string }{
		{`show 1`, `Num`, ``},
		{`show "hi"`, `T1 Num`, list},
		{`add 1 2`, `Num`, ``},
		{`x -> add x 1`, `Num -> Num`, ``},
		{`[1, "a"]`, `[Num, T1 Num]`, list},
		{`[1; 2]`, `[Num; Num]`, ``},
		{`x -> y -> [x, y]`, `a -> b -> [a, b]`, ``},
		{`x -> x`, `a -> a`, ``},
		{`stdin`, `T1 Num`, list},
		// Num, but stdin is still a list somewhere inside it, so the equation is
		// in the set: it covers every type the analysis rendered, not just this one.
		{`equal 1 stdin`, `Num`, list},
		{`seq 1 "a"`, `T1 Num`, list},
		{`string 1`, `T1 Num`, list},
	}
	for _, c := range cases {
		got, eqs := analyze(t, c.src)
		if got != c.want || eqs != c.eqs {
			t.Errorf("%s\n  got  %-24s [%s]\n  want %-24s [%s]", c.src, got, eqs, c.want, c.eqs)
		}
	}
}

// TestRecursion checks that a self-recursive function over lists produces a
// list type — the cycle in the graph — rather than an ever-deeper pile of
// pairs.
func TestRecursion(t *testing.T) {
	cases := []struct{ src, want, eqs string }{
		{`let length = { [] -> 0, [h, t] -> add 1 (length t) } in length`, `T1 a -> Num`, list},
		{`let sum = { [] -> 0, [h, t] -> add h (sum t) } in sum`, `T1 Num -> Num`, list},
		{`let map = f -> { [] -> [], [h, t] -> [f h, map f t] } in map`, `(a -> b) -> T1 a -> T1 b`, list},
		{`let repeat = x -> [x, repeat x] in repeat`, `a -> T1 a`, infinite},
	}
	for _, c := range cases {
		got, eqs := analyze(t, c.src)
		if got != c.want || eqs != c.eqs {
			t.Errorf("%s\n  got  %-24s [%s]\n  want %-24s [%s]", c.src, got, eqs, c.want, c.eqs)
		}
	}
}

// TestPolymorphism checks that a let-bound name is generalised — used at two
// types in the same program — while a lambda parameter is not, and that a
// helper closing over a parameter stays tied to it.
func TestPolymorphism(t *testing.T) {
	cases := []struct{ src, want, eqs string }{
		{`let id = x -> x in [id 1, id "s"]`, `[Num, T1 Num]`, list},
		{`let fix = f -> let x = f x in x in fix`, `(a -> a) -> a`, ``},
		{`let twice = f -> x -> f (f x) in twice`, `(a -> a) -> a -> a`, ``},
		{`let apply = f -> x -> f x in apply`, `(a -> b) -> a -> b`, ``},
	}
	for _, c := range cases {
		got, eqs := analyze(t, c.src)
		if got != c.want || eqs != c.eqs {
			t.Errorf("%s\n  got  %-24s [%s]\n  want %-24s [%s]", c.src, got, eqs, c.want, c.eqs)
		}
	}
}

// TestUnions checks the shapes a multi-case lambda accepts and returns.
func TestUnions(t *testing.T) {
	cases := []struct{ src, want, eqs string }{
		{`{ [] -> 0, [a] -> 1 }`, `([] | [a]) -> Num`, ``},
		{`{ 0 -> "zero", n -> "other" }`, `Num -> T1 Num`, list},
		{`{ [] -> 0, [h, t] -> h }`, `([] | [Num, a]) -> Num`, ``},
	}
	for _, c := range cases {
		got, eqs := analyze(t, c.src)
		if got != c.want || eqs != c.eqs {
			t.Errorf("%s\n  got  %-24s [%s]\n  want %-24s [%s]", c.src, got, eqs, c.want, c.eqs)
		}
	}
}

// TestOperators checks the four operators, which are the only places where an
// application is written without juxtaposition.
func TestOperators(t *testing.T) {
	cases := []struct{ src, want, eqs string }{
		{`1 > add 2`, `Num`, ``},
		{`add 2 < 1`, `Num`, ``},
		{`x -> x > add 1 > mul 2`, `Num -> Num`, ``},
		{`add 1 *> mul 2`, `Num -> Num`, ``},
		{`add 1 <* mul 2`, `Num -> Num`, ``},
		{`string *> show`, `a -> T1 Num`, list},
	}
	for _, c := range cases {
		got, eqs := analyze(t, c.src)
		if got != c.want || eqs != c.eqs {
			t.Errorf("%s\n  got  %-24s [%s]\n  want %-24s [%s]", c.src, got, eqs, c.want, c.eqs)
		}
	}
}

// TestConflicts checks that the two things the analysis can actually call
// wrong are reported, and that ordinary code is not.
func TestConflicts(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{`let x = 3 4 in x`, 1},
		{`add "hi" 2`, 1},
		{`let f = x -> add x 1 in f 2`, 0},
		{`{ [] -> 0, [h, t] -> h }`, 0},
		{`let id = x -> x in [id 1, id "s"]`, 0},
	}
	for _, c := range cases {
		tokens := syntax.LexContent("test.th", c.src)
		program := syntax.ParseProgram(tokens)
		modules := map[string]*syntax.Module{}
		resolution := syntax.Resolve(program, modules)
		analysis := types.Infer(program, modules, resolution)
		if len(analysis.Warnings) != c.want {
			t.Errorf("%s\n  got %d warnings, want %d: %v", c.src, len(analysis.Warnings), c.want, analysis.Warnings)
		}
	}
}
