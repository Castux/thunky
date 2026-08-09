# Project Euler in Thunky

One file per problem, each stating the problem, the answer, and whatever the
solution had to work around. Run one with:

```sh
thunky examples/euler/p001-multiples.þ
```

Timings are from a 2024 laptop, native build.

| # | File | Answer | Time |
|---|------|--------|------|
| 1 | `p001-multiples.þ` | 233168 | 0.3 s |
| 2 | `p002-even-fibonacci.þ` | 4613732 | 0.1 s |
| 3 | `p003-largest-prime-factor.þ` | 6857 | 0.1 s |
| 4 | `p004-palindrome-product.þ` | 906609 | 9.4 s |
| 5 | `p005-smallest-multiple.þ` | 232792560 | 0.1 s |
| 6 | `p006-sum-square-difference.þ` | 25164150 | 0.1 s |
| 7 | `p007-nth-prime.þ` | 104743 | 9.7 s |
| 9 | `p009-pythagorean-triplet.þ` | 31875000 | 1.4 s |

## Problems not attempted, and why

**Problems whose input is a data file** (8, 11, 13, 18, 22, 42, 59, 67, …).
Thunky reads standard input and nothing else, so the data would have to be
pasted into the source. That is possible, but the data has to be *correct* —
reproducing a 1000-digit constant or a 100-row grid from memory is not
verifiable, and a single wrong digit yields a confidently wrong answer. These
need the real files, fed on stdin or embedded from a download.

See `../TASKS-FINDINGS.md` for what these programs revealed about the language
and its standard library.
