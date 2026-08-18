# Chapter 7: Let and Recursion

So far you have used `let` informally to give names to expressions. This chapter covers how `let` actually works, why it enables recursion naturally, and how to write recursive programs.

---

## What `let` is

`let` is not an assignment statement. It is an expression:

```thunky-static
let name = expr in body
```

This says: evaluate `body` with `name` bound to `expr`. The result of the whole thing is the result of `body`. Outside of `body`, `name` does not exist.

Because `let` is an expression, it can appear anywhere an expression can:

```
let x = 5 in add x x > show    -- 10
```

And `let` expressions can be nested:

```
let x = 10 in
  let y = 20 in
    add x y > show    -- 30
```

---

## Multiple bindings

You can bind several names in one `let`:

```
let
  width  = 4,
  height = 6
in
  mul width height > show    -- 24
```

Bindings are separated by commas. They are **mutually visible**: each right-hand side can refer to any other name in the group:

```
let
  a = 5,
  b = add a 1    -- a is visible here
in
  show [a, b]    -- [5, 6]
```

This mutual visibility has a crucial consequence.

---

## Recursion falls out of mutual visibility

A name can appear in its own definition:

```
let
  fact = {
    1 -> 1,
    n -> sub 1 n > fact > mul n
  }
in
  fact 5 > show    -- 120
```

`fact` refers to `fact` in its own body. This is not a special syntax for recursion — it is a direct consequence of mutual visibility within a `let` group.

In Thunky, this works because of lazy evaluation (Chapter 10): the right-hand side of a binding is not evaluated immediately. By the time `fact` is actually called, the name `fact` already refers to the correct function.

---

## Mutual recursion

Two functions that call each other go in the same `let` group:

```
let
  isEven = { 0 -> 1, n -> sub 1 n > isOdd },
  isOdd  = { 0 -> 0, n -> sub 1 n > isEven }
in
  show [isEven 4, isOdd 3]    -- [1, 1]
```

Both names are in scope for both definitions.

---

## Inner bindings shadow outer ones

```
let x = 10 in
  let x = 20 in
    show x    -- 20
```

The outer `x` is not modified. The inner `let` creates a new binding that shadows it within the inner scope.

Shadowing covers the *body* of the inner `let`, and also its own right-hand sides — so `let x = ... x ...` refers to the new `x`, not the outer one. Exercise 7.6 shows why that matters.

---

## Recursive data structures

Since `let` bindings are mutually visible and lazily evaluated, a binding can refer to itself to define an infinite structure:

```
let ones = [1, ones] in
  show ones
```

`ones` is a cons cell whose tail is itself — an infinite list of `1`s. Thunky holds this as a suspended thunk and will not crash as long as you only force a finite prefix. Chapter 10 covers laziness in full.

---

## Building recursive functions

The usual template: base case first, recursive case second.

### `sum` over a list

```
let sum = {
  []     -> 0,
  [h, t] -> sum t > add h
} in
  sum [1; 2; 3; 4; 5] > show    -- 15
```

### `map` — apply a function to each element

```
let map = f -> {
  []     -> [],
  [h, t] -> [f h, map f t]
} in
  map (mul 2) [1; 2; 3] > show    -- [2; 4; 6]
```

`map f` is a partial application — a function waiting for the list.

### `filter` — keep elements satisfying a predicate

```
let filter = p -> {
  []     -> [],
  [h, t] -> p h > { 1 -> [h, filter p t], 0 -> filter p t }
} in
  filter (gt 3) [1; 2; 3; 4; 5] > show    -- [4; 5]
```

`p h > { 1 -> ..., 0 -> ... }` pipes the boolean result into the dispatch lambda. `gt 3 x` returns `1` if `x > 3`.

---

## Accumulator pattern

Naive recursion builds a call stack as deep as the list. The **accumulator pattern** avoids this:

```
let
  reverseAcc = acc -> {
    []     -> acc,
    [h, t] -> reverseAcc [h, acc] t
  },
  reverse = reverseAcc []
in
  reverse [1; 2; 3; 4; 5] > show    -- [5; 4; 3; 2; 1]
```

The result accumulates in `acc` rather than on the call stack. In lazy Thunky this matters less than in strict languages, but the pattern is worth knowing.

---

## The `fix` combinator (optional detour)

The standard library provides `fix` in `core`:

```thunky-static
fix f = f (fix f)
```

`fix` returns the fixed point of `f`, enabling recursion without `let`:

```
import core in
  core.fix (fact -> { 1 -> 1, n -> sub 1 n > fact > mul n }) 5 > show
```

In practice you will always use `let`. `fix` is the theoretical underpinning and is occasionally useful for passing a recursive function as a first-class value.

---

## Summary

- `let name = expr in body` — binds `name` within `body`.
- Multiple bindings with commas; all names are mutually visible.
- Recursion is a direct consequence of mutual visibility.
- Inner `let` shadows outer bindings.
- Self-referential definitions work because of laziness.

---

## Exercises

### Exercise 7.1 — Fibonacci

Write a recursive `fib` (1-indexed: `fib 1 = 1`, `fib 2 = 1`, `fib 3 = 2`, …). Display the first 10 values.

<details>
<summary>Solution</summary>

```
let fib = {
  1 -> 1,
  2 -> 1,
  n -> add (fib (sub 1 n)) (fib (sub 2 n))
} in
  show [fib 1, fib 2, fib 3, fib 4, fib 5, fib 6, fib 7, fib 8, fib 9, fib 10]
```

Output: `[1, 1, 2, 3, 5, 8, 13, 21, 34, 55]`

Warning: this naive version recomputes subproblems. Chapter 10 shows how to define Fibonacci as an efficient lazy stream instead.

