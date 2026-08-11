<p align="center">
  <img src="logo.svg" alt="Thunky" width="112" height="112">
</p>

# Thunky (Þunky)

Thunky (also written Þunky) is a toy programming language built to explore compiler construction,
pure functional programming, and lazy evaluation. It is minimalistic
and functional: a program is a single expression, there is one primitive
type (number) and one compound type (tuple), functions are pure, and evaluation
is lazy throughout.

## Quick overview

- **Lazy evaluation** — expressions are reduced only as far as needed, enabling
  infinite lists and self-referential definitions as ordinary programming tools.
- **One primitive, one constructor** — the only types at runtime are numbers,
  tuples (including empty), and functions. Lists and strings are layered on
  tuples.
- **Pattern matching everywhere** — function application is pattern matching;
  a lambda with multiple cases `{ pat -> body, … }` is the primary control-flow
  construct.
- **Purity** — no mutable state, no implicit effects; output is produced only
  via the `show`, `write`, and `peek` builtins.
- **Four operators** — `>` (pipe), `<` (reverse-pipe), `*>` and `<*` (forward
  and backward composition) — cover the common function-chaining idioms.
- **Standard library in Thunky** — `core`, `list`, `math`, `text`, `maybe`,
  `comb`, `heap`, `table`, `hashmap`, `bit`, `big` and `json` are written in the
  language itself and embedded in the binary. `bit` and `big` are worth a look
  as a demonstration: 32-bit bitwise operations and arbitrary-precision
  arithmetic, both built from the ordinary float64 arithmetic with no builtins
  added for them.

For the full language reference see [docs/LANGUAGE.md](docs/LANGUAGE.md).

## Usage

```sh
thunky <path>
```

Runs the program at `<path>` on the G-machine. Modules are searched for first
beside the program, then in the current directory (`name.th` or `name.þ`), then
in the embedded standard library — so a program can be shipped with its helper
modules and run by path from anywhere. Errors are reported with source
locations; runtime errors include a reduction trace.

To inspect the compiler's intermediate forms instead of running the program, pass
one or more dump flags:

```sh
thunky --dump-ast       <path>   # the parsed AST
thunky --dump-core      <path>   # the lowered Core IR (slots, captures, thunks)
thunky --dump-bytecode  <path>   # the compiled flat bytecode
thunky --types          <path>   # inferred types, one line per binding
thunky --types-all      <path>   # inferred type of every expression
```

Any dump flag emits the requested stage(s) to stdout and skips execution. Add
`--to-file` to write each one to a sibling file instead (`.ast`, `.ir`, `.bc`,
`.types`, `.types-all`).
See [docs/implementation/0.Overview.md](docs/implementation/0.Overview.md#inspecting-the-stages) for the format.

## Example

The program below demonstrates imports, recursive and mutually-visible `let`
bindings, a lazy infinite stream, pattern-matching lambdas, and the pipe and
compose operators.

```
import list, math, core in

let

  -- Primality test: n is prime when no divisor exists in [2, sqrt n]
  divides = d -> n -> eq 0 (mod n d),
  isPrime = n -> range 2 (floor (sqrt n)) > noneMatch (divides n) > and (gte 2 n),
  primes  = upFrom 2 > filter isPrime,    -- lazy infinite stream of primes

  -- Fibonacci as a self-referential lazy stream (laziness makes this safe)
  fibs = prepend [1;1] (zipWith add fibs (tail fibs)),

  -- Insertion sort: lambda with cases for structural dispatch, foldr to build result
  insert = x -> {
    []     -> [x;],
    [h, t] -> if (lte h x) [h, insert x t] [x, [h, t]]
  },
  isort = foldr insert []

in

show [
  take 10 primes,            -- [2; 3; 5; 7; 11; 13; 17; 19; 23; 29]
  take 10 fibs,              -- [1; 1; 2; 3; 5; 8; 13; 21; 34; 55]
  isort [5; 2; 8; 1; 9; 3]  -- [1; 2; 3; 5; 8; 9]
]
```

Key things illustrated:

- `upFrom 2 > filter isPrime` — the infinite list of naturals filtered to
  primes; only as many elements are produced as `take 10` demands.
- `fibs` refers to itself in its own definition; laziness prevents infinite
  regress.
- `insert` dispatches on the empty list `[]` vs a cons cell `[h, t]` via a
  lambda with two cases; `if` from `core` handles the comparison branch.
- `lte h x` reads as "`x ≤ h`" (threshold-first argument order); `gte 2 n`
  reads as "`n ≥ 2`".
- `foldr insert []` builds the sorted list right-to-left using insertion.
- The program body is a single `show` call that prints a 3-tuple of lists.

## Documentation

| Document | Contents |
|----------|----------|
| [docs/tutorial/](docs/tutorial/README.md) | Hands-on tutorial: 14 chapters from first program to a complete build, with exercises |
| [docs/LANGUAGE.md](docs/LANGUAGE.md) | Full language reference: grammar, types, operators, builtins, standard library |
| [docs/implementation/](docs/implementation/0.Overview.md) | How the compiler works, stage by stage: lexer, parser, resolver, Core IR, bytecode, G-machine |
| [docs/implementation/IMPROVEMENTS.md](docs/implementation/IMPROVEMENTS.md) | Proposals for future optimization |

## Try it in the browser

The compiler and runtime also build to WebAssembly (`main_wasm.go`), powering a
static documentation site with a playground: every Thunky code snippet in the
language reference and tutorial is editable and runnable in place, and the
playground offers a full editor with example programs, stdin, stage dumps
(AST / Core IR / bytecode), and shareable URLs.

Its example picker carries everything in `examples/`, including all of
[Project Euler](examples/euler/README.md) and
[Rosetta Code](examples/rosetta/README.md). Choosing one that reads input
pre-fills the stdin box with its data file, and one that imports a local module
(`examples/euler/euler.th`) has it fetched and handed to the runtime — the
browser has no filesystem to resolve an import against, so the page supplies
those modules itself.

The site is deployed to GitHub Pages by `.github/workflows/pages.yml`. One-time
setup on the GitHub repository:

1. **Settings → Pages → Build and deployment → Source: "GitHub Actions"**
   (not "Deploy from a branch").
2. The workflow deploys on pushes to `v1` (the current main development
   branch); adjust the `branches:` trigger if that changes. The *Run workflow*
   button (workflow_dispatch) deploys manually from any state.

The workflow builds the wasm binary with the pinned Go version, assembles the
site, smoke-tests the wasm build under Node against `examples/core_tests.þ`,
and publishes. To build and preview locally:

```sh
web/build.sh            # assembles the site (incl. the wasm build) into _site/
python -m http.server -d _site
node web/smoke.mjs _site examples/core_tests.þ   # headless check of the wasm build
```

## License

[MIT](LICENSE.md) — see LICENSE.md for the full text.
