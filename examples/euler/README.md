# Project Euler in Thunky

One file per problem, each stating the problem, the answer, and whatever the
solution had to work around.

```sh
thunky examples/euler/p001-multiples.þ
```

Problems whose input is a data file read it from **standard input**, since that
is the only input Thunky has:

```sh
thunky examples/euler/p011-grid-product.þ < examples/euler/data/p011-grid.txt
```

The files in `data/` are the official ones, downloaded from projecteuler.net —
the ones given inline in a problem statement were extracted from that
problem's page and checked for shape (1000 digits, 20×20, 100×50, 15 rows).

Timings are from the native build on an Intel Core i7-6700K (4 GHz, 4 cores)
with 16 GB of RAM.

| # | File | Input | Answer | Time |
|---|------|-------|--------|------|
| 1 | `p001-multiples.þ` | | 233168 | 0.3 s |
| 2 | `p002-even-fibonacci.þ` | | 4613732 | 0.1 s |
| 3 | `p003-largest-prime-factor.þ` | | 6857 | 0.1 s |
| 4 | `p004-palindrome-product.þ` | | 906609 | 9.4 s |
| 5 | `p005-smallest-multiple.þ` | | 232792560 | 0.1 s |
| 6 | `p006-sum-square-difference.þ` | | 25164150 | 0.1 s |
| 7 | `p007-nth-prime.þ` | | 104743 | 9.7 s |
| 8 | `p008-largest-product.þ` | `data/p008-number.txt` | 23514624000 | 0.4 s |
| 9 | `p009-pythagorean-triplet.þ` | | 31875000 | 1.4 s |
| 11 | `p011-grid-product.þ` | `data/p011-grid.txt` | 70600674 | 0.5 s |
| 12 | `p012-triangle-divisors.þ` | | 76576500 | 3.7 s |
| 13 | `p013-large-sum.þ` | `data/p013-numbers.txt` | 5537376230 | 1.4 s |
| 14 | `p014-longest-collatz.þ` | | 837799 | 4 min 15 s |
| 16 | `p016-power-digit-sum.þ` | | 1366 | 0.15 s |
| 17 | `p017-number-letters.þ` | | 21124 | 0.2 s |
| 18 | `p018-maximum-path-sum.þ` | `data/p018-triangle.txt` | 1074 | 0.1 s |
| 19 | `p019-counting-sundays.þ` | | 171 | 0.2 s |
| 20 | `p020-factorial-digit-sum.þ` | | 648 | 0.1 s |
| 21 | `p021-amicable-numbers.þ` | | 31626 | 2.8 s |
| 22 | `p022-names-scores.þ` | `data/p022-names.txt` | 871198282 | 3.9 s |
| 24 | `p024-lexicographic-permutation.þ` | | 2783915460 | 0.1 s |
| 25 | `p025-thousand-digit-fibonacci.þ` | | 4782 | 5.3 s |
| 29 | `p029-distinct-powers.þ` | | 9183 | 44 s |
| 30 | `p030-digit-fifth-powers.þ` | | 443839 | 30 s |
| 34 | `p034-digit-factorials.þ` | | 40730 | 4.1 s |
| 36 | `p036-double-base-palindromes.þ` | | 872187 | 16 s |
| 40 | `p040-champernowne.þ` | | 210 | 5.5 s |
| 42 | `p042-coded-triangle-words.þ` | `data/p042-words.txt` | 162 | 0.6 s |
| 48 | `p048-self-powers.þ` | | 9110846700 | 14 s |
| 52 | `p052-permuted-multiples.þ` | | 142857 | 94 s |
| 97 | `p097-large-non-mersenne.þ` | | 8739992577 | 0.2 s |
| 59 | `p059-xor-decryption.þ` | `data/p059-cipher.txt` | 129448 | 5.6 s |
| 67 | `p018-maximum-path-sum.þ` | `data/p067-triangle.txt` | 7273 | 4.8 s |

Problem 67 is problem 18 with a bigger triangle and no change to the program:
the bottom-up `foldr` never sees the combinatorial explosion the problem warns
about.

Problem 34 is the one to read for method rather than answer. Scanning its
2.5-million range did not finish in ten minutes; searching digit *multisets*
instead — 11440 of them — takes four seconds, because the digit-factorial sum
does not depend on the order of the digits.

Problem 14 prints progress as it goes: four minutes of silence is not a
readable program, and `seq` on a `write` is how a pure fold says where it is.

Problems 13, 16, 20, 25, 29 and 48 use the `big` library for arbitrary-precision
arithmetic, and 12 and 21 use `euler.th` — the local module beside them holding
the number theory these problems keep needing (`primes`, `factorise`,
`divisorCount`, `divisorSum`, `totient`, …). Both of those exist because of what
this exercise turned up; see `../TASKS-FINDINGS.md`.

See `../TASKS-FINDINGS.md` for the full list of what these exposed.
