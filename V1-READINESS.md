# Thunky — public v1 readiness report

*2026-08-14. Audit of compiler internals, tests, CI, documentation, web site, and repo hygiene.
Findings marked **[verified]** were reproduced directly; the rest come from code inspection.*

**Verdict: close, but not ready.** The core is in good shape — the golden suite passes 77/77,
`go vet` is clean, there are zero TODO/FIXME markers in `internal/`, and the docs are unusually
thorough for a project this size. What is *not* ready falls into five clusters: two trivially
reachable interpreter crashes, a broken flagship README example, a CI pipeline that cannot fail
on a correctness regression, a handful of web-launch table stakes (link previews, a11y,
share links), and repo hygiene (dead prototype tree, scratch files, no release story).

---

## 1. Launch blockers

### 1.1 The README's flagship example is wrong **[verified]**

`README.md:74` uses `range 2 (floor (sqrt n))` in `isPrime`. `range` is half-open
(`core/list.þ:351`), so the square-root divisor itself is never tested. The program prints
`[4; 5; 6; 7; 8; 9; 11; 13; 15; 17]`, not the ten primes promised at `README.md:90`.
The playground's copy of the same program (`web/playground.js:11`) correctly uses `rangeIncl`.
This is the first code a visitor reads and runs.

### 1.2 Division by zero crashes with a raw Go panic **[verified]**

`internal/value/prims.go:118-124` does `int(b) / int(a)` and `int(b) % int(a)` with no zero
guard; `finishBuiltin` only checks the operands are numbers. `show (div 0 10)` produces
`panic: runtime error: integer divide by zero`, a full goroutine dump, and exit 2 — not a
positioned `RuntimeError`. There is no `tests/cases/errors/` case for it, which is why it
survived. In the playground the same input surfaces as a generic "worker stopped" host error.

### 1.3 An oversized numeric literal panics the lexer **[verified]**

`internal/syntax/lexer.go:30-36` panics when `strconv.ParseFloat` returns `ErrRange`
(any ~310+ digit literal). The panic string isn't the parser's `"expect"` sentinel, so
`Recover()` re-panics it (`parser.go:372`) → Go stack dump instead of a diagnostic.

Related: neither `main.go` nor `main_wasm.go` has a top-level `recover()`. Every remaining
assertion panic in the compiler reaches the user as a Go crash. A blanket
"internal compiler error — please report" handler is cheap insurance.

### 1.4 CI cannot catch a broken compiler

- The only workflow is `.github/workflows/pages.yml` — a Pages deploy. **Nothing runs
  `tests/run.sh`.** A regression that fails all 77 golden cases still deploys green.
- The one language-touching CI step, `web/smoke.mjs` on `examples/core_tests.þ`, does no
  output comparison, and `core_tests.þ` prints its failures and **always exits 0**. A build
  where 300 of 471 stdlib assertions fail is a green build.
- `check-examples.mjs` runs static-only in CI (`--run` deliberately skipped); no example
  program is ever executed.

Minimum fix: a `test.yml` workflow running `tests/run.sh` + `go vet`, and make
`core_tests.þ` (or a wrapper) exit non-zero on any failed assertion.

### 1.5 Advent of Code inputs are in the repo

