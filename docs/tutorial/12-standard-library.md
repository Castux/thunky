# Chapter 12: The Standard Library

Thunky ships with a standard library embedded in the binary — no installation needed. This chapter walks through each module with enough detail to use it effectively.

Import modules explicitly:

```thunky-static
import core, list, math in
  ...
```

Names from imported modules are available unqualified (just `map`, `sort`, etc.) and also qualified (`list.map`, `list.sort`). When two modules export the same name, the later import wins for unqualified access; use qualified names to disambiguate.

---

## `core` — combinators, booleans, and control flow

`core` is the foundational module. You will use it in nearly every program of any size.

### Identity and composition

```
import core in
show [
  core.id 42,                -- 42 (identity function)
  (core.compose (mul 2) (add 1)) 5,  -- mul 2 (add 1 5) = 12
  (core.flip sub) 10 3       -- sub 3 10 = 10 - 3 = 7
]
```

- `id x = x` — useful as a no-op placeholder in pipelines.
- `compose f g` — right-to-left composition; same as `<*`.
- `flip f x y = f y x` — reverses the first two arguments.

### `const` and `on`

```
import core, math in
let
  alwaysFive = core.const 5,
  compareByAbs = core.on lt math.abs    -- compare by absolute value
in
  show [alwaysFive 100, alwaysFive "anything", compareByAbs (math.negate 3) 2]
```

- `const c x = c` — ignores its second argument.
- `on f g x y = f (g x) (g y)` — applies `g` to both arguments before comparing with `f`.

### `ap` and `fork`

`on` runs *one* function over *two* arguments. `ap` and `fork` do the opposite: they
run *two* functions over the *same* argument and combine the results.

```
import core, list in
let mean = core.fork fdiv list.length list.sum in
show [
  core.ap add (mul 2) 5,    -- add 5 (mul 2 5) = 15
  mean [1; 2; 3; 4]         -- fdiv (length xs) (sum xs) = 10 / 4 = 2.5
]
```

- `fork h f g x = h (f x) (g x)` — apply `f` and `g` to `x`, combine with `h`.
- `ap f g x = f x (g x)` — the same, except `f x` *is* the combining function.

These two are the reason a pipeline can stay point-free when a value has to be
used twice, which `compose`, `flip` and `const` cannot manage on their own.
[Chapter 13](13-point-free-style.md) is about when that is worth doing — and, at
least as often, when it is not.

### Conditionals

```
import core, maybe in
show [
  core.if 1 "yes" "no",             -- "yes"
  core.if 0 "yes" "no",             -- "no"
  -- the first true branch, or the default
  maybe.when 0 10 > maybe.elseWhen 0 20 > maybe.elseWhen 1 30 > maybe.default 99
]
```

`core.if cond t f` — selects `t` if `cond = 1`, `f` if `cond = 0`. Runtime error for any other value.

For more than two branches, chain `maybe.when` / `maybe.elseWhen` / `maybe.default`:

```thunky-static
maybe.when cond1 val1 > maybe.elseWhen cond2 val2 > maybe.default default
```

Each condition is tested in turn and the first true one wins; conditions after it are never evaluated, and neither are the values of branches that lose. `when` builds a `maybe`, `elseWhen` fills it in if it is still `none`, and `default` unwraps it. There is no special syntax here — they are ordinary functions, so a chain is built and read like any other pipeline.

If a branch is the last one, drop the `default` and the result stays a `maybe`: a chain that matches nothing is `none`.

### Boolean logic

```
import core in
show [
  core.and 1 1,    -- 1
  core.and 1 0,    -- 0
  core.or  0 1,    -- 1
  core.not 1       -- 0
]
```

`and` and `or` are short-circuiting: if the first argument determines the result, the second is not evaluated. This is pure laziness — not a special instruction.

### Tuple accessors

```
import core in
show [core.first [10, 20], core.second [10, 20]]    -- [10, 20]
```

### `curry` and `uncurry`

```
import core in
let
  sumPair = core.uncurry add,    -- takes a 2-tuple instead of two args
  addCurried = core.curry ([x, y] -> add x y)
in
  show [sumPair [3, 4], addCurried 3 4]    -- [7, 7]
```

- `uncurry f [x, y] = f x y` — adapt a curried function to take a pair.
- `curry f x y = f [x, y]` — adapt a pair-taking function to be curried.

---

## `math` — numeric operations

