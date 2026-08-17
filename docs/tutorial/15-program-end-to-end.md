# Chapter 15: A Program End to End

Everything in this chapter you already know. There is no new syntax here — no new keyword, no new builtin, no new library module. What is new is the *scale*: instead of an expression that demonstrates one idea, we build a whole program, take input from the outside world, and split it across files.

The program is a **reverse Polish notation calculator**. It reads expressions like `3 4 + 2 *`, evaluates them, and prints the results.

This shape of program suits Thunky unusually well. An RPN evaluator is a state machine: a stack, and a token that transforms it. In an imperative language you write that as a loop over tokens mutating a stack variable. In Thunky the loop is a fold, the stack is a list, and "transform the state with this token" is exactly `foldl`'s step function. The whole evaluator is one line, and it is one line because the problem was already shaped like a fold — you just had to see it. That is the skill Chapter 9 was about, and this chapter is where it pays off.

---

## Step 1: tokens

Input arrives as a string. A string is a list of code points (Chapter 4), and `text.split` cuts one on a separator:

```
import list, text in
  "3 4 + 2 *" > text.split " " > text.join "|" > write
```

Output: `3|4|+|2|*`

We print with `text.join` and `write` rather than `show`, because `show` renders a string as the list of numbers it actually is — `show` on that token list would give `[[51;]; [52;]; [43;]; [50;]; [42;]]`, which is correct but unreadable. When strings are the interesting thing, `write` is the tool.

---

## Step 2: classifying a token

Each token is either a number to push or an operator to apply. `text.isDigit` classifies a single code point; `list.allMatch` lifts it to the whole token:

```
import list, text in
let isNumber = tok -> list.allMatch text.isDigit tok in
  show (text.split " " "3 4 + 2 *" > list.map isNumber)
```

Output: `[1; 1; 0; 1; 0]`

Note what `allMatch` does on the *empty* string: it returns `1`, because vacuously every character of nothing is a digit. So the empty token classifies as a number. Hold that thought — it comes back to bite us at the end of this section.

---

## Step 3: the stack is a list

We need a stack: push on one end, pop two off the same end. That is a cons list, and pushing is just building a cons cell.

```
let push = x -> stack -> [x, stack] in
  show ([] > push 3 > push 4)
```

Output: `[4; 3]`

There is nothing to implement. `[x, stack]` *is* the push, and `show` renders the result as `[4; 3]` because a stack built this way is a proper list with the top at the head. Popping is a pattern: `[b, [a, rest]]` binds `b` to the top, `a` to the one below it, and `rest` to everything under those. Pattern matching (Chapter 5) gives you a two-element pop in one destructuring — no arity check, no index arithmetic.

---

## Step 4: applying an operator

```
import text in
let
  apply = tok -> [b, [a, rest]] ->
    [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ]
in
  show [apply "-" [2, [10, []]], apply "/" [4, [20, []]], apply "+" [4, [3, [99, []]]]]
```

Output: `[[8;], [5;], [7; 99]]`

Two things are worth slowing down for.

**The multi-case lambda dispatches on the operator string.** `{ "+" -> …, "-" -> … }` is an ordinary matcher over string patterns, and `tok > { … }` applies it to the token. This is a four-way branch with no `if`, no equality test written by hand, and no dictionary lookup — just the pattern matcher doing what it does. Adding an operator means adding a line.

**The subtraction and division cases are the ones to get right.** Thunky's arithmetic is threshold-first: `sub a b` is `b - a`, and `fdiv a b` is `b / a`. The operands come off the stack in reverse — for `10 2 -` the pattern binds `b = 2` (pushed last, on top) and `a = 10`. We want `a - b = 8`. That is `sub b a`, which reads as "subtract `b`, from `a`" and evaluates to `10 - 2 = 8`. Likewise `20 4 /` wants `a / b = 5`, which is `fdiv b a`. The double reversal — stack order *and* threshold-first order — cancels out, and the result is that both arguments appear in the "wrong" order twice. `add` and `mul` are commutative so they hide the issue; `-` and `/` do not. Test them.

---

## Step 5: the fold

`foldl` walks a list left to right, threading an accumulator: `foldl f z xs` computes `f (f (f z x1) x2) x3`. That is precisely "start with an empty stack, and for each token in order, produce a new stack". The accumulator is the stack, the element is the token, and the step function is what we just wrote.

