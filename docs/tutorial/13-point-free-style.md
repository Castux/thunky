# Chapter 13: Point-Free Style

Chapter 3 ended with a rule: a lambda whose argument is used **once, at the innermost position** is a composition in disguise, and `*>` can delete the argument entirely.

This chapter is about what happens when the argument is used **twice**. That case is not a composition and `*>` cannot reach it; `core` provides two combinators for it, `ap` and `fork`. They are worth using about half the time they apply, and the second half of this chapter is about recognising the other half.

---

## Where Chapter 3's rule stops

Here is the rule working:

```
let
  pointful  = x -> add 1 (mul 2 (sub 3 x)),
  pointfree = sub 3 *> mul 2 *> add 1
in
  show [pointful 10, pointfree 10, pointful 0, pointfree 0]
```

Output: `[15, 15, -5, -5]`

`x` appears once, at the bottom of the nest. Every stage takes the previous stage's result and nothing else, so the chain of stages *is* the function.

Now here is Gray code encoding, which XORs a number with itself shifted right one bit:

```
import list, bit in
let encode = n -> bit.bXor n (bit.shiftRight 1 n) in
  rangeIncl 0 7 > map encode > show
```

Output: `[0; 1; 3; 2; 6; 7; 5; 4]`

No amount of `*>` will remove that `n`. It appears twice, and the two occurrences are on *different branches* — one goes straight into `bXor`, the other detours through `shiftRight` first. Composition is a straight line; this is a fork and a join.

That shape has a name, and once you can see it you will find it everywhere:

| Shape | Argument used | Combinator |
|-------|---------------|------------|
| `x -> f (g x)` | once, innermost | `g *> f` |
| `x -> h (f x) (g x)` | twice, two branches | `fork h f g` |
| `x -> f x (g x)` | twice, and `f x` is already the join | `ap f g` |
| `x -> y -> f (g x) (g y)` | two arguments, one function | `on f g` |

---

## `fork` — two functions, one argument, a combiner

```thunky-static
fork h f g x = h (f x) (g x)
```

Read it left to right: **combine with `h`** the results of **`f`** and **`g`**, both applied to the same argument.

The most common case is two predicates joined by `and` or `or`. Project Euler 1 asks for the numbers below 1000 divisible by 3 or 5:

```
import core, list in
let
  divisibleBy = d -> n -> eq 0 (mod d n),
  pointful    = n -> or (divisibleBy 3 n) (divisibleBy 5 n),
  pointfree   = fork or (divisibleBy 3) (divisibleBy 5)
in
  show [range 1 20 > filter pointful; range 1 20 > filter pointfree]
```

Output: `[[3; 5; 6; 9; 10; 12; 15; 18]; [3; 5; 6; 9; 10; 12; 15; 18]]`

`fork or (divisibleBy 3) (divisibleBy 5)` says "divisible by 3, or divisible by 5" with no filler. The pointful version says the same thing plus the word `n` three times.

The branches do not have to be single functions — each one can be a whole pipeline. This is a Sudoku board's box index, which combines a row index and a column index that are themselves derived from the cell index:

```
import core, list in
let
  rowOf = div 9,
  colOf = mod 9,
  boxOf = fork add (rowOf *> div 3 *> mul 3) (colOf *> div 3)
in
  map boxOf [0; 4; 8; 40; 80] > show
```

Output: `[0; 1; 2; 4; 8]`

The clearest case is one where a comment already describes the shape. Each row of Pascal's triangle is the previous row added to itself, shifted one place each way:

```
import core, list in
let rows = iterate (fork (zipWith add) (prepend [0;]) (append [0;])) [1;] in
  take 6 rows > show
```

Output: `[[1;]; [1; 1]; [1; 2; 1]; [1; 3; 3; 1]; [1; 4; 6; 4; 1]; [1; 5; 10; 10; 5; 1]]`

The comment above this line in `examples/streams.þ` reads *"shift left, shift right, add"*. The `fork` version is that sentence, in order, with nothing else in it. When the point-free form turns out to be the comment, the comment is describing a shape the code itself does not show.

---

## `ap` — when the combiner is already there

Sometimes the joining function is not a separate thing you supply — it is what the first branch produces. `bit.bXor n` is already "xor with n"; it only needs the second operand.

```thunky-static
ap f g x = f x (g x)
```

Gray code, finally point-free:

```
import core, list, bit in
let encode = ap bit.bXor (bit.shiftRight 1) in
  rangeIncl 0 7 > map encode > show
```

