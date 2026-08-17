package types

import (
	"fmt"
	"strings"

	"github.com/Castux/thunky/internal/source"
)

// Report renders the inferred types: one line per module binding, then the
// program's own type, then anything the analysis could not make sense of.
func Report(a *Analysis, programPath string) string {
	var out strings.Builder

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
		}
		out.WriteString("\n")
	}

	fmt.Fprintf(&out, "-- %s\n", programPath)
	fmt.Fprintf(&out, "  %s : %s\n", "<program>", a.Program)

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