`foldl` is the right fold here specifically because the machine runs *forwards*. `foldr` would associate from the right, feeding the last token first — meaningless for a stack machine, where `3 4 -` and `4 3 -` must differ.

```
import list, text in
let
  step = stack -> tok ->
    { 1 -> [text.stringToInt tok, stack],
      0 -> stack > ([b, [a, rest]] ->
             [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ])
    } (list.allMatch text.isDigit tok)
in
  show (list.foldl step [] (text.split " " "3 4 + 2 *"))
```

Output: `[14;]`

The step function is a matcher applied to the classification flag: `{ 1 -> push, 0 -> apply } (isNumber tok)`. The branches come first and the value being matched comes last, which reads backwards at first but is just application — the same shape as Exercise 12.3.

Note the argument order: `step = stack -> tok -> …`, accumulator first. That is the order `foldl` calls it in.

---

## The whole thing

```
import list, text in
let
  step = stack -> tok ->
    { 1 -> [text.stringToInt tok, stack],
      0 -> stack > ([b, [a, rest]] ->
             [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ])
    } (list.allMatch text.isDigit tok),
  rpn = s -> s > text.split " " > list.foldl step [] > list.head
in show [rpn "3 4 +", rpn "3 4 + 2 *", rpn "10 2 -", rpn "20 4 /"]
```

Output: `[7, 14, 8, 5]`

`rpn` is one pipeline: split into tokens, fold them into a stack, take the head. If the expression was well formed the final stack holds exactly one value, and `list.head` is the answer.

---

## Watching the machine run

Swap `foldl` for `scanl` and you get every intermediate accumulator instead of just the last one — a free trace of the stack machine, which is the debugging tool you want when an expression gives the wrong number:

```
import list, text in
let
  step = stack -> tok ->
    { 1 -> [text.stringToInt tok, stack],
      0 -> stack > ([b, [a, rest]] ->
             [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ])
    } (list.allMatch text.isDigit tok)
in
  show (list.scanl step [] (text.split " " "3 4 + 2 *"))
```

Output: `[[]; [3;]; [4; 3]; [7;]; [2; 7]; [14;]]`

Empty stack, push 3, push 4, add, push 2, multiply. Because `scanl` is lazy it costs nothing you were not already going to pay — the intermediate stacks are the same cons cells `foldl` built, just kept instead of dropped.

---

## A tokeniser that survives real input

Here is the bite promised in step 2. `text.split " "` splits on *every* separator occurrence, so a double space produces an empty token, an empty token classifies as a number, and `text.stringToInt ""` is `0`. The calculator silently pushes a phantom zero:

```
import list, text in
let
  step = stack -> tok ->
    { 1 -> [text.stringToInt tok, stack],
      0 -> stack > ([b, [a, rest]] ->
             [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ])
    } (list.allMatch text.isDigit tok),
  rpn = s -> s > text.split " " > list.foldl step [] > list.head
in show [list.allMatch text.isDigit "", text.stringToInt "", rpn "3  4 +"]
```

Output: `[1, 0, 4]`

`3  4 +` should be `7`; it is `4`, because the stack was `[4, 0, 3]` and `+` added the top two. No error, no warning — the wrong answer, which is the worst kind of bug.

The fix is a tokeniser that splits on *runs* of whitespace and drops empties. `text.splitWith` takes a predicate rather than a literal separator, so it handles tabs too:

```
import list, text in
let
  step = stack -> tok ->
    { 1 -> [text.stringToInt tok, stack],
      0 -> stack > ([b, [a, rest]] ->
             [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ])
    } (list.allMatch text.isDigit tok),
  tokens = s -> s > text.splitWith text.isSpace > list.filter (t -> gt 0 (list.length t)),
  rpn    = s -> s > tokens > list.foldl step [] > list.head
in show [rpn "3  4 +", rpn "  10 2 -  ", rpn "3 4 + 2 *"]
```

Output: `[7, 8, 14]`

`gt 0 (list.length t)` is `length t > 0` — threshold-first again. Splitting the tokeniser out of `rpn` also means the evaluator no longer cares where tokens come from, which is what makes the next two sections possible.

---

## Reading input with `stdin`

`stdin` is not a function. It is a **value**: standard input presented as a lazy list of Unicode code points, with `[]` for end of input. Every list function in the library works on it directly, because it *is* a list — one whose cells happen to be produced by reading the input as you force them.

