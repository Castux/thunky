package types

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Castux/thunky/internal/source"
)

// Collecting declarations happens in two passes, because a body may name another
// declared type:
//
//	--> BigInt = [Num, List Num]
//	--> BigFloat = [BigInt, Num, Num]
//	--> Table k v = List [k, v]
//
// Pass one reads every annotation's head — name, parameters, body text — without
// parsing the body, so that arities are known before anything is resolved. Pass
// two resolves bodies in dependency order, each in the scope of the unit that
// declared it: its own module's declarations plus those of the modules it
// imports, qualified or not. That is the rule the language already uses for
// values.
//
// A cycle between *distinct* declarations (`A = [B]`, `B = [A]`) cannot be
// expanded by substitution and is reported. A declaration referring to itself is
// not a cycle in that sense — it is the ordinary recursive case, and becomes a
// cycle in the pattern.

// A rawDecl is an annotation head, before its body has been parsed.
type rawDecl struct {
	mod    string // the module that declared it; "" for the program
	name   string
	params []string
	body   string
	pos    source.SourcePos
}

func declKey(mod, name string) string { return mod + "\x00" + name }

// CollectRawDecls reads the declaration heads from one unit's comments. Bodies
// are kept as text; signatures and malformed lines are handled elsewhere.
func CollectRawDecls(u unit) ([]rawDecl, []Warning) {
	var raws []rawDecl
	var warns []Warning

	for _, pos := range u.file.Comments {
		text := u.file.Text[pos.Start : pos.Start+pos.Length]
		if !strings.HasPrefix(text, annotMarker) {
			continue
		}
		body := strings.TrimSpace(text[len(annotMarker):])
		if body == "" {
			continue
		}

		eq := strings.Index(body, "=")
		colon := strings.Index(body, ":")
		isDecl := eq >= 0 && (colon < 0 || eq < colon)
		if !isDecl {
			if colon >= 0 {
				// A signature; checked later, once type names can be resolved.
				name := strings.TrimSpace(body[:colon])
				if !isVarName(name) {
					warns = append(warns, Warning{
						Message: fmt.Sprintf("annotation: %q is not a binding name; a signature is `name : Type`", name),
						Pos:     pos,
					})
				} else if strings.TrimSpace(body[colon+1:]) == "" {
					warns = append(warns, Warning{Message: "annotation: a signature needs a type after ':'", Pos: pos})
				}
				continue
			}
			warns = append(warns, Warning{
				Message: "annotation: expected `Name params = Type` or `name : Type`",
				Pos:     pos,
			})
			continue
		}

		head, rest := strings.TrimSpace(body[:eq]), strings.TrimSpace(body[eq+1:])
		words := strings.Fields(head)
		if len(words) == 0 {
			warns = append(warns, Warning{Message: "annotation: no name before '='", Pos: pos})
			continue
		}
		name, params := words[0], words[1:]
		if !isTypeName(name) {
			warns = append(warns, Warning{
				Message: fmt.Sprintf("annotation: type name %q must start with a capital letter", name),
				Pos:     pos,
			})
			continue
		}
		bad := false
		for _, p := range params {
			if !isVarName(p) {
				warns = append(warns, Warning{
					Message: fmt.Sprintf("annotation: parameter %q must be a lowercase name", p),
					Pos:     pos,
				})
				bad = true
				break
			}
		}
		if bad {
			continue
		}
		if dup := firstDuplicate(params); dup != "" {
			warns = append(warns, Warning{
				Message: fmt.Sprintf("annotation: parameter %q appears twice", dup),
				Pos:     pos,
			})
			continue
		}
		if rest == "" {
			warns = append(warns, Warning{Message: "annotation: nothing after '='", Pos: pos})
			continue
		}
		raws = append(raws, rawDecl{mod: u.mod, name: name, params: params, body: rest, pos: pos})
	}
	return raws, warns
}