`examples/aoc2025/` ships puzzle input files (`aoc01.txt` …). Advent of Code explicitly asks
that inputs not be republished. They are not in the playground catalogue, but they are in the
public tree. Remove or replace with synthetic inputs before v1. (The Euler set is fine: all
41 solved problems are ≤ 100, within Project Euler's publishing tolerance.)

---

## 2. Compiler & runtime robustness

- **Fatal stack overflow on deep data.** The reducer is iterative, but `DeepEqual`
  (`internal/value/equal.go:43`), `FullNormalForm` (`equal.go:77`), `computeHash`
  (`builtins.go:104`), and `show` on non-list cons chains (`show.go:103`) recurse on the Go
  stack — unrecoverable overflow, `recover()` can't catch it. The recursive-descent parser has
  the same exposure on pathological nesting (`((((…`).
- **No resource limits in the engine.** No step, time, or allocation budget anywhere in
  `machine.go`. The 20 s cap in `web/runner.js:9` is the only limit in the system and lives in
  the browser harness. The CLI has nothing (no signal handling either), and a `--timeout` or
  `--max-steps` would need engine support that doesn't exist.
- **Diagnostics go to stdout, columns are bytes.** `source.Log` prints via `fmt.Printf`
  (stdout) while color detection stats **stderr** (`source.go:46` vs `:84`), so
  `thunky prog.þ > out.txt` writes ANSI codes into the file and diagnostics can't be separated
  from program output. Columns/caret math is byte-based (`source.go:76,86-93`) — misaligned on
  any non-ASCII line, in a language whose files are named `.þ`. The lexer's
  "unexpected character" slices one byte (`lexer.go:135`), producing mojibake for non-ASCII,
  and an unterminated string is reported as `unexpected character '''`.
- **CLI gaps.** No `--version` (no version string exists in the source at all) and no
  `--help` — both hit "Unknown flag", exit 1 **[verified]**. Usage prints only when no path is
  given, to stdout. Multiple positional args are silently accepted, last wins. `--to-file`
  without a dump flag is a silent no-op. No exit-code distinction between user error, compile
  error, and internal error.
- **Native builds leak local paths.** Panic traces show `C:/Users/Noé/...` and `D:/dev/thunky`
  **[verified]** — build releases with `-trimpath` (the wasm build already does).
- **Latent nil-derefs** (not currently reachable, but one edit away): `source.Log` dereferences
  `loc.File` unchecked (`source.go:67`), reachable if a builtin collision produces a zero
  `SourcePos` (`resolve.go:79`); `handleImports` assumes the module map is pre-validated
  (`resolve.go:90`); the bounds guard at `parser.go:16` is `>` where it means `>=`.

---

## 3. Testing

State: 77 golden cases in 18 categories, all passing locally **[verified]**; 471 stdlib
assertions in `examples/core_tests.þ`. But:

- **Zero Go unit tests.** No `*_test.go` anywhere against ~5,000 lines of compiler/runtime.
  Everything is end-to-end through the binary; internal invariants are tested only
  transitively by 77 four-line programs.
- **Coverage gaps in `tests/cases`:** no CLI-flag tests (the entire `--dump-*` inspection mode
  is untested), no local-module resolution tests (not one `.th` file under `tests/` — search
  order, extension precedence, shadowing of embedded core, transitive imports: all unexercised;
  this is the first thing a real user does), no div-by-zero (see §1.2), no deep-recursion /
  stack-limit pin, only 3 laziness cases, and 9 of 12 stdlib modules appear only in
  `core_tests.þ`.
- **`tests/run.ps1` silently reports success with zero tests under Windows PowerShell 5.1**
  **[verified]**: the file is UTF-8 without BOM, so PS 5.1 misreads the literal `þ` in the
  `-Filter` and matches nothing — "0 passed, 0 failed", exit 0. It works under `pwsh` (PS 7),
  which is not installed by default. Either add a BOM + a "found 0 cases" guard, or require
  and check for pwsh.
- **The only differential check (bench oracle) requires an uncommitted, hand-built binary**
  (`thunky.oracle.exe`), unreconstructible in CI.
- Stale test docs: `tests/README.md` lists 17 categories (omits `lexing`); `bench/README.md`
  calls the suite "differential + golden" (it's golden-only) and has 6 broken doc links from
  the docs reorg.

---

## 4. Documentation

Overall quality is high — module/builtin tables in LANGUAGE.md match the code exactly, all
~70 tutorial cross-references survived the chapter renumbering, quoted diagnostics match the
source verbatim. The drift that remains:

- `README.md:113` says "14 chapters"; there are 15 (stale since `e7321db` added
  point-free style).
- `README.md:139-140` says Pages deploys on pushes to **`v1`**; `pages.yml:4-5` triggers on
  **`master`**. Decide the branch story before v1 (a stale `v1` branch also exists on origin).
- `README.md:24` and `docs/LANGUAGE.md:30` say output happens via `show`/`write`/`peek` —
  omitting `bwrite`, contradicting three other sections that say four.
- `docs/tutorial/01-getting-started.md:5` says "two runtime types"; LANGUAGE.md and README
  say three (functions).
- The tutorial never introduces `bit`, `big`, or `json` (`12-standard-library.md` summary
  omits them; `bit` is then used unannounced in chapter 13). Chapter 12's topic list in the
  tutorial README omits `list`.
- Broken links from the `0a2cf42` docs reorg: `docs/implementation/0.Overview.md:6` and
  `2.Parser.md:12` link to `LANGUAGE.md` relative to the wrong directory; the 6 `bench/README.md`
  links above. `1.Lexer.md` is orphaned (the Overview links every stage's chapter except Lex).
- `docs/LANGUAGE.md:676-678` claims five undocumented helpers exist in the stdlib; the real
  count is ~60 (34 in `big`, 21 in `json`, …). Either document the intended-public ones
  (`intFromDigits`, `intMagnitude` look like API) or state the convention differently.
- `0.Overview.md:76-80` omits `--to-file` from the stage-inspection table the README points at;
  `:16-18` claims every case has a `.exit` file (only 21/77 do).
  `4.Core IR and Lowering.md:83-84` cites `CoreApp`/`CoreConst`; the types are `core.App`/`core.Const`.
- **Working-notes documents are linked from public docs.** `IMPROVEMENTS.md` is linked from
  `README.md:116` but is a status-tagged worklog ("Status: implemented", "Measured and
  rejected"). `PERF-ANALYSIS.md` opens with an erratum disclaiming its own body and cites
  five identifiers that no longer exist. `examples/TASKS-FINDINGS.md` is a first-person design
  journal (§6 is stale — `text.lines`/`words`/`unlines`/`unwords` all exist now) referenced by
  three shipped files. Decide per file: polish, move to `etc/`, or keep-and-label; fix the
  references either way.
- `README.md:122` promises every doc snippet is "runnable in place"; `thunky-static` blocks
  (~15 uses) are editable but not runnable.

---

## 5. Web site & playground

Solid architecture (worker + `terminate()` interrupt, fresh instance per run, dark mode,
no web fonts, no personal info). Launch gaps, in priority order:

1. **No social metadata, no meta description, no 404 page.** Zero OG/Twitter tags — a
   "Show HN" link unfurls as a bare URL. Content is fetched client-side, so crawlers and
   no-JS visitors see only "Loading…" (no `<noscript>` anywhere). Six meta tags per page,
   one `og:image`, one `404.html`. Cheapest, highest-reach fix in this report.
2. **Keyboard trap (WCAG 2.1.2 Level A).** Every CodeMirror instance binds Tab with no escape
   (`playground.js:159`, `site.js:133`); tutorial pages instantiate dozens per page. Bind
   `Esc → blur`. Also missing: `aria-live` on `#output`/`#status`, visible `:focus-visible`
   styles, a skip link.
3. **Share links break for ~13 of 76 catalogue examples.** The URL encodes only the program —
   not stdin, dump stage, or modules (`playground.js:274`) — so shared Euler/stdin examples
   fail for the recipient. A corrupt/truncated `#code=` payload is silently discarded
   (`playground.js:169`). Also: plain base64 (not base64url), no compression, no size guard.
4. **The "self-contained site" claim is false** (`build.sh:6-7`): CodeMirror 5.65.16 and
   marked 12.0.2 load from two CDNs, pinned but with **no SRI hashes** and no fallback — if
   the CDN is blocked, the playground throws at top level and renders a dead shell. Vendoring
   both (~250 KB, vs the 3.87 MB wasm already shipped) fixes supply-chain, offline, and
   third-party-request privacy in one move.
5. **No memory story.** No wasm memory cap; a runaway lazy allocation (the mistake a lazy
   language invites) OOM-kills the tab. Also: after Stop/timeout the new worker recompiles the
   3.87 MB wasm — cache the compiled `WebAssembly.Module` on the main thread and post it.
6. **`build.sh` is bash-only** on a Windows dev box — consider `go run web/build.go`. Go
   toolchain unpinned locally vs pinned "1.25" in CI (a `wasm_exec.js` mismatch is a hard
   runtime failure).
7. `site.js:88` hardcodes `REPO_BLOB` to `Castux/thunky/blob/master/` — every doc link 404s
   on rename/transfer/branch change, and v1 docs will link to whatever master says later.
   `100vh` (not `dvh`) cuts off the playground bottom on mobile; CodeMirror 5 is poor on touch
   (treat phone *editing* as out of scope, keep reading excellent).
8. No `favicon.ico`/`apple-touch-icon`, no `theme-color`, no `robots.txt`.

---

## 6. Repo hygiene & distribution

- **No way to get the tool.** README's Usage section assumes a `thunky` binary exists but
  never says how to build or install one — no `go install` line, no releases, no prebuilt
  binaries. Tags are `alpha` and `v0`; no release workflow, no CHANGELOG, no version string
  in the binary. For a "public v1" this is the definition of not ready.
- **`etc/experiment/` is a tracked ~4,400-line abandoned prototype** (plus `etc/old-lua/`) —
  267 files, its own `package main` with `panic("unimplemented")` stubs that `go build ./...`
  compiles. Fine to keep history in git; shipping it in the v1 tree is a decision to make
  explicitly (a subtree that dwarfs `internal/` confuses first readers).
- **`test.th` is a tracked scratch file** at the root (it's even listed in `.gitignore` —
  it was committed before the rule). Remove it.
- No CONTRIBUTING or issue templates — defensible for a toy project, but a one-paragraph
  "issues welcome, this is a learning project" note sets expectations.
- LICENSE (MIT) is fine.

---

## 7. Already in good shape

Test suite green 77/77 with byte-exact goldens including error-message text; `go vet` clean;
zero debt markers and no dead code in `internal/`; positioned diagnostics end-to-end with a
reduction trace (genuinely good for a lazy language); docs/builtins/stdlib tables verified
accurate against the code; worker interrupt architecture correct; complete dark palette;
no personal info or absolute paths in the site; well-commented code throughout.

---

## Suggested order of work

1. Fix the README example (`rangeIncl`) and the 14→15 chapter count. *(minutes)*
2. Guard div/mod by zero + `ParseFloat` range as positioned errors; add golden cases;
   add a top-level `recover()` in both mains. *(hours)*
3. Add a CI test workflow (`tests/run.sh` + `go vet`); make `core_tests.þ` failures
   non-zero-exit so the existing smoke step can actually fail. *(hours)*
4. Remove AoC inputs; delete `test.th`; decide `etc/`'s fate; align the deploy-branch story
   (`v1` vs `master`) across README/workflow/`REPO_BLOB`. *(hours)*
5. Site launch pass: OG/meta/404, Tab escape + `aria-live`, share-link stdin/modules,
   vendor the two CDN libs. *(a day)*
6. Release story: `go install` instructions, `--version`/`--help`, `-trimpath` builds,
   a `v1.0.0` tag with prebuilt binaries (a 20-line goreleaser config covers all three OSes).
7. Then the longer tail: diagnostics to stderr + rune columns, deep-data iterative builtins,
   local-module test cases, Go unit tests for lexer/parser/resolver, worklog-docs cleanup.