That means a word counter is just list functions:

```
import list, text in
let
  chars = stdin,
  lines = chars > text.split [text.lf;] > list.filter (l -> gt 0 (list.length l)),
  words = chars > text.splitWith text.isSpace > list.filter (w -> gt 0 (list.length w))
in
  show [list.length chars, list.length lines, list.length words]
```

Run it from the command line with input piped in:

```sh
printf 'the cat sat\non the mat\n' | thunky wc.þ
```

```text
[23, 2, 6]
```

`[text.lf;]` is the one-element string containing a line feed — Thunky has no escape sequences, so `text.lf` (the code point `10`) wrapped in a one-element list is how you write `"\n"`. Note the `[x;]` semicolon: `[text.lf]` would be a one-*tuple*, not a one-element list (Chapter 4).

Because `stdin` is a single shared stream, referring to it twice — as `chars` above does, once for lines and once for words — does not read the input twice. The cons cells are memoized like any other lazy list (Chapter 10), so the second traversal walks cells the first one already produced.

> **Empty input.** Every runnable block in this chapter still works when standard input is empty, which is what a snippet gets when you press Run on the documentation site — the counter above prints `[0, 0, 0]`. To see anything interesting, run these from a shell with input piped in.

### You only read as far as you force

Laziness applies to input the same way it applies to everything else. This program reads the first line and stops:

```thunky-static
import list, text, core in
  stdin > list.takeWhile (c -> core.not (eq text.lf c)) > write
```

```sh
yes "hello world" | thunky firstline.þ
```

```text
hello world
```

`yes` produces an infinite stream, and the program terminates anyway: `takeWhile` stops forcing cells at the first line feed, so no further character is ever read. Nothing about that is special-cased for I/O — it is the same reason `list.take 5 (list.upFrom 1)` terminates.

### The calculator as a filter

Put the two halves together and the RPN evaluator becomes a Unix filter: one expression per line in, one answer per line out.

```thunky-static
import list, text in
let
  step = stack -> tok ->
    { 1 -> [text.stringToInt tok, stack],
      0 -> stack > ([b, [a, rest]] ->
             [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ])
    } (list.allMatch text.isDigit tok),
  rpn   = s -> s > text.split " " > list.foldl step [] > list.head,
  lines = stdin > text.split [text.lf;] > list.filter (l -> gt 0 (list.length l))
in
  lines > list.map (l -> flatten [l; " = "; string (rpn l)]) > list.map write > eval
```

```sh
printf '3 4 +\n100 25 1 + /\n' | thunky rpncalc.þ
```

```text
3 4 + = 7
100 25 1 + / = 3.8461538461538463
```

The output line is `flatten [l; " = "; string (rpn l)]` — three strings concatenated, which for lists of code points is just `flatten` over a list of them. `list.map write` builds a lazy list of write actions and `eval` forces it; the writes are what actually happen when that list is forced. Filtering out empty lines means a trailing newline in the input does not produce a spurious blank result.

---

## Splitting it into modules

The program is now big enough to have two distinct jobs: *evaluating* an RPN expression, and *being a command-line filter*. Those split cleanly along Chapter 14's lines — one module with the domain logic, one program that does I/O.

> Like Chapter 14, these blocks span two files, so they have no Run button: the browser has no filesystem to put `rpn.þ` in. Save them next to each other and run them with the `thunky` command line.

```thunky-static
-- rpn.þ
import list, text in

module

isNumber = tok -> list.allMatch text.isDigit tok,

apply = tok -> [b, [a, rest]] ->
  [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ],

step = stack -> tok -> {
  1 -> [text.stringToInt tok, stack],
  0 -> apply tok stack
} (isNumber tok),

evalLine = line -> line > text.split " " > list.foldl step [] > list.head
```

```thunky-static
-- main.þ
import list, text, rpn in

let
  lines  = stdin > text.split [text.lf;] > list.filter (l -> gt 0 (list.length l)),
  render = line -> flatten [line; " = "; string (rpn.evalLine line)]
in
  lines > list.map render > list.map write > eval
```

```sh
printf '3 4 +\n3 4 + 2 *\n10 2 -\n20 4 /\n' | thunky main.þ
```

```text
3 4 + = 7
3 4 + 2 * = 14
10 2 - = 8
20 4 / = 5
```

Three things about this split are worth noting.

