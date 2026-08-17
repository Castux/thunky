package types_test

import (
	"strings"
	"testing"

	"github.com/Castux/thunky/internal/syntax"
	"github.com/Castux/thunky/internal/types"
)

// run analyses a source and returns the rendered program type, the equations,
// and the warnings.
func run(t *testing.T, src string) (string, string, []types.Warning) {
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
	res := syntax.Resolve(program, modules)
	a := types.Infer(program, modules, res)
	eqs := make([]string, len(a.Equations))
	for i, e := range a.Equations {
		eqs[i] = e.Header() + " = " + e.Body
	}
	return a.Program, strings.Join(eqs, "; "), a.Warnings
}

// TestDeclNamesShapes checks that a declaration replaces the generated name for
// the shape it matches, and only for that shape.
func TestDeclNamesShapes(t *testing.T) {
	cases := []struct{ name, src, want, eqs string }{
		{
			"list", "--> List a = [] | [a, List a]\n" +
				"let length = { [] -> 0, [h, t] -> add 1 (length t) } in length",
			"List a -> Num", "List a = [] | [a, List a]",
		},
		{
			// Not recursive, so this is the case a recursion-only scheme would miss.
			"maybe", "--> Maybe a = [] | [a]\n" +
				"let find = { [] -> [], [h, t] -> [h] } in find",
			"([] | [a, b]) -> Maybe a", "Maybe a = [] | [a]",
		},
		{
			// An unterminated list is a different shape and must not be named List.
			"stream not list", "--> List a = [] | [a, List a]\n" +
				"let repeat = x -> [x, repeat x] in repeat",
			"a -> T1 a", "T1 a = [a, T1 a]",
		},
		{
			"both", "--> List a = [] | [a, List a]\n--> Stream a = [a, Stream a]\n" +
				"let repeat = x -> [x, repeat x] in repeat",
			"a -> Stream a", "Stream a = [a, Stream a]",
		},
		{
			// A declaration nothing matches is not listed.
			"unused", "--> Weird a = [a, a, a]\nadd 1 2",
			"Num", "",
		},
	}
	for _, c := range cases {
		got, eqs, warns := run(t, c.src)
		if got != c.want || eqs != c.eqs {
			t.Errorf("%s\n  got  %-28s [%s]\n  want %-28s [%s]", c.name, got, eqs, c.want, c.eqs)
		}
		if len(warns) != 0 {
			t.Errorf("%s: unexpected warnings: %v", c.name, warns)
		}
	}
}

// TestDeclDoesNotChangeInference is the property that makes annotations safe: a
// declaration names a shape and nothing else, so adding one can never change
// what the analysis concluded, and in particular can never narrow what a
// function is found to accept. A heterogeneous list still analyses cleanly with
// a `List a` declaration in scope.
func TestDeclDoesNotChangeInference(t *testing.T) {
	body := `let f = { [] -> [], [h, t] -> [h, f t] } in f [1; "a"; 2]`
	bare, _, wBare := run(t, body)
	decl, _, wDecl := run(t, "--> List a = [] | [a, List a]\n"+body)

	if len(wBare) != 0 || len(wDecl) != 0 {
		t.Fatalf("warnings changed: bare %v, declared %v", wBare, wDecl)
	}
	// The rendering differs only by the name substituted for the shape.
	if bare == decl {
		t.Fatalf("declaration had no effect at all: %s", bare)
	}
	if strings.ReplaceAll(decl, "List", "T1") != bare {
		t.Errorf("declaration changed more than the name:\n  bare     %s\n  declared %s", bare, decl)
	}
	// And the element type is still the union it always was: heterogeneity
	// survives naming, because the element is a union rather than one shape.
	if !strings.Contains(bare, "T2 Num") {
		t.Errorf("expected a union element type, got %s", bare)
	}
}

// TestAnnotErrors checks that a malformed annotation is reported rather than
// silently doing nothing, and that a valid signature is accepted quietly.
func TestAnnotErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"--> list a = [a]\nadd 1 2", "must start with a capital"},
		{"--> T a = [] | [a, Nope a]\nadd 1 2", `unknown type "Nope"`},
		{"--> T a a = [a]\nadd 1 2", `parameter "a" appears twice`},
		{"--> T a = [a, b]\nadd 1 2", `unbound type variable "b"`},
		{"--> T = [Num,\nadd 1 2", "unexpected end of type"},
		{"--> Sig : Num\nadd 1 2", "is not a binding name"},
		{"--> nope :\nadd 1 2", "needs a type after"},
		{"--> garbage\nadd 1 2", "expected `Name params = Type`"},
	}
	for _, c := range cases {
		_, _, warns := run(t, c.src)
		if len(warns) != 1 || !strings.Contains(warns[0].Message, c.want) {
			t.Errorf("%q\n  got  %v\n  want one warning containing %q", c.src, warns, c.want)
		}
	}

	// A signature naming an undeclared type is reported, since it cannot be
	// checked against anything.
	_, _, warns := run(t, "--> length : List a -> Num\nlet length = x -> 1 in length")
	if len(warns) != 1 || !strings.Contains(warns[0].Message, `unknown type "List"`) {
		t.Errorf("expected an unknown-type complaint, got %v", warns)
	}

	// With the type declared, a true signature is silent.
	if _, _, warns := run(t,
		"--> List a = [] | [a, List a]\n--> length : List a -> Num\n"+
			"let length = { [] -> 0, [h, t] -> add 1 (length t) } in length"); len(warns) != 0 {
		t.Errorf("a valid signature should be silent, got %v", warns)
	}
}

