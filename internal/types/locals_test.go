package types_test

import (
	"strings"
	"testing"

	"github.com/Castux/thunky/internal/syntax"
	"github.com/Castux/thunky/internal/types"
)

// analyzeProgram runs the front end over a program with no modules and returns
// the whole analysis, which is what a local-binding test needs to look at.
func analyzeProgram(t *testing.T, src string) *types.Analysis {
	t.Helper()
	tokens := syntax.LexContent("test.th", src)
	if tokens == nil {
		t.Fatalf("lexing failed: %s", src)
	}
	program := syntax.ParseProgram(tokens)
	if program == nil {
		t.Fatalf("parsing failed: %s", src)
	}
	modules := map[string]*syntax.Module{}
	return types.Infer(program, modules, syntax.Resolve(program, modules))
}

// localNamed finds a reported local by its dotted path.
func localNamed(es []types.Entry, path string) (types.Entry, bool) {
	for _, e := range es {
		if e.Name == path {
			return e, true
		}
	}
	return types.Entry{}, false
}

func localNames(es []types.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}

// TestLocalsReported checks that the names inside a `let` are reported. A program
// is usually one `let` with everything in it, so without this most of what a
// program defines has no line in the report.
func TestLocalsReported(t *testing.T) {
	a := analyzeProgram(t, "let double = n -> add n n, greet = s -> s in double 21")

	if len(a.Locals) != 2 {
		t.Fatalf("expected 2 locals, got %v", localNames(a.Locals))
	}
	if e, ok := localNamed(a.Locals, "double"); !ok || e.Type != "Num -> Num" {
		t.Errorf("double: got %q (found %v)", e.Type, ok)
	}
	// Source order, not map order.
	if a.Locals[0].Name != "double" || a.Locals[1].Name != "greet" {
		t.Errorf("locals are not in source order: %v", localNames(a.Locals))
	}
}

// TestLocalsNamedByPath checks the naming. `go` and `step` recur many times over
// in a library, so a bare name would not say which one is being reported.
func TestLocalsNamedByPath(t *testing.T) {
	a := analyzeProgram(t, "let outer = x -> let inner = y -> add x y in inner 1 in outer 2")
	if _, ok := localNamed(a.Locals, "outer.inner"); !ok {
		t.Errorf("expected outer.inner, got %v", localNames(a.Locals))
	}
}

// TestLocalsInModules checks that a module's locals are attached to it rather
// than to the program, since that is where they are written.
func TestLocalsInModules(t *testing.T) {
	modSrc := "module\n\nmagAdd = a -> let go = carry -> add carry a in go 0\n"
	a := analyzeWithModule(t, "m", modSrc, "import m in m.magAdd 1")

	for _, mod := range a.Modules {
		if mod.Name != "m" {
			continue
		}
		if _, ok := localNamed(mod.Locals, "magAdd.go"); !ok {
			t.Errorf("expected magAdd.go among m's locals, got %v", localNames(mod.Locals))
		}
		if len(a.Locals) != 0 {
			t.Errorf("a module's locals are not the program's: %v", localNames(a.Locals))
		}
		return
	}
	t.Fatal("module m not reported")
}

// TestLocalsUnreferenced checks that a binding nothing refers to still gets a
// type: the walk forces every binding of every `let` it enters, so a local is
// reported whether or not anything uses it.
func TestLocalsUnreferenced(t *testing.T) {
	a := analyzeProgram(t, "let used = 1, unused = x -> [x, x] in used")

	e, ok := localNamed(a.Locals, "unused")
	if !ok {
		t.Fatalf("an unreferenced local was not reported: %v", localNames(a.Locals))
	}
	if e.Type != "a -> [a, a]" {
		t.Errorf("unused: got %q", e.Type)
	}
}

// TestLocalSignature checks that a signature on a `let` binding is displayed as
// the author's, exactly as it is for a module binding.
func TestLocalSignature(t *testing.T) {
	a := analyzeProgram(t, "--> List a = [] | [a, List a]\n"+
		"let\n"+
		"--> count : List [Num, Num] -> Num\n"+
		"  count = { [] -> 0, [[a, b], t] -> add a (count t) }\n"+
		"in count")

	e, ok := localNamed(a.Locals, "count")
	if !ok {
		t.Fatalf("count not reported: %v", localNames(a.Locals))
	}
	if !e.Given {
		t.Errorf("expected the signature to be displayed, got inferred %q", e.Type)
	}
	if e.Type != "List [Num, Num] -> Num" {
		t.Errorf("got %q", e.Type)
	}
}

// TestLocalSignatureContradicted checks that a wrong claim on a local is caught
// and shown beside the finding, as it is for a module binding.
func TestLocalSignatureContradicted(t *testing.T) {
	a := analyzeProgram(t, "let\n"+
		"--> f : Num -> Num\n"+
		"  f = x -> [x, x]\n"+
		"in f")

	e, ok := localNamed(a.Locals, "f")
	if !ok {
		t.Fatalf("f not reported: %v", localNames(a.Locals))
	}
	if !strings.Contains(e.Inferred, "[Num, Num]") {
		t.Errorf("expected the finding beside the claim, got %q", e.Inferred)
	}
	var reported bool
	for _, w := range a.Warnings {
		if strings.Contains(w.Message, "does not hold") {
			reported = true
		}
	}
	if !reported {
		t.Error("a contradicted signature on a local should be warned about")
	}
}