`math` extends the arithmetic builtins with useful derived functions.

```
import math in
show [
  math.succ 5,        -- 6  (add 1)
  math.pred 5,        -- 4  (sub 1)
  math.abs (negate 7),-- 7
  math.max 3 7,       -- 7
  math.min 3 7,       -- 3
  math.clamp 0 10 15, -- 10 (clamped to upper bound)
  math.even 4,        -- 1
  math.odd 7,         -- 1
  math.gcd 18 48,     -- 6
  math.factorial 6    -- 720
]
```

`math.digits n` breaks a non-negative integer into its decimal digits as a list:

```
import math in show (math.digits 12345)    -- [1; 2; 3; 4; 5]
```

Rounding functions: `math.floor`, `math.ceil`, `math.trunc`, `math.round`.

---

## `maybe` — optional values

`maybe` gives you a type-safe way to represent "a value that might not be there", avoiding the need for sentinel values like `-1` or `null`.

A maybe value is either:
- `maybe.none` — absent (`[]`, the empty tuple)
- `maybe.some x` — present (`[x]`, a one-element tuple)

Since these are just tuples, you can pattern match on them directly:

```
import maybe in
let
  result = maybe.some 42
in
  show < { [] -> "nothing", [x] -> add x 1 } result    -- 43
```

### Constructors and predicates

```
import maybe in
show [
  maybe.none,             -- []
  maybe.some 99,          -- [99]
  maybe.isSome (maybe.some 1),  -- 1
  maybe.isNone maybe.none       -- 1
]
```

### Transforming maybe values

```
import maybe in
show [
  maybe.fmap (mul 2) (maybe.some 5),   -- [10]  (applied inside)
  maybe.fmap (mul 2) maybe.none,       -- []    (propagated)
  maybe.default 0 (maybe.some 42),     -- 42    (extract or default)
  maybe.default 0 maybe.none           -- 0
]
```

`fmap f m` — applies `f` to the value inside `some`, propagates `none`.

`default def m` — extracts the value from `some`, or returns `def` if `none`.

### Chaining: `andThen`

`andThen f m` applies `f` to the value in `some`, or propagates `none`:

```
import maybe in
let
  safeHead = { [] -> maybe.none, [h, t] -> maybe.some h },
  safeDiv = x -> y -> { 0 -> maybe.none, n -> maybe.some (fdiv n x) } y
in
  show [
    safeHead [10; 20; 30] > maybe.andThen (safeDiv 100),  -- [10]
    safeHead []           > maybe.andThen (safeDiv 100)   -- []
  ]
```

`andThen` is the maybe equivalent of "and then, if that succeeded, do this". It chains operations that might fail without nested if-checks.

### Where maybe values come from

Many standard library functions return maybes:

- `list.find p xs` — `some x` for the first match, or `none`
- `list.headSafe xs`, `list.tailSafe xs`, etc.
- `list.lookup key assoc` — `some value` or `none`
- `table.get key t` — `some value` or `none`

---

## `list` — comprehensive list processing

You have already seen most of `list` in Chapters 4 and 8. Here are the parts not yet covered.

### `flatMap`

`flatMap f xs` maps `f` over `xs` and concatenates the results:

```
import list in
show (list.flatMap (x -> [x; mul x 10]) [1; 2; 3])
```

Output: `[1; 10; 2; 20; 3; 30]`

Useful when each element produces a variable-length output.

### `mapFilter`

`mapFilter f xs` maps `f` over `xs`; `f` returns a `maybe`. Elements where `f` returns `none` are dropped; `some x` contributes `x`:

```
import list, maybe in
let safeRecip = { 0 -> maybe.none, x -> maybe.some (fdiv x 1) } in
  show (list.mapFilter safeRecip [1; 0; 2; 0; 4])
```

Output: `[1; 0.5; 0.25]` — zeros are dropped automatically.

### `scanl` and `scanr`

Running totals:

```
import list in
show [
  list.scanl add 0 [1; 2; 3; 4],    -- [0; 1; 3; 6; 10] (running sum)
  list.scanr add 0 [1; 2; 3; 4]     -- [10; 9; 7; 4; 0]
]
```

`scanl f z xs` produces the list of all intermediate `foldl` results. The first element is always `z`.

### `groupBy`

Groups consecutive elements with the same classifier:

```
import list in
show (list.groupBy equal [1; 1; 2; 2; 3; 1; 1])
```