**The entry point is named `evalLine`, not `eval`.** `eval` is a builtin, and a module-level binding shadows it for every other binding in the module — exactly the trap Chapter 14 describes with `vec2.add`. Nothing in `rpn.þ` calls the builtin `eval` today, so it would work; `main.þ` does call it, and that call is fine because `rpn`'s bindings are not in `main`'s scope unless `rpn` is imported (which it is). Do not rely on that. Pick a name that does not collide.

**`main.þ` imports `list` and `text` for itself.** `rpn.þ` imports them too, but transitive imports are not re-exported — `main.þ` uses `list.map` and `text.split` in its own body, so it needs its own import clause.

**The module has no idea input exists.** `rpn.evalLine` takes a string and returns a number. It can be tested from a `show` at the top level, called from a web front end, or driven from a file — and `main.þ` is the only thing that has to change. That separation is worth more than the line count suggests.

---

## Where to go next

You have the whole language now. The tutorial ends here; the reference does not.

**[docs/LANGUAGE.md](../LANGUAGE.md)** is the complete language and library reference — grammar, evaluation semantics, every builtin, every standard library module. From this point it is your primary document.

The **`examples/`** directory in the repository holds programs written to be read, each pushing on something this tutorial only introduced:

- **`examples/examples.þ`** — a tour of the whole language in one runnable file: application, the four operators, pattern matching, recursion, laziness, the tuple/list distinction, strings, and a pass over the standard library. Every section prints what it computes, so the output read next to the source is the explanation. Start here if you want a single page to refresh from.

- **`examples/streams.þ`** — infinite lists as an everyday tool. A lazy prime sieve, self-referential `fibonacci` and `hamming` (the latter defined as the merge of its own multiples), Pascal's triangle and look-and-say via `iterate`, Newton's method as a stream terminated by a separate `within` predicate, and the Collatz record-holders as an infinite filter over an infinite map. The clearest demonstration that laziness plus memoization make self-reference an *efficient* construction, not just a legal one (Chapters 9 and 10).

- **`examples/wordfreq.þ`** — a word-frequency report over `stdin`, with a bar chart. Shows the input as what it is — a lazy list of code points that ordinary list functions consume — and refines the strictness lesson of `random-chisquare.þ`: `foldlStrict` forces the accumulator's spine, and a single `seq` on the count just updated forces the value, without the O(size of map) cost of forcing the entire map at every step.

- **`examples/dijkstra.þ`** — shortest paths with a purely functional priority queue. No mutable distance array and no decrease-key: stale entries are simply re-inserted and skipped when they surface. The settled nodes are produced as a lazy list in distance order, so `find` over it stops the search as soon as the target is reached.

- **`examples/huffman.þ`** — optimal prefix coding, where the `heap` *is* the algorithm: two pops and an insert per step. The tree carries no tag field — a leaf is a 1-tuple and an internal node a 2-tuple, so a two-case lambda tells them apart by arity, which is the usual way to encode a sum type when the only constructor is the tuple. Encodes, decodes, packs to bytes, and reports the compression.

- **`examples/sokoban.þ`** — an interactive terminal game (terminal only; it waits for input and redraws with ANSI escapes). The three things it demonstrates are interaction without an IO type — reading a line is `span (neq lf) stdin`, and forcing a cell that has not been typed yet blocks; ordering effects with `seq`, which is how the prompt is made to appear *before* the read; and undo for free, since a move builds a new board rather than changing one, so the undo stack is just a list of previous states.

- **`examples/life.þ`** — Conway's Life animated in the terminal (terminal only). The step function never indexes the grid: the 3x3 neighbourhood sum is a horizontal `zipWith3` of each row against its two rotations, then a vertical one of that grid against its rotations, which is nine additions per cell, no indexing, and a torus for free. It is also the clearest example of the language having no clock — the frame rate is a counted loop that computes nothing.

- **`examples/countdown.þ`** — a solver for the Countdown numbers game. It builds every legal expression tree over a set of source numbers, and it is fast because of laziness and sharing: subsets are processed smallest-first, each subset's results are bound to a name so the interpreter shares them, and every larger subset reads them back instead of rebuilding. It is the best worked example of memoization-by-sharing in the repository (Chapter 10).

- **`examples/sudoku.þ`** — constraint propagation. A board is a `hashmap` from cell index to value, peer lists are precomputed once at startup, and the search is a lazy list of solutions produced by `flatten`-ing recursive branches — so asking for the first solution never explores the rest of the tree.

