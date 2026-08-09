# What the Euler and Rosetta programs revealed

Running notes from writing `examples/euler/` and `examples/rosetta/`. The point
of those directories is not the puzzles; it is to find what the language makes
awkward and what the standard library should have been carrying all along.

Ordered by how much it cost, not by how hard it would be to fix.

---

## 1. No arbitrary-precision integers — **addressed, `big`**

Numbers are float64, exact to 2^53. Four problems in the first batch (13, 16,
20, 25) are *about* numbers larger than that, and they are not exotic — 2^1000,
100!, a 1000-digit Fibonacci term, a sum of 50-digit numbers. Each of those four
files carried its own copy of the same fifteen lines of decimal-digit
arithmetic, because there was nowhere shared to put one (see §4).

`core/big.þ` now provides arbitrary-precision integers (little-endian 32-bit
limbs with a sign) and floats (bigint mantissa, binary exponent, and a precision
in significand bits that travels with the value). The four problems import it,
and got faster doing so: base 2^32 instead of base 10 took problem 25 from
14.6 s to 5.3 s and problem 16 from 0.8 s to 0.15 s.

Still open: `intDivMod` is binary long division, one pass per bit of the
dividend. That is fine at these sizes and would want a proper base-2^32
schoolbook division if anything leans on it hard.

## 2. No bitwise operations — **addressed, `bit`**

Problem 59 is XOR decryption, and with no `bxor` each XOR was eight `div`/`mod`
extractions and a weighted sum.

`core/bit.þ` now provides the 32-bit operations, built from the arithmetic that
already exists rather than from new builtins: a 32-bit value and the sum of two
of them are exactly representable in a float64, so the results are exact. The
logical operations walk both operands two bits at a time and stop when both are
exhausted, so small numbers cost only their own width.

## 3. Comparators: where the bugs were, but working as intended

Every bug written in the first batch was a threshold-first inversion, and every
one was silent — `hailstone.þ` reported the longest Collatz start below 10000 as
`1`, and `binary-search.þ` found only the exact midpoint.

Recorded here as a fact about where mistakes land, not as a defect. The
convention is deliberate, and the explicit form reads unambiguously when it
matters:

```
if (a > lt b) …          -- "is a less than b", left to right
```

Both bugs above were written in the bare `lt x y` form inside a fold. The
lesson is to write the pipe form in a comparator, not to change the convention.

## 4. Modules resolved against the working directory — **fixed**

`thunky examples/euler/prog.þ` could not find `examples/euler/helper.th`,
because local modules were searched for only in the working directory. That is
what forced the bignum duplication in §1.

The search order is now: the directory holding the program, then the working
directory, then the embedded library. `examples/euler/euler.th` — the number
theory those problems kept re-deriving — exists because of this change.

One consequence worth knowing: a local file shadows a library of the same name,
and that now includes the program itself. A program stored as `json.þ` shadows
the `json` library for its own run, which is why `rosetta/json-load-print.þ` is
not called `json.þ`.

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

## 7. No prime utilities — **addressed locally, not in the library**

Number theory is most of Project Euler, and every such program re-derived
`divides`, a prime test, a prime stream, factorisation and divisor lists.

`examples/euler/euler.th` now carries them: `divides`, `primes`, `isPrime`,
`factorise`, `divisorCount`, `divisorSum`, `divisors`, `totient`, `triangle`,
`amicable`. Problems 12 and 21 are three lines each on top of it.

It is deliberately *not* in `core/`: it is domain code for one puzzle set, and
belongs next to the programs that use it. Whether a general `prime` module
belongs in the standard library is a separate question — the factorisation and
divisor functions here would be most of it.

Writing it turned up a bug worth recording, because the language gave no help:
`factorise` emitted a phantom factor `[1, 1]` when the last prime divided out
exactly, since the "square passed the remainder, so it is prime" branch fired on
the leftover 1. It silently doubled every divisor count, and surfaced as a
division by zero three functions away.

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
- JSON is now covered by `core/json.þ`, which was written as part of this
  exercise; see the reference for the tagged-value representation.
- `splitEqual` is what every other library calls `chunksOf`.
- No rounding-to-places or number formatting: `huffman.þ` hand-rolls `round2`.
- No `math.isSquare` / integer square root; problem 42 gets there through
  `isInteger` on a float square root, which is exact enough here but is luck,
  not design.

## 11. The performance ceiling

Native build on an Intel Core i7-6700K (4 GHz, 4 cores), 16 GB RAM: problem 4
at 9.4 s, problem 7 at 9.7 s, problem 25 at
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
