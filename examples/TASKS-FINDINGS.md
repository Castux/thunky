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

Division has since been rewritten as schoolbook (Knuth Algorithm D), one pass
per limb instead of one per bit: 12.7x on a 338-digit-by-96-digit benchmark.
And problem 48 added `intPowMod`, because reducing at every squaring is the
difference between operands the size of the modulus and operands the size of
1000^1000, which has 3001 digits.

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

## 11. A library that ignores the argument convention breaks piping, silently

Self-inflicted, and the best finding of the batch. `big` was written with
mathematical argument order — `intMod a b` meaning a mod b — while every
builtin puts the operand first and the value last, so that `mod 10 x` is
"reduce x modulo ten" and partial application works.

Problem 97 then read:

```
total > big.intMod modulus          -- means: modulus mod total
```

which is `modulus mod total`, not `total mod modulus`. The answer came back as
exactly 10^10 — a remainder equal to its own modulus, which is impossible — and
the first suspicion fell on the freshly written schoolbook division. The
division was fine. Two rounds of probing, and two more of my own "expected"
values turning out to be the wrong ones, before the argument order was the
remaining explanation.

Every non-commutative operation in `big` now matches the builtins: `intSub k n`
is n - k, `intMod d n` is n mod d, `intPow e b` is b^e. The library also gained
`intLt`/`intLte`/`intGt`/`intGte` (and the float equivalents), which are
threshold-first like the builtin comparators, so a bignum can be filtered and
sorted the same way a number can. `intCompare` deliberately stays as "compare a
to b": a three-way result is not a predicate.

The general lesson is not about this library. A convention that holds
everywhere is load-bearing: code written against it *reads* correct, so a
library that breaks it produces a wrong answer that looks right, and the
suspicion lands anywhere but the argument order.

## 12. Builtins have no namespace, so a `let` can hide one

`rosetta/man-or-boy-test.þ` needs a store, and its three obvious operations are
`new`, `read` and `write`. Naming one of them `write` shadowed the output
builtin for the whole program, so the report at the end —

```
… > map write > eval
```

— mapped the *store* function over the lines and printed nothing at all. No
error: `write` is a perfectly good function of three arguments, `map` applied it
to one, and `eval` forced the resulting closures, which is a no-op. Exit code 0,
empty output.

This is §5's problem without the import: a name in scope wins, and a builtin has
no qualified form to fall back on. `list.map` can be reached past a shadowing
`map`, but there is no `builtin.write`.

Cheap to avoid once known — the file now uses `newCell`/`readCell`/`writeCell`
and says why — but "your program silently produces no output" is a poor first
symptom.

## 13. Mixing plain numbers and bignums makes a silent range cliff

`intPowMod m e b` took its exponent as a plain number, which is convenient and
correct up to 2^53. `rosetta/rsa-code.þ` then wanted to decrypt, where the
exponent is the size of the modulus — 32 digits here. Passing it as a number
made `pred e` a no-op, since a float64 that large cannot represent e - 1, and
the recursion never terminated. Not an error, not a wrong answer: a hang.

`intPowModBig` now takes the exponent as an arbitrary-precision integer and
walks its bits. The general shape is worth remembering when designing an API
that spans both kinds of number: every plain-number parameter is a range limit
that the type system cannot state and the caller cannot see, and the failure
lands far from the cause.

## 14. The `,` / `;` distinction fails at run time, in pattern position

Known and documented, but worth recording what it actually costs. Writing
`rosetta/zebra-puzzle.þ` cost three debugging cycles to two instances of it:

- `flatMap ([red, green, ivory, yellow, blue] -> …)` over `comb.permutations`.
  A permutation of five things is a five-element *list*; that pattern is a
  five-*tuple*. It matches nothing, so the failure is `no pattern matched value
  [1; 2; 3; 4; 5]` at run time, pointing at the lambda rather than at the
  mistake.
- `guardAll [rightOf green ivory]` — a one-element tuple where a one-element
  list was meant, written one line below a comment warning about exactly this.
  It failed inside `foldr` in the standard library, three frames from the
  typo.

Both are the same root cause: the two shapes share a syntax, differing only in
a separator, and nothing checks the intent before the value is used. The
error message names the value's shape but nothing can name the *expected* one,
because a pattern that matches nothing is not itself illegal.

An accessor note in the same file: `core.first` and `core.second` read 2-tuples
only, and there is no generic tuple indexing, so a 7-tuple has to be
destructured by pattern. Reasonable, and worth knowing before reaching for
`first` on something wider.

## 15. Nothing says what a long program is doing

Problem 14 runs for four minutes. Nothing in the language reports progress: no
clock, no counter, no logging that is not also a value. What works is `seq` on a
`write` inside the fold, discarding the result and keeping it for the output —
the same trick the animation uses for frames.

Two things bit while doing it. The milestone test has to match the fold's
stride: the fold visits only odd starts, and no odd number is a multiple of
50000, so the obvious `mod 50000 n == 0` never fired and the run stayed silent.
And a progress line is a side effect in a pure fold, so it only happens if
something forces it — writing it as an unused `let` binding does nothing at all.