Output: `[[1; 1]; [2; 2]; [3;]; [1; 1]]`

`groupBy f xs` groups consecutive runs where `f head element = 1`.

### `spans`, `tails`, `inits`

```
import list in
show [
  list.tails [1; 2; 3],    -- [[1; 2; 3]; [2; 3]; [3;]; []]
  list.inits [1; 2; 3]     -- [[]; [1;]; [1; 2]; [1; 2; 3]]
]
```

`tails xs` is all successive suffixes; `inits xs` is all successive prefixes.

### `partition`

`partition p xs` returns `[matching, rest]`:

```
import list in
show (list.partition (gt 3) [1; 2; 3; 4; 5])    -- [[4; 5], [1; 2; 3]]
```

Both halves are lists. This is more efficient than calling `filter` twice.

### Infinite lists

All the lazy list generators:

```
import list in
show [
  list.take 5 (list.upFrom 1),       -- [1; 2; 3; 4; 5]
  list.take 5 (list.downFrom 10),    -- [10; 9; 8; 7; 6]
  list.take 6 (list.cycle [1; 2; 3]),-- [1; 2; 3; 1; 2; 3]
  list.take 4 (list.repeat "x")      -- ["x"; "x"; "x"; "x"]
]
```

---

## `text` — string operations

Strings are lists of code points. `text` adds higher-level operations.

### Character literals

There are no escape sequences in Thunky strings. Use `text.char` and named constants:

```
import text in
show [
  text.char "A",    -- 65
  text.lf,          -- 10 (line feed)
  text.tab,         -- 9
  text.space        -- 32
]
```

`text.char "X"` extracts the code point of the first character — idiomatic for single character values.

### Formatting

```
import text in
write (text.join ", " ["one"; "two"; "three"])    -- one, two, three
```

`join sep strings` concatenates a list of strings with `sep` between them.

```
import text in
show [
  text.padLeft 5 (text.char "0") "42",    -- "00042"
  text.padRight 5 (text.char " ") "hi",   -- "hi   "
  text.trim "  hello  "                    -- "hello"
]
```

### Building a message with `format`

Assembling a line out of `flatten` and `string` gets noisy as soon as there is
more than one value in it. `format` takes a template instead: `%s` inserts a
value that is already a string, `%v` inserts any value rendered with `string`.

```
import text in
text.format "%s scored %v out of %v" ["Ada"; 449; 461] > write
```

Output:

```text
Ada scored 449 out of 461
```

Any `%` that does not begin `%s` or `%v` is an ordinary character, so `"5% of"`
needs nothing special; write `%%s` or `%%v` for those two characters literally.
Since string literals may span lines, one template can hold a whole block:

```
import text in
text.format "sample:  %s
%v characters, %v distinct" ["abracadabra"; 11; 5] > write
```

Output:

```text
sample:  abracadabra
11 characters, 5 distinct
```

### Printing lists and tables

A program that ends by printing several lines does not need to write them one at
a time. `writeList` prints one element per line, turning numbers into digits and
leaving strings alone:

```
import text in
text.writeList ["first"; "second"; 42]
```

Output:

```text
first
second
42
```

`writeTable` does the same for rows of cells, sizing each column to its widest
entry. Columns that hold only numbers are right-aligned:

```
import text in
text.writeTable [
  ["word"; "count"];
  ["the"; 1204];
  ["quick"; 37];
  ["extraordinarily"; 5]
]
```

Output:

```text
word             count
the              1204
quick            37
extraordinarily  5
```

Each row is a list of cells, written with semicolons — `["the"; 1204]`. As
Chapter 4 describes, `["x", "y"]` is the same value as the list `["x"; 121]`,
so the separator is what selects a row of two cells.

A column is right-aligned when every cell in it reads as a number. `"count"` is
text, so that column is left-aligned here; a table without the header row would
line its counts up on the units digit.

Both functions return their argument, so they fit in a pipeline. `text.tableLines`
is the pure counterpart of `writeTable`, returning the lines instead of printing
them, for a report assembled before it is written.

### Character classification

```
import text in
show [
  text.isDigit (text.char "5"),    -- 1
  text.isAlpha (text.char "a"),    -- 1
  text.isSpace (text.char " "),    -- 1
  text.toUpper (text.char "a")     -- 65 (code point for 'A')
]
```

### Parsing