Output: `[0; 1; 3; 2; 6; 7; 5; 4]`

`examples/rosetta/gray-code.þ` opens with the comment *"Encoding is one line: n XOR (n >> 1)"*, and `ap bit.bXor (bit.shiftRight 1)` is that line transcribed.

The most frequent `ap` in practice is the palindrome test. Compare a thing with its own reverse:

```
import core, list in
let isPalindrome = ap equal reverse in
  show [isPalindrome "racecar", isPalindrome "banana", string 12321 > isPalindrome]
```

Output: `[1, 0, 1]`

The longhand form, `s -> equal s (reverse s)`, is the kind of lambda that gets retyped in file after file because it has no name. That is the signal to look for: not that a lambda *can* be made point-free, but that the same lambda keeps reappearing.

`ap` is also the natural shape for a round-trip property, where you compare a value with what a pair of inverse functions does to it:

```
import core, list, bit in
let
  encode = ap bit.bXor (bit.shiftRight 1),
  decode = g -> foldl (acc -> shift -> bit.bXor acc (bit.shiftRight shift acc))
    g [1; 2; 4; 8; 16]
in
  rangeIncl 0 1000 > allMatch (ap eq (encode *> decode)) > show
```

Output: `1`

`ap eq (encode *> decode)` is "equal to its own round trip", which is exactly what the test is checking.

### Choosing between them

`fork` names the combiner up front; `ap` lets the first branch be the combiner. They are two spellings of one idea, and each converts to the other:

```
import core, list in
let
  viaAp   = ap equal reverse,
  viaFork = fork id equal reverse,
  forked  = fork append (take 1) (length *> string),
  apped   = ap (take 1 *> append) (length *> string)
in
  show [
    [viaAp "level", viaFork "level", viaAp "abc", viaFork "abc"],
    [forked "aaa" > equal (apped "aaa"), forked "bb" > equal (apped "bb")]
  ]
```

Output: `[[1, 1, 0, 0], [1, 1]]`

So `ap f g` is `fork id f g`, and `fork h f g` is `ap (f *> h) g`. Prefer **`fork` when you would have to invent a composition to fold the combiner into a branch**, and **`ap` when the branch is already a function of two arguments** with the first one supplied. `fork fdiv length sum` reads as "divide the length by the sum"; the `ap` spelling of it, `ap (length *> fdiv) sum`, buries the operator in the middle of a pipeline where nobody looks for it.

---

## `on` — the same idea on the other axis

`core.on` belongs to the same family. Where `ap` and `fork` run *two functions* over *one argument*, `on` runs *one function* over *two arguments*:

```thunky-static
on f g x y = f (g x) (g y)
```

It is how you build a comparator that sorts by a projection:

```
import core, list in
  sortWith (on gt second) [[1, 3]; [2, 9]; [3, 5]] > show
```

Output: `[[2, 9]; [3, 5]; [1, 3]]`

`on gt second` is "compare by the second element, descending" — which is why word-frequency code all over `examples/` ends in `sortWith (on gt second)`.

The four combinators divide up cleanly:

| | one function | two functions |
|---|---|---|
| **one argument** | `f` | `fork h f g`, `ap f g` |
| **two arguments** | `on f g` | *(write the lambda)* |

`compose` (and its operators `*>` and `<*`) fills the trivial corner: one function, one argument, applied in sequence.

---

## When to keep the lambda

Everything above is the case *for* these combinators. The case against matters more, because the failure mode of a new tool is using it everywhere.

Across `examples/`, the number of lambdas with a `fork` or `ap` shape that are **better left as lambdas** is roughly equal to the number worth rewriting. The reasons divide into four kinds.

### The combinator exists but the spelling is a puzzle

Pairing a value with something derived from it — `[n, f n]` — is one of the most common lambdas in the whole corpus. It *is* a fork. Its combiner is "build a 2-tuple", and Thunky spells that `curry id`:

```
import core, list in
let
  pointful  = n -> [n, mul n n],
  pointfree = fork (curry id) id (ap mul id)
in
  show [rangeIncl 1 5 > map pointful; rangeIncl 1 5 > map pointfree]
```

Output: `[[[1, 1]; [2, 4]; [3, 9]; [4, 16]; [5, 25]]; [[1, 1]; [2, 4]; [3, 9]; [4, 16]; [5, 25]]]`