// TestAnnotNotInStrings checks the reason comment spans come from the lexer
// rather than a re-scan of the text: a `-->` inside a string literal is not an
// annotation, and a naive scan would have taken it for one.
func TestAnnotNotInStrings(t *testing.T) {
	_, eqs, warns := run(t, `show "--> List a = [] | [a, List a]"`)
	if len(warns) != 0 {
		t.Errorf("a string containing an annotation produced warnings: %v", warns)
	}
	if strings.Contains(eqs, "List") {
		t.Errorf("an annotation inside a string literal was read: %s", eqs)
	}
}

// TestAnnotIndifferentToPlacement checks that a declaration applies wherever it
// sits in the file, since it names a shape rather than attaching to a binding.
func TestAnnotIndifferentToPlacement(t *testing.T) {
	before, eqsBefore, _ := run(t, "--> List a = [] | [a, List a]\nlet n = { [] -> 0, [h, t] -> add 1 (n t) } in n")
	after, eqsAfter, _ := run(t, "let n = { [] -> 0, [h, t] -> add 1 (n t) } in n\n--> List a = [] | [a, List a]")
	if before != after || eqsBefore != eqsAfter {
		t.Errorf("placement mattered:\n  before %s [%s]\n  after  %s [%s]", before, eqsBefore, after, eqsAfter)
	}
}

// TestAnnotNotes checks the two things that can go quietly wrong with names
// rather than with types: two names for one shape, and one name twice.
func TestAnnotNotes(t *testing.T) {
	// Text and List are the same shape — structurally they are one type. Naming
	// both is legal and the report says which name it used.
	_, _, warns := run(t, "--> List a = [] | [a, List a]\n--> Text = [] | [Num, Text]\n"+
		"let n = { [] -> 0, [h, t] -> add 1 (n t) } in n \"hi\"")
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "name the same shape") {
		t.Errorf("expected one alias note, got %v", warns)
	}

	// One name, two bodies: one of them is not taking effect, which is worth
	// saying out loud.
	_, _, warns = run(t, "--> List a = [] | [a, List a]\n--> List a = [a, List a]\nadd 1 2")
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "declared twice") {
		t.Errorf("expected one redeclaration note, got %v", warns)
	}

	// The same name declared identically twice is not worth mentioning.
	if _, _, warns = run(t, "--> List a = [] | [a, List a]\n--> List a = [] | [a, List a]\nadd 1 2"); len(warns) != 0 {
		t.Errorf("an identical redeclaration should be silent, got %v", warns)
	}
}

// runWithModule analyses a program plus one module, so signatures can be tested
// where they actually belong — inside a module, resolving names across an import.
func runWithModule(t *testing.T, modName, modSrc, progSrc string) []types.Warning {
	t.Helper()
	modTokens := syntax.LexContent(modName+".th", modSrc)
	if modTokens == nil {
		t.Fatalf("lexing module failed:\n%s", modSrc)
	}
	mod := syntax.ParseModule(modTokens)
	if mod == nil {
		t.Fatalf("parsing module failed:\n%s", modSrc)
	}
	mod.Name = modName

	tokens := syntax.LexContent("test.th", progSrc)
	program := syntax.ParseProgram(tokens)
	if program == nil {
		t.Fatalf("parsing program failed:\n%s", progSrc)
	}
	modules := map[string]*syntax.Module{modName: mod}
	res := syntax.Resolve(program, modules)
	return types.Infer(program, modules, res).Warnings
}

func messages(ws []types.Warning) string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Message
	}
	return strings.Join(out, " | ")
}

// TestSignatureHolds checks that a true claim is silent, including one that is
// deliberately narrower than what inference found.
func TestSignatureHolds(t *testing.T) {
	srcs := []string{
		"--> List a = [] | [a, List a]\n" +
			"let\n  --> n : List a -> Num\n  n = { [] -> 0, [h, t] -> add 1 (n t) }\nin n",
		// Narrower than the finding, and true: specialisation is not an error.
		"--> List a = [] | [a, List a]\n" +
			"let\n  --> n : List Num -> Num\n  n = { [] -> 0, [h, t] -> add 1 (n t) }\nin n",
		// A variable claims nothing concrete, so it never contradicts.
		"let\n  --> f : a -> b\n  f = x -> add x 1\nin f",
	}
	for _, src := range srcs {
		if _, _, warns := run(t, src); len(warns) != 0 {
			t.Errorf("expected no warnings for\n%s\n  got %s", src, messages(warns))
		}
	}
}

