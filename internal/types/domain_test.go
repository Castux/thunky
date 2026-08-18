package types_test

import (
	"strings"
	"testing"
)

// The declarations every case below is written against. Kept in one place so a
// test reads as the signature it is about.
const prelude = "--> List a = [] | [a, List a]\n" +
	"--> Maybe a = [] | [a]\n"

func warnings(t *testing.T, src string) string {
	t.Helper()
	_, _, warns := run(t, prelude+src)
	msgs := make([]string, len(warns))
	for i, w := range warns {
		msgs[i] = w.Message
	}
	return strings.Join(msgs, " | ")
}

func expectWarning(t *testing.T, src, want string) {
	t.Helper()
	if got := warnings(t, src); !strings.Contains(got, want) {
		t.Errorf("%s\n  got  %s\n  want a warning containing %q", src, got, want)
	}
}

func expectSilence(t *testing.T, src string) {
	t.Helper()
	if got := warnings(t, src); got != "" {
		t.Errorf("%s\n  got  %s\n  want no warning", src, got)
	}
}

// TestOverlappingShapesAreStillWrong is the case disjointness could never see.
// `Maybe a` and `List a` share the empty tuple, so they are not disjoint; but a
// 1-tuple is not something a list function can take, and the call fails.
func TestOverlappingShapesAreStillWrong(t *testing.T) {
	expectWarning(t, "let\n"+
		"  --> some : a -> Maybe a\n"+
		"  some = x -> [x],\n"+
		"  --> rev : List a -> List a\n"+
		"  rev = { [] -> [], [h, t] -> rev t }\n"+
		"in rev (some 5)",
		"passed Maybe Num to rev, which takes List a")
}

// TestDepthIsWhereTheMistakeIs is the tuple-for-list slip the language most
// invites, at the size where the arity matches and only the nesting is wrong:
// `[[1,2],[3,4]]` is a pair whose second field is a pair of numbers, so it is a
// list exactly one cell deep and then a number where the tail should be.
func TestDepthIsWhereTheMistakeIs(t *testing.T) {
	expectWarning(t, "let\n"+
		"  --> len : List a -> Num\n"+
		"  len = { [] -> 0, [h, t] -> add 1 (len t) }\n"+
		"in len [[1, 2], [3, 4]]",
		"field 2 of the pair of field 2 of the pair: a number is not admitted")

	// The same call written as a list is fine, and so is the empty list.
	expectSilence(t, "let\n"+
		"  --> len : List a -> Num\n"+
		"  len = { [] -> 0, [h, t] -> add 1 (len t) }\n"+
		"in add (len [[1, 2]; [3, 4]]) (len [])")
}

// TestVariance checks that a function-typed parameter is contravariant. A
// callback that accepts *more* than the declaration promises to pass it is fine;
// one that accepts less is not.
func TestVariance(t *testing.T) {
	const apply = "  --> useOnList : (List Num -> Num) -> Num\n" +
		"  useOnList = f -> f [1; 2],\n"

	// Accepts lists and numbers: wider than required, so nothing is assumed.
	expectSilence(t, "let\n"+apply+
		"  wide = { [] -> 0, [h, t] -> 1, n -> 0 }\n"+
		"in useOnList wide")

	// Accepts only numbers: narrower than what will arrive.
	expectWarning(t, "let\n"+apply+
		"  narrow = n -> add n 1\n"+
		"in useOnList narrow",
		"the argument: the empty tuple or a pair is not admitted")
}

// TestSignatureVariableClaimsNothing is what keeps the check honest about which
// side it is checking. `core.if : Num -> a -> a -> a` gets `a` from its first
// branch; checking the second branch against that would be checking it against
// the first branch, not against anything the author wrote.
func TestSignatureVariableClaimsNothing(t *testing.T) {
	expectSilence(t, "let\n"+
		"  --> pick : Num -> a -> a -> a\n"+
		"  pick = c -> x -> y -> x,\n"+
		"  --> some : a -> Maybe a\n"+
		"  some = x -> [x]\n"+
		"in pick 1 (some 5) []")
}

// TestInstantiationCarriesTheElement checks the half of a call that is not a
// check at all: `a` is one node across the whole signature, so an argument that
// pins it hands the element type back out through the result.
func TestInstantiationCarriesTheElement(t *testing.T) {
	got, _, _ := run(t, prelude+"let\n"+
		"  --> rev : List a -> List a\n"+
		"  rev = { [] -> [], [h, t] -> rev t }\n"+
		"in rev [1; 2; 3]")
	if got != "List Num" {
		t.Errorf("the element type should reach the result, got %q", got)
	}
}

// TestUntrustedCalleeIsNotContained checks the boundary. A callee with no
// signature has a parameter type inference arrived at by joining whatever
// reached it — narrower than intended, and polluted by other call sites — so it
// is held to disjointness only.
func TestUntrustedCalleeIsNotContained(t *testing.T) {
	// isEmpty's patterns mention only the empty tuple, so its inferred domain is
	// `[]`. Passing a pair is not an error, and saying so would be noise.
	expectSilence(t, "let\n"+
		"  isEmpty = { [] -> 1, l -> 0 }\n"+
		"in isEmpty [1; 2]")

	// Outright disjointness is still reported there.
	expectWarning(t, "let\n"+
		"  double = n -> add n n\n"+
		"in double [1; 2]",
		"passed")
}