</details>

---

### Exercise 7.2 — Power function

Write `myPow exp n = n ^ exp` recursively. Base case: `myPow 0 n = 1`.

<details>
<summary>Solution</summary>

```
let myPow = exp -> n ->
  exp > { 0 -> 1, e -> myPow (sub 1 e) n > mul n }
in
  show [myPow 0 5, myPow 1 5, myPow 3 2, myPow 10 2]
```

Output: `[1, 5, 8, 1024]`

</details>

---

### Exercise 7.3 — Collatz sequence

The Collatz sequence from `n`: if `n = 1`, stop; if `n` is even, `n / 2`; if odd, `3n + 1`. Return the full sequence as a list.

<details>
<summary>Solution</summary>

```
import core in
let collatz = {
  1 -> [1;],
  n -> [n, core.if (eq 0 (mod 2 n)) (div 2 n) (add 1 (mul 3 n)) > collatz]
} in
  collatz 27 > show
```

The Collatz sequence from 27 has 112 elements.

</details>

---

### Exercise 7.4 — Two ways to carry state

Part 1. Write `scan depth s` that returns `1` if the string `s` has balanced parentheses and `0` otherwise. The current nesting depth is carried as an argument: `(` increases it, `)` decreases it, and a `)` at depth `0` is an immediate failure. At the end of the string the input is balanced exactly when the depth is back to `0`. Test it on `"(a(b)c)"`, `"(()"`, `")("` and `"abc"`.

Part 2. State does not have to live in an argument — it can live in *which function is running*. Write `outside` and `inside`, mutually recursive, that between them delete every parenthesised section of a string: `"ab(xx)cd(yyy)e"` becomes `"abcde"`.

<details>
<summary>Solution</summary>

```
import core, maybe, text in
let scan = depth -> {
  [] -> eq 0 depth,
  [c, t] ->
    maybe.when       (eq (text.char "(") c) (scan (add 1 depth) t)
    > maybe.elseWhen (eq (text.char ")") c) (core.if (eq 0 depth) 0 (scan (sub 1 depth) t))
    > maybe.default                         (scan depth t)
} in
  show [scan 0 "(a(b)c)", scan 0 "(()", scan 0 ")(", scan 0 "abc"]
```

Output: `[1, 0, 0, 1]`

The recursion walks the string one code point at a time while the argument `depth` carries the state. `"(()"` ends at depth `1`, so the base case reports `0`; `")("` fails early on the leading `)`.

Part 2 — the same information, held in the call graph instead:

```
import core, text in
let
  outside = { [] -> [], [c, t] -> core.if (eq (text.char "(") c) (inside t) [c, outside t] },
  inside  = { [] -> [], [c, t] -> core.if (eq (text.char ")") c) (outside t) (inside t) }
in
  outside "ab(xx)cd(yyy)e" > write
```

Output: `abcde`

There is no flag argument: `outside` copies characters and `inside` drops them, and each hands control to the other on the delimiter. Two states, two functions — this is what mutual recursion is for.

</details>

---

### Exercise 7.5 — Accumulator

Rewrite `sum` using the accumulator pattern.

<details>
<summary>Solution</summary>

```
let
  sumAcc = acc -> {
    []     -> acc,
    [h, t] -> sumAcc (add acc h) t
  },
  mySum = sumAcc 0
in
  mySum [1; 2; 3; 4; 5] > show
```

Output: `15`

</details>

---

### Exercise 7.6 — The shadowing that isn't

Earlier in this chapter, `let x = 10 in let x = 20 in show x` printed `20`: the inner binding shadows the outer one. Now predict `let x = 1 in let x = add 1 x in show x`. Does it print `2`?

Do not run this one on the playground until you have read the answer.

<details>
<summary>Solution</summary>

It prints nothing. It hangs forever, with no error:

```thunky-static
let x = 1 in let x = add 1 x in show x
```

The right-hand side of a binding is evaluated in the scope that *includes* the binding itself — that is exactly the rule that makes recursion work. So the `x` inside `add 1 x` is the new `x`, not the outer one, and the definition reads `x = add 1 x`. Forcing it forces itself, forever. Thunky cannot report this as an error because a self-referential binding is legal and useful (`let ones = [1, ones]`).

Shadowing only takes effect for the *body* of the `let`, never for its own right-hand sides. If you want the outer value, use a different name:

```
let x = 1 in
  let y = add 1 x in
    show y
```

Output: `2`

</details>

---

### Exercise 7.7 — Recursion off the list

Recursion is not a list technique; it follows whatever shape the data has. Represent a binary tree as either `[]` (empty) or a 3-tuple `[value, left, right]`, and write `size` (number of nodes), `total` (sum of the values) and `depth` (longest path from the root). Run all three on the tree with root `5`, left subtree `3`, and right subtree `8` with children `1` and `9`.

<details>
<summary>Solution</summary>

```
import core in
let
  tree = [5, [3, [], []], [8, [1, [], []], [9, [], []]]],
  size  = { [] -> 0, [v, l, r] -> add 1 (add (size l) (size r)) },
  total = { [] -> 0, [v, l, r] -> add v (add (total l) (total r)) },
  depth = { [] -> 0, [v, l, r] -> add 1 (core.if (gt (depth r) (depth l)) (depth l) (depth r)) }
in
  show [size tree, total tree, depth tree]
```

Output: `[5, 26, 3]`

All three have the same skeleton: one base case for `[]`, one recursive case that pattern-matches the node and combines the results of both subtrees. `gt (depth r) (depth l)` is threshold-first, so it asks "is `depth l` greater than `depth r`?" — making the `core.if` a maximum.

</details>
