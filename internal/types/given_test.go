package types_test

import (
	"strings"
	"testing"

	"github.com/Castux/thunky/internal/types"
)

// entryFor finds one module binding's reported type.
func entryFor(a *types.Analysis, name string) (types.Entry, bool) {
	for _, m := range a.Modules {
		for _, e := range m.Entries {
			if e.Name == name {
				return e, true
			}
		}
	}
	return types.Entry{}, false
}

// TestGivenSignatureIsDisplayed checks that an annotated binding shows the
// author's signature rather than the inferred shape. That is what makes
// annotating a library worth doing: the report becomes the documented interface,
// verified, instead of a restatement of what inference happened to find.
func TestGivenSignatureIsDisplayed(t *testing.T) {
	// `count`'s inferred argument is `List [Num, a]` — b is unused, so the second
	// field stays a variable. The signature says `List [Num, Num]`, which is
	// narrower, true, and what a reader wants to see.
	modSrc := "module\n\n" +
		"--> List a = [] | [a, List a]\n\n" +
		"--> count : List [Num, Num] -> Num\n" +
		"count = { [] -> 0, [[a, b], t] -> add a (count t) }\n"
	a := analyzeWithModule(t, "m", modSrc, "import m in m.count [[1, 2];]")

	for _, w := range a.Warnings {
		if strings.Contains(w.Message, "annotation") || strings.Contains(w.Message, "does not hold") {
			t.Fatalf("unexpected warning: %s", w.Message)
		}
	}
	e, ok := entryFor(a, "count")
	if !ok {
		t.Fatal("count not reported")
	}
	if !e.Given {
		t.Errorf("count should be marked as given, got inferred %q", e.Type)
	}
	if e.Type != "List [Num, Num] -> Num" {
		t.Errorf("expected the signature verbatim, got %q", e.Type)
	}
	if e.Inferred != "" {
		t.Errorf("a holding signature should record no disagreement, got %q", e.Inferred)
	}
}

// TestGivenSignatureNamesReachThePreamble is the subtle consequence of
// displaying a signature: nothing ever *renders* the shapes it names, so without
// help their equations would be missing from the report that mentions them.
func TestGivenSignatureNamesReachThePreamble(t *testing.T) {
	modSrc := "module\n\n" +
		"--> List a = [] | [a, List a]\n\n" +
		"--> ident : List Num -> List Num\n" +
		"ident = x -> x\n"
	a := analyzeWithModule(t, "m", modSrc, "import m in 0")

	var found bool
	for _, eq := range a.Equations {
		if eq.Name == "List" {
			found = true
		}
	}
	if !found {
		t.Errorf("List is named by a displayed signature but missing from the preamble; got %v", a.Equations)
	}
}

// TestGivenSignatureContradictedShowsBoth checks that a wrong claim is still
// displayed as written, with the finding recorded beside it. Hiding the claim
// would make the report disagree with the source; hiding the finding would make
// the disagreement invisible.
func TestGivenSignatureContradictedShowsBoth(t *testing.T) {
	modSrc := "module\n\n" +
		"--> List a = [] | [a, List a]\n\n" +
		"--> n : Num -> Num\n" +
		"n = { [] -> 0, [h, t] -> add 1 (n t) }\n"
	a := analyzeWithModule(t, "m", modSrc, "import m in 0")

	e, ok := entryFor(a, "n")
	if !ok {
		t.Fatal("n not reported")
	}
	if e.Type != "Num -> Num" {
		t.Errorf("the claim should be shown as written, got %q", e.Type)
	}
	if !strings.Contains(e.Inferred, "List") {
		t.Errorf("the finding should be recorded beside it, got %q", e.Inferred)
	}
	var reported bool
	for _, w := range a.Warnings {
		if strings.Contains(w.Message, "does not hold") {
			reported = true
		}
	}
	if !reported {
		t.Error("a contradicted signature should also be warned about")
	}
}

// TestCoverageCounts checks the provenance summary, which is how a reader knows
// how much of a report is authored rather than inferred.
func TestCoverageCounts(t *testing.T) {
	modSrc := "module\n\n" +
		"--> List a = [] | [a, List a]\n\n" +
		"--> one : Num -> Num\n" +
		"one = x -> x,\n" +
		"two = x -> x\n"
	a := analyzeWithModule(t, "m", modSrc, "import m in 0")
	given, total := a.Coverage()
	if given != 1 || total != 2 {
		t.Errorf("expected 1 of 2 given, got %d of %d", given, total)
	}
}
