Thunky (Þunky): a toy programming language and its compiler

- Purely functional, dynamic, lazily evaluated. Language definition in docs/LANGUAGE.md (README.md is the short tour).
- Compiler in Go. Architecture: `internal/{source,syntax,value,core,backend,types}` (`types` is the `--types` static analysis, not part of execution). Demos/tests: `examples/`, `tests/`.
- Browser build: `main_wasm.go` (js/wasm entry) + `web/` (docs site & playground, assembled by `web/build.sh`, deployed to GitHub Pages by `.github/workflows/pages.yml`, smoke-tested headlessly with `node web/smoke.mjs`).

These reference documents MUST be kept up to date. They should be both human-readable and useful as context for an LLM.

I am a 15 years programmer, I worked in network security, games, education.

- Do not dumb down anything, unless I specifically ask for clarification, simplification or explanations.
- Keep your style concise and to the point. You are a tool. No chitchat.
- Do not make assumptions beyond what is in the context files. In doubt, propose several solutions and ask.

The unit tests in examples/core_tests.þ, written in Thunky itself, can be used as a quick regression test. If they all pass, it is likely that the compiler is still working.

## Tuple vs list literals — a frequent mistake

In Thunky, `[` … `]` uses **commas for tuples** and **semicolons for lists**:

- `[a, b]` — 2-element tuple (fixed-size, not a cons cell)
- `[a; b]` — 2-element list = cons cell `[a, [b, []]]`
- `[a, b, c]` — 3-element **tuple**
- `[a; b; c]` — 3-element **list** = `[a, [b, [c, []]]]`
- `[a]` — 1-element **tuple** — NOT a list, NOT a cons cell
- `[a;]` — 1-element **list** = cons cell `[a, []]`

The critical case is single-element containers. `[[k, v]]` is a 1-element tuple containing a 2-tuple — it is **not** a list and will fail at runtime when any list function (prepend, filter, lookup, …) tries to traverse it. The correct form is `[[k, v];]`.

More examples — the separator determines the type, regardless of what the elements are:
- `[[1, 2], [3, 4], [5, 6]]` — 3-element **tuple** of three 2-tuples
- `[[1, 2]; [3, 4]; [5, 6]]` — 3-element **list** of three 2-tuples ✓

When writing list literals with compound elements (pairs, tuples as elements), always check that the outer separator is `;`, not `,`. Whenever there is only one element, the trailing `;` in `[x;]` is mandatory to make it a list.

## Comparator convention

The builtin comparators (`lt`, `lte`, `gt`, `gte`) are **threshold-first**: the first argument is the reference value, the second is the value being tested. `lt 4` is the predicate "is less than 4". The natural reading is through the pipe: `x > lt 4` reads as "is x less than 4?". Written as `lt 4 x` it appears reversed but means the same thing: `x < 4`.

The whole standard library is built around this convention:
- `filter (lt 10) xs` — keep elements less than 10
- `takeWhile (gt 0) xs` — take while positive
- `sortWith lt xs` — ascending sort (elements less than pivot go before it)
- `heap.merge lt` — max-heap (larger value wins, i.e., `lt threshold value = 1` when `value > threshold`)

**`heap` specifically:** `cmp a b = 1` means `a` beats `b` (goes to root). With `cmp = lt`: `lt a b = 1` when `b < a`, so `a` wins when `a > b` — that's a **max-heap**. With `cmp = gt`: **min-heap**. Use `heap.sortAsc` / `heap.sortDesc` to avoid thinking about this.
