# Rosetta Code tasks in Thunky

One file per task, chosen to stress different parts of the language rather than
to be easy.

```sh
thunky examples/rosetta/fizzbuzz.þ
```

| Task | File | What it exercises |
|------|------|-------------------|
| FizzBuzz | `fizzbuzz.þ` | `case` chain over divisibility |
| 100 doors | `100-doors.þ` | simulated, not reasoned: a fold of passes over the door list |
| Happy numbers | `happy-numbers.þ` | `math.digits`; the unhappy cycle is caught by its fixed point 4 |
| Towers of Hanoi | `towers-of-hanoi.þ` | the recursive definition, verbatim |
| Roman numerals | `roman-numerals.þ` | encode and decode; decode negates a letter when a larger one follows |
| Ackermann | `ackermann.þ` | direct transcription; A(3,4) = 125 |
| Sorting algorithms | `sorting-algorithms.þ` | quick, insertion, merge and bubble, all over cons cells |
| Pascal's triangle | `pascals-triangle.þ` | one `iterate`; the triangle is an infinite lazy list |
| Hailstone (Collatz) | `hailstone.þ` | longest start below 10000 is 6171, 262 elements |
| Binary search | `binary-search.þ` | a **negative result**: no O(1) indexing, so it loses to a linear scan |
| Caesar cipher | `caesar-cipher.þ` | strings are code-point lists, so this is one `map` |
| 99 bottles | `99-bottles.þ` | |
| Levenshtein distance | `levenshtein-distance.þ` | DP without arrays: the table is built a row at a time |
| N-queens | `n-queens.þ` | backtracking as `flatMap`; 92 solutions, produced lazily |
| Matrix multiplication | `matrix-multiplication.þ` | `transpose` turns the definition into the implementation |
| JSON | `json-load-print.þ` | the `json` library: parse, walk by path, build, print pretty |
| Gray code | `gray-code.þ` | `bit`: encode is one xor, decode folds the value onto itself |
| CRC-32 | `crc-32.þ` | `bit` against four published checksums — the library's real-world test |
| LZW compression | `lzw-compression.þ` | both dictionaries as hashmaps; includes the cScSc case |
| Sieve of Eratosthenes | `sieve-of-eratosthenes.þ` | a real sieve, but O(n log n) without arrays |

See `../TASKS-FINDINGS.md` for what these revealed about the standard library.
