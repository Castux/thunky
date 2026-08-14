# Thunky Tutorial

A hands-on introduction to Thunky for programmers who are new to functional and lazy programming styles.

The tutorial assumes you know how to write code in some language, but no prior exposure to functional programming, Haskell, Elm, or laziness is required. Those concepts are introduced as they come up.

After completing it, you will know how to write Thunky programs and will have a solid foundation applicable to other lazy functional languages.

---

## Chapters

| # | Chapter | Topics |
|---|---------|--------|
| 1 | [Getting Started](01-getting-started.md) | Running programs, numbers, arithmetic, `show`, tuples |
| 2 | [Functions and Application](02-functions-and-application.md) | Lambdas, currying, partial application, higher-order functions |
| 3 | [Pipes and Composition](03-pipes-and-composition.md) | `>`, `<`, `*>`, `<*`; the `import` clause |
| 4 | [Data: Tuples, Lists and Strings](04-data.md) | Cons cells, list literals, `[x]` vs `[x;]`, strings, `eq` vs `equal` |
| 5 | [Pattern Matching](05-pattern-matching.md) | Number, string, tuple and list patterns; multi-case lambdas |
| 6 | [Errors and Debugging](06-errors-and-debugging.md) | Reading diagnostics, the common failures, `peek` |
| 7 | [Let and Recursion](07-let-and-recursion.md) | Bindings, mutual visibility, recursion, accumulators |
| 8 | [The List Library](08-list-library.md) | `map`, `filter`, folds, and deriving the rest from `foldr` |
| 9 | [Thinking Functionally](09-thinking-functionally.md) | Replacing loops: `range`, `map`, `filter`, `fold` |
| 10 | [Lazy Evaluation](10-lazy-evaluation.md) | Thunks, memoization, infinite lists, self-referential data |
| 11 | [Performance and Space](11-performance-and-space.md) | WHNF vs normal form, `seq`, `foldlStrict`, sharing |
| 12 | [The Standard Library](12-standard-library.md) | `core`, `math`, `maybe`, `list`, `text`, `table`, `hashmap`, `comb`, `heap`, `bit`, `big`, `json` |
| 13 | [Point-Free Style](13-point-free-style.md) | `ap`, `fork`, `on`; when to drop the lambda and when to keep it |
| 14 | [Modules](14-modules.md) | Writing and importing modules, qualified access, design |
| 15 | [A Program End to End](15-program-end-to-end.md) | Building a complete program, reading `stdin` |

---

## Suggested reading order

Go through the chapters in order — each one builds on the previous. Pipes are introduced in chapter 3 so that every subsequent chapter can use them freely; you will see them throughout from that point on.

Chapters 1–8 are the core language: after them you can write real programs. Chapters 9–11 are about *thinking* in the language rather than new syntax, 12–13 are the library and the combinator style it enables, and 14–15 are code organisation and putting it all together.

The exercises at the end of each chapter are worth doing; they are not just repetition but apply the ideas to new problems. On the [documentation site](https://castux.github.io/thunky/) each one comes with an editor you can type your answer into and run in place.

If you get stuck on an exercise, check the solution, understand why it works, then close it and rewrite it from scratch. Reading solutions without coding them in does not help.

---

## Quick reference

The full language reference is in [docs/LANGUAGE.md](../LANGUAGE.md). Once you finish the tutorial, that document is your primary reference.