- **`examples/random-chisquare.þ`** — a written-up case study in strictness. It tallies a million pseudorandom values into bins and explains, in comments, exactly why `foldlStrict` plus an `eval` inside the step function is required and why plain `foldl` falls over: the accumulator chain and the unforced counts inside the map are two separate space leaks. Read it alongside Chapter 11.

- **`examples/core_tests.þ`** — the compiler's own regression suite, written in Thunky. Useful as a broad usage catalogue for the standard library.

---

## Summary

- A stack machine is a `foldl`: the accumulator is the stack, the element is the token, the step function is the transition. `foldl` (not `foldr`) because the machine runs forwards.
- A stack is a cons list. `[x, stack]` pushes; the pattern `[b, [a, rest]]` pops two.
- A multi-case lambda over string patterns is a dispatch table.
- Threshold-first arithmetic reverses twice with a stack: `a b -` is `sub b a`. Verify the non-commutative operators.
- `text.split sep` produces empty tokens on repeated separators; `text.splitWith p` plus a non-empty filter is the robust tokeniser.
- `stdin` is a lazy list of code points, not a function. Every list function applies. It is one shared stream, so referring to it repeatedly reads the input once.
- Laziness covers input: you read exactly as far as you force, and no further.
- Split domain logic into a module that knows nothing about I/O, and keep the reading and writing in the program file.

---

## Exercises

### Exercise 14.1 — More operators

Add exponentiation `^` and remainder `%` to the calculator, so that `2 10 ^` is `1024` and `17 5 %` is `2`. Both `pow` and `mod` are threshold-first — check the argument order carefully before you trust the output.

<details>
<summary>Solution</summary>

```
import list, text in
let
  step = stack -> tok ->
    { 1 -> [text.stringToInt tok, stack],
      0 -> stack > ([b, [a, rest]] ->
             [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b,
                       "/" -> fdiv b a, "^" -> pow b a, "%" -> mod b a }, rest ])
    } (list.allMatch text.isDigit tok),
  rpn = s -> s > text.split " " > list.foldl step [] > list.head
in
  show [rpn "2 10 ^", rpn "17 5 %", rpn "3 4 + 2 ^", rpn "2 3 ^ 5 %"]
```

Output: `[1024, 2, 49, 3]`

Adding an operator is adding a case to the matcher; nothing else in the program moves. `pow a b` is `b ^ a` and `mod a b` is `b mod a`, so both take the same `b a` reversal as `-` and `/`: `pow b a` is `a ^ b`, and for `2 10 ^` that is `2 ^ 10 = 1024`. Had you written `pow a b` you would have got `10 ^ 2 = 100` — a plausible-looking wrong answer, which is why the test cases here use non-symmetric operands.

</details>

---

### Exercise 14.2 — Reject bad input

`rpn "3 4 + +"` crashes with a pattern match failure, and `rpn "3 4"` quietly returns the wrong element. Write `rpnSafe` that returns `maybe.some v` for a well-formed expression and `maybe.none` otherwise, *without* evaluating the bad ones.

An expression is well formed when every token is a number or a known operator, and the stack depth — which rises by one per number and falls by one per operator — is at least 1 after every token and exactly 1 at the end. `list.scanl` gives you the depth after each token in one pass.

<details>
<summary>Solution</summary>

```
import list, text, maybe, core in
let
  isNumber = tok -> list.allMatch text.isDigit tok,
  known    = tok -> core.or (isNumber tok) (list.contains tok ["+"; "-"; "*"; "/"]),

  step = stack -> tok ->
    { 1 -> [text.stringToInt tok, stack],
      0 -> stack > ([b, [a, rest]] ->
             [ tok > { "+" -> add a b, "-" -> sub b a, "*" -> mul a b, "/" -> fdiv b a }, rest ])
    } (isNumber tok),

  valid = toks ->
    let depths = list.scanl (d -> tok -> core.if (isNumber tok) (add 1 d) (sub 1 d)) 0 toks in
      core.and (list.allMatch known toks)
        (core.and (list.tail depths > list.allMatch (gte 1)) (eq 1 (list.last depths))),

  rpnSafe = s ->
    let toks = s > text.splitWith text.isSpace > list.filter (t -> gt 0 (list.length t)) in
      core.if (core.and (gt 0 (list.length toks)) (valid toks))
        (maybe.some (list.foldl step [] toks > list.head))
        maybe.none
in
  show [rpnSafe "10 2 - 3 *", rpnSafe "3 4 + +", rpnSafe "3 4", rpnSafe "3 4 x", rpnSafe ""]
```

