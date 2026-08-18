package types_test

import (
	"strings"
	"testing"
)

// The declarations every case here needs, plus a partial `head` to inherit from.
const assertPrelude = "--> List a = [] | [a, List a]\n" +
	"--> Maybe a = [] | [a]\n"

const headDecl = "--> head : List a! -> a\n  head = [h, t] -> h,\n"

// TestAssertionInherited is the point of the feature: a caller that passes an
// argument the callee's assumption might not hold for is making the same
// assumption, and has to say so. Without this a library could launder every
// assumption by wrapping it in one more function.
func TestAssertionInherited(t *testing.T) {
	src := assertPrelude + "let\n" + headDecl +
		"--> nth : Num -> List a -> a\n" +
		"  nth = n -> l -> head l\n" +
		"in nth"
	_, _, warns := run(t, src)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "nth inherits it") {
		t.Fatalf("expected nth to inherit head's assumption, got %s", messages(warns))
	}
	if !strings.Contains(warns[0].Message, "argument 1 of head") {
		t.Errorf("the message should name the asserted position, got %q", warns[0].Message)
	}
}

// TestAssertionAcknowledged checks the other half: a caller that carries a mark
// has said so, and is not asked twice.
func TestAssertionAcknowledged(t *testing.T) {
	src := assertPrelude + "let\n" + headDecl +
		"--> nth : Num -> List a! -> a\n" +
		"  nth = n -> l -> head l\n" +
		"in nth"
	if _, _, warns := run(t, src); len(warns) != 0 {
		t.Errorf("a marked caller should be silent, got %s", messages(warns))
	}
}

// TestAssertionDischargedByShape checks that a call whose argument is written as
// a shape the callee matches needs no mark. This is what lets the safe wrappers
// in list delegate to the partial ones: `last [h, t]` passes a pair, and last
// matches pairs.
func TestAssertionDischargedByShape(t *testing.T) {
	srcs := []string{
		// A tuple written at the call site.
		assertPrelude + "let\n" + headDecl +
			"--> firstOf : List a -> Maybe a\n" +
			"  firstOf = { [] -> [], [h, t] -> [head [h, t]] }\n" +
			"in firstOf",
		// A non-empty list literal.
		assertPrelude + "let\n" + headDecl +
			"--> one : Num\n  one = head [1;]\n" +
			"in one",
		// A non-empty string literal, which is a list of code points.
		assertPrelude + "let\n" + headDecl +
			"--> zero : Num\n  zero = head \"0\"\n" +
			"in zero",
	}
	for _, src := range srcs {
		if _, _, warns := run(t, src); len(warns) != 0 {
			t.Errorf("expected silence for\n%s\n  got %s", src, messages(warns))
		}
	}
}

// TestAssertionNotDischargedByEmpty checks the rule is not a rubber stamp: an
// argument written as the *empty* list is exactly the case head cannot handle.
func TestAssertionNotDischargedByEmpty(t *testing.T) {
	src := assertPrelude + "let\n" + headDecl +
		"--> bad : Num\n  bad = head []\n" +
		"in bad"
	_, _, warns := run(t, src)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "bad inherits it") {
		t.Errorf("passing [] should not discharge, got %s", messages(warns))
	}
}

// TestAssertionAliasNotLaundered records the hole this would otherwise leave.
// `char = head` has no call site at all, so nothing would be checked, and every
// caller would see char's signature while none saw head's.
func TestAssertionAliasNotLaundered(t *testing.T) {
	mod := "module\n\n" + assertPrelude + "\n" +
		"--> head : List a! -> a\n" +
		"head = [h, t] -> h,\n\n" +
		"--> char : List Num -> Num\n" +
		"char = head\n"
	a := analyzeWithModule(t, "m", mod, "import m in 0")
	var found bool
	for _, w := range a.Warnings {
		if strings.Contains(w.Message, "char inherits it") {
			found = true
		}
	}
	if !found {
		t.Errorf("an alias should inherit the assumption, got %s", messages(a.Warnings))
	}
}

// TestAssertionThroughComposition checks that `*>` inherits. There is no
// expression for the argument — it is whatever the previous function returned —
// so nothing can discharge it, which is the honest answer rather than a gap.
func TestAssertionThroughComposition(t *testing.T) {
	src := assertPrelude + "let\n" + headDecl +
		"--> drop1 : List a -> List a\n" +
		"  drop1 = { [] -> [], [h, t] -> t },\n" +
		"--> second : List a -> a\n" +
		"  second = drop1 *> head\n" +
		"in second"
	_, _, warns := run(t, src)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "second inherits it") {
		t.Errorf("composition should inherit, got %s", messages(warns))
	}
}

// TestAssertionPositionCounted checks that the demand is keyed to the position
// that carries the mark, not to any argument. Marking the second argument must
// not fire on a call that only fills the first.
func TestAssertionPositionCounted(t *testing.T) {
	// `at` asserts its *second* argument; `partial` supplies only the first.
	src := assertPrelude + "let\n" +
		"--> at : Num -> List a! -> a\n" +
		"  at = n -> [h, t] -> h,\n" +
		"--> partial : Num -> List a -> a\n" +
		"  partial = n -> at n\n" +
		"in partial"
	if _, _, warns := run(t, src); len(warns) != 0 {
		t.Errorf("supplying only the unasserted argument should be silent, got %s", messages(warns))
	}
}

// TestAssertionDischargedBySignature checks that a name whose binding carries a
// signature discharges an assumption its claimed type rules out. A signature is
// the author's claim rather than a join, so it still means something at a call
// site where the inferred type would not.
func TestAssertionDischargedBySignature(t *testing.T) {
	mod := "module\n\n" + assertPrelude +
		"--> Stream a = [a, Stream a]\n\n" +
		"--> head : List a! -> a\n" +
		"head = [h, t] -> h,\n\n" +
		"--> ones : Stream Num\n" +
		"ones = [1, ones],\n\n" +
		"--> first : Num\n" +
		"first = head ones\n"
	a := analyzeWithModule(t, "m", mod, "import m in 0")
	for _, w := range a.Warnings {
		if strings.Contains(w.Message, "asserted with") {
			t.Errorf("a stream should discharge head's assumption, got %s", w.Message)
		}
	}

	// And a claim that does *not* rule the empty case out still inherits.
	mod = strings.Replace(mod, "--> ones : Stream Num", "--> ones : List Num", 1)
	a = analyzeWithModule(t, "m", mod, "import m in 0")
	var found bool
	for _, w := range a.Warnings {
		if strings.Contains(w.Message, "first inherits it") {
			found = true
		}
	}
	if !found {
		t.Errorf("a List claim should not discharge, got %s", messages(a.Warnings))
	}
}
