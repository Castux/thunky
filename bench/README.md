# Thunky — performance benchmarks

A small set of heavy programs used to measure the Thunky engine: the flat
bytecode ([5.Bytecode and Compiler](../docs/implementation/5.Bytecode%20and%20Compiler.md)) run
by the spineless-tagless G-machine
([6.The G-machine](../docs/implementation/6.The%20G-machine.md)). Thunky has a **single
execution engine**, so there is no cross-backend comparison — the headline number
is wall-clock per case.

These are **not** regression tests. The correctness suite lives in
[tests/](../tests/) (fast, byte-exact goldens) and the in-language
standard-library unit tests in [examples/core_tests.þ](../examples/core_tests.þ);
both run in CI. The programs here are deliberately slow — each runs roughly
**10–30 s** — so they are run by hand.

## Running

```
bench/run.sh                 # all cases, one run each
bench/run.sh fib             # only bench/cases/fib.þ
bench/run.sh --reps 3        # run each case 3× and keep the fastest
pwsh bench/run.ps1 [name]    # PowerShell equivalent (-Reps N)
```

Each runner builds `thunky.bench.exe`, runs every `bench/cases/*.þ`, times each,
and prints a table of per-case and total wall-clock.

**Optional oracle baseline.** If a frozen binary `thunky.oracle.exe` is present
in the repo root, each case is also timed against it and the engine's output is
checked to still match the oracle's (a perf number is meaningless if the engine
has changed behaviour). The table then gains `oracle(s)`, `speedup`
(`oracle / engine`), and `match` columns; a non-`ok` `match` is a correctness
regression to fix before trusting any timing. To create one, build at a known-good
commit: `go build -o thunky.oracle.exe .`.

Timings are wall-clock; close other load for stable numbers, and prefer
`--reps`/`-Reps` to discount one-off noise.

## What each case stresses

Each program isolates a particular runtime cost. Several are **sentinels** for the
optimizations proposed in [IMPROVEMENTS.md](../docs/implementation/IMPROVEMENTS.md): their
absolute time (or, with an oracle, their ratio) is the number to watch when that
work lands.

| Case | Kind | Stresses |
|------|------|----------|
| `fib` | common algorithm | raw β-reduction + number-pattern dispatch; touches no module, so the cleanest test of closure application and matching |
| `ackermann` | common algorithm | very deep non-tail recursion; the closure-apply path and reducer spine |
| `collatz` | common algorithm | integer-arithmetic builtins + user-driven conditionals + per-number recursion |
| `sort` | common algorithm | compound-data allocation (quicksort cons churn), comparison closures, and cross-module references (now resolved to direct slot indexes, not a name lookup — see [5.Bytecode and Compiler §Module and program layout](../docs/implementation/5.Bytecode%20and%20Compiler.md#module-and-program-layout)) |
| `list-churn` | **sentinel** | per-activation graph allocation — the dominant cost the eager-evaluation / frame-reuse work ([IMPROVEMENTS.md §1, §5](../docs/implementation/IMPROVEMENTS.md)) would cut |
| `match-dispatch` | **sentinel** | matcher worst case — 19 literal clauses failing before a catch-all ([IMPROVEMENTS.md §5, fail-fast matcher](../docs/implementation/IMPROVEMENTS.md)) |

## Sizing

Each program is sized to land in the 10–30 s window on the development machine. To
retarget, edit the single size constant near the top of each `.þ` (the `take`
count, the `range` bound, or the argument to `fib`/`go`). Two constraints worth
knowing:

- **Deep-thunk ceiling.** A flat `sum`/`foldl`/`foldr` over a list of length *N*
  forces an *N*-deep chain on the Go stack; past roughly a few hundred thousand
  it overflows. `collatz` and `sort` stay well under it; `list-churn` and
  `match-dispatch` avoid it by chunking and by O(log n)-depth balanced recursion
  respectively. Scale by widening the work, not by deepening a single fold.
- Inputs are reproducible (a fixed LCG seed, fixed ranges), so output hashes are
  stable across runs and the oracle match check is meaningful.
