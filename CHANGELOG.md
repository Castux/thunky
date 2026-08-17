# Changelog

Notable changes to Thunky. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[semantic versioning](https://semver.org/spec/v2.0.0.html), where the public
interface is the language, the standard library, and the command line.

## [1.0.0] — 2026-08-17

First public release.

### Added

- `--version` and `--help`. Both used to be rejected as unknown flags.
- Prebuilt binaries for Linux, macOS and Windows on amd64 and arm64, published
  from a tag, and `go install github.com/Castux/thunky@latest`. The module path
  changed from `thunky` to `github.com/Castux/thunky` to make that possible.
- A documented exit-code contract: `0` success, `1` an error in the program,
  `2` a bad command line, `70` a failed assertion inside the compiler.
- The site gains link-preview metadata, a 404 page, a sitemap, and icons; the
  playground's share links now carry standard input, the dump stage and any
  local modules, not just the program text.
- CI runs the golden suite, the standard-library suite and the documentation
  snippet checker on Linux and Windows, and the Pages deploy is gated on them.
- A headless-browser test of the site (`web/page-smoke.mjs`), covering the
  documentation shell, a runnable snippet, the playground, and share-link
  round-trips. No dependencies: it speaks the DevTools protocol directly.
- The standard-library suite grew from 471 to 575 assertions and now covers
  every binding documented in the language reference. An API audit found 73
  documented bindings with no assertion; all of them worked, and all of them
  are now pinned.

### Changed

- Diagnostics now go to **standard error** rather than standard output, so
  `thunky prog.þ > out.txt` captures only what the program printed. Colour
  detection already looked at stderr, so redirecting used to write ANSI escapes
  into the file.
- Columns and caret widths in diagnostics are counted in characters rather than
  bytes, which was wrong on any line containing non-ASCII.
- CodeMirror and marked are served from the site itself rather than from two
  CDNs, so the site is genuinely self-contained and makes no third-party
  request.
- Advent of Code puzzle inputs are no longer in the repository, as Advent of
  Code asks. The solutions remain; bring your own input.

### Fixed

- `div` and `mod` by zero crashed with a Go panic and a goroutine dump instead
  of a located runtime error.
- A numeric literal too large for a 64-bit float panicked out of the lexer,
  past the parser's recovery, as a Go stack trace. It is now a diagnostic.
- Any remaining assertion failure inside the compiler is reported as an
  internal-compiler-error bug report rather than a bare crash. (A stack
  overflow on pathologically deep input remains fatal: Go cannot recover one.)
- An unterminated string literal was reported as `unexpected character '''`,
  and a stray non-ASCII character was sliced one byte wide into mojibake.
- The README's flagship example used the half-open `range` where it meant
  `rangeIncl`, so it printed non-primes, and its insertion sort had its
  branches swapped, so it sorted descending.
- Every CodeMirror editor on the site was a keyboard trap: Tab indented and
  nothing moved focus out. Escape now blurs.
- `tests/run.ps1` silently reported success while running no tests at all under
  Windows PowerShell 5.1, and once that was fixed, fed every program a spurious
  BOM on standard input.
- Every share link in the older `#code=` format, and the uncompressed `#u=`
  fallback, failed to load: restoring one touched a `let` binding declared
  below the bootstrap, and the resulting error was reported to the user as a
  corrupt link.
- `stdin` and `bstdin` compiled to an identical `PushConst` of a live stream
  object, so the two were indistinguishable in a bytecode dump. They now have
  their own opcodes, which had been implemented but never emitted.

## [v0] — earlier

The pre-1.0 history: the language, the standard library, the G-machine
backend, the documentation and the playground, developed together over 450-odd
commits. See the git log.
