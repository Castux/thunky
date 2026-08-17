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

// TestDeclCrossReference checks that a declaration body may name another
// declared type, resolved in dependency order.
func TestDeclCrossReference(t *testing.T) {
	// Grid is defined through Row, which is defined through List: all three have to
	// resolve, deepest first, for this to parse at all.
	_, _, warns := run(t, "--> List a = [] | [a, List a]\n"+
		"--> Row = [Num, List Num]\n"+
		"--> Grid = [Row, Num]\n"+
		"add 1 2")
	if len(warns) != 0 {
		t.Fatalf("dependency-ordered resolution failed: %s", messages(warns))
	}

	// And one that genuinely matches. Both pair fields are used, so both are
	// pinned to Num — matching is exact, and an unused field would stay an
	// unconstrained variable that `List [Num, Num]` does not describe.
	got, eqs, warns := run(t, "--> List a = [] | [a, List a]\n"+
		"--> Pairs = List [Num, Num]\n"+
		"let f = { [] -> 0, [[a, b], t] -> add (add a b) (f t) } in f")
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %s", messages(warns))
	}
	if got != "Pairs -> Num" {
		t.Errorf("expected the cross-referencing name to be used\n  got  %s\n  eqs  %s", got, eqs)
	}
}

// TestDeclCrossReferenceScope checks that a declaration body obeys the same
// import rules as a signature.
func TestDeclCrossReferenceScope(t *testing.T) {
	modSrc := "module\n\n--> Box a = [a]\n\nwrap = x -> [x]\n"

	// Imported, unqualified and qualified both fine.
	for _, prog := range []string{
		"import boxes in\n--> Pair a = [Box a, Box a]\nlet p = [[1], [2]] in p",
		"import boxes in\n--> Pair a = [boxes.Box a, boxes.Box a]\nlet p = [[1], [2]] in p",
	} {
		if ws := runWithModule(t, "boxes", modSrc, prog); len(ws) != 0 {
			t.Errorf("expected no warnings for\n%s\n  got %s", prog, messages(ws))
		}
	}

	// Not imported: out of scope, exactly as for a value.
	ws := runWithModule(t, "boxes", modSrc, "--> Pair a = [Box a, Box a]\nlet p = [[1], [2]] in p")
	if len(ws) != 1 || !strings.Contains(ws[0].Message, `unknown type "Box"`) {
		t.Errorf("expected Box out of scope, got %s", messages(ws))
	}
}

// TestDeclMutualRecursionRejected checks that two declarations defined through
// each other are reported rather than looping.
func TestDeclMutualRecursionRejected(t *testing.T) {
	_, _, warns := run(t, "--> A = [B]\n--> B = [A]\nadd 1 2")
	if len(warns) == 0 || !strings.Contains(messages(warns), "mutual recursion") {
		t.Errorf("expected a mutual-recursion complaint, got %s", messages(warns))
	}
}

// TestDeclCyclicPatternTerminates is a regression test. Expanding a reference
// ties the recursive knot with a real pointer rather than a marker, so every walk
// over a pattern needs a cycle guard; one of them did not have it and the
// analyser overflowed its stack on `Table k v = List [k, v]`.
func TestDeclCyclicPatternTerminates(t *testing.T) {
	_, _, warns := run(t,
		"--> List a = [] | [a, List a]\n--> Table k v = List [k, v]\nlet t = [[1, 2];] in t")
	for _, w := range warns {
		if strings.Contains(w.Message, "annotation") {
			t.Errorf("unexpected annotation warning: %s", w.Message)
		}
	}
}

// TestDeclModulePreference checks that a module's own name wins for a shape two
// declarations both describe. Without it, `Table k v = List [k, v]` makes every
// list of pairs in the library read as Table — list's own zip and lookup included.
func TestDeclModulePreference(t *testing.T) {
	modSrc := "module\n\n" +
		"--> List a = [] | [a, List a]\n" +
		"--> Table k v = List [k, v]\n\n" +
		"count = { [] -> 0, [[a, b], t] -> add (add a b) (count t) }\n"
	progSrc := "import tables in\n" +
		"--> List a = [] | [a, List a]\n" +
		"--> Pairs = List [Num, Num]\n" +
		"let g = { [] -> 0, [[a, b], t] -> add (add a b) (g t) } in g"

	a := analyzeWithModule(t, "tables", modSrc, progSrc)
	for _, w := range a.Warnings {
		if strings.Contains(w.Message, "annotation:") {
			t.Fatalf("unexpected annotation error: %s", w.Message)
		}
	}

	// The module's binding uses the module's name for the shape...
	var inModule string
	for _, m := range a.Modules {
		for _, e := range m.Entries {
			if e.Name == "count" {
				inModule = e.Type
			}
		}
	}
	if !strings.Contains(inModule, "Table") {
		t.Errorf("module binding should prefer its own name, got %q", inModule)
	}
	// ...and the program's uses the program's.
	if !strings.Contains(a.Program, "Pairs") {
		t.Errorf("program should prefer its own name, got %q", a.Program)
	}
}

// analyzeWithModule is runWithModule, returning the whole analysis.
func analyzeWithModule(t *testing.T, modName, modSrc, progSrc string) *types.Analysis {
	t.Helper()
	modTokens := syntax.LexContent(modName+".th", modSrc)
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
	return types.Infer(program, modules, syntax.Resolve(program, modules))
}