Both compute the same thing. One of them you can read. `curry id` as a tuple constructor and `ap mul id` as squaring are both correct and both a riddle; `n -> [n, mul n n]` is neither. This shape occurs at five sites in `examples/`, and the lambda is the right call at every one.

### The arity does not fit

`fork` takes exactly two branches. Conway's Game of Life sums three:

```thunky-static
horizontal = row -> zipWith3 sum3 (rotateLeft row) row (rotateRight row)
```

There is no three-branch `fork` in `core`, and adding one to serve a single call site is worse than the lambda. The same goes for the very common three-way conditional `x -> if (p x) a (g x)`: forcing it through `fork` needs a synthetic combiner like `fork (c -> v -> if c 1 v) p g`, which has a lambda in it anyway.

### It needs a `flip` to line up

If the argument you want to vary is not in the position the function expects, the point-free form has to reorder it first:

```thunky-static
-- from examples/sokoban.þ
onTarget = position -> and (contains position boxes) (contains position targets)

-- point-free, and worse
onTarget = fork and (flip contains boxes) (flip contains targets)
```

Every `flip` in a point-free expression is a small tax on the reader, who has to hold an argument order in their head that is not written down. One `flip` is usually the point where the lambda wins.

### It is technically possible and genuinely unreadable

`fork` can be built out of the other combinators in `core` — `on` duplicates its function argument, and `flip id x` is "apply to `x`", which is between them enough to get there:

```
import core, list in
let
  readable = fork append (take 1) (length *> string),
  horror   = flip (flip (flip id *> on append) (take 1)) (length *> string)
in
  write < (groupBy equal *> flatMap horror) "aaabbc"
```

Output: `3a2b1c`

`readable` and `horror` are the same function. Three nested `flip`s to avoid naming one variable is not a style, it is a dare. This is the reason `ap` and `fork` are named bindings in `core` rather than something you are expected to assemble.

### The test

Before replacing a lambda, ask: **does the point-free form name a concept, or does it only avoid naming a variable?**

- `ap equal reverse` names *is a palindrome*. Keep it.
- `ap bit.bXor (bit.shiftRight 1)` names *xor with itself shifted*. Keep it.
- `fork or (divisibleBy 3) (divisibleBy 5)` names *divisible by 3 or 5*. Keep it.
- `fork (curry id) id (ap mul id)` names nothing. It is `n -> [n, mul n n]` with the helpful part deleted.

A named lambda argument is documentation. `s -> equal s (reverse s)` is perfectly good code; `ap equal reverse` is better only because "palindrome" is a concept that deserves a name, and a lambda repeated across four files is evidence that it needs one. Point-free is not a goal. It is what the code looks like when the shape is already there.

---

## Summary

| Name | Definition | Use when |
|------|------------|----------|
| `f *> g` | `x -> g (f x)` | the argument is used once, innermost |
| `fork h f g` | `x -> h (f x) (g x)` | two branches, and you name the combiner |
| `ap f g` | `x -> f x (g x)` | two branches, and the first one *is* the combiner |
| `on f g` | `x -> y -> f (g x) (g y)` | one function, two arguments — comparators |

- `ap f g` is `fork id f g`; `fork h f g` is `ap (f *> h) g`.
- `compose`, `flip` and `const` pass their argument along exactly once, so none of them can duplicate it. That is why `ap` and `fork` have to exist as their own bindings.
- In other languages: `ap` is the **S** combinator (Haskell's `ap`, or `<*>` on functions; Ramda's `R.ap`; J's *hook*), and `fork` is the **Φ** combinator (Haskell's `liftA2`; Ramda's `converge`; J's *fork*, which is where the name comes from).
- Reach for a combinator when it names something. Keep the lambda when it does not.

---

## Exercises

### Exercise 13.1 — Name the shape

For each lambda, say whether it is a `*>` composition, an `ap`, a `fork`, or best left alone — then rewrite the ones worth rewriting.

```thunky-static
a = xs -> length (filter even xs)
b = n -> mul n (succ n)
c = xs -> zip xs (reverse xs)
d = n -> if (even n) (div 2 n) (succ (mul 3 n))
```

<details>
<summary>Solution</summary>

```
import core, list, math in
let
  a = filter even *> length,
  b = ap mul succ,
  c = ap zip reverse,
  d = n -> if (even n) (div 2 n) (succ (mul 3 n))
in
  show [a [1; 2; 3; 4; 5; 6], b 7, c [1; 2; 3], d 6, d 7]
```

Output: `[3, 56, [[1, 3]; [2, 2]; [3, 1]], 3, 22]`

