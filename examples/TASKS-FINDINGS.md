# What the Euler and Rosetta programs revealed

Running notes from writing `examples/euler/` and `examples/rosetta/`. The point
of those directories is not the puzzles; it is to find what the language makes
awkward and what the standard library should have been carrying all along.

Ordered by how much it cost, not by how hard it would be to fix.

---

## 1. No arbitrary-precision integers

Numbers are float64, exact to 2^53. Four problems in the first batch (13, 16,
20, 25) are *about* numbers larger than that, and they are not exotic — 2^1000,
100!, a 1000-digit Fibonacci term, a sum of 50-digit numbers.

Each of those four files therefore carries its own copy of the same fifteen
lines: a little-endian decimal digit list, `fromInt`, a carry pass, `zipLong`,
and `bigAdd`. That is the clearest "should be factored out" signal in the whole
exercise — the same code, written four times, because there is nowhere to put
it (see §4).

**Wanted:** a `bignum` module — `fromInt`, `fromString`, `toString`, `add`,
`sub`, `mul`, `mulSmall`, `compare`, `digits`. Nothing exotic; the digit-list
representation used in these files is already most of it.

## 2. No bitwise operations

Problem 59 is XOR decryption. With no `bxor`, each XOR is eight `div`/`mod`
extractions, an add per bit, and a weighted sum — about twenty arithmetic
operations where a machine has one instruction. It works and it is even
readable, but it puts a whole category of task (hashing, checksums, bit
twiddling, most binary formats) out of comfortable reach.

**Wanted:** `band`, `bor`, `bxor`, `bnot`, `shl`, `shr` as builtins, or at
minimum a `bits` module with `toBits` / `fromBits` so the conversion is written
once.

## 3. Comparators are the main source of bugs

Every bug written in this batch was a threshold-first inversion, and every one
was **silent** — a plausible wrong answer, never an error:

- `hailstone.þ`: `lt (second best) (second candidate)` reads as "best <
  candidate" and means the reverse. It reported the longest Collatz start below
  10000 as `1`.
- `binary-search.þ`: `lt target value` reads as "target < value" and means
  "value < target", so both branches went the wrong way and only the exact
  midpoint was ever found.

The convention itself is fine in a pipe (`x > lt 4`). It misleads in the one
place it is used most: as a two-argument comparator passed to a fold.

**Wanted:** key-based helpers that remove the comparator from user code
entirely — `list.maximumBy f`, `list.minimumBy f`, `list.sortOn f`, where `f`
extracts a comparable key. Both bugs above would have been impossible to write:
`maximumBy second` and `sortOn first` say what they mean. This is the single
highest-value addition on this page.

## 4. Modules resolve against the working directory, not the importing file

```
$ thunky examples/euler/usetiny.þ
Module not found: tiny (looked for tiny.th and core/tiny.th)
$ cd examples/euler && thunky usetiny.þ
42
```

A program cannot be shipped with a helper module next to it and run by path.
This is what forced the bignum duplication in §1: a shared `bignum.th` in
`examples/euler/` would only work when run from inside that directory, so every
example that needs it would stop working from the repository root.

**Wanted:** resolve imports relative to the importing file's directory (falling
back to the working directory and then the embedded library).

## 5. Import is all-or-nothing and unqualified

Every binding of an imported module lands unqualified in the importing scope, so
a module exporting `add` or `map` silently shadows the builtin or the `list`
version. The bignum code in these files uses `bigAdd`, not `add`, purely to stay
out of the way — a workaround at the definition site for something the import
site should control.

**Wanted:** selective import (`import bignum (bigAdd, bigMul)`) or
qualified-only import.

## 6. No `lines` / `words`

Every program that reads standard input opens with the same incantation:

```
stdin > split [lf;] > filter (isEmpty *> not)
```

and the ones reading numbers add `> map (line -> line > split " " > filter
(isEmpty *> not) > map stringToInt)`. Five of the Euler files start this way.

