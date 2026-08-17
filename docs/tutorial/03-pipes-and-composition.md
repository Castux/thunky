# Chapter 3: Pipes and Composition

You have been writing function calls as `f (g (h x))`. That works, but it has a problem: the code reads inside-out. The last operation applied (`f`) appears first in the source, and the first operation (`h`) appears deepest in the nesting. For a short expression this is tolerable; for a longer chain it becomes a chore to read.

Thunky has four binary operators — `>`, `<`, `*>`, `<*` — that exist solely to address this. They are not arithmetic; they are ways to chain and compose function calls. Introducing them now means every example from this point forward can be written in the clearest possible order.

---

## The problem with nesting

Suppose you want to take a number, add 3, multiply by 2, then subtract 1:

```
show (sub 1 (mul 2 (add 3 10)))    -- 25
```

The operations apply in this order: `add 3`, then `mul 2`, then `sub 1`. But in the source, `sub 1` is outermost and `add 3` is innermost. You read it backwards.

---

## Pipe right: `>`

`>` threads a value through a sequence of functions, left to right. Each stage receives the result of the previous one:

```thunky-static
a > f > g > h    ≡    h (g (f a))
```

The same computation, written with `>`:

```
show (10 > add 3 > mul 2 > sub 1)    -- 25
```

Or sending the result straight into `show`:

```
10 > add 3 > mul 2 > sub 1 > show
```

The value flows left to right. The code reads in the same order as the operations execute.

### How it parses

Application binds tighter than `>`, so `f x > g` means `(f x) > g`. A partially-applied function like `add 3` is evaluated before the pipe takes over.

```
show (5 > add 1 > mul 2)    -- (5 + 1) * 2 = 12
```

---

## Pipe left: `<`

`<` is the mirror: values flow right to left.

```thunky-static
h < g < f < a    ≡    h (g (f a))
```

Its main use is eliminating a single pair of outer parentheses:

```
show (add 3 4)     -- with parens
```

```
show < add 3 4     -- same thing, no parens
```

`f < x` is exactly `f x`. In Haskell this is written `$`; in Thunky it is `<`.

Use `>` for multi-step pipelines; use `<` when you want to drop one level of nesting on a single call.

---

## No mixing without parentheses

The four operators cannot be mixed in a single chain:

```thunky-static
a > f < g      -- SYNTAX ERROR
(a > f) < g    -- fine: parenthesise the sub-chain
```

The compiler will tell you if you forget. Use parentheses to combine different operators.

---

## Compose right: `*>`

`>` applies to a specific value. `*>` builds a **new function** by chaining two functions, without a value present yet:

```thunky-static
(f *> g) x    ≡    g (f x)
```

Use it when you want to name a reusable composed transformation:

```
let addThenDouble = add 1 *> mul 2 in
  show [addThenDouble 5, addThenDouble 10, addThenDouble 0]    -- [12, 22, 2]
```

`add 1 *> mul 2` is the function "add 1, then multiply by 2". Chaining: `f *> g *> h` applies `f` first, then `g`, then `h`.

---

## Compose left: `<*`

`<*` is composition in the mathematical direction — rightmost runs first:

```thunky-static
(f <* g) x    ≡    f (g x)
```

This matches the traditional notation `f ∘ g` ("f after g"). In Haskell this is `(.)`.

```
show ((mul 2 <* add 1) 5)    -- mul 2 (add 1 5) = 12
```

`mul 2 <* add 1` means "add 1 first, then mul 2". The written order is backwards from the execution order.

Use `*>` when you think of a pipeline flowing left-to-right; use `<*` when you prefer the mathematical "f after g" form.

---

## Choosing the right operator

| Situation | Operator |
|-----------|----------|
| Chain operations on a specific value, left-to-right | `>` |
| Eliminate a single pair of outer parentheses | `<` |
| Build a reusable pipeline function, left-to-right | `*>` |
| Build a reusable pipeline function, right-to-left | `<*` |

Default to `>` for most pipelines. Use `*>` when you want to name a composed function. Use `<` sparingly to clean up a single level of nesting.

---

## Practical examples

### Temperature converter

```
let celsiusToFahrenheit = mul 9 *> fdiv 5 *> add 32 in
  show [celsiusToFahrenheit 0, celsiusToFahrenheit 100, celsiusToFahrenheit 37]
```

Output: `[32, 212, 98.6]`

Three stages composed into one named function. Contrast with the nested version:

```
let celsiusToFahrenheit = c -> add 32 (fdiv 5 (mul 9 c)) in
  show [celsiusToFahrenheit 0, celsiusToFahrenheit 100, celsiusToFahrenheit 37]
```

### Building step-by-step

```
let process = mul 3 *> sub 1 *> fdiv 10 in
  show (process 7)
```

`mul 3 7 = 21` → `sub 1 21 = 20` → `fdiv 10 20 = 2`.

Output: `2`

---

## Using the standard library: `import`

The examples from here on use functions like `map`, `filter` and `sort`. These
are not builtins — they live in the standard library, which is **not imported
automatically**. To reach them, put an import clause in front of your
expression:

```
import list in
  [3; 1; 4; 1; 5; 9] > sort > show
```

`import list in <expression>` makes the `list` module's names available for the
whole expression, both qualified as `list.sort` and unqualified as `sort`. You
can import several modules at once: `import list, math in …`. The library
modules are `core`, `list`, `math`, `text`, `maybe`, `table`, `hashmap`, `comb`
and `heap`; Chapter 12 tours them, and Chapter 14 covers writing your own.

