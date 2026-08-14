# Chapter 1: Getting Started

Welcome to Thunky — a small, purely functional, lazily evaluated programming language. This tutorial assumes you know how to program (you've written loops, functions, variables in some language) but have no particular experience with functional or lazy styles. Those ideas will be introduced as you need them.

Thunky is deliberately minimal. There are three runtime types: **numbers**, **tuples**, and **functions**. There are no variables, no loops, no classes, no mutation. A complete program is a single expression. This sounds limiting; by the end of this tutorial you will see it is not.

---

## Running a program

Save your code to a file (`.þ` or `.th` both work) and run it:

```sh
thunky myprogram.þ
```

Any output the program produces goes to standard output. Compilation errors are reported with source locations; runtime errors include a reduction trace showing where things went wrong.

The rest of this chapter assumes you have Thunky installed and can run programs.

---

## The program is an expression

In most languages a program is a sequence of statements: declarations, assignments, loops, function calls. In Thunky a program is a single **expression** — a thing that evaluates to a value.

The simplest possible program:

```thunky-raw
42
```

Run that. Nothing happens. There is no implicit print in Thunky — producing a value and displaying it are separate things. To see output, you use `show`:

```
show 42
```

`show` prints its argument and returns it. That second file produces:

```text
42
```

`show` always returns its argument, which means you can thread it through larger expressions for debugging without changing the result. For now, just put it at the outermost level.

There is also `peek`, which prints the same way but stops after 100 elements —
useful once your values get large, and the only safe choice for the infinite
structures of Chapter 10. Everything below works with either.

---

## Numbers and arithmetic

The only primitive type is the number. Both integers and fractions are the same type (backed by a 64-bit float):

```
show 3.14
```

```
show 100
```

Arithmetic is done with builtin functions. There are no operators like `+` or `*` — instead you call functions:

| Builtin     | What it does                    |
|------------|----------------------------------|
| `add a b`  | `a + b`                          |
| `sub a b`  | `b - a` (note the order!)        |
| `mul a b`  | `a * b`                          |
| `div a b`  | `b ÷ a` (integer, truncating)    |
| `fdiv a b` | `b / a` (floating-point)         |
| `mod a b`  | `b mod a`                        |
| `sqrt a`   | `√a`                             |

Function application in Thunky is just juxtaposition — put the function and its arguments next to each other:

```
show (add 3 4)     -- 7
```

```
show (mul 6 7)     -- 42
```

```
show (sub 1 10)    -- 9  (10 - 1)
```

The parentheses around `add 3 4` are grouping — they tell the parser that `add 3 4` is the argument to `show`, not that `3` or `4` is. In Thunky, parentheses *only group*; they never create tuples or affect types.

### The argument order of `sub`, `div`, `mod`

These three have their arguments in reverse order compared to what you might expect. `sub 1 10` is `10 - 1 = 9`. The reason is **partial application**: `sub 1` as a function means "subtract one from the argument", which is far more useful than "subtract the argument from one". You will see why this matters in Chapter 2.

---

## Comments

A comment starts with `--` and goes to the end of the line:

```
-- This whole line is a comment
show (add 1 2)   -- and this part too
```

---

## Nesting expressions

You can nest calls as deep as you like. Parentheses group sub-expressions:

```
show (add (mul 3 4) (sub 1 10))   -- 12 + 9 = 21
```

This is prefix notation — the function comes first, then its arguments. If you have written Lisp or FORTH, this pattern is familiar. For newcomers: read it inside out, innermost parentheses first.

---

## Multiple outputs with `show`

A program is one expression, but that expression can use `show` multiple times. `show` returns its argument, so you can chain calls or embed them:

```
show (show 10)
```

This prints `10` twice — once for the inner call, once for the outer. More usefully:

```
show (add (show 3) (show 4))
```

This prints three lines:

```text
3
4
7
```

The inner `show`s fire as their values are needed, and the outer `show` prints the sum. This is useful for debugging — you can peek at intermediate values without restructuring your program.

For now, you will mostly use a single `show` at the top level.

---

## Tuples

A **tuple** is a fixed-size collection of values written with square brackets and commas:

```
show [1, 2, 3]       -- a 3-element tuple
```

```
show [42]            -- a 1-element tuple
```

```
show []              -- the empty tuple
```

```
show [1, [2, 3]]     -- a tuple containing a tuple
```

Tuples are the only compound type. Lists, the subject of Chapter 4, are built from tuples. For now, use them to group values you want to print together:

```
show [add 3 4, mul 6 7, sub 1 10]    -- [7, 42, 9]
```

When you `show` a tuple of numbers, they are printed with commas. When a tuple happens to be shaped like a list (which we haven't covered yet), it prints with semicolons.

---

## Summary

- A Thunky program is a single expression.
- `show` prints a value and returns it.
- The only primitive type is number; compound data is built with tuples `[a, b, c]`.
- Arithmetic builtins: `add`, `sub`, `mul`, `div`, `fdiv`, `mod`, `sqrt`.
- Arguments are passed by juxtaposition: `add 3 4`.
- Parentheses group expressions; they do not create tuples.
- `sub a b` computes `b - a` (threshold-first ordering).

---

## Exercises

### Exercise 1.1 — Basic arithmetic

Write a program that computes and displays:
- The number of seconds in a week (7 days × 24 hours × 60 minutes × 60 seconds).
- The hypotenuse of a right triangle with legs 3 and 4 (`√(3² + 4²)` — use `sqrt`, `mul`, `add`).

<details>
<summary>Solution</summary>

```
show (mul 7 (mul 24 (mul 60 60)))
```

Output: `604800`

```
show (sqrt (add (mul 3 3) (mul 4 4)))
```

Output: `5`

</details>

---

### Exercise 1.2 — Printing a table

Print the squares of 1, 2 and 3 — `1`, `4`, `9`. First display them together as one tuple; then get them onto three separate lines. (For the second part you need a way to run three `show` calls in one program: remember a program is a single expression.)

<details>
<summary>Solution</summary>

Display all three as a tuple:

```
show [mul 1 1, mul 2 2, mul 3 3]
```

Output: `[1, 4, 9]`

To print each on its own line instead, put the three `show` calls in a list and
force it with `eval`, which evaluates every element:

```
eval [show (mul 1 1); show (mul 2 2); show (mul 3 3)]
```

Output:

```text
1
4
9
```

</details>

---

### Exercise 1.3 — Remainders

Compute `17 mod 5` and `100 mod 7` and display them together as a tuple.

<details>
<summary>Solution</summary>

```
show [mod 5 17, mod 7 100]
```

Output: `[2, 2]`

</details>

---

### Exercise 1.4 — Digits without a loop

The tens digit of `1234` is `3`. Extract it using nothing but `div` and `mod` — arithmetic is the only tool you have so far, and it is enough.

<details>
<summary>Solution</summary>

```
show (mod 10 (div 10 1234))
```

Output: `3`

`div 10 1234` is `1234 ÷ 10 = 123`, truncating the ones digit away; `mod 10 123` then keeps the new last digit. Dividing shifts digits off the right, taking the remainder reads the rightmost one.

</details>

---

### Exercise 1.5 — Thinking about order

Without running anything, predict the output of:

```
show (sub 10 (sub 3 20))
```

Then verify by running it.

<details>
<summary>Solution</summary>

`sub 3 20` = `20 - 3` = `17`. Then `sub 10 17` = `17 - 10` = `7`.

Output: `7`

</details>

---

### Exercise 1.6 — There are no negative literals

There is no unary minus in Thunky, and `-` is not part of a number token, so this does not even lex:

```thunky-static
show -273.15
```

Produce the tuple `[-273.15, -3, 3]` anyway.

<details>
<summary>Solution</summary>

```
show [sub 273.15 0, sub 3 0, 3]
```

Output: `[-273.15, -3, 3]`

`sub a b` is `b - a`, so `sub 273.15 0` is `0 - 273.15`. Negative *values* are perfectly ordinary; it is only the negative *literal* that is missing. (`import math in ...` also gives you `math.negate`.)

</details>

---

### Exercise 1.7 — How many lines?

Count the lines each of these prints, before running it: first `show (show 3)`, then `show (add (show 3) (show 3))`.

<details>
<summary>Solution</summary>

```
show (show 3)
```

Output:

```text
3
3
```

```
show (add (show 3) (show 3))
```

Output:

```text
3
3
6
```

Every `show` prints, and it also hands its argument on, so a nested `show` prints twice: once from the inside, once from the outside. In the second program the two inner `show`s each print their own `3` and the outer one prints the sum.

</details>