```
import list, text in
show [
  text.stringToInt "123",                 -- 123
  text.stringToFloat "3.14",              -- 3.14
  text.split "," "a,b,c" > list.length,   -- 3 (the pieces are strings)
  text.startsWith "he" "hello"            -- 1
]
```

`split sep s` splits `s` on all occurrences of `sep`. `splitOne sep s` splits on the first occurrence only, returning a 2-element list.

### A practical example: CSV row parser

```
import text, list in
let parseCSV = row -> text.split "," row > list.map text.trim in
  parseCSV "  Alice , 30 , engineer " > list.map write > eval
```

Output:

```text
Alice
30
engineer
```

Note the `list.map write`: the fields are strings, and `show` would render each
as its list of code points. `write` prints them as text.

---

## `table` — association lists

A table is a list of `[key, value]` pairs. Keys are compared with structural equality.

```
import table in

let
  t = table.empty
    > table.set "name" "Alice"
    > table.set "age"  30
    > table.set "role" "engineer"
in
  show [
    table.get "name" t,              -- ["Alice"]
    table.getOr "unknown" "city" t,  -- "unknown"
    table.keys t,                    -- ["role"; "age"; "name"]
    table.containsKey "age" t        -- 1
  ]
```

`table.set k v t` upserts: removes all existing entries for `k`, then prepends `[k, v]`. Because tables are association lists (first match wins), prepending is the correct update strategy.

### Updating values

```
import table in
let t = table.fromList [["x", 10]; ["y", 20]] in
  show (table.update "x" (mul 3) t)
```

Output: `[["x", 30]; ["y", 20]]`

`update k f t` applies `f` to the value for key `k` if present; no-op if absent.

---

## `hashmap` — hash maps

`hashmap` provides an O(log n) key-value store backed by a binary search tree keyed on hash codes. It is the right tool when your dataset is large enough that the O(n) lookup in `table` becomes a bottleneck.

Under the hood, each key is reduced to a number by the builtin `hash` (which works on any value — numbers, strings, tuples). The BST is ordered by hash code; on a hash collision, `equal` resolves which key actually matches.

```
import hashmap in

let
  h = hashmap.empty
    > hashmap.set "name" "Alice"
    > hashmap.set "age"  30
    > hashmap.set "role" "engineer"
in
  show [
    hashmap.get "name" h,       -- ["Alice"]  (some)
    hashmap.getOr 0 "age" h,    -- 30
    hashmap.get "city" h        -- []          (none)
  ]
```

Results from `get` are `maybe` values — the same `[x]` / `[]` convention as the rest of the library.

### API

| Function | Description |
|----------|-------------|
| `empty` | the empty hashmap |
| `singleton k v` | single-entry hashmap |
| `fromList pairs` | build from `[[k, v]; ...]` list |
| `get k m` | `some v` if `k` is present, `none` otherwise |
| `getOr def k m` | value for `k`, or `def` if absent |
| `set k v m` | insert or replace the entry for `k` (O(log n)) |
| `remove k m` | remove the entry for `k` (O(log n)) |
| `update k f m` | apply `f` to the value at `k`; no-op if absent |
| `updateOr k f def m` | apply `f` if present; insert `def` if absent |
| `keys m` | all keys in hash order |
| `values m` | all values in hash order |
| `keyValues m` | all `[k, v]` pairs in hash order |

### Counting with `updateOr`

Tallying occurrences is the canonical hashmap pattern:

```
import hashmap, list, text in

let
  tally  = list.foldr (w -> m -> hashmap.updateOr w (add 1) 1 m) hashmap.empty,
  words  = text.split " " "the cat sat on the mat the cat sat"
in
  words > tally > hashmap.keyValues > show
```

`updateOr w (add 1) 1 m` means: if `w` is already in the map, add 1 to its count; otherwise insert it with count 1. `foldr` threads the map through each word from right to left.

### `hashmap` vs `table`

| | `table` | `hashmap` |
|-|---------|-----------|
| Lookup | O(n) | O(log n) |
| Key order | insertion order (last `set` wins) | hash order |
| Readable as literal | yes — just a list | no |
| Switch to when | small maps, ad-hoc data | many keys, performance matters |

Both modules use `equal` for key comparison so they support the same key types. For maps you construct inline and read once, `table` is simpler. For maps you update repeatedly in a loop, reach for `hashmap`.

---

## `comb` — combinatorics