// resolveDecls parses every declaration body in dependency order and returns the
// resolved declarations grouped by the module that declared them.
func resolveDecls(units []unit, raws []rawDecl) (map[string][]Decl, []Warning) {
	var warns []Warning

	byUnit := map[string]unit{}
	for _, u := range units {
		byUnit[u.mod] = u
	}

	index := map[string]*rawDecl{}
	order := make([]string, 0, len(raws))
	for i := range raws {
		k := declKey(raws[i].mod, raws[i].name)
		if prev, dup := index[k]; dup {
			// A second declaration of one name in one module. Only the first takes
			// effect, so say so when they differ — the other one is doing nothing.
			if prev.body != raws[i].body {
				warns = append(warns, Warning{
					Message: fmt.Sprintf("annotation: %s is declared twice with different bodies (%s and %s); the first is used.",
						raws[i].name, prev.body, raws[i].body),
					Pos: raws[i].pos,
				})
			}
			continue
		}
		index[k] = &raws[i]
		order = append(order, k)
	}

	// lookup finds the declaration a reference in unit u names, applying the same
	// visibility rule as values: the unit's own module first, then its imports
	// with a later one shadowing an earlier.
	lookup := func(u unit, ref string) (string, bool) {
		if mod, name, ok := splitQualified(ref); ok {
			if !contains(u.imports, mod) {
				return "", false
			}
			k := declKey(mod, name)
			_, exists := index[k]
			return k, exists
		}
		if k := declKey(u.mod, ref); index[k] != nil {
			return k, true
		}
		for i := len(u.imports) - 1; i >= 0; i-- {
			if k := declKey(u.imports[i], ref); index[k] != nil {
				return k, true
			}
		}
		return "", false
	}

	resolved := map[string]Decl{}
	state := map[string]int{} // 0 unseen, 1 in progress, 2 done

	var visit func(k string, chain []string) bool
	visit = func(k string, chain []string) bool {
		switch state[k] {
		case 2:
			return true
		case 1:
			r := index[k]
			warns = append(warns, Warning{
				Message: fmt.Sprintf("annotation: %s is defined in terms of itself through %s; "+
					"mutual recursion between declarations is not supported",
					r.name, strings.Join(namesOf(index, chain), " → ")),
				Pos: r.pos,
			})
			return false
		}
		state[k] = 1
		r := index[k]
		u := byUnit[r.mod]

		for _, ref := range referencedTypes(r.body) {
			if ref == r.name {
				continue // its own recursion, not a dependency
			}
			dep, ok := lookup(u, ref)
			if !ok {
				continue // reported by the parser, with a better message
			}
			if !visit(dep, append(chain, k)) {
				state[k] = 2
				return false
			}
		}

		// Build the scope from what is resolved so far and parse the body.
		scope := scopeFor(u, index, resolved, lookup)
		params := map[string]int{}
		for i, p := range r.params {
			params[p] = i
		}
		pat, err := parseTypeExpr(r.body, typeExprOpts{
			self: r.name, params: params, scope: scope, allowTop: true,
		})
		if err != "" {
			warns = append(warns, Warning{Message: "annotation: " + err, Pos: r.pos})
			state[k] = 2
			return false
		}
		resolved[k] = Decl{Mod: r.mod, Name: r.name, Params: r.params, Body: r.body, Pat: pat, Pos: r.pos}
		state[k] = 2
		return true
	}

	for _, k := range order {
		visit(k, nil)
	}

	out := map[string][]Decl{}
	for _, k := range order {
		if d, ok := resolved[k]; ok {
			out[index[k].mod] = append(out[index[k].mod], d)
		}
	}
	return out, warns
}

// scopeFor builds the type names visible in a unit out of the declarations
// resolved so far.
func scopeFor(u unit, index map[string]*rawDecl, resolved map[string]Decl,
	lookup func(unit, string) (string, bool),
) typeScope {
	s := typeScope{byName: map[string]Decl{}, byMod: map[string]map[string]Decl{}}
	add := func(mod string, tbl map[string]Decl) {
		for k, r := range index {
			if r.mod != mod {
				continue
			}
			if d, ok := resolved[k]; ok {
				if tbl != nil {
					tbl[d.Name] = d
				}
				s.byName[d.Name] = d
			}
		}
	}
	for _, imp := range u.imports {
		tbl := map[string]Decl{}
		add(imp, tbl)
		s.byMod[imp] = tbl
	}
	add(u.mod, nil) // the unit's own declarations win
	return s
}

func splitQualified(ref string) (mod, name string, ok bool) {
	if i := strings.Index(ref, "."); i > 0 {
		return ref[:i], ref[i+1:], true
	}
	return "", "", false
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func namesOf(index map[string]*rawDecl, chain []string) []string {
	out := make([]string, 0, len(chain))
	for _, k := range chain {
		if r := index[k]; r != nil {
			out = append(out, r.name)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return false })
	return out
}