// TestAssertedResult checks `!` in the position exhaustiveness never looks at. A
// claim narrower than the analysis can confirm — a stream is a list that is
// never empty, and no amount of joining shows that — is the author's to make,
// and the mark is how it stays countable.
func TestAssertedResult(t *testing.T) {
	src := func(mark string) string {
		return "--> Stream a = [a, Stream a]\n" +
			"let\n" +
			"  --> cyc : List a -> Stream a" + mark + "\n" +
			"  cyc = l -> l\n" +
			"in cyc [1; 2]"
	}

	// `cyc` really does return whatever it was given, so the analysis is right
	// that it cannot rule out the empty list.
	expectWarning(t, src(""), "does not hold")
	// With the mark, the claim is the author's to make.
	expectSilence(t, src("!"))
}

// TestAssertedCountedOnAResult checks that the mark is totalled wherever it is,
// so the assumptions a project rests on stay countable rather than accumulating
// in the one position nothing else looks at.
func TestAssertedCountedOnAResult(t *testing.T) {
	modSrc := "module\n\n" +
		"--> Stream a = [a, Stream a]\n\n" +
		"--> repeat : a -> Stream a!\n" +
		"repeat = x -> [x, repeat x]\n"
	a := analyzeWithModule(t, "m", modSrc, "import m in 0")
	e, ok := entryFor(a, "repeat")
	if !ok {
		t.Fatal("repeat not reported")
	}
	if e.Asserted != 1 {
		t.Errorf("the mark should be counted, got %d", e.Asserted)
	}
}

// TestDeclarationDrivesInference is the property that separates this from a
// report. `some 5` on its own is a 1-tuple and nothing more; through a signature
// it is a Maybe, which is what lets the call below be caught with no annotation
// on the binding that holds it.
func TestDeclarationDrivesInference(t *testing.T) {
	got, _, _ := run(t, prelude+"let\n"+
		"  --> some : a -> Maybe a\n"+
		"  some = x -> [x],\n"+
		"  held = some 5\n"+
		"in held")
	if got != "Maybe Num" {
		t.Errorf("a use of an annotated binding should be its signature, got %q", got)
	}

	expectWarning(t, "let\n"+
		"  --> some : a -> Maybe a\n"+
		"  some = x -> [x],\n"+
		"  --> rev : List a -> List a\n"+
		"  rev = { [] -> [], [h, t] -> rev t },\n"+
		"  held = some 5\n"+
		"in rev held",
		"passed Maybe Num to rev")
}

// TestRecursionNeedsNoKnot checks what a signature buys the walk. A recursive
// call is served from the declared type, so the definition does not have to be
// entered at any particular point and the type does not have to be discovered by
// tying its own argument to its own result.
func TestRecursionNeedsNoKnot(t *testing.T) {
	got, _, warns := run(t, prelude+"let\n"+
		"  --> len : List a -> Num\n"+
		"  len = { [] -> 0, [h, t] -> add 1 (len t) }\n"+
		"in len")
	if got != "List a -> Num" {
		t.Errorf("got %q", got)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

// TestConsumedVariableMustAgree is the case a variable position gets wrong if it
// is treated as claiming nothing. `map` hands the caller's own callback the
// elements of the caller's own list, so the two arguments have to agree; `pick`
// (a stand-in for `core.if`) merely hands one of them back, and there the union
// is the answer rather than an error.
func TestConsumedVariableMustAgree(t *testing.T) {
	const m = "  --> map : (a -> b) -> List a -> List b\n" +
		"  map = f -> { [] -> [], [h, t] -> [f h, map f t] },\n"

	expectWarning(t, "let\n"+m+"  inc = n -> add n 1\nin map inc [[1; 2]; [3; 4]]",
		"which takes List Num")
	expectSilence(t, "let\n"+m+"  inc = n -> add n 1\nin map inc [1; 2; 3]")
}

// TestConsumedVariableIsStillJoined is the limit of the rule above. An
// accumulator is consumed *and* produced by the same callback, so its value is
// assembled from several arguments and the node is never complete while the
// arguments are being checked one at a time. Holding it to containment would
// report `partition`, whose seed is `[[], []]` and whose step always returns a
// pair of cons cells.
func TestConsumedVariableIsStillJoined(t *testing.T) {
	expectSilence(t, "let\n"+
		"  --> foldr : (a -> b -> b) -> b -> List a -> b\n"+
		"  foldr = f -> z -> { [] -> z, [h, t] -> f h (foldr f z t) },\n"+
		"  --> partition : List a -> [List a, List a]\n"+
		"  partition = foldr (x -> [ts, fs] -> [[x, ts], fs]) [[], []]\n"+
		"in partition [1; 2]")

	// And a callback that matches a narrower shape than the element type admits
	// is how the library is written, not a mistake.
	expectSilence(t, "let\n"+
		"  --> map : (a -> b) -> List a -> List b\n"+
		"  map = f -> { [] -> [], [h, t] -> [f h, map f t] },\n"+
		"  rows = [[1; 2]; [3; 4]]\n"+
		"in map ([x, y] -> add x 1) rows")
}