**Wanted:** `text.lines`, `text.words`, `text.unlines`, `text.unwords`.

## 7. No prime utilities

Number theory is most of Project Euler, and every such program re-derives the
same primitives: `divides`, a prime test, a prime stream, factorisation, divisor
lists. `p001` and `p003` each define `divides` inline; `p007` defines the prime
stream that half a dozen later problems will want.

**Wanted:** a `prime` module — `primes` (lazy stream), `isPrime`, `factorise`,
`divisors`, `divisorSum`, `totient`. This is the highest-leverage *domain*
addition, as opposed to §3 which is the highest-leverage *general* one.

## 8. Dynamic programming has to be rewritten, not transcribed

`rosetta/levenshtein-distance.þ` fills an (n+1)x(m+1) table, which without
arrays becomes one row at a time, each row a fold carrying the cell to its left.
That much is fine — it is arguably clearer than the array version. The trap is
that the row must be accumulated *reversed*, so that "the cell to my left" is
`head` and not `last`; written the obvious way the algorithm is O(n^3) rather
than O(n^2), with nothing to warn you.

Same shape as §3: the language does not resist the wrong version, it just makes
it slower.

## 9. No ordered container, and no O(1) indexing

`rosetta/binary-search.þ` is written as a negative result. Binary search needs
random access; the only compound type is the tuple, so a sequence is a cons list
and `nth` is O(n). The faithful implementation is *slower* than a linear scan:
O(log n) probes of O(n) each. `hashmap` is a tree but keyed by hash, so it gives
no ordering — there is no sorted map, no sorted set, no "next key after".

Not obviously worth fixing in a language of this shape, but worth knowing the
boundary: algorithms whose cost model assumes arrays do not transfer.

## 10. Smaller gaps noticed in passing

- `list.windows n` — sliding windows. Problem 8 does `tails > map (take 13) >
  filter (length = 13)`, which is the standard workaround.
- `splitEqual` is what every other library calls `chunksOf`.
- No rounding-to-places or number formatting: `huffman.þ` hand-rolls `round2`.
- No `math.isSquare` / integer square root; problem 42 gets there through
  `isInteger` on a float square root, which is exact enough here but is luck,
  not design.

## 11. The performance ceiling

Native build, 2024 laptop: problem 4 at 9.4 s, problem 7 at 9.7 s, problem 25 at
14.6 s, problem 67 at 4.8 s, problem 22 at 3.9 s. Everything else in the batch
is under a second.

This is fine for a toy language and it is *not* a complaint, but it does bound
the exercise: sieve-scale problems (problem 10 sums the primes below two
million) are not reachable by any formulation these programs could use.

---

## What worked well, and should not be lost

- **`string` and `equal` on any value.** Palindrome detection in problem 4 is
  `equal digits (reverse digits)` with no digit arithmetic at all.
- **Laziness plus self-reference.** The prime stream in problem 7 is defined in
  terms of itself and needs no bound chosen in advance; memoization makes it
  efficient without being asked.
- **`foldr` over rows.** Problems 18 and 67 are the same program, and the 100-row
  version runs as easily as the 15-row one — the "brute force is impossible"
  framing of problem 67 never arises.
- **Pattern matching on cons cells.** All four sorting algorithms in
  `rosetta/sorting-algorithms.þ` read like their textbook definitions.
- **`hashmap` keyed by a tuple.** `[x, y]` as a key made the problem 11 grid
  clean and kept neighbour lookup off the O(n) list path.
- **`flatMap` as backtracking.** `rosetta/n-queens.þ` builds each next row by
  flat-mapping the partial solutions, which makes the 92 solutions a lazy list:
  asking for the first costs one solution's worth of work.
- **`transpose`.** Matrix multiplication becomes the definition read aloud.
- **Pipes.** Nearly every solution reads as a single left-to-right sentence,
  which is why the ones that do not (the comparator folds of §3) stand out.