```
import list, comb in
show [
  comb.choose 2 [1; 2; 3; 4],         -- all 2-element subsets
  comb.permutations [1; 2; 3],         -- all orderings
  comb.crossPairs [1; 2] [10; 20]      -- Cartesian product
]
```

`subsets l` — the power set (all subsets, shortest first).

These operations can produce exponentially many results, so use `take` if you only need a few.

---

## `heap` — priority queues

A leftist heap provides O(log n) insertion, merge, and pop.

```
import heap, list in

let
  h = [5; 2; 8; 1; 9; 3] > heap.fromList gt    -- min-heap (gt comparator)
in
  show [
    heap.top h,                    -- 1 (minimum)
    heap.toList gt h,              -- [1; 2; 3; 5; 8; 9]
    heap.sortAsc [5; 2; 8; 1; 9]  -- [1; 2; 5; 8; 9]
  ]
```

Comparator semantics: `cmp a b = 1` means `a` beats `b` (goes to root).
- `lt` → max-heap (largest at root)
- `gt` → min-heap (smallest at root)

Use `sortAsc` / `sortDesc` when you just want a sorted list without thinking about comparators.

---

## `bit`, `big`, `json` — the three specialist modules

The remaining three modules solve one problem each, and are worth knowing exist
even before you need them. All three are written in Thunky, with no builtin
added for any of them — which is the point: `bit` and `big` in particular are
built entirely out of the ordinary float64 arithmetic of Chapter 2.

**`bit`** — bitwise operations on 32-bit integers, for hashing, checksums, and
anything that thinks in words. Chapter 13 uses it without introduction; this is
that introduction.

```
import bit, text in
[
  string (bit.bAnd 12 10);      -- 8, from 1100 & 1010
  string (bit.bXor 12 10);      -- 6
  string (bit.shiftLeft 3 1);   -- 8; threshold-first, so this shifts 1 by 3
  string (bit.popCount 255);    -- 8
  bit.toHex 48879               -- 0000beef
] > text.writeList
```

**`big`** — arbitrary-precision integers and floats, for when 2⁵³ is not enough.
Values are opaque: build them with `intFromNumber` / `intFromString`, combine
them with `intAdd` and friends, and read them back with `intToString`.

```
import big in
big.intPow 100 (big.intFromNumber 2) > big.intToString > write
```

That is 2¹⁰⁰ exactly, all thirty-one digits of it. `big` also carries floats at
a chosen precision — `floatSqrt`, `floatDiv`, `floatToString` — which is how the
Project Euler solutions that need a thousand digits of a square root get them.

**`json`** — a parser and printer. A parsed value is a tagged 2-tuple —
`["number", n]`, `["object", pairs]`, and so on — so a JSON document is ordinary
Thunky data you can pattern-match on. `get`, `at` and `path` reach into it
without unpacking the tags by hand. Both `parse` and the accessors return
maybe values, since either can fail, which is why `maybe.value` appears twice
here.

```
import json, maybe, text in
let doc = json.parse '{"name": "Ada", "scores": [7, 9, 8]}' > maybe.value in
[
  json.path ["name";] doc > maybe.value > json.stringify;      -- "Ada"
  json.path ["scores"; 1] doc > maybe.value > json.stringify   -- 9
] > text.unlines > write
```

Note the single quotes around the document: Thunky strings have no escape
sequences, so a string holding double quotes is written with single ones.