Output: `[[24], [], [], [], []]`

`scanl` returns the initial `0` followed by the depth after each token, so `list.tail depths` is the depths that must all be `≥ 1` (`gte 1 d` is `d >= 1`) and `list.last depths` is the final depth. `3 4 + +` gives depths `[0; 1; 2; 1; 0]` — the trailing `0` fails; `3 4` gives `[0; 1; 2]` — the last is `2`, not `1`.

The evaluation never happens for a rejected expression, and not because of the branch ordering: `core.if` is an ordinary function, and its unchosen argument is a thunk that is simply never forced. Laziness is doing the guarding.

</details>

---

### Exercise 14.3 — A formatted report

You are given sales records as `"region,amount"` strings. Total the amounts per region, sort by total descending, and print an aligned two-column table — region left-aligned in a field of 8, total right-aligned in a field of 5.

```thunky-static
["north,120"; "south,80"; "north,45"; "east,200"; "south,15"; "east,30"]
```

Use `hashmap` for the tally and `list.foldlStrict` for the fold, as Chapter 11 recommends for accumulator loops.

<details>
<summary>Solution</summary>

```
import list, text, hashmap, core in
let
  rows = ["north,120"; "south,80"; "north,45"; "east,200"; "south,15"; "east,30"],

  parse = row -> text.split "," row > ([r; a] -> [r, text.stringToInt a]),

  totals = rows
    > list.map parse
    > list.foldlStrict (m -> [r, a] -> eval (hashmap.updateOr r (add a) a m)) hashmap.empty,

  line = [r, t] -> flatten [
    text.padRight 8 text.space r;
    text.padLeft 5 text.space (string t)
  ],

  report = totals > hashmap.keyValues > list.sortWith (core.on gt core.second) > list.map line
in
  report > list.map write > eval
```

Output:

```text
east      230
north     165
south      95
```

Watch the two bracket forms. `parse` matches the *list* `text.split` returns with `[r; a]` (a two-element list), then builds the *tuple* `[r, t]`; `line` destructures the tuple `hashmap.keyValues` hands back with `[r, t]`. Getting these confused is the single most common source of pattern match failures (Chapter 4).

`hashmap.updateOr r (add a) a m` adds `a` to the running total for `r`, or inserts `a` if this is the first record for that region. The `eval` inside the step function forces the accumulated total, not just the map spine — without it you build a chain of unforced `add` thunks, which is the leak `examples/random-chisquare.þ` documents at length.

</details>

---

### Exercise 14.4 — Word frequency from standard input

Write a filter that reads text on standard input and prints the three most frequent words with their counts, one per line, most frequent first. Split on whitespace runs, tally with `hashmap`, and format each line as `word: count`.

<details>
<summary>Solution</summary>

```thunky-static
import list, text, hashmap, core in
let
  words  = stdin > text.splitWith text.isSpace > list.filter (w -> gt 0 (list.length w)),
  counts = words > list.foldlStrict (m -> w -> eval (hashmap.updateOr w (add 1) 1 m)) hashmap.empty,
  report = counts > hashmap.keyValues > list.sortWith (core.on gt core.second) > list.take 3
             > list.map ([w, n] -> flatten [w; ": "; string n])
in report > list.map write > eval
```

```sh
echo "the cat sat on the mat the cat" | thunky wordfreq.þ
```

```text
the: 3
cat: 2
mat: 1
```

This is the whole chapter in six lines: `stdin` as a lazy list, a tokeniser that survives real whitespace, a strict fold into a `hashmap`, a comparator built with `core.on`, and `list.map write > eval` to emit the result.

`core.on gt core.second` compares two `[word, count]` pairs by their counts, and since `gt a b` is `b > a` the sort comes out descending. Words tied on count come out in hash order, which is arbitrary but stable for a given input — `sat` and `on` also have count 1 here, and `mat` won the third slot only by where it landed in the map.

There is no Run button on this one: with the empty standard input a documentation snippet gets, it correctly prints nothing at all. Save it and pipe something in.

</details>
