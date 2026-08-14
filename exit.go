package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
)

// repoURL is where a user is sent to report a compiler bug. Shared by the native
// and browser entry points.
const repoURL = "https://github.com/Castux/thunky"

// version is the release this binary was built from. A release build stamps the
// tag over it:
//
//	go build -trimpath -ldflags "-X main.version=v1.0.0" .
//
// Left as "dev", it means a build from a working tree; versionString then falls
// back to the VCS stamp the Go toolchain records, so even an unstamped
// `go install` reports the commit it came from.
var version = "dev"

func versionString() string {
	v := version
	if v == "dev" {
		if revision, modified := vcsStamp(); revision != "" {
			v = "dev-" + revision
			if modified {
				v += "-dirty"
			}
		}
	}
	return fmt.Sprintf("thunky %s (%s/%s, %s)", v, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// vcsStamp reads the commit the Go toolchain embeds when building from a
// checkout. Absent when building from a module cache or with -buildvcs=false.
func vcsStamp() (revision string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if len(setting.Value) > 12 {
				revision = setting.Value[:12]
			} else {
				revision = setting.Value
			}
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}

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
