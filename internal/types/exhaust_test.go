package types_test

import (
	"strings"
	"testing"
)

const listDecl = "--> List a = [] | [a, List a]\n"

// TestExhaustiveDemanded checks the point of the whole feature: a signature is a
// claim about the domain, so the patterns have to cover it. Inference cannot
// find these — it reports `last : List a -> a` by itself, because the recursive
// call ties the tail to the argument and the nested `[]` pattern pulls the empty
// alternative in.
func TestExhaustiveDemanded(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"head", listDecl + "--> head : List a -> a\nlet head = [h, t] -> h in head",
			"does not cover the empty tuple",
		},
		{
			"last", listDecl + "--> last : List a -> a\n" +
				"let last = { [x, []] -> x, [h, t] -> last t } in last",
			"does not cover the empty tuple",
		},
		{
			"maybe", "--> Maybe a = [] | [a]\n--> value : Maybe a -> a\nlet value = [x] -> x in value",
			"does not cover the empty tuple",
		},
		{
			// The second argument is the one at fault, and the message says so.
			"second argument", listDecl + "--> f : Num -> List a -> a\n" +
				"let f = n -> [h, t] -> h in f",
			"argument 2 does not cover",
		},
	}
	for _, c := range cases {
		_, _, warns := run(t, c.src)
		if len(warns) != 1 || !strings.Contains(warns[0].Message, c.want) {
			t.Errorf("%s\n  got  %s\n  want one warning containing %q", c.name, messages(warns), c.want)
		}
	}
}

// TestExhaustiveSatisfied checks that a total function is silent, so the check
// is not simply complaining about everything.
func TestExhaustiveSatisfied(t *testing.T) {
	srcs := []string{
		// Both alternatives handled.
		listDecl + "--> n : List a -> Num\n" +
			"let n = { [] -> 0, [h, t] -> add 1 (n t) } in n",
		// A variable pattern covers everything, catch-all style.
		listDecl + "--> f : List a -> Num\nlet f = { [] -> 0, l -> 1 } in f",
		// A bare variable parameter claims nothing to cover.
		listDecl + "--> f : List a -> Num\nlet f = l -> 0 in f",
		// A position the signature leaves as a type variable is not checked.
		"--> f : a -> Num\nlet f = [h, t] -> 1 in f",
	}
	for _, src := range srcs {
		if _, _, warns := run(t, src); len(warns) != 0 {
			t.Errorf("expected silence for\n%s\n  got %s", src, messages(warns))
		}
	}
}

// TestAssertedSilences checks the escape hatch, and that it is position-level:
// marking the argument is what silences an argument complaint.
func TestAssertedSilences(t *testing.T) {
	src := listDecl + "--> head : List a! -> a\nlet head = [h, t] -> h in head"
	if _, _, warns := run(t, src); len(warns) != 0 {
		t.Errorf("`!` should silence the demand, got %s", messages(warns))
	}

	// Marking the *result* does not excuse the argument. The result has to be a
	// concrete type for the mark to be legal at all, hence Num rather than `a`.
	src = listDecl + "--> f : List Num -> Num!\nlet f = [h, t] -> h in f"
	_, _, warns := run(t, src)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "argument 1 does not cover") {
		t.Errorf("a result mark should not excuse the argument, got %s", messages(warns))
	}
}

// TestAssertedPrecedence pins the answer to "is it List a! or (List a)!": the
// mark binds to the whole type at that position, and marking a bare type
// variable is an error because a variable claims nothing to over-claim.
func TestAssertedPrecedence(t *testing.T) {
	body := "let head = [h, t] -> h in head"

	// Both spellings mean the same thing, and both silence the demand.
	for _, sig := range []string{"List a! -> a", "(List a)! -> a"} {
		if _, _, warns := run(t, listDecl+"--> head : "+sig+"\n"+body); len(warns) != 0 {
			t.Errorf("%s should be accepted and silence the demand, got %s", sig, messages(warns))
		}
	}

	// `!` on a variable is rejected, whether written inside an application or alone.
	for _, sig := range []string{"List (a!) -> a", "a! -> a"} {
		_, _, warns := run(t, listDecl+"--> head : "+sig+"\n"+body)
		if len(warns) == 0 || !strings.Contains(messages(warns), "claims nothing") {
			t.Errorf("%s should be rejected, got %s", sig, messages(warns))
		}
	}
}

// TestAssertedCounted checks that the marks are totalled, so a project can see
// how many unverified assumptions it is resting on rather than losing them in
// the noise.
func TestAssertedCounted(t *testing.T) {
	modSrc := "module\n\n" +
		"--> List a = [] | [a, List a]\n\n" +
		"--> head : List a! -> a\n" +
		"head = [h, t] -> h,\n\n" +
		"--> safe : List a -> Num\n" +
		"safe = { [] -> 0, l -> 1 }\n"
	a := analyzeWithModule(t, "m", modSrc, "import m in 0")
	marks, sigs := a.Assertions()
	if marks != 1 || sigs != 1 {
		t.Errorf("expected 1 mark in 1 signature, got %d in %d", marks, sigs)
	}
}

// TestPartialByDelegationDemanded checks the division of labour between the two
// checks. Exhaustiveness sees only a binding's own patterns, so it has nothing to
// say about `nth`, which never matches on the list — it calls head. The demand
// comes from the assertion pass instead (see assert.go), which is why the mark is
// required here rather than merely tolerated.
func TestPartialByDelegationDemanded(t *testing.T) {
	// Each signature has to sit immediately above its own binding, so the bindings
	// go on separate lines: attachment is by adjacency.
	src := listDecl +
		"let\n" +
		"--> head : List a! -> a\n" +
		"  head = [h, t] -> h,\n" +
		"--> nth : Num -> List a -> a\n" +
		"  nth = n -> l -> head l\n" +
		"in nth"
	_, _, warns := run(t, src)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "nth inherits it") {
		t.Fatalf("expected the assertion pass to demand a mark on nth, got %s", messages(warns))
	}

	// And with the mark, silence — from both checks.
	src = strings.Replace(src, "--> nth : Num -> List a -> a", "--> nth : Num -> List a! -> a", 1)
	if _, _, warns := run(t, src); len(warns) != 0 {
		t.Errorf("a marked nth should be silent, got %s", messages(warns))
	}
}
