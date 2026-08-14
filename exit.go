package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

// repoURL is where a user is sent to report a compiler bug. Shared by the native
// and browser entry points.
const repoURL = "https://github.com/Castux/thunky"

// Exit codes. Everything the user can provoke is exitError (a lexing, parsing,
// resolution or runtime error, all reported as positioned diagnostics) or
// exitUsage (a bad command line); exitInternal is reserved for a failed
// assertion inside the compiler. A script can therefore tell "the program is
// wrong" from "the compiler is broken" without parsing output.
const (
	exitError    = 1
	exitUsage    = 2
	exitInternal = 70
)

// catchInternalError turns an unhandled panic into a bug report rather than a
// bare Go stack dump. Every error a program can cause is reported as a
// diagnostic long before this point (source.Log for the front end,
// backend.RunSafe for the machine), so reaching here means an assertion inside
// the compiler failed: a bug in Thunky, never in the program being compiled.
//
// Deferred from both entry points. It cannot catch a stack overflow, which is
// fatal and unrecoverable in Go — the recursive builtins and the
// recursive-descent parser can still be crashed by pathologically deep input.
func catchInternalError() {
	r := recover()
	if r == nil {
		return
	}
	fmt.Fprintf(os.Stderr,
		"internal compiler error: %v\n\n"+
			"This is a bug in Thunky, not in the program being compiled.\n"+
			"Please report it at %s/issues, with the program that triggered it\n"+
			"and the trace below.\n\n%s\n",
		r, repoURL, debug.Stack())
	os.Exit(exitInternal)
}
