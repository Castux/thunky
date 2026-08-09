# Thunky, in Thunky

A Thunky interpreter written in Thunky: lexer, parser, evaluator and driver, in
795 lines across five files, against the 3626 lines of Go it models.

```sh
examples/interpreter/bundle.sh myprogram.þ | thunky examples/interpreter/interp.þ
examples/interpreter/bundle.sh myprogram.þ input.txt | thunky examples/interpreter/interp.þ
examples/interpreter/run-cases.sh          # the conformance corpus
```

| File | |
|------|--|
| `lex.th` | source text to tokens, with offsets |
| `parse.th` | tokens to a syntax tree; desugars lists, pipes and composition |
| `value.th` | runtime values, and rendering them the way the compiler does |
| `eval.th` | environments, pattern matching, application, evaluation, primitives |
| `interp.þ` | the driver: reads a bundle, loads modules, runs the program |
| `bundle.sh` | packs a program, the standard library and its input into one stream |
| `run-cases.sh` | runs `tests/cases/` through it and diffs against `.expected` |

## What it scores

**56 of the 77 conformance cases pass**, compared against the `.expected` files
the *compiler* produced — so nothing here is graded against a hand-written
answer. Every failure is a diagnostic case:

| Failing | Why |
|---------|-----|
| 11 `errors/` | the compiler prints a located diagnostic and exits 1; this reports and carries on |
| 3 `lexing/`, 6 `parsing/` | the message text and format differ from the compiler's |
| 1 `output/peek-truncates` | `peek`'s width and depth bounding is not reproduced |

No case that tests *language semantics* fails. Numbers, strings, tuples, lists,
patterns (including nested and string patterns), multi-case lambdas, `let` with
mutual recursion, all four operators, imports and qualified names, laziness,
infinite structures, `stdin` and `bstdin` all behave.

## The two things worth reading it for

**Laziness is inherited, not implemented.** The evaluator has no thunk type, no
memo table, and no forcing machinery. An application is one line:

```
(eq tag parse.astApply) (apply (expression env a) (expression env b))
```

The host does not evaluate the argument in order to build that call, so the
object language is call-by-name for free; and because the host memoises every
value once forced, and the argument is a single shared host value, it is
call-by-*need*. On a strict host the same interpreter needs an explicit thunk
object and a mutable store. That an interpreted `take 5 (upFrom 1)` terminates,
and an interpreted self-referential `fibs` produces `[1; 1; 2; 3; 5; 8; 13; 21]`,
is the evidence.

**The recursive knots tie themselves.** A `let` group's environment contains
values evaluated *in that same environment*:

```
letEnv = outer -> bindings ->
    let inner = bindings > foldl (acc -> [name, expr] ->
        bind name (expression inner expr) acc) outer
    in inner,
```

The module table is the same shape one level up — a module's bindings are
evaluated in an environment built from the modules it imports, and the table is
defined in terms of itself, so circular imports work and nothing is parsed until
something names it. Bundling all twelve standard library modules therefore costs
nothing for a program that imports two.

## What it cost

Roughly 150x: `tests/cases/laziness/infinite-take.þ` runs in 49 ms natively and
7.5 s interpreted. About 7 s of that is fixed — parsing and evaluating the
bundled standard library — so the marginal cost of a bigger program is smaller
than the ratio suggests.

## Deliberately not done

**Diagnostics.** The compiler reports errors with a source location, the
offending line, a caret span and a reduction trace, then exits non-zero. This
interpreter reports parse errors with a line and column, and runtime errors as a
message, then keeps going with an empty tuple. Matching the compiler's output
would mean carrying positions through every value and every application, which
is a bigger change than the evaluator itself — and it is what those 20 cases
are testing.

**A module system for the interpreted program.** The interpreter can only read
standard input, so its input is a bundle; a program cannot import a module the
bundle did not carry.

## Notes from building it

Four bugs are recorded in the source where they happened, because each is a
property of the language rather than a slip:

- **The argument order of `sub`, hidden by commutativity.** `apply` appends
  arguments as they arrive, and `runPrimitive` reversed them again. `add 1 2`
  was right, so arithmetic looked fine; `sub 1 n` computed `1 - n`, so a
  countdown ran *away* from its base case. It surfaced as 92,028 evaluations of
  a three-step recursion, and was found by counting evaluations rather than by
  reading code — there is no debugger, and `peek` prints a value rather than a
  trace.
- **`eval` is a builtin with no namespace.** The evaluator's main function was
  called `eval`, which shadowed the builtin for every importer; the driver's
  `run > eval` then built a partial application and printed nothing at all —
  exit 0, no output, no error. It is called `expression` for that reason. A
  later missed rename left one `eval extended body`, where the *builtin* `eval`
  silently accepted an environment and returned it.
- **`[head, tail]` versus `[head; tail]` in the value representation.** A cons
  cell's items are a two-element list, and written with a comma it is a
  two-tuple instead — so every list was one element long, and `write "hello"`
  printed `h`.
- **No exceptions, so failure is a value.** Every parser returns
  `[ok, value, rest]` or `[error, message, offset]` and every combinator passes
  failures along; `andThen` exists to do that. It reads well, but it is why the
  evaluator cannot abort — a runtime error can only be reported and returned.