That is all you need for now — an import clause at the top, and the names are
in scope.

---

## Looking ahead

Once you reach the chapters on lists (Chapters 4 and 8), pipes become indispensable. A pipeline like:

```
import list in
[3; 1; 4; 1; 5; 9]
  > filter (gt 2)
  > sort
  > take 4
  > show
```

reads exactly as it executes. Without `>`:

```
import list in
show (take 4 (sort (filter (gt 2) [3; 1; 4; 1; 5; 9])))
```

From this point forward, examples in this tutorial use pipes wherever they improve clarity.

---

## Summary

| Operator | `a OP b OP c` means | Use for |
|----------|---------------------|---------|
| `>`      | `c (b a)`           | forward pipelines on a value |
| `<`      | `a (b c)`           | eliminating one pair of parens |
| `*>`     | `\x -> c (b (a x))` | forward function composition |
| `<*`     | `\x -> a (b (c x))` | backward function composition |

- Operators cannot be mixed without parentheses.
- Application binds tighter: `f x > g` is `(f x) > g`.
- `*>` and `<*` build new functions; `>` and `<` apply to a value.

---

## Exercises

### Exercise 3.1 — Rewrite with `>`

Rewrite the following without nested parentheses, using `>` and placing `show` at the end:

```
show (sub 1 (fdiv 4 (mul 3 (add 2 10))))
```

<details>
<summary>Solution</summary>

Operations in execution order: `add 2`, `mul 3`, `fdiv 4`, `sub 1`.

```
10 > add 2 > mul 3 > fdiv 4 > sub 1 > show
```

`10+2=12`, `12*3=36`, `36/4=9`, `9-1=8`. Output: `8`

</details>

---

### Exercise 3.2 — Build a composed function

Using `*>`, define a function `f` that squares a number (`pow 2`), then divides by 10 (`fdiv 10`), then adds 1. Apply it to 5, 10, and 20.

<details>
<summary>Solution</summary>

```
let f = pow 2 *> fdiv 10 *> add 1 in
  show [f 5, f 10, f 20]
```

`pow 2 5 = 25`, `25/10 = 2.5`, `2.5+1 = 3.5`. For 10: `100/10+1 = 11`. For 20: `400/10+1 = 41`.

Output: `[3.5, 11, 41]`

</details>

---

### Exercise 3.3 — Forward vs backward compose

Predict the output of each, then verify:

```
show ((mul 2 *> add 10) 5)
```

```
show ((mul 2 <* add 10) 5)
```

```
show (5 > mul 2 > add 10)
```

<details>
<summary>Solution</summary>

`mul 2 *> add 10`: `mul 2` runs first → `10`, then `add 10` → `20`. Output: `20`

`mul 2 <* add 10`: `add 10` runs first → `15`, then `mul 2` → `30`. Output: `30`

`5 > mul 2 > add 10`: same as (a) — `10`, then `20`. Output: `20`

(a) and (c) are identical computations written differently.

</details>

---

### Exercise 3.4 — Go point-free

The lambda `x -> add 1 (mul 2 (sub 3 x))` names its argument only to thread it through three stages. Rewrite it with `*>` so the argument disappears entirely, and check the two versions agree on `10` and on `0`.

<details>
<summary>Solution</summary>

```
let
  pointful  = x -> add 1 (mul 2 (sub 3 x)),
  pointfree = sub 3 *> mul 2 *> add 1
in
  show [pointful 10, pointfree 10, pointful 0, pointfree 0]
```

Output: `[15, 15, -5, -5]`

Reading the nested version inside-out gives `sub 3`, `mul 2`, `add 1` — exactly the left-to-right order of the `*>` chain. A lambda whose argument is used once, at the innermost position, is always a composition in disguise.

The condition matters: *used once, innermost*. When the argument appears twice, or anywhere but the bottom of the nest, no chain of `*>` can reach it and `core` has to supply a combinator instead. [Chapter 13](13-point-free-style.md) picks that up.

</details>

---

### Exercise 3.5 — The two composes are the same function

`f *> g` and `g <* f` should describe the same function. Verify it: with `f = mul 3` and `g = add 4`, check that both agree on `0`, `1`, `5` and `100`. (Use `list.map` and `equal`.)

<details>
<summary>Solution</summary>

```
import list in
let
  f   = mul 3,
  g   = add 4,
  fwd = f *> g,
  bwd = g <* f
in
  show (list.map (x -> equal (fwd x) (bwd x)) [0; 1; 5; 100])
```

Output: `[1; 1; 1; 1]`

Functions cannot be compared directly: `equal fwd bwd` is `0`, because equality on functions is identity, not behaviour. "Same function" therefore has to be tested pointwise, by applying both to a sample of inputs. The result prints with semicolons because `list.map` returns a list, not a tuple.

</details>

---

### Exercise 3.6 — Invert a pipeline

This chapter built `celsiusToFahrenheit = mul 9 *> fdiv 5 *> add 32`. Write `toCelsius`, its inverse, also as a `*>` chain. Check it on `32`, `212` and `98.6`.

<details>
<summary>Solution</summary>

```
let toCelsius = sub 32 *> mul 5 *> fdiv 9 in
  show [toCelsius 32, toCelsius 212, toCelsius 98.6]
```

Output: `[0, 100, 37]`

Inverting a pipeline means reversing the stage order *and* inverting each stage: `add 32` becomes `sub 32`, `fdiv 5` becomes `mul 5`, `mul 9` becomes `fdiv 9`. Threshold-first ordering pays off here — every inverse is still a one-token partial application.

</details>
