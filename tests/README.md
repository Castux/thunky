# Compiler regression tests

These are **golden** tests of the single execution engine (the flat-bytecode
G-machine): every program is run once and its combined stdout/stderr and exit
code must be **byte-identical** to the recorded expectation. The golden files
were produced by the pre-rewrite tree-walking interpreter (the frozen oracle),
so any divergence is an engine bug — or a deliberate change that must be
re-blessed.

They test *compiler correctness* and are deliberately separate from the
in-language standard-library unit tests in
[`examples/core_tests.þ`](../examples/core_tests.þ), which check what the
library computes rather than whether the engine reproduces the oracle.

## Running

Two equivalent ways, pick whichever fits your loop:

```sh
tests/run.sh                 # bash (Git Bash / WSL / macOS / Linux)
tests/run.sh patterns        # only the "patterns" category
pwsh tests/run.ps1           # PowerShell (Windows)
powershell -File tests/run.ps1 patterns
```

Each builds the binary, runs every case, and prints a categorized `PASS`/`FAIL`
list with a final count; a non-zero exit means at least one case diverged, or
that no case ran at all. The shell runners feed each case's input as raw bytes,
so the invalid-UTF-8 case works.

Both harnesses run in CI (`.github/workflows/test.yml`), on Linux and on
Windows PowerShell 5.1 — they are two implementations of the same contract and
have drifted apart before.

After a deliberate output change, regenerate the expectations and review the
diff before committing:

```sh
tests/run.sh --bless [category]    # rewrites <name>.expected / <name>.exit
```

## Layout

```text
tests/cases/<category>/<name>.þ          one test program
tests/cases/<category>/<name>.in         optional: fed to that program as stdin
tests/cases/<category>/<name>.expected   golden combined stdout+stderr
tests/cases/<category>/<name>.exit       golden exit code (only when non-zero)
```

Categories (each with at least two cases): `arithmetic`, `comparison`,
`literals`, `tuples`, `lists`, `strings`, `operators`, `lambdas`, `patterns`,
`let`, `laziness`, `lexing`, `modules`, `partial-application`, `higher-order`,
`output`, `stdin`, `errors`.

The `lexing` and `errors` categories cover programs that *should* fail: a stray
character or an unterminated string for the first, a located runtime error for
the second (non-exhaustive match, applying a non-function, a non-number or a
zero divisor passed to an arithmetic builtin, a bad `write`, invalid UTF-8 on
`stdin`). The recorded message and exit code must be reproduced exactly.

## Adding a test

Drop a `.þ` program into the right category (add a sibling `.in` if it reads
standard input), run the harness with `--bless`, and review the generated
`.expected` file — it becomes the authority. The program must produce
**deterministic** output — use `show`, `peek`, `write`, or `eval` to force and
print a result so the comparison has something to diff. There is nothing else
to register; the harnesses discover files by walking `tests/cases`.