`peek` is the nearest thing to a debugger here, and it prints a value rather
than a message about one.

## 16. The performance ceiling

Native build on an Intel Core i7-6700K (4 GHz, 4 cores), 16 GB RAM. The slowest
so far: problem 14 at 4 min 15 s, problem 29 at 44 s, problem 36 at 16 s,
problem 48 at 14 s, problem 7 at 9.7 s, problem 4 at 9.4 s. Most are under a
second.

It also changes which algorithms are worth writing. Problem 34's range scan —
2.5 million candidates, the standard approach — did not finish in ten minutes;
searching digit multisets instead, 11440 of them, takes four seconds. The
reformulation is better on any machine, but here it is the difference between
an answer and no answer, so the interpreter's speed keeps pushing the solutions
toward the sharper method rather than the obvious one.

This is fine for a toy language and it is *not* a complaint, but it bounds the
exercise. Problem 14 only finishes at all because three observations cut a
million chains of ~150 steps down to a handful each; the brute force it
describes is out of reach, and so is anything sieve-scale (problem 10 sums the
primes below two million).

---

## The self-interpreter

`examples/interpreter/` is a Thunky interpreter in Thunky — 795 lines against
the 3626 lines of Go it models — scored against the same `tests/cases/`
corpus the compiler is. **56 of 77 pass**, and every failure is a diagnostic
case: 20 test error messages the interpreter does not reproduce, and one tests
`peek`'s truncation. No case testing language semantics fails.

Two claims it settles, both of which had been guesses:

**Laziness is inherited from the host.** The evaluator has no thunk type, no
memo table and no forcing machinery. `apply (expression env f) (expression env
arg)` does not evaluate the argument, so the object language is call-by-name for
free; the host memoises the shared value, so it is call-by-*need*. An
interpreted `take 5 (upFrom 1)` terminates and an interpreted self-referential
`fibs` works. On a strict host the same interpreter needs explicit thunks and a
mutable store.

**The recursive knots tie themselves.** A `let` group's environment is defined
in terms of itself, and the module table one level up likewise, so mutual
recursion and circular imports need no fixup pass. Modules are also parsed only
when named, so bundling all twelve standard library files costs nothing.

And one it confirms the hard way: **diagnostics are the expensive part of a
language implementation, and the part a pure language makes hardest.** Reaching
56/77 took the evaluator; the remaining 21 need positions carried through every
value and every application, plus a way to abort — which, with no exceptions, is
a redesign rather than an addition.

The bugs are in §17.

## 17. What building the interpreter cost

Four bugs, each a property of the language rather than a slip, all recorded in
the source where they happened:

- **Commutativity hid an argument-order bug.** `apply` appends arguments as they
  arrive; `runPrimitive` reversed them again. `add 1 2` was therefore right and
  `sub 1 n` computed `1 - n`, so an interpreted countdown ran away from its base
  case. It appeared as 92,028 evaluations of a three-step recursion, and was
  found by *counting evaluations* with a `seq`-and-`write` in the evaluator —
  there is no debugger, and `peek` prints a value, not a trace.
- **`eval` shadowed, twice.** The evaluator's main function was called `eval`,
  which shadows the builtin for every importer, so the driver's `run > eval`
  built a partial application and produced no output, no error, exit 0. After
  renaming it, one missed call site left the *builtin* `eval` accepting an
  environment and returning it. §12 with consequences.
- **`[head, tail]` where `[head; tail]` was meant**, in the definition of a cons
  cell — so every list was one element long and `write "hello"` printed `h`.
  §14, in a data representation this time.
- **Failure as a value, everywhere.** Every parser returns ok-or-error and every
  combinator has to pass failures along. It reads well and it is honest work,
  but it is also why the evaluator cannot abort, and therefore why a fifth of
  the corpus is out of reach.

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
- **`hashmap` keyed by anything.** `[x, y]` as a key made the problem 11 grid
  clean, and problem 29 keys it by *bigints* — `hash` works on any value, so
  counting 9801 distinct 200-digit powers is one insert each instead of the
  O(n^2) structural comparison `nub` would do. LZW keys one dictionary by
  strings and the other by numbers, in the same program.
- **The `bit` library survived CRC-32.** Four published checksums, all exact —
  a real-world check on arithmetic-built bitwise operations, not a self-test.
- **Self-reference instead of mutation.** `euler/p031-coin-sums.þ` is the
  textbook coin-counting table, whose defining move is that the row being
  written is also being read: `ways[i] += ways[i - coin]`. Here the row is
  simply defined in terms of itself, and laziness resolves the elements in the
  order the imperative loop would have. The construct that usually forces
  mutation is the one the language is best at.
- **`flatMap` as backtracking.** `rosetta/n-queens.þ` builds each next row by
  flat-mapping the partial solutions, which makes the 92 solutions a lazy list:
  asking for the first costs one solution's worth of work.
- **`transpose`.** Matrix multiplication becomes the definition read aloud.
- **Pipes.** Nearly every solution reads as a single left-to-right sentence,
  which is why the ones that do not (the comparator folds of §3) stand out.