// TestSignatureContradiction checks the claims that are actually wrong.
func TestSignatureContradiction(t *testing.T) {
	cases := []struct{ src, want string }{
		{
			"--> List a = [] | [a, List a]\n" +
				"let\n  --> n : Num -> Num\n  n = { [] -> 0, [h, t] -> add 1 (n t) }\nin n",
			"claimed a number",
		},
		{
			// One arrow too many.
			"--> List a = [] | [a, List a]\n" +
				"let\n  --> n : List a -> Num -> Num\n  n = { [] -> 0, [h, t] -> add 1 (n t) }\nin n",
			"claimed a function",
		},
		{
			// The tuple arity is wrong.
			"let\n  --> f : [Num, Num, Num] -> Num\n  f = { [a, b] -> add a b }\nin f",
			"claimed a 3-tuple",
		},
	}
	for _, c := range cases {
		_, _, warns := run(t, c.src)
		if len(warns) != 1 || !strings.Contains(warns[0].Message, c.want) {
			t.Errorf("%s\n  got  %s\n  want one warning containing %q", c.src, messages(warns), c.want)
		}
	}
}

// TestSignatureAttachment checks the adjacency rule and what happens when it
// cannot be satisfied.
func TestSignatureAttachment(t *testing.T) {
	cases := []struct{ src, want string }{
		{"let\n  --> wrong : Num\n  other = 1\nin other", `signature names "wrong"`},
		{"let x = 1 in x\n--> late : Num", "has no binding after it"},
		{"let\n  --> f : Num\n  --> f : Num -> Num\n  f = x -> x\nin f", "already has a signature"},
	}
	for _, c := range cases {
		_, _, warns := run(t, c.src)
		if len(warns) == 0 || !strings.Contains(messages(warns), c.want) {
			t.Errorf("%s\n  got  %s\n  want a warning containing %q", c.src, messages(warns), c.want)
		}
	}
}

// TestSignatureScope checks that type names resolve with the module rules the
// language already uses for values: a module's own declarations, those of the
// modules it imports, and qualified module.Name.
func TestSignatureScope(t *testing.T) {
	modSrc := "module\n\n--> Box a = [a]\n\n--> wrap : a -> Box a\nwrap = x -> [x]\n"

	// A module's signature may use its own declaration.
	if ws := runWithModule(t, "boxes", modSrc, "import boxes in boxes.wrap 1"); len(ws) != 0 {
		t.Errorf("a module's own type should be in scope, got %s", messages(ws))
	}

	// The program may use the imported module's type, qualified or not.
	for _, prog := range []string{
		"import boxes in\nlet\n  --> u : Box Num\n  u = boxes.wrap 1\nin u",
		"import boxes in\nlet\n  --> u : boxes.Box Num\n  u = boxes.wrap 1\nin u",
	} {
		if ws := runWithModule(t, "boxes", modSrc, prog); len(ws) != 0 {
			t.Errorf("imported type should be in scope for\n%s\n  got %s", prog, messages(ws))
		}
	}

	// A module that is not imported is not in scope, qualified or not.
	bare := "let\n  --> u : Box Num\n  u = 1\nin u"
	ws := runWithModule(t, "boxes", modSrc, bare)
	if len(ws) != 1 || !strings.Contains(ws[0].Message, `unknown type "Box"`) {
		t.Errorf("expected Box to be out of scope, got %s", messages(ws))
	}

	qual := "let\n  --> u : boxes.Box Num\n  u = 1\nin u"
	ws = runWithModule(t, "boxes", modSrc, qual)
	if len(ws) != 1 || !strings.Contains(ws[0].Message, "not imported here") {
		t.Errorf("expected boxes to be unimported, got %s", messages(ws))
	}
}

// TestSignatureArity checks that a declared type must be given its parameters.
func TestSignatureArity(t *testing.T) {
	src := "--> Pair a b = [a, b]\nlet\n  --> f : Pair Num -> Num\n  f = { [a, b] -> a }\nin f"
	_, _, warns := run(t, src)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "takes 2 parameter(s), given 1") {
		t.Errorf("expected an arity complaint, got %s", messages(warns))
	}
}

// TestSignatureRejectsTop checks that `?` cannot be claimed: it says nothing, so
// checking it would always succeed and hide a mistake.
func TestSignatureRejectsTop(t *testing.T) {
	_, _, warns := run(t, "let\n  --> f : ? -> Num\n  f = x -> 1\nin f")
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "claims nothing") {
		t.Errorf("expected `?` to be rejected, got %s", messages(warns))
	}
}
