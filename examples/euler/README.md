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

Timings are from a 2024 laptop, native build.

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
| 16 | `p016-power-digit-sum.þ` | | 1366 | 0.15 s |
| 18 | `p018-maximum-path-sum.þ` | `data/p018-triangle.txt` | 1074 | 0.1 s |
| 20 | `p020-factorial-digit-sum.þ` | | 648 | 0.1 s |
| 21 | `p021-amicable-numbers.þ` | | 31626 | 2.8 s |
| 22 | `p022-names-scores.þ` | `data/p022-names.txt` | 871198282 | 3.9 s |
| 25 | `p025-thousand-digit-fibonacci.þ` | | 4782 | 5.3 s |
| 42 | `p042-coded-triangle-words.þ` | `data/p042-words.txt` | 162 | 0.6 s |
| 59 | `p059-xor-decryption.þ` | `data/p059-cipher.txt` | 129448 | 5.6 s |
| 67 | `p018-maximum-path-sum.þ` | `data/p067-triangle.txt` | 7273 | 4.8 s |

Problem 67 is problem 18 with a bigger triangle and no change to the program:
the bottom-up `foldr` never sees the combinatorial explosion the problem warns
about.

Problems 13, 16, 20 and 25 use the `big` library for arbitrary-precision
arithmetic, and 12 and 21 use `euler.th` — the local module beside them holding
the number theory these problems keep needing (`primes`, `factorise`,
`divisorCount`, `divisorSum`, `totient`, …). Both of those exist because of what
this exercise turned up; see `../TASKS-FINDINGS.md`.

See `../TASKS-FINDINGS.md` for the full list of what these exposed.
