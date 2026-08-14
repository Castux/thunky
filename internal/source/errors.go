package source

import (
	"fmt"
	"os"
	"strings"
)

// A RuntimeError is raised (by panic) when a program cannot continue: a
// non-exhaustive match, applying a non-function, a non-number passed to an
// arithmetic primitive, a malformed argument to write. It carries what is needed
// to report the failure in the programmer's terms: a message, the source span of
// the offending expression (when known), and a reduction trace.
//
// The trace is the closest thing a lazy language has to a stack trace. The Go call
// stack is useless here — a thunk is built in one place and forced in another, so
// the host stack does not mirror the program's logical call structure. Instead the
// machine records the names of the bindings (let / module / pattern) whose
// evaluation was in progress when the error happened; see collectTrace in
// machine.go. Anonymous intermediate thunks are skipped, leaving a readable
// skeleton like `while reducing: a → b → c`.
//
// RuntimeError is recovered at the run boundary (Run in machine.go). Bare,
// non-RuntimeError panics are left to propagate with their Go stack trace: they
// indicate a compiler/machine bug, not a program error.
type RuntimeError struct {
	Message string
	Pos     SourcePos
	HasPos  bool
	Trace   []string
}

func (e *RuntimeError) Error() string { return e.Message }

// ReportRuntimeError prints a RuntimeError: the message (located at its source
// span when one is known) followed by the chain of bindings that led to it.
func ReportRuntimeError(err *RuntimeError) {
	if err.HasPos {
		Log(err.Message, err.Pos, SeverityError)
	} else {
		fmt.Fprintln(os.Stderr, "runtime error: "+err.Message)
	}
	if len(err.Trace) > 0 {
		fmt.Fprintln(os.Stderr, "while reducing: "+strings.Join(err.Trace, " → "))
	}
}