`a` uses `xs` once at the bottom — a plain composition. `b` and `c` use their argument twice with the first branch already a two-argument function applied to it, so both are `ap`. `d` is the Collatz step, and it is left alone: it is a three-way conditional, not a two-branch join, and the `fork` spelling would need a synthetic combiner containing a lambda.

</details>

---

### Exercise 13.2 — Rewrite it, then argue against yourself

The Gregorian leap-year rule is a `fork` of a `fork`. Write it point-free, check it against the spelled-out version on 1900, 1996, 2000, 2023 and 2024 — then say which one you would commit, and why.

<details>
<summary>Solution</summary>

```
import core, list in
let
  spelled = y -> and (eq 0 (mod 4 y)) (or (neq 0 (mod 100 y)) (eq 0 (mod 400 y))),
  forked  = fork and (mod 4 *> eq 0) (fork or (mod 100 *> neq 0) (mod 400 *> eq 0)),
  years   = [1900; 1996; 2000; 2023; 2024]
in
  show [map spelled years; map forked years]
```

Output: `[[0; 1; 1; 0; 1]; [0; 1; 1; 0; 1]]`

They agree. Commit `spelled`.

The rule is *"divisible by 4, unless divisible by 100, unless divisible by 400"*, and the spelled-out version has that sentence's structure visible in its parentheses. The `forked` version is not wrong and is arguably more regular, but it makes you decode `mod 100 *> neq 0` back into "not a century year" three times before you can check the rule against the one you remember. This is the test from the chapter: the point-free form names no concept the spelled-out one leaves unnamed. It only deletes `y`.

</details>

---

### Exercise 13.3 — Derive one from the other

`core` defines `ap` and `fork` independently. Show that either one suffices: define `myAp` using only `fork`, and `myFork` using only `ap` and `*>`, and check both against the originals.

<details>
<summary>Solution</summary>

```
import core, list in
let
  myAp   = f -> g -> fork id f g,
  myFork = h -> f -> g -> ap (f *> h) g
in
  show [
    [ap equal reverse "level", myAp equal reverse "level", myAp equal reverse "abc"],
    [fork add (mul 2) (add 1) 5, myFork add (mul 2) (add 1) 5]
  ]
```

Output: `[[1, 1, 0], [16, 16]]`

`fork id f g x` is `id (f x) (g x)`, and `id` applied to two arguments applies the first to the second — so it is `f x (g x)`, which is `ap f g x`. Going the other way, folding `h` onto the end of the `f` branch turns `f`'s result into the combiner that `ap` expects.

</details>

---

### Exercise 13.4 — Run-length encoding

Write `rle`, which turns `"aaaaaabbbccddddee"` into `"6a3b2c4d2e"`: each run of equal characters becomes its length followed by the character. Use `list.groupBy equal` to split the runs, and a single combinator for the per-run step.

<details>
<summary>Solution</summary>

```
import core, list in
let rle = groupBy equal *> flatMap (fork append (take 1) (length *> string)) in
  write < rle "aaaaaabbbccddddee"
```

Output: `6a3b2c4d2e`

`groupBy equal` splits the string into runs of equal neighbours. For each run, `take 1` is the character as a one-element string and `length *> string` is the count rendered as digits; `append suffix xs` puts `xs` first, so the count comes out ahead of the character. `flatMap` concatenates the chunks.

Note `take 1` rather than `head`: `append` needs a list on both sides, and `head` would give a bare code point.

</details>

---

### Exercise 13.5 — Why they have to be primitives

`compose`, `flip` and `const` cannot be assembled into `fork`, no matter how they are stacked. Explain why, in one sentence, by looking at their definitions.

<details>
<summary>Solution</summary>

```thunky-static
compose = f -> g -> x -> f (g x)
flip    = f -> x -> y -> f y x
const   = c -> x -> c
```

Every bound variable on the right-hand side appears **at most once**: `compose` uses `x` once, `flip` uses `x` and `y` once each, `const` uses `x` zero times. Composing functions that each use their input at most once can only ever produce another function that uses its input at most once — there is nothing in the set that *duplicates*, and `fork h f g x = h (f x) (g x)` needs `x` twice.

`on` is the exception that proves it: `on f g x y = f (g x) (g y)` mentions `g` twice, so it can duplicate — just not the argument you want. The `horror` example above exploits exactly that loophole, smuggling the data into the `g` slot with `flip id x`, and the result demonstrates why a named `fork` is the better answer.

</details>