The full interface of all three is in
[the language reference](../LANGUAGE.md#14-standard-library).

---

## Summary

| Module | What it provides |
|--------|-----------------|
| `core` | `id`, `flip`, `compose`, `const`, `on`, `ap`, `fork`, `if`, `and`, `or`, `not`, `curry`, `uncurry`, `fix` |
| `math` | `succ`, `pred`, `abs`, `max`, `min`, `clamp`, `even`, `odd`, `gcd`, `factorial`, `digits`, rounding |
| `maybe` | Optional values: `none`, `some`, `fmap`, `andThen`, `default`, `value`, `orElse`, `when`, `elseWhen` |
| `list` | Complete list library: construction, higher-order, folding, slicing, sorting, infinite lists |
| `text` | Strings: `char`, constants, formatting, classification, parsing |
| `table` | Association lists (O(n)): `get`, `set`, `update`, `keys`, `values`, `merge` |
| `hashmap` | Hash maps (O(log n)): `get`, `set`, `remove`, `update`, `updateOr`, `keys`, `values`, `keyValues` |
| `comb` | `choose`, `permutations`, `crossPairs`, `subsets` |
| `heap` | Priority queues: `insert`, `top`, `pop`, `sortAsc`, `sortDesc` |
| `bit` | 32-bit bitwise operations: `bAnd`, `bOr`, `bXor`, `shiftLeft`, `popCount`, `toHex` |
| `big` | Arbitrary precision: `intFromString`, `intAdd`, `intPow`, `intToString`, `floatSqrt` |
| `json` | `parse`, `stringify`, `get`, `at`, `path` over tagged 2-tuples |

---

## Exercises

### Exercise 12.1 — Maybe chaining

Write a function `safeSqrt` that returns `maybe.some (sqrt x)` if `x ≥ 0`, `maybe.none` otherwise. Then chain it with a function that multiplies by 10, and test on `[4, -1, 9, -4]`.

<details>
<summary>Solution</summary>

```
import maybe, list, math, core in
let
  safeSqrt = x -> core.if (gte 0 x) (maybe.some (sqrt x)) maybe.none
in
  list.map (x -> safeSqrt x > maybe.fmap (mul 10)) [4; math.negate 1; 9; math.negate 4] > show
```

Output: `[[20]; []; [30]; []]`

</details>

---

### Exercise 12.2 — Word frequency

Using `text.split`, `list.sortWith`, and `table`, write a program that counts the frequency of each word in the string `"the cat sat on the mat the cat sat"`. Display the word-count pairs sorted by count (descending).

<details>
<summary>Solution</summary>

```
import text, list, table, core in

let
  words = text.split " " "the cat sat on the mat the cat sat",
  count = list.foldr (w -> t -> table.updateOr w (add 1) 1 t) table.empty words,
  pairs = table.keys count > list.map (w -> [w, table.getOr 0 w count])
in
  show (list.sortWith (core.on gt core.second) pairs)
```

Output: something like `[["the", 3]; ["cat", 2]; ["sat", 2]; ["on", 1]; ["mat", 1]]`

Note: `core.on gt second` compares pairs by their second element (the count), descending.

</details>

---

### Exercise 12.3 — Parse and filter in one pass

The string `"12,,7,abc,30,4x,0"` is a comma-separated field list in which some fields are not numbers: one is empty, one is letters, one is a number with a letter stuck to it. Split it, and in a single pass keep only the fields that are entirely digits, converted to numbers. Then sum them.

Write a `toNumber` that returns a `maybe` — `maybe.some` for a valid field, `maybe.none` otherwise — and let `list.mapFilter` do the filtering. Note that `0` is a perfectly good value, so a sentinel like `-1` or `0` would not work here; the `maybe` is what makes "no value" distinct from "the value zero".

<details>
<summary>Solution</summary>

```
import list, text, maybe in
let
  toNumber = s -> {
      1 -> maybe.some (text.stringToInt s),
      0 -> maybe.none
    } (mul (gt 0 (list.length s)) (list.allMatch text.isDigit s)),
  fields = text.split "," "12,,7,abc,30,4x,0"
in
  show [fields > list.mapFilter toNumber, fields > list.mapFilter toNumber > list.sum]
```

Output: `[[12; 7; 30; 0], 49]`

A field is valid when it is non-empty (`gt 0 (list.length s)`, i.e. `length s > 0`) *and* every character is a digit; `mul` on two 0/1 values is logical and. The matcher `{ 1 -> …, 0 -> … }` is applied to that flag, which is why the validity test appears after the branches rather than before.

</details>

---

### Exercise 12.4 — Top-k without sorting

Write `topK k xs` returning the `k` largest elements of `xs` in descending order, using `heap` rather than `list.sort`. Test it with `topK 3 [5; 17; 2; 99; 4; 42; 8]`.

Remember the comparator convention: `cmp a b = 1` means `a` wins the root, so `lt` builds a *max*-heap.

<details>
<summary>Solution</summary>

```
import list, heap in
let topK = k -> xs -> xs > heap.fromList lt > (h -> list.take k (heap.toList lt h)) in
  show (topK 3 [5; 17; 2; 99; 4; 42; 8])
```

Output: `[99; 42; 17]`

`heap.toList` is lazy — it is defined as `[top h, toList cmp (pop cmp h)]`, one cons cell per pop — so `list.take k` performs exactly `k` pops and the remaining `n - k` elements are never ordered at all. With `lt` as the comparator the root is the largest element, so the pops come out descending and no reversal is needed.

</details>

---

### Exercise 12.5 — Number formatting

Using `text` and `list`, write a function `commaFormat` that takes a non-negative integer and formats it with commas every three digits: `1234567` → `"1,234,567"`.

Hint: `string n` gives you the digit characters as a list of code points. Use `list.mapIndex` to decide, for each character, whether a comma should follow it.

<details>
<summary>Solution</summary>

A comma follows position `i` when the number of characters remaining after it is non-zero and divisible by 3:

```
import list, text, core in

let
  commaFormat = n ->
    let
      s    = n > string,
      sLen = s > list.length,
      step = i -> c ->
        let after = sub (add i 1) sLen in   -- sLen - i - 1  (digits remaining)
          core.if (mul (gt 0 after) (eq 0 (mod 3 after)))
            [c; text.char ","]
            [c;]
    in
      s > list.mapIndex step > list.flatten
in
  commaFormat 1234567 > write
```

Output: `1,234,567`

`sub a b = b - a`, so `sub (add i 1) sLen = sLen - (i + 1)` = digits to the right of position `i`. A comma is inserted after digit `i` when that count is positive and divisible by 3.

</details>

---

### Exercise 12.6 — Aligned columns with `string`

`string n` renders a number as a string (a list of code points), which is what lets you feed numbers to the `text` formatting functions. Use it with `text.padRight` and `text.padLeft` to print `[["alice", 42]; ["bob", 7]; ["carolyn", 1234]]` as a two-column table: names left-aligned in a field of 8, scores right-aligned in a field of 5.

<details>
<summary>Solution</summary>

```
import list, text in
let
  row = [name, score] -> flatten [
    text.padRight 8 text.space name;
    text.padLeft 5 text.space (string score)
  ],
  rows = [["alice", 42]; ["bob", 7]; ["carolyn", 1234]]
in
  rows > list.map row > list.map write > eval
```

Output:

```text
alice      42
bob         7
carolyn  1234
```

Since a string *is* a list of code points, concatenating two padded fields is just `flatten` over a list of them. `list.map write` builds a lazy list of write actions and `eval` forces it, which is what actually performs the output.

</details>

---

### Exercise 12.7 — Word frequency with `hashmap`

Rewrite the word-frequency counter from Exercise 12.2 using `hashmap` instead of `table`. Then display the five most frequent words, sorted by frequency descending.

<details>
<summary>Solution</summary>

```
import hashmap, list, text, core in

let
  sentence = "the cat sat on the mat the cat sat on the mat and the cat",
  words    = sentence > text.split " ",
  counts   = words > list.foldr (w -> m -> hashmap.updateOr w (add 1) 1 m) hashmap.empty,
  pairs    = counts > hashmap.keyValues,
  top5     = pairs > list.sortWith (core.on gt core.second) > list.take 5
in
  top5 > show
```

Output: `[["the", 5]; ["cat", 3]; ["mat", 2]; ["sat", 2]; ["on", 2]]`

`hashmap.updateOr w (add 1) 1 m` increments `w`'s count if present, inserts 1 if not. `core.on gt core.second` compares pairs by their second element (the count) in descending order.

</details>

---

### Exercise 12.8 — Combinatorics: Pythagorean triples

Use `comb.choose 3` to enumerate every 3-element subset of `[1 … 20]`, then keep the ones `[a; b; c]` satisfying `a² + b² = c²`. Display the triples and how many there are.

`comb.choose k xs` yields subsets in the order the elements appear in `xs`, so within each subset `a < b < c` already — you do not need to sort or to consider permutations.

<details>
<summary>Solution</summary>

```
import list, comb in
let triples =
  comb.choose 3 (list.rangeIncl 1 20)
    > list.filter ([a; b; c] -> eq (add (mul a a) (mul b b)) (mul c c))
in
  show [triples, list.length triples]
```

Output: `[[[3; 4; 5]; [5; 12; 13]; [6; 8; 10]; [8; 15; 17]; [9; 12; 15]; [12; 16; 20]], 6]`

There are `C(20,3) = 1140` subsets, which is small enough to enumerate outright. `comb` counts grow explosively, though — `choose 3` of 200 elements is over a million subsets — so for larger inputs pair it with `list.take`, or narrow the search before you generate.

</details>
