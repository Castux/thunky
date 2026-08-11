# Thunky — Language Reference

Thunky (also written Þunky) is a minimalistic, purely functional language with lazy evaluation. A
whole program is a single expression, there is one primitive type and one
constructed type, functions are pure, and evaluation is lazy throughout.

## Contents

1. [Design principles](#1-design-principles)
2. [Running a program](#2-running-a-program)
3. [Programs and modules](#3-programs-and-modules)
4. [Lexical elements](#4-lexical-elements)
5. [Grammar](#5-grammar)
6. [Values and types](#6-values-and-types)
7. [Expressions](#7-expressions)
8. [Operators](#8-operators)
9. [Lambdas and pattern matching](#9-lambdas-and-pattern-matching)
10. [Bindings: `let`](#10-bindings-let)
11. [Lists, strings, and other sugar](#11-lists-strings-and-other-sugar)
12. [Lazy evaluation](#12-lazy-evaluation)
13. [Built-in functions](#13-built-in-functions)
14. [Standard library](#14-standard-library)

---

## 1. Design principles

- **A program is an expression.** There are no statements. Running a program
  evaluates its single body expression; output happens only through the
  side-effecting builtins `peek`, `show`, and `write`.
- **One primitive, one constructor.** The only primitive type is the number; the
  only way to build compound data is the tuple. Lists and strings are not
  separate types — they are conventions built on tuples (see
  [§11](#11-lists-strings-and-other-sugar)).
- **Purity.** Functions have no mutable state and no side effects beyond the
  output builtins. A value, once bound, never changes.
- **No variables, only bindings.** Names are bound to expressions by `let … in`
  or by lambda patterns; they are never reassigned.
- **Dynamic typing.** There are no type declarations and no static type checks.
  Passing the wrong shape of value to a builtin, or a value no clause matches, is
  a *run-time* error.
- **Pattern matching is fundamental.** Function application *is* pattern matching:
  every lambda matches its argument against a pattern, and a lambda with multiple
  cases chooses among them by matching.
- **Laziness.** Expressions are evaluated as late as possible and only as far as
  needed. This makes infinite and self-referential data structures ordinary tools
  (see [§12](#12-lazy-evaluation)).

## 2. Running a program

```sh
thunky <path>
```

Loads the file at `<path>`, loads any modules it imports, resolves and lowers
everything, and evaluates the program body on the G-machine. Anything the program
prints via `peek`, `show`, `write`, or `bwrite` appears on standard output. A
lexical, syntactic, or name-resolution error is reported with a located diagnostic
and the program does not run; a run-time error is reported with a source location
and a reduction trace.

**Module search order.** For each import `name`, the runtime looks for
`name.th` (or `name.þ`) in, in order: the **directory holding the program being
run**, then the **current working directory**, then the standard library
embedded in the binary (see [§14](#14-standard-library)). A program can
therefore be shipped with its helper modules beside it and run by path from
anywhere — `thunky some/where/prog.þ` resolves `import helper` to
`some/where/helper.th`.

A local file always shadows the built-in module of the same name, which
includes the program itself: a program stored as `json.þ` shadows the `json`
library for its own run.

In the browser there is no filesystem to search, so the embedded library is all
that resolves by default; the playground supplies a program's local modules to
the runtime explicitly, and those are searched first, in the same order.

There is **no automatic library import**. A program that wants the standard
library must say so explicitly:

```
import list in
  show (sum [1; 2; 3; 4])
```

## 3. Programs and modules

A *program* is an optional import clause followed by a single expression:

```thunky-static
import math, list, mymodule in
  <expression>
```

A *module* is a file named `<name>.th` (or `<name>.þ`) that begins with an optional import clause,
then the keyword `module`, then a comma-separated list of bindings. Every binding
in a module is public:

```thunky-static
import math in

module

double = x -> mul 2 x,
square = x -> mul x x
```

Imports make the names of the imported module's bindings available **unqualified**
in the importing unit. They can also be reached **qualified** as `module.name`,
but only for modules explicitly listed in the `import` clause — a module that was
loaded transitively (because another module imports it) is not in scope and cannot
be accessed by either form. When two imported modules export the same name, a later
import in the `import` clause shadows the earlier one's unqualified name; the
qualified form `module.name` always reaches the intended binding and so
disambiguates.

```thunky-static
import list, mod2 in
  show mod2.foo            -- qualified access; mod2 must be in the import clause
```

Modules may import one another, and **circular imports are permitted** — each
module is loaded exactly once regardless of import order, and because all module
bindings are created before any is evaluated, mutually recursive definitions across
modules resolve correctly.

The standard library modules (`list`, `math`, `text`, etc.) are ordinary modules
embedded in the binary; see [§14](#14-standard-library) for the full catalogue.

## 4. Lexical elements

Source must be UTF-8. Whitespace separates tokens but is otherwise insignificant.

- **Comments** start with `--` and run to the end of the line.
- **Identifiers** match `[a-zA-Z_][a-zA-Z0-9_]*`.
- **Keywords** are reserved and may not be used as identifiers: `let`, `in`,
  `module`, `import`.
- **Numbers** match `[0-9]+(\.[0-9]+)?`. There is a single numeric type; integer
  and fractional literals both denote it.
- **Strings** are enclosed in matching single quotes `'…'` or double quotes
  `"…"`. A string denotes a list of its Unicode code points (see
  [§11](#11-lists-strings-and-other-sugar)). A string cannot contain its own
  quote character.
- **Symbols**: `->`, `<*`, `*>`, `>`, `<`, `.`, `=`, `,`, `;`, `(`, `)`, `{`,
  `}`, `[`, `]`. The multi-character symbols are matched before their prefixes.

## 5. Grammar

The grammar is a [Parsing Expression Grammar](https://en.wikipedia.org/wiki/Parsing_expression_grammar).
Terminals `Name`, `Number`, and `String` are as defined in [§4](#4-lexical-elements).

**This grammar is the authoritative definition of the syntax.** The sections that
follow explain and illustrate it, but they introduce no syntax beyond what is
written here — there are no hidden forms in the prose.

```ebnf
Program := Import? Expr
Module  := Import? 'module' ListBinding

Import      := 'import' Name ( ',' Name )* 'in'
ListBinding := Binding ( ',' Binding )*
Binding     := Name '=' Expr

Expr   := Let | Lambda | Operation
Let    := 'let' ListBinding 'in' Expr
Lambda := Pattern '->' Expr

Pattern      := Name | Number | String | TuplePattern | ListPattern
TuplePattern := '[' ']' | '[' Pattern ( ',' Pattern )* ']'
ListPattern  := '[' Pattern ';' ']' | '[' Pattern ( ';' Pattern )+ ']'

Operation :=
    Operand ( '>'  Operand )* |
    Operand ( '<'  Operand )* |
    Operand ( '*>' Operand )* |
    Operand ( '<*' Operand )*
Operand     := Application | AtomicExpr
Application := AtomicExpr+

AtomicExpr := QualifiedName | Name | Number | String
            | Tuple | List | Lambda | '(' Expr ')'
QualifiedName := Name '.' Name

Tuple       := '[' ']' | '[' Expr ( ',' Expr )* ']'
List        := '[' Expr ';' ']' | '[' Expr ( ';' Expr )+ ']'
Lambda      := LambdaCase | '{' LambdaCase ( ',' LambdaCase )* '}'
LambdaCase  := Pattern '->' Expr
```

Notes:

- There is **no operator precedence among the four operators**, and they may not
  be mixed within a single chain. Use parentheses to combine different operators.
- Application binds tighter than any operator: in `f x > g`, `f x` is applied
  first.
- A `Pattern` is syntactically a restricted `Expr`. The parser parses the
  left-hand side of a `->` as an expression and then checks that it is a legal
  pattern; this is why tuple/list patterns and tuple/list expressions look
  identical.

## 6. Values and types

At run time a value is one of:

- a **number** — a signed value with a fractional part (internally a 64-bit
  float). `eq`, `lt`, and the arithmetic builtins operate on numbers;
- a **tuple** — an ordered, fixed-length sequence of values, written
  `[e1, e2, …]`. The empty tuple is `[]`. Tuples are the only compound type;
- a **function** — a lambda, builtin, or composition. Functions are first-class
  values.

### Truth values

There is no boolean type. By convention **0 is false and 1 is true**. The
comparison builtins (`eq`, `lt`) and the `core` module's logical functions return
`0` or `1`, and `core.if` matches its condition against exactly `1` or `0`
(any other value is a non-exhaustive-match error).

## 7. Expressions

### Atomic expressions

- **Number literal** — e.g. `42`, `3.14`.
- **String literal** — e.g. `"hi"`, denoting a list of code points.
- **Name** — a reference to a binding (from a `let`, a lambda pattern, an import,
  or a builtin). A name must be bound; an unbound name is a compile-time error.
- **Qualified name** — `module.name`, referring to a binding exported by an
  imported module.
- **Tuple** — `[e1, e2, …]`, a comma-separated sequence in brackets. `[]` is the
  empty tuple, `[e]` is a one-element tuple.
- **List** — `[e1; e2; …]`, a semicolon-separated sequence in brackets; sugar for
  nested tuples (see [§11](#11-lists-strings-and-other-sugar)). `[e;]` is a
  one-element list.
- **Lambda** (brace form) — `{ pattern -> body, … }`, a lambda with one or more
  cases written in braces (see [§9](#9-lambdas-and-pattern-matching)).
- **Parenthesized expression** — `( e )`, grouping only. Parentheses do **not**
  form tuples.

> **Brackets vs. parentheses.** `( … )` only groups. `[ … ]` builds tuples (with
> `,`), lists (with `;`), or the empty value `[]`. `{ … }` is the brace form of
> lambda.

### Application

Functions are applied by juxtaposition: `f x`. Application is left-associative,
so `f a b c` means `((f a) b) c`. Combined with currying (a function returning a
function), this gives multi-argument functions and partial application:

```
let add3 = x -> y -> z -> add x (add y z) in
  add3 10 20 30                     -- 60
```

```
let addFive = add 5 in addFive 10   -- 15  (partial application)
```

## 8. Operators

Thunky has four binary operators, all concerning function application or
composition. They come in two symmetrical pairs — *pipe* (which applies to a
value present in the chain) and *compose* (which builds a new function) — each
pair pointing in either direction.

| Operator | Name | `a OP b OP c` is | Direction |
|----------|------|------------------|-----------|
| `>` | pipe right | `c (b a)` | value flows left → right |
| `<` | pipe left | `a (b c)` | value flows right → left |
| `*>` | compose right | `\x -> c (b (a x))` | `a` runs first |
| `<*` | compose left | `\x -> a (b (c x))` | `c` runs first |

`>` (pipe right) threads a value through a sequence of functions and reads in
evaluation order; `<` (pipe left) is ordinary application written with the
function on the left, useful for removing parentheses; `*>` and `<*` build a new
function by composition without a value yet present, in the forward and
mathematical directions respectively.

None of the four may be mixed with another in the same chain without parentheses,
and application binds tighter than all of them. The pipe operators associate so
that the value flows through the chain in the named direction; the compose
operators associate to the right.

```
show (5 > add 1 > mul 2)          -- 12 : mul 2 (add 1 5)
```

```
show (mul 2 < add 1 < 5)          -- 12 : mul 2 (add 1 5)
```

```
show ((add 1 *> mul 2) 5)         -- 12 : add 1 first, then mul 2
```

```
show ((add 1 <* mul 2) 5)         -- 11 : mul 2 first, then add 1
```

## 9. Lambdas and pattern matching

### Lambda

A lambda is `pattern -> body`. All functions are anonymous; bind one to a name
with `let` if you want to refer to it.

```
let increment = x -> add x 1 in increment 10    -- 11
```

The body extends as far right as possible, so `x -> y -> e` is
`x -> (y -> e)`.

### Patterns

A pattern decides whether a value is accepted and what names it binds:

- **Name pattern** (`x`) — matches any value and binds it to `x`. **It does not
  force the value** (see [§12](#12-lazy-evaluation)).
- **Number pattern** (`0`, `42`) — matches only a number equal to it; this forces
  the argument to a number.
- **Tuple pattern** (`[a, b]`, `[]`) — matches a tuple of the same length, matching
  each element against the corresponding sub-pattern. Forces the argument only far
  enough to determine it is a tuple of that arity; the elements themselves stay
  unforced unless a nested pattern forces them.
- **List pattern** (`[a; b; c]`, `[a;]`) — matches a *proper list of exactly that
  length* (see [§11](#11-lists-strings-and-other-sugar)), matching each element
  against the corresponding sub-pattern.
- **String pattern** (`"hello"`, `""`) — matches a value that is exactly the list
  of the string's Unicode code points, and binds nothing. It is, in effect, a
  fixed-length list pattern whose elements are number patterns for the code
  points; `""` matches the empty list. Like a list pattern it forces only the
  list spine and the heads it compares.

Tuple and list patterns are **recursive**: their sub-patterns are themselves
patterns of any kind — names, numbers, or further tuple and list patterns — so
patterns nest to arbitrary depth and may mix matching with binding at each level.

```
([a, [b, c]] -> add a (add b c)) [1, [2, 3]]   -- 6; a tuple pattern nested inside a tuple pattern
```

```
{
  [0, n] -> n,                         -- a number pattern nested in a tuple pattern
  [m, n] -> add m n
} [0, 7]                               -- 7
```

```
([[p; q]; [r; s]] -> add p s) [[1; 2]; [3; 4]]   -- 5; list patterns nested inside a list pattern
```

(Recall that a list pattern matches the cons-cell structure of a proper list of
exactly that length, and that lists *are* tuples — so a tuple pattern can also
match a value written with list syntax. Clauses are tried in order, and the first
structural match wins.)

Names introduced by a pattern shadow outer bindings of the same name within the
body.

Applying a lambda to a value that does not match its pattern is a run-time error.

### Multiple-case lambda

A lambda can be written with braces to give it multiple cases, each with its own
pattern. The cases are comma-separated:

```thunky-static
{ pattern1 -> body1, pattern2 -> body2, … }
```

When applied, the argument is matched against each case's pattern in order; the
first case that matches is chosen and its body returned. If no case matches,
it is a run-time error.

```
{ 0 -> 1, 1 -> 0, n -> add n 100 } 7    -- 107; 0↦1, 1↦0, otherwise +100
```

```
{
  []     -> 0,                          -- empty tuple
  [a, b] -> add a b                     -- 2-tuple → sum
} [3, 4]                                -- 7
```

This is how `if`, list functions, and most of the standard library are written.

## 10. Bindings: `let`

```thunky-static
let name1 = expr1, name2 = expr2, … in body
```

`let` binds names within `body`. Bindings in one `let` are **mutually visible**:
each right-hand side may refer to any name in the group, including itself. Because
of laziness this enables recursion, mutual recursion, and self-referential data:

```
let
  a = 5,
  b = 6
in
  show (add a b)                       -- 11
```

```
let
  fact = { 1 -> 1, n -> mul n (fact (sub 1 n)) }
in
  show (fact 5)                        -- 120
```

Inner bindings shadow outer ones:

```
let a = 10 in
  let a = 20 in
    show a                             -- 20
```

A self-referential lazy structure is an ordinary definition:

```
import list in
let fibonacci = prepend [1; 1] (zipWith add fibonacci (tail fibonacci)) in
  show (take 10 fibonacci)             -- [1; 1; 2; 3; 5; 8; 13; 21; 34; 55]
```

## 11. Lists, strings, and other sugar

### Lists are tuples

A list is a convention layered on tuples:

- the **empty list** is the empty tuple `[]`;
- a **non-empty list** is a 2-tuple of head and tail: `[head, tail]` (a *cons
  cell*).

So a list of three elements `a`, `b`, `c` is `[a, [b, [c, []]]]`. The list
literal `[a; b; c]` is **syntactic sugar** for exactly that nesting:

```thunky-static
[a; b; c]   ≡   [a, [b, [c, []]]]
[a;]        ≡   [a, []]
[]          ≡   []                     -- the empty list and empty tuple coincide
```

Note the distinction:

- `[a]` is a **one-element tuple** — a single value in brackets.
- `[a;]` is a **one-element list** — i.e. `[a, []]`.

Because lists are just tuples, list patterns are sugar too: the list pattern
`[a; b]` matches the cons-cell structure of a proper two-element list, equivalent
to matching `[a, [b, []]]`. The standard library's list functions destructure
cons cells directly with 2-tuple patterns `[h, t]` and the empty pattern `[]`:

```
import math in
let length = {
  []      -> 0,
  [h, t]  -> succ (length t)
} in
  show (length [1; 2; 3])              -- 3
```

When a tuple has the shape of a list, output routines render it with list syntax
(`[1; 2; 3]`); otherwise they use tuple syntax (`[1, 2]`).

### Strings are lists of code points

A string literal denotes the list of its Unicode code points, so any list
function works on strings. The `write` builtin prints a list of code points as
characters.

```
import list in write "hello"           -- prints: hello
```

```
import list in write (prepend "ab" "cd")  -- prints: abcd
```

## 12. Lazy evaluation

Evaluation is lazy: an expression is reduced only when something needs its value,
and then only to the depth required. Reduction is forced by:

- **applying a function** — the function position of an application is reduced
  until it is known to be a function (a closure, builtin, or composition);
- **matching a number pattern** — the argument is reduced fully to a number and
  compared;
- **matching a tuple or list pattern** — the argument is reduced just far enough
  to learn it is a tuple of the right shape; the elements remain unreduced thunks
  until something forces them. Matching a *name* pattern forces nothing — it only
  binds;
- **calling an arithmetic or comparison builtin** — the numeric arguments are
  reduced to numbers and type-checked;
- **`eval` / `show` / `peek` / `write`** — `eval` and `show` force a value to
  *full* normal form (everything inside it); `peek` does the same but with output
  bounded in width and depth; `write` forces the spine and elements of a code-point
  list.

Every expression is **memoized**: once reduced to weak head normal form, the
result is shared, so it is never recomputed and further reduction resumes where it
stopped. This holds for *all* values — not just names bound by `let` or a pattern,
but also unnamed arguments, tuple fields, and the intermediate results of a pipe.
Together with mutual `let` bindings, memoization makes self-referential definitions
efficient — `fibonacci` above computes each element once.

A consequence for the output builtins: `peek`, `show`, `write`, and `bwrite`
return their argument, but because every value is computed at most once, threading
a value through several of them prints each one's view once — `xs > peek > show`
prints exactly two lines, not three.

A consequence of laziness is that infinite structures are fine as long as only a
finite part is forced:

```
import list in
  show (take 5 (upFrom 1))             -- [1; 2; 3; 4; 5]
```

## 13. Built-in functions

Builtins are the primitive operations available without importing anything;
everything else is defined in the standard library modules in Thunky itself.

The binary arithmetic and comparison builtins take their arguments in an order
chosen for **partial application and piping**: the *first* argument is the
right-hand operand. This makes `sub 1` mean "subtract one", `mod 10` mean "reduce
modulo ten", `div 2` mean "halve", and `lt 0` mean "is greater than zero". All
comparison builtins follow the same **threshold-first, value-second** convention:
`(lt 0)`, `(lte 0)`, `(gte 0)`, `(gt 0)` read naturally as predicates meaning
"is negative", "is non-positive", "is non-negative", and "is positive".

| Builtin | Arity | Result | Notes |
|---------|-------|--------|-------|
| `add a b` | 2 | `a + b` | |
| `mul a b` | 2 | `a * b` | |
| `sub a b` | 2 | `b - a` | argument order: `sub 1 x = x - 1` |
| `div a b` | 2 | `b / a` (integer) | truncating integer division |
| `fdiv a b` | 2 | `b / a` (real) | floating-point division |
| `mod a b` | 2 | `b mod a` (integer) | `mod 10 x = x mod 10` |
| `fmod a b` | 2 | `b mod a` (real) | floating-point remainder |
| `pow a b` | 2 | `b ^ a` (real) | `pow 2 x = x²` |
| `sqrt a` | 1 | `√a` | |
| `eq a b` | 2 | `1` if `a = b` else `0` | numbers only |
| `lt a b` | 2 | `1` if `b < a` else `0` | `lt 0 x = x < 0` |
| `lte a b` | 2 | `1` if `b ≤ a` else `0` | `lte 10 x = x ≤ 10` |
| `gte a b` | 2 | `1` if `b ≥ a` else `0` | `gte 0 x = x ≥ 0` |
| `gt a b` | 2 | `1` if `b > a` else `0` | `gt 0 x = x > 0` |
| `neq a b` | 2 | `1` if `a ≠ b` else `0` | numbers only |
| `equal a b` | 2 | `1` if `a` and `b` are structurally equal else `0` | works on any values; forces as needed; functions compare equal only by identity |
| `seq a b` | 2 | `b` | forces `a` to weak head normal form first; the tool for strictness (see `list.foldlStrict`) |
| `eval a` | 1 | `a`, forced to full normal form | identity otherwise; breaks laziness |
| `peek a` | 1 | `a` | prints `a` (width/depth-bounded), then returns it |
| `show a` | 1 | `a` | prints `a` (unbounded), then returns it |
| `write a` | 1 | `a` | prints `a` as text (list of code points), then returns it |
| `bwrite a` | 1 | `a` | prints `a` as raw bytes (list of integers 0–255), then returns it |
| `string a` | 1 | string representation of `a` | returns the same text `show` would print, as a list of code points |
| `hash a` | 1 | a number derived from the structure of `a` | used by the `hashmap` module; equal values always produce equal hashes; not cryptographic |
| `stdin` | — (a value) | standard input as a lazy list of Unicode code points | see below |
| `bstdin` | — (a value) | standard input as a lazy list of raw byte values | see below |

`peek`, `show`, `write`, and `bwrite` are the only ways a program produces
output; each returns its argument so it can be inserted into an expression.
`bwrite` is the binary counterpart to `write`: it treats each element as a raw
byte value (`0`–`255`) with no Unicode encoding. `string` is the pure counterpart
to `show`: it returns the same unbounded representation as a list of code points
instead of printing it, useful for building formatted output. `add`, `mul`, and
`eq` are commutative, so their argument order is immaterial.

Passing a non-number to an arithmetic builtin, or anything `write` cannot read as
a code-point list, is a run-time error.

`seq a b` is the strictness primitive: it forces `a` to weak head normal form and
returns `b` (unevaluated). It is the building block for strict accumulation — a
left fold that builds its accumulator lazily (`foldl`, `foldr`) holds the whole
chain unforced until the end, which for a long list is an *O(n)*-deep, *O(n)*-live
computation; `list.foldlStrict` uses `seq` to force the accumulator at each step and
so runs in constant depth and live space. Unlike `eval`, `seq` forces only to weak
head normal form (the outermost constructor), not all the way down.

`seq` is a builtin for speed, not for expressiveness: it can be written in Thunky
itself. Matching a constructor or literal pattern forces the subject to weak head
normal form in order to test it, and that happens even when a later catch-all case
is the one that matches — so a two-case lambda forces its argument and then ignores
what it found:

```
let
	mySeq = a -> b -> a > { 0 -> b, anything -> b },
	x = peek 42
in
	mySeq x 'forced x first' > write
```

This behaves exactly like `seq` for every value type, keeps `b` unforced, and makes
a strict fold stack-safe. It is only about 20% slower than the builtin (one extra
closure application and match per step), which is why `seq` stays a primitive.

### Comparator convention

The binary comparison builtins follow a **threshold-first** convention: the first argument is the reference value (the threshold), and the second is the value being tested. `lt 4` is a predicate meaning "is less than 4". The natural reading is with the pipe operator:

```
let x = 3 in
show [
  x > lt 4,      -- 1 : is x less than 4?
  x > gt 0,      -- 1 : is x positive?
  x > gte 100    -- 0 : is x at least 100?
]
```

Written as a direct call `lt 4 x` the order looks reversed, but the meaning is the same: `x < 4`. Think of the first argument as *fixing the threshold* and producing a predicate.

This convention makes partial application idiomatic:

```
import list in
let xs = [3; 8; 0; 12; 1] in
show [
  filter (lt 10) xs,        -- [3; 8; 0; 1]  : keep elements less than 10
  takeWhile (gt 0) xs,      -- [3; 8]        : take while positive
  sortWith lt xs            -- [0; 1; 3; 8; 12] : ascending, elements lt pivot go before it
]
```

The entire standard library is built around this convention. Functions that accept a comparator (like `heap.merge`, `heap.insert`) follow the same rule: `cmp a b = 1` means `a` beats `b`. With `cmp = lt`, larger values win (max-heap); with `cmp = gt`, smaller values win (min-heap).

### Standard input

`stdin` and `bstdin` are not functions but **values**: each is the standard input
presented as a list, in the usual cons-cell representation
([§11](#11-lists-strings-and-other-sugar)). They are **lazy** — input is read only
as far as the list is forced, and forcing a cell whose data has not yet arrived
**blocks** until it does. End of input is the empty list `[]`, so the ordinary
list functions work on them directly.

- `stdin` decodes the input as UTF-8 text into a list of Unicode code points. A
  byte sequence that is not valid UTF-8 is a run-time error. It is the natural
  input counterpart to `write`.
- `bstdin` ("binary stdin") is the input as a list of raw byte values (`0`–`255`),
  with no decoding.

Both draw from the same underlying input and denote a single shared stream:
every reference to `stdin` (or `bstdin`) sees the same once-read sequence, so the
same character is never delivered twice.

```
import list in
  write (take 5 stdin)        -- echo the first five characters of the input
```

## 14. Standard library

The standard library is a collection of Thunky modules **embedded in the
Thunky binary** — no separate installation is needed. Modules are written in
Thunky itself and are split by concern across twelve files in `core/`.

**Module search order.** For every `import name`, the runtime checks the
directory of the program being run, then the working directory, then the
embedded library ([§2](#2-running-a-program)). Placing your own `list.þ` (or any
other core module name) beside your program **shadows** the built-in one for
that run.

### Core modules

Each module can be imported directly when only part of the library is needed.

The tables below list each module's public interface. Thunky has no privacy
mechanism — every binding in a module is importable — so a few implementation
helpers are reachable but deliberately undocumented and not covered by any
stability promise: `heap.rankOf`, `heap.makeNode`, `hashmap.minNode`,
`hashmap.removeMin`, and `text.parsePositive`.

---

#### `core` — combinators, tuple accessors, and boolean logic

**Combinators**

| Name | Description |
|------|-------------|
| `id` | identity: `id x = x` |
| `compose f g` | right-to-left composition: `(compose f g) x = f (g x)` |
| `flip f` | flip first two arguments: `flip f x y = f y x` |
| `curry f` | `curry f x y = f [x, y]` |
| `uncurry f` | `uncurry f [x, y] = f x y` |
| `const c` | constant function: `const c x = c` |
| `on f g` | apply `g` to two arguments then `f`: `(on f g) x y = f (g x) (g y)` |
| `fix f` | fixed-point combinator: `fix f = f (fix f)` |
| `first [a, b]` | first element of a 2-tuple |
| `second [a, b]` | second element of a 2-tuple |

**Booleans / control flow**

| Name | Description |
|------|-------------|
| `true` | the constant `1` |
| `false` | the constant `0` |
| `if cond t f` | conditional; `cond` must be `0` or `1` |
| `case cond1 val1 cond2 val2 … else defval` | chained conditional: the value of the first condition equal to `1`, else `defval` |
| `else` | the constant that terminates a `case` chain (an ordinary value, not syntax) |
| `and a b` | logical and (short-circuits: if `a` is `0`, `b` is not evaluated) |
| `or a b` | logical or (short-circuits: if `a` is `1`, `b` is not evaluated) |
| `not b` | logical not |

---

#### `maybe` — optional values

A *maybe* value is either `none` (absent) or `some x` (present). The representation is a 0- or 1-element tuple.

**Construction and tests**

| Name | Description |
|------|-------------|
| `none` | the absent value, `[]` |
| `some x` | `x` wrapped as a present value, `[x]` |
| `isSome m` | `1` if `m` is `some x` |
| `isNone m` | `1` if `m` is `none` |
| `isMaybe m` | `1` if `m` is a valid maybe value (`none` or `some x`) |

**Transformation and extraction**

| Name | Description |
|------|-------------|
| `fmap f m` | apply `f` to the wrapped value; propagate `none` |
| `andThen f m` | apply `f` to the wrapped value and return its result; propagate `none`. `f` may return a maybe (chaining) or any other value (extract and map) |
| `orElse other m` | `m` if it is `some`, otherwise `other` |
| `value m` | the wrapped value; runtime error on `none` |
| `default def m` | the wrapped value, or `def` if `none` |
| `mapDefault def f m` | `f` applied to the wrapped value, or `def` if `none` |

`some` and `none` are plain values — `some` is sugar for `x -> [x]` and `none`
is `[]`. Because maybes are single-element tuples, tuple pattern matching works
directly: `[x] -> …` matches `some x` and `[] -> …` matches `none`.

```
import core, maybe in

let safeDivide = x -> y ->
  if (eq y 0)
    (maybe.none)
    (maybe.some (fdiv y x))       -- fdiv is threshold-first: fdiv y x is x / y
in
show [
  maybe.fmap (mul 2) (safeDivide 10 5),   -- [4]  (some 4)
  maybe.fmap (mul 2) (safeDivide 10 0),   -- []   (none)
  maybe.default 0 (safeDivide 10 0)       -- 0
]
```

---

#### `math` — numeric operations

The arithmetic and comparison primitives (`add`, `mul`, `sub`, `div`, `fdiv`,
`mod`, `fmod`, `pow`, `sqrt`, `eq`, `lt`, `lte`, `gte`, `gt`, `neq`) are
builtins, always in scope without an import — see [§13](#13-built-in-functions).
This module adds the derived operations.

| Name | Description |
|------|-------------|
| `succ` | `add 1` |
| `pred` | `sub 1` |
| `negate n` | arithmetic negation |
| `signum n` | sign of `n`: `-1`, `0`, or `1` |
| `abs n` | absolute value |
| `max a b` | larger of `a`, `b` |
| `min a b` | smaller of `a`, `b` |
| `clamp lo hi n` | clamp `n` to `[lo, hi]` |
| `trunc n` | round towards zero |
| `floor n` | round towards −∞ |
| `ceil n` | round towards +∞ |
| `round n` | round half-up |
| `isInteger n` | `1` if `n` has no fractional part |
| `even n` | `1` if `n mod 2 = 0` |
| `odd n` | `1` if `n mod 2 ≠ 0` |
| `gcd a b` | greatest common divisor |
| `lcm a b` | least common multiple |
| `factorial n` | `n!` for non-negative integer `n` |
| `digits n` | decimal digits of non-negative integer `n` as a list (`digits 123 = [1; 2; 3]`, `digits 0 = [0;]`) |

---

#### `list` — list operations and infinite lists

**Construction**

| Name | Description |
|------|-------------|
| `emptyList` | the empty list, `[]` |
| `cons h t` | a new cons cell with head `h` and tail `t` |
| `singleton x` | the one-element list `[x;]` |
| `replicate n x` | a list of `n` copies of `x` |
| `range a b` | `[a; a+1; …; b-1]` — half-open, `b` excluded |
| `rangeIncl a b` | `[a; a+1; …; b]` — both endpoints inclusive; `range a (succ b)` |

**Inspection**

| Name | Description |
|------|-------------|
| `isList x` | `1` if `x` is a proper list |
| `isEmpty l` | `1` if `l` is the empty list |
| `length l` | number of elements |
| `head l` | first element; crashes on `[]` |
| `tail l` | everything but the first element; crashes on `[]` |
| `last l` | last element; crashes on `[]` |
| `init l` | everything but the last element; crashes on `[]` |
| `nth n l` | element at zero-based index `n`; crashes if out of bounds |
| `contains e l` | `1` if `e` occurs in `l` (structural equality) |
| `maximum l` | largest element; crashes on `[]` |
| `minimum l` | smallest element; crashes on `[]` |
| `lookup key l` | search an association list `[[k1, v1]; …]`; `some v` for the first matching key, or `none` |

`head`, `tail`, `last`, and `init` crash on the empty list. Each has a safe
counterpart returning a `maybe` instead:

| Name | Description |
|------|-------------|
| `headSafe l` | `some h`, or `none` if `l` is empty |
| `tailSafe l` | `some t`, or `none` if `l` is empty |
| `lastSafe l` | `some x`, or `none` if `l` is empty |
| `initSafe l` | `some l'`, or `none` if `l` is empty |

**Modification**

| Name | Description |
|------|-------------|
| `prepend prefix xs` | `prefix` followed by `xs`; `prepend prefix` is a partial that prepends a fixed prefix |
| `append suffix xs` | `xs` followed by `suffix`; natural in a pipe: `xs > append suffix` |
| `reverse l` | reverse the order of elements |
| `intersperse sep l` | insert `sep` between every pair of elements |
| `intercalate sep ls` | flatten a list of lists with `sep` between each |
| `remove e l` | remove the first occurrence of `e` |
| `removeAll val l` | remove every occurrence of `val` |
| `replace old new l` | replace the first occurrence of `old` with `new` |
| `replaceAll old new l` | replace every occurrence of `old` with `new` |
| `replaceAt idx val l` | overwrite the element at zero-based index `idx` |
| `insertAt idx val l` | insert `val` before index `idx`, shifting the rest right |
| `removeAt idx l` | remove the element at zero-based index `idx` |
| `nub l` | remove duplicates, keeping the first occurrence of each value |

**Higher-order**

| Name | Description |
|------|-------------|
| `map f l` | apply `f` to every element |
| `mapIndex f l` | apply `f index element`, with zero-based indices |
| `filter p l` | keep the elements for which `p` returns `1` |
| `filterIndex p l` | keep the elements for which `p index element` returns `1` |
| `mapFilter f l` | apply `f` (returning a `maybe`) to each element, dropping `none` and unwrapping `some` — a transformation and a filter in one pass |
| `flatMap f l` | map `f` over `l`, then flatten the resulting list of lists |
| `find p l` | `some x` for the first element satisfying `p`, or `none` |
| `findIndex p l` | `some i` for the zero-based index of the first element satisfying `p`, or `none` |
| `foldr f acc l` | right-associative reduction: `f l1 (f l2 (… (f ln acc)))` |
| `foldl f acc l` | left-associative reduction: `f (… (f (f acc l1) l2) …) ln` |
| `foldlStrict f acc l` | `foldl` that forces the accumulator at each step (via `seq`), so a long list runs in constant depth and live space — see [§13](#13-built-in-functions) |
| `scanl f acc l` | successive left-fold results, starting with `acc`: `[z; f z a; f (f z a) b; …]` |
| `scanr f acc l` | successive right-fold results, ending with `acc` |

**Aggregation**

| Name | Description |
|------|-------------|
| `sum l` | sum of a list of numbers (`0` for `[]`) |
| `product l` | product of a list of numbers (`1` for `[]`) |
| `orList l` | logical OR over a list of booleans |
| `andList l` | logical AND over a list of booleans |
| `anyMatch p l` | `1` if `p` holds for at least one element |
| `allMatch p l` | `1` if `p` holds for every element (`1` for `[]`) |
| `noneMatch p l` | `1` if `p` holds for no element (`1` for `[]`) |
| `count p l` | number of elements satisfying `p` |

**Slicing and splitting**

| Name | Description |
|------|-------------|
| `take n l` | the first `n` elements |
| `drop n l` | everything after the first `n` elements |
| `slice a b l` | the elements between indices `a` and `b`, `b` excluded |
| `takeWhile p l` | the longest prefix for which `p` holds |
| `dropWhile p l` | what remains after that prefix |
| `span p l` | `[prefix for which p holds, rest]` |
| `splitAt n l` | `[first n elements, rest]` — `span` by index |
| `splitEqual n l` | split `l` into consecutive sublists of length `n` (the last may be shorter) |
| `tails l` | all successive suffixes: `[l; tail l; …; []]` |
| `inits l` | all successive prefixes: `[[]; …; init l; l]` |
| `spans l` | `inits` zipped with `tails` — `[prefix, suffix]` at every split point; used to search for substrings |
| `groupBy f l` | split into runs of consecutive elements where `f head elem = 1`; `groupBy equal` groups equal neighbours |
| `partition p l` | `[matching, rest]` |
| `flatten l` | concatenate a list of lists into one list |

**Zipping and reshaping**

| Name | Description |
|------|-------------|
| `zip a b` | pair up elements: `[[a1, b1]; [a2, b2]; …]`, stopping at the shorter |
| `zipWith f a b` | combine two lists element-wise with `f` |
| `zip3 a b c` | as `zip`, for three lists |
| `zipWith3 f a b c` | as `zipWith`, for three lists |
| `unzip l` | turn `[[a1, b1]; …]` into `[[a1; …], [b1; …]]` |
| `transpose rows` | turn a list of rows into a list of columns; stops at the shortest row |

**Sorting**

| Name | Description |
|------|-------------|
| `sortWith f l` | quicksort; `f pivot elem = 1` if `elem` belongs before `pivot` |
| `sort l` | ascending sort using the builtin `lt`; numbers only — for strings use `text.sortStrings` |

**Infinite lists**

Each of these produces a lazy, unbounded list; consume it with `take`,
`takeWhile`, or a pattern match.

| Name | Description |
|------|-------------|
| `iterate f x` | `[x; f x; f (f x); …]` |
| `upFrom n` | `[n; n+1; n+2; …]` |
| `downFrom n` | `[n; n-1; n-2; …]` |
| `repeat x` | `[x; x; x; …]` |
| `cycle l` | `l` repeated end to end, forever |
| `times n f x` | *not* a list: apply `f` to `x` exactly `n` times (the `n`th element of `iterate f x`) |

```
import list in
show [
  list.take 8 (list.upFrom 1),         -- [1; 2; 3; 4; 5; 6; 7; 8]
  list.sort [3; 1; 4; 1; 5],           -- [1; 1; 3; 4; 5]
  list.take 6 (list.cycle [1; 2; 3])   -- [1; 2; 3; 1; 2; 3]
]
```

---

#### `comb` — combinatorics

| Name | Description |
|------|-------------|
| `subsets l` | power set — all subsets of `l` (including `[]`, shortest first) |
| `subsetsWithRest n l` | all ways to pick `n` elements; each result is `[chosen, rest]` |
| `choose n l` | all `n`-element subsets of `l` (= `map first (subsetsWithRest n l)`) |
| `pairs l` | all ordered pairs `[a, b]` where `a` appears before `b` (= `choose 2`) |
| `crossPairs a b` | Cartesian product — all pairs `[x, y]` with `x` from `a`, `y` from `b` |
| `permutations l` | all permutations of `l` |

```
import list, comb in
show [
  comb.choose 2 [1; 2; 3],              -- [[1; 2]; [1; 3]; [2; 3]]
  comb.permutations [1; 2; 3],          -- [[1; 2; 3]; [1; 3; 2]; [2; 1; 3]; …]
  comb.crossPairs [1;2] [3;4],          -- [[1; 3]; [1; 4]; [2; 3]; [2; 4]]
  comb.subsets [1; 2]                   -- [[]; [2;]; [1;]; [1; 2]]
]
```

---

#### `heap` — purely functional priority queues (leftist heaps)

Provides O(log n) insertion, merging, and popping.

**Comparator semantics:** `cmp a b = 1` means `a` beats `b` (goes to root).
- `cmp = lt` → **max-heap** (largest at root): `lt a b = 1` when `b < a`, so `a` wins when `a > b`.
- `cmp = gt` → **min-heap** (smallest at root).

Use `sortAsc` / `sortDesc` for sorting without reasoning about the comparator.

| Name | Description |
|------|-------------|
| `emptyHeap` | the empty heap |
| `isEmpty h` | `1` if `h` is empty |
| `singleton x` | a heap containing only `x` |
| `merge cmp h1 h2` | merge two heaps (O(log n)) |
| `insert cmp x h` | insert `x` into `h` (O(log n)) |
| `top h` | return the root element; crashes on empty heap |
| `topSafe h` | return `some` root or `none` if empty |
| `pop cmp h` | remove the root and return the new heap; crashes on empty (O(log n)) |
| `popSafe cmp h` | return `some` (new heap) or `none` if empty |
| `fromList cmp l` | build a heap from a list |
| `toList cmp h` | extract elements in comparator order (largest-first for `lt`, smallest-first for `gt`) |
| `sortBy cmp l` | heapsort using the provided comparator |
| `sortAsc l` | sort ascending (smallest first); equivalent to `list.sort` |
| `sortDesc l` | sort descending (largest first) |

---

#### `table` — association lists

A *table* is a list of `[key, value]` pairs. Keys are compared with `equal` (structural equality). All key-based operations use the **first match**, so prepending a new entry for a key naturally shadows any existing entry — this is the standard association-list model.

Because a table is just a list, all `list` functions work on it directly. This module adds the semantic wrappers.

| Name | Description |
|------|-------------|
| `empty` | the empty table |
| `singleton k v` | single-entry table |
| `fromList pairs` | treat a list of `[k, v]` pairs as a table (identity; documents intent) |
| `toList t` | convert a table to a list of `[k, v]` pairs (identity) |
| `get k t` | `some v` for the first entry with key `k`, or `none` |
| `getOr def k t` | value for key `k`, or `def` if absent |
| `containsKey k t` | `1` if any entry has key `k` |
| `containsValue v t` | `1` if any entry has value `v` |
| `set k v t` | upsert: remove all entries for `k`, prepend `[k, v]`; O(n) |
| `remove k t` | remove all entries for key `k` |
| `update k f t` | apply `f` to the value for `k` if present; no-op if absent |
| `updateOr k f def t` | apply `f` if `k` is present; set `k` to `def` if absent |
| `keys t` | list of all keys, in order |
| `values t` | list of all values, in order |
| `mapValues f t` | apply `f` to each value, leaving keys unchanged |
| `filterByKey p t` | keep only entries where `p k` holds |
| `filterByValue p t` | keep only entries where `p v` holds |
| `merge t1 t2` | left-biased union: for duplicate keys, `t1`'s entry is found first |

```
import table in
let t = table.empty
      > table.set "x" 10
      > table.set "y" 20
in
show [table.get "x" t; table.getOr 0 "z" t]   -- [[10]; 0]
```

> **Syntax note.** A table literal with a single entry must use list notation: `[["key", val];]` (semicolon). `[["key", val]]` is a 1-element *tuple*, not a list, and will fail at runtime when any table function tries to traverse it.

---

#### `hashmap` — hash maps

A *hashmap* is a binary search tree keyed by hash codes, providing O(log n) insertion, lookup, and removal. Keys are hashed with the builtin `hash` and compared for equality with `equal`. Key enumeration order is hash order (neither insertion order nor sort order).

Use `hashmap` instead of `table` when the number of entries is large enough that O(n) traversal matters. For small maps and readable literals, `table` is simpler.

| Name | Description |
|------|-------------|
| `empty` | the empty hashmap |
| `singleton k v` | single-entry hashmap |
| `fromList pairs` | build a hashmap from a list of `[k, v]` pairs |
| `get k m` | `some v` if `k` is present, `none` otherwise; O(log n) |
| `getOr def k m` | value for `k`, or `def` if absent; O(log n) |
| `set k v m` | insert or replace the entry for `k`; O(log n) |
| `remove k m` | remove the entry for `k`; O(log n) |
| `update k f m` | apply `f` to the value at `k`; no-op if absent; O(log n) |
| `updateOr k f def m` | apply `f` if `k` is present; insert `def` if absent; O(log n) |
| `keys m` | all keys in hash order |
| `values m` | all values in hash order |
| `keyValues m` | all `[k, v]` pairs in hash order |

```
import hashmap in
let m = hashmap.empty
      > hashmap.set "x" 10
      > hashmap.set "y" 20
in
show [hashmap.get "x" m; hashmap.getOr 0 "z" m]   -- [[10]; 0]
```

---

#### `bit` — bitwise operations on 32-bit integers

Built from the existing arithmetic; there are no bitwise builtins. A 32-bit
value and the sum of two of them are exactly representable in the float64 a
Thunky number is, so the results are exact. Every operation takes and returns a
value in `[0, 2^32)`; use `wrap` to bring anything else into range.

The names are prefixed with `b` because an unqualified import would otherwise
put `and`, `or` and `not` beside `core`'s logical operations of the same name.

| Name | Description |
|------|-------------|
| `width`, `size`, `maxValue` | `32`, `2^32`, `2^32 - 1` |
| `wrap n` | bring any integer into `[0, 2^32)` by two's-complement wrapping |
| `bAnd a b`, `bOr a b`, `bXor a b` | the binary logical operations |
| `bNot n` | one's complement within 32 bits |
| `shiftLeft k n`, `shiftRight k n` | shift by `k`; left shifts drop bits past bit 31 |
| `rotateLeft k n`, `rotateRight k n` | rotate by `k` |
| `bitAt k n`, `testBit k n` | bit `k` (0 is least significant) as `0` or `1` |
| `setBit k n`, `clearBit k n`, `flipBit k n` | single-bit edits |
| `popCount n` | how many bits are set |
| `toBits n`, `fromBits bits` | to and from a list of 32 bits, least significant first |
| `toHex n` | eight lowercase hex digits |

```
import bit in
show [bit.bXor 12 10, bit.popCount 255, bit.toHex 3735928559]
```

---

#### `big` — arbitrary-precision integers and floats

A Thunky number is a float64: exact for integers to 2^53, and 53 bits of
significand. This module lifts both limits.

An **integer** is `[sign, magnitude]`, the magnitude a little-endian list of
32-bit limbs; zero is `[1, []]`. A **float** is `[mantissa, exponent,
precision]`, meaning `mantissa * 2^exponent` held to `precision` significand
bits — precision travels with the value, so a computation inherits it instead of
consulting a global setting. Results are truncated toward zero.

The non-commutative operations put the operand first and the value last —
`intSub k n` is `n - k`, `intMod d n` is `n mod d`, `intPow e b` is `b^e` — for
the same reason the `sub`, `div`, `mod` and `pow` builtins do: it is the order
that makes partial application and piping work. `n > intMod m` reduces n; had
the library used mathematical order it would silently have reduced m by n.

Base 2^32 is the largest that keeps a limb, and the sum of two limbs, exactly
representable. Multiplication splits limbs into 16-bit halves, whose products
stay under 2^32, because a full limb product would reach 2^64.

Division is Knuth's Algorithm D: the dividend is consumed a limb at a time into
a running remainder, and each quotient limb is estimated from the top two limbs
of the remainder over the top limb of the divisor, then corrected downward.
Normalising both operands so the divisor's top limb has its high bit set — which
cannot change the quotient — bounds that correction at two steps, which is what
makes the estimate safe rather than hopeful.

| Name | Description |
|------|-------------|
| `intZero`, `intOne` | constants |
| `intFromNumber n`, `intFromString s` | conversion in; exact below 2^53 for a number |
| `intToString n`, `intToNumber n`, `intDigits n` | conversion out (`intDigits` is decimal, most significant first) |
| `intAdd a b`, `intMul a b` | arithmetic |
| `intSub k n` | `n - k` |
| `intMulSmall k n` | multiply by a plain number, cheaper than a full multiply |
| `intDivMod d n`, `intDiv d n`, `intMod d n` | truncating division of `n` by `d` (schoolbook, one pass per limb); the remainder takes the sign of `n` |
| `intPow e b` | `b^e` by squaring, non-negative `e` |
| `intPowMod m e b` | `b^e mod m` for a plain-number `e`, reducing at every step so the operands stay the size of `m` |
| `intPowModBig m e b` | the same with `e` an arbitrary-precision integer — a plain `e` cannot exceed 2^53 |
| `intNegate n`, `intAbs n`, `intSign n`, `intIsZero n` | sign handling |
| `intCompare a b` | `-1`, `0` or `1` as `a` is less than, equal to, or greater than `b` |
| `intLt t n`, `intLte t n`, `intGt t n`, `intGte t n` | threshold-first predicates, like the builtins |
| `floatFromNumber p n`, `floatFromInt p n` | build at precision `p` (bits) |
| `floatWithPrecision p x` | the same value carried at another precision |
| `floatAdd a b`, `floatMul a b` | arithmetic; the result takes the larger precision |
| `floatSub k x`, `floatDiv d x` | `x - k` and `x / d` |
| `floatNegate x`, `floatIsZero x`, `floatCompare a b` | as for integers |
| `floatLt t x`, `floatLte t x`, `floatGt t x`, `floatGte t x` | threshold-first predicates |
| `floatSqrt x` | Newton's method to the working precision |
| `floatToString digits x` | decimal rendering with that many places, truncated |

```
import big in
[
  big.intToString (big.intPow (big.intFromNumber 2) 100);
  big.floatToString 20 (big.floatSqrt (big.floatFromNumber 200 2))
] > map write > eval
```

---

#### `json` — JSON parsing and printing

JSON's types do not map onto Thunky's without ambiguity — a JSON string is a
list of code points and so is an array of numbers — so a value is a tagged
2-tuple:

```thunky-static
["null",   0]                  ["string", code points]
["bool",   0 or 1]             ["array",  list of values]
["number", n]                  ["object", list of [key, value] pairs]
```

The tags are strings so that a consumer can match them literally, which is the
point of the shape:

```thunky-static
total = {
  ["number", n]      -> n,
  ["array", items]   -> items > map total > sum,
  ["object", pairs]  -> pairs > map (second *> total) > sum,
  other              -> 0
}
```

Objects keep their pairs in document order, so printing a parsed document gives
it back unchanged; `get` is therefore O(n) in the number of keys, and `toTable`
hands the pairs to the `table` module.

| Name | Description |
|------|-------------|
| `parse text` | `some value`, or `none` if the document is not valid JSON |
| `stringify value` | compact text |
| `stringifyPretty indent value` | one member or element per line, indented |
| `jsonNull`, `jsonTrue`, `jsonFalse` | constants |
| `jsonBool b`, `jsonNumber n`, `jsonString s`, `jsonArray items`, `jsonObject pairs` | constructors |
| `typeOf value` | `"null"`, `"bool"`, `"number"`, `"string"`, `"array"` or `"object"` |
| `isNull value` | |
| `get key value` | an object member, or `none` |
| `at index value` | an array element, or `none` |
| `path steps value` | follow a chain of keys (strings) and indices (numbers) |
| `asNumber`, `asString`, `asBool`, `asArray`, `asObject` | extract, or `none` on a type mismatch |
| `toTable value` | an object's pairs as a `table` |

`parse` reports failure as `none` without a location: locating an error would
mean threading a position through every parser, which is deliberately not done
yet.

```
import json, maybe in
json.parse '{"a":[1,2]}' > maybe.value > json.stringifyPretty 2 > write
```

---

#### `text` — string formatting, parsing, and value rendering

Strings are lists of Unicode code points, so all `list` functions work on them;
this module adds higher-level formatting and parsing.

**Characters**

Thunky string literals have no escape sequences, so characters that would
normally be written as `\t`, `\n`, etc. must be expressed as integers.

| Name | Description |
|------|-------------|
| `char s` | the code point of the first character of `s` — an alias for `head`, and the idiomatic character literal: `char 'A'` = `65`, `char '0'` = `48` |

The named constants cover the characters that cannot be written literally:

| Constant | Value | Usual escape |
|----------|-------|--------------|
| `nul` | 0 | `\0` |
| `tab` | 9 | `\t` |
| `lf` | 10 | `\n` |
| `cr` | 13 | `\r` |
| `esc` | 27 | `\e` |
| `space` | 32 | |
| `dquote` | 34 | `\"` |
| `quote` | 39 | `\'` (string delimiter — cannot appear in a literal) |
| `backslash` | 92 | `\\` |

**Formatting**

| Name | Description |
|------|-------------|
| `join sep strings` | intercalate `sep` between each string in the list |
| `lines s` | split on line feeds, treating a preceding carriage return as part of the same break (so `"\n"` and `"\r\n"` both end a line, a lone `"\r"` is left alone); a single trailing newline does not add an empty last line, so `lines ""` is `[]` |
| `unlines ls` | join with line feeds, without a trailing one — `unlines (lines s)` returns `s` when `s` does not end in a newline |
| `words s` | split on runs of whitespace, discarding the empty pieces around them |
| `unwords ws` | join with single spaces |
| `trim s` | remove leading/trailing whitespace |
| `padLeft n fill s` | left-pad `s` with code point `fill` to minimum width `n` |
| `padRight n fill s` | right-pad `s` with code point `fill` to minimum width `n` |

**Character classification and conversion**

All functions below take a Unicode code point (an integer, as found in a Thunky string).

| Name | Description |
|------|-------------|
| `isDigit c` | `1` if `c` is `'0'`–`'9'` |
| `isLower c` | `1` if `c` is `'a'`–`'z'` |
| `isUpper c` | `1` if `c` is `'A'`–`'Z'` |
| `isAlpha c` | `1` if `c` is a letter |
| `isAlphaNum c` | `1` if `c` is a letter or digit |
| `isSpace c` | `1` if `c` is `space`, `tab`, `lf`, or `cr` |
| `toLower c` | lowercase the character; non-uppercase returned unchanged |
| `toUpper c` | uppercase the character; non-lowercase returned unchanged |
| `stringToLower s` | lowercase a whole string |
| `stringToUpper s` | uppercase a whole string |
| `digitToInt c` | digit character → integer (`char '5'` → `5`) |
| `intToDigit n` | integer → digit character (`5` → `char '5'`) |

**Parsing**

| Name | Description |
|------|-------------|
| `stringToInt s` | parse a decimal integer string (optional leading `'-'`) |
| `stringToFloat s` | parse a decimal float string (optional `'-'` and `'.'`) |
| `startsWith pref s` | `1` if `s` begins with `pref` |
| `endsWith suff s` | `1` if `s` ends with `suff` |
| `containsSub sub s` | `1` if `sub` appears anywhere in `s` |
| `splitOne sep s` | split `s` on the first occurrence of `sep`; returns `[before; after]`, or `[s;]` if not found |
| `split sep s` | split `s` on all occurrences of `sep`; returns a list of strings |
| `replaceSub old new s` | replace the first occurrence of `old` with `new` in `s` |
| `replaceAllSub old new s` | replace every occurrence of `old` with `new` in `s` |
| `splitOneWith f s` | split `s` at the first run of characters satisfying `f`; returns `maybe.some [before, matched, after]` or `maybe.none` |
| `splitWith f s` | split `s` on all runs satisfying `f`; returns a list of the non-matching sections (may include empty strings at the boundaries) |
| `replaceOneWith f repl s` | replace the first run satisfying `f` with `repl match` (a function of the matched run); returns `s` unchanged if no run is found |
| `replaceAllWith f repl s` | replace every run satisfying `f` with `repl match`; returns `s` unchanged if no run is found |

`stringToInt` and `stringToFloat` do no validation — non-digit characters produce
wrong results silently. `split "" s` splits `s` into individual single-character strings.

**Ordering**

| Name | Description |
|------|-------------|
| `ltString a b` | `1` if `b` sorts strictly before `a` lexicographically (threshold-first, like the numeric `lt`) |
| `gtString a b` | `1` if `b` sorts strictly after `a` |
| `sortStrings l` | the strings of `l` in ascending lexicographic order |

The builtin comparators are numeric, so `list.sort` cannot order strings; use
`text.sortStrings` (or `list.sortWith text.ltString`). Comparison is code point
by code point, so uppercase sorts before lowercase, and a string sorts before
any string that extends it (`"ab"` before `"abc"`). The `String` suffix is
deliberate: an unqualified `import text` would otherwise shadow the numeric
`lt`/`gt` builtins and `list.sort`.

```
import text, list in
eval < map write [
  text.join ", " ["a"; "b"; "c"];     -- a, b, c
  map text.toUpper "hello"            -- HELLO
]
```

```
import text in
show [text.stringToInt "123", text.split "," "one,two,three"]
```
