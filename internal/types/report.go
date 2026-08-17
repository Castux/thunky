package types

import (
	"fmt"
	"strings"

	"github.com/Castux/thunky/internal/source"
)

// definitions renders the recursive-type equations the rest of the report refers
// to. They are printed first because every signature below may mention them.
func definitions(a *Analysis) string {
	if len(a.Equations) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("-- types\n")
	width := 0
	for _, e := range a.Equations {
		if n := len(e.Header()); n > width {
			width = n
		}
	}
	for _, e := range a.Equations {
		// A declared name came from a `--> Name … = …` annotation; the rest were
		// generated for shapes nothing had named.
		mark := ""
		if e.Declared {
			mark = "  (declared)"
		}
		fmt.Fprintf(&out, "  %-*s = %s%s\n", width, e.Header(), e.Body, mark)
	}
	out.WriteString("\n")
	return out.String()
}

// Report renders the inferred types: one line per module binding, then the
// program's own type, then anything the analysis could not make sense of.
func Report(a *Analysis, programPath string) string {
	var out strings.Builder
	out.WriteString(definitions(a))

	for _, mod := range a.Modules {
		if len(mod.Entries) == 0 {
			continue
		}
		fmt.Fprintf(&out, "-- %s\n", mod.Name)
		width := 0
		for _, e := range mod.Entries {
			if len(e.Name) > width {
				width = len(e.Name)
			}
		}
		for _, e := range mod.Entries {
			fmt.Fprintf(&out, "  %-*s : %s\n", width, e.Name, e.Type)
			// A contradicted signature is still shown as written — it is what the
			// author claims — with the finding underneath, where the disagreement is
			// impossible to miss.
			if e.Inferred != "" {
				fmt.Fprintf(&out, "  %-*s   ^ does not hold; inferred %s\n", width, "", e.Inferred)
			}
		}
		out.WriteString("\n")
	}

	fmt.Fprintf(&out, "-- %s\n", programPath)
	fmt.Fprintf(&out, "  %s : %s\n", "<program>", a.Program)

	// Provenance once, rather than a marker on every line: after a library is
	// annotated most types are the author's, and saying so per line is noise.
	if given, total := a.Coverage(); total > 0 {
		fmt.Fprintf(&out, "\n-- %d of %d module types are given signatures; %d inferred\n",
			given, total, total-given)
		// The `!` marks are the assumptions the analysis was told to take on
		// trust. Totalling them keeps them auditable rather than letting them
		// accumulate unnoticed.
		if marks, sigs := a.Assertions(); marks > 0 {
			fmt.Fprintf(&out, "-- %d position(s) in %d signature(s) asserted with `!`\n", marks, sigs)
		}
	}

	if len(a.Warnings) > 0 {
		fmt.Fprintf(&out, "\n-- %d place(s) the shapes did not line up\n", len(a.Warnings))
		for _, w := range a.Warnings {
			fmt.Fprintf(&out, "  %s: %s\n", where(w.Pos), w.Message)
		}
	}

	return out.String()
}

// ReportAll renders one line per expression, in source order: where it is, what
// kind of node it is, and what it was inferred to be.
func ReportAll(a *Analysis) string {
	var out strings.Builder
	out.WriteString(definitions(a))
	file := ""
	for _, e := range a.Exprs {
		if f := fileName(e.Pos); f != file {
			file = f
			fmt.Fprintf(&out, "-- %s\n", file)
		}
		line, column, _ := e.Pos.LineCol()
		fmt.Fprintf(&out, "  %4d:%-3d %-14s %s\n", line, column, e.Kind, e.Type)
	}
	return out.String()
}

func where(pos source.SourcePos) string {
	line, column, _ := pos.LineCol()
	return fmt.Sprintf("%s:%d:%d", fileName(pos), line, column)
}
