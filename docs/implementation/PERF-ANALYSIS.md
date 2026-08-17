# Runtime performance analysis

> **This is a historical document.** It is the survey that motivated the
> reducer restructure and several of the fixes in
> [IMPROVEMENTS.md](IMPROVEMENTS.md), kept for the reasoning and the
> measurements rather than as a description of the current engine. The code has
> moved on: `runMatch`, `runBuiltin`, `getNumber`, `EvalPrimNumbers` and
> `Machine.ModEnvs` no longer exist under those names, and findings that quote
> them describe a shape the reducer no longer has. Read
> [6.The G-machine](6.The%20G-machine.md) for what it does now.

Survey of `internal/backend` (the push/enter reducer and compiler) and the
`internal/value` runtime it drives, looking for design-level performance flaws of
the same class as the recently fixed "thunks not all memoized" bug — i.e. costs
baked into the data model or the hot loops, not micro-optimisations.

Findings are ordered by estimated impact. Each notes the mechanism, where it
lives, and a possible direction. None are bugs; they are design costs.

> **Note (reducer restructure).** The reducer has since been made *suspendable*:
> `runFrom`/`runMatch` are now a single `runCode` that, at a strict point (a `Prim`
> or `Match*` instruction), snapshots itself onto the explicit stack instead of
> forcing through a re-entrant `WHNF`. This removed the Go-stack recursion that made
> deep evaluation overflow. It supersedes finding #4 below (re-entrant forcing no
> longer allocates a fresh reduction stack) and changes finding #9 (`StackFrame`
> gained `Cont`/`Prim` pointers and now unions four payloads). Findings that quote
> the old `runFrom`/`runMatch` split or `WHNF`-based operand forcing predate it;
> their *allocation* observations (per-frame, per-arg) still broadly hold and have
> not been re-profiled against the new loop.

---

## 1. Runtime string-keyed map lookup on every module reference  — `machine.go` `PushModule`  — ✅ FIXED

> **Resolved.** `Machine.ModEnvs map[string][]Value` is now `moduleEnvs
> [][]Value`, built in `Run` indexed by module index, and `PushModule` is
> `m.moduleEnvs[in.A][in.B]` — no string, no hash. See
> [5.Bytecode and Compiler](5.Bytecode%20and%20Compiler.md#module-and-program-layout).
> The original analysis below is kept for the rationale.
>
> A benchmark on the five working `bench/cases` showed only ~1% wall-clock change
> — but those programs each import only ~2 modules, so the lookup table is tiny
> and cheap to probe. `PushModule` itself executes millions of times (e.g.
> list-churn 4.55M, collatz 4.09M; `fib` does zero, as it touches no module), so
> the win is real and grows with module count; it is simply not the dominant cost
> in these allocation-bound microbenchmarks. The change stands on principle: a
> compile-time-known slot has no business being a hash lookup in the dispatch
> loop.

```go
case PushModule:
    modName := m.Prog.ModuleNames[in.A]
    operands = append(operands, m.ModEnvs[modName][in.B])
```

Every access to a module binding (i.e. every call into the standard library or
any other module) does a `map[string][]value.Value` hash lookup, keyed by the
module *name string*, inside the instruction dispatch loop. This is pure runtime
overhead for something fully known at compile time.

`ModEnvs` is built once in `Run()` by iterating `ModuleOrder`. The instruction
already carries `in.A`, a module index (`ModuleNames[A]`). Resolve module
environments into a `[][]value.Value` indexed directly by that integer (built in
`Run`, same order as `ModuleNames`), and `PushModule` becomes
`m.moduleEnvs[in.A][in.B]` — two slice indexes, no hashing, no string.

Impact: high. Module-qualified references are pervasive (the whole stdlib is
modules), and this sits in the innermost build loop, duplicated in both `runFrom`
and `runMatch`.

---

## 2. The `Value` representation is 32 bytes and every field access type-asserts — `value.go`

```go
type Value struct {
    Tag Tag        // uint8
    Num float64    // 8 bytes
    Ref any        // 16 bytes (interface: type word + data word)
}
```

`any` makes `Value` 32 bytes (8 pad + 8 Num + 16 interface). `Value` is *the*
copied object: the operand buffer, every `args` slice, every `StackFrame.Arg`,
every closure `Env`/`locals` slot holds one by value. Copies dominate the
interpreter, so its width is a first-order cost.

The `Tag` already discriminates the variant, so the interface's type word is
redundant — yet every accessor pays for it:

```go
func (v Value) Thunk() *Thunk { return v.Ref.(*Thunk) }
```

`v.Ref.(*Thunk)` is a checked type assertion (compares the interface's type word)
on every single thunk/cons/tuple/closure access in the hot loops.

Direction: store `Ref unsafe.Pointer` instead of `any`. That shrinks `Value` to
24 bytes (25% less to copy everywhere) and turns each accessor into an unchecked
`(*Thunk)(v.Ref)` cast — no type-word compare — which is sound precisely because
`Tag` is the authority. This is the single biggest structural lever in the
runtime, and it touches one file's constructors/accessors plus the union doc.

Impact: high, but invasive and `unsafe`. Worth prototyping behind a benchmark.

---

## 3. A heap allocation per function application (the frame) — `machine.go` `reduce`/`runMatch`

```go
case value.ClosureTag:
    ...
    locals := make([]value.Value, closure.Frame)
    control, stack = m.runMatch(closure.Code, locals, closure.Env, ...)
```

Every closure application allocates a fresh `locals` slice. For recursive
functions this is one heap allocation per call — the classic interpreter tax.
Module bindings and the entry body also each allocate (`Run`).

Frames must be heap-allocated *when they escape* (a thunk or closure created in
the body captures `locals` by reference — see `MakeThunk`/`StoreLet`/`captureEnv`
storing `locals`). But many function bodies create no escaping thunk/closure, so
their frame could be stack-reused. The compiler already computes captures
(`ClosureTemplate.Capture`); a per-body "frame escapes" bit derived during
lowering/compilation would let the machine reuse a single growable frame arena
for non-escaping calls.

Impact: high for call-heavy / recursive code. This is the closest analogue to
the memoization flaw — a systematic per-operation allocation.

---

## 4. Re-entrant forcing allocates a fresh reduction stack every time — `machine.go` WHNF

```go
func WHNF(v value.Value) value.Value {
    ...
    return globalMachine.reduce(v, nil)   // always starts from a nil stack
}
```

Every force that goes through `WHNF` (pattern matching's `MatchNumber` /
`MatchTuple` / `MatchString`, the prim kernels, `show`, `equal`, `eval`,
`walkList` for `write`/`bwrite`) re-enters `reduce` with a `nil` stack. The first
`append` then allocates a new backing array. So strict structural traversal —
forcing each element of a list to print or compare it — allocates a reduction
stack *per node*.

The top-level `reduce` reuses its stack across iterations, but none of the
re-entrant forces benefit. A pool of reduction-stack slices (grab on WHNF entry,
return on exit), or threading a reusable scratch stack on the `Machine`, would
remove this per-force allocation. Note this interacts with `opstack` reuse, which
already exploits the "never overlap" property — the same idea applies to the
reduction stack across non-overlapping WHNF calls.

Impact: high for output/equality/eval-heavy programs and deep pattern matching.

---

## 5. `equal` and `eval` allocate a map and populate it on every node — `equal.go`

```go
func DeepEqual(a, b Value, seen map[comparisonPair]bool) bool {
    pair := comparisonPair{a, b}
    if seen[pair] { return true }
    seen[pair] = true          // done for EVERY node, including numbers
    ...
}
```

Two compounding costs:

- A `map[comparisonPair]bool` (and for `eval`, `map[*Thunk]bool`) is allocated
  per top-level `equal`/`eval` call (`EvalStructuralBuiltin`). Maps are
  expensive to set up and grow.
- `seen` is written for *every* recursive comparison, including `NumberTag`
  leaves and `ClosureTag`. `comparisonPair{a, b Value}` is 64 bytes (two 32-byte
  `Value`s, each containing an interface that must be hashed). So comparing two
  flat lists of N numbers does ~N map inserts of 64-byte interface-bearing keys —
  the cycle guard costs far more than the comparison.

The acyclic case (the overwhelming majority) pays the full cyclic-detection
price. Directions: only record entries for nodes that can actually close a cycle
(cons/tuple/thunk, never numbers/closures); skip the map entirely until a
back-edge is possible (e.g. allocate lazily on first repeat, or bound by depth);
and avoid hashing `Value` interfaces by keying on the `Ref` pointers. Same shape
of fix applies to `FullNormalForm`'s `seen`.

Impact: medium–high for any program leaning on `equal`/`eval` (including the
`core_tests.þ` regression suite, which is equality-driven).

---

## 6. Per-arithmetic-op allocations and double tag-checking — `machine.go` `Prim`, `prims.go` `EvalPrim`

```go
case Prim:
    ...
    args := make([]value.Value, arity)     // slice per prim call
    copy(args, operands[n-arity:])
    ...
    result := m.runBuiltin(op, args, ...)
```

Each compiled primitive call materialises an `args` slice. For 2-ary arithmetic
that is a 2-element `[]Value` (64 bytes) per `add`/`sub`/`mul`/… Whether it
escapes to the heap depends on Go's escape analysis through `runBuiltin`; given
`args[i] = WHNF(args[i])` mutation and the structural-builtin path sharing the
signature, it likely does. The math kernels could read operands directly from the
buffer (`operands[n-2]`, `operands[n-1]`) without a slice, at least for the
fixed-arity numeric ops.

Then the work is done twice:

```go
// runBuiltin, math path:
for i := range args { args[i] = WHNF(args[i]); if args[i].Tag != value.NumberTag { allNumbers = false } }
...
return value.EvalPrim(op, args)            // EvalPrim's getNumber re-checks Tag != NumberTag on each operand
```

`runBuiltin` already forced and verified every operand is a `NumberTag`, then
`EvalPrim`'s `getNumber` re-tests `v.Tag != NumberTag` (with a panic branch) for
each operand of each op. An unchecked `EvalPrimNumbers` variant (operands known
numeric) removes a branch per operand from the hottest path in the language.

Impact: medium; it is the arithmetic inner loop, so it scales with numeric work.

---

## 7. `Builtin` first-class application allocates twice per saturation — `machine.go` argFrame/`BuiltinTag`

```go
newArgs := make([]value.Value, len(b.Args)+1)
copy(newArgs, b.Args)
newArgs[len(b.Args)] = frame.Arg
```

When a primitive is used as a *first-class value* (partially applied, passed to a
higher-order function — e.g. `fold add`), each argument application copies the
whole `Args` slice into a new one, and builds a fresh `*Builtin` for each partial
step. A 2-ary op applied via the spine allocates a 1-arg `Builtin`+slice then a
2-arg slice. Saturated calls in source position avoid this (they compile to
`Prim`), so this only bites higher-order use of operators — but that is idiomatic
functional code. A small-array inline (fixed `[N]Value` for arity ≤ small bound)
or accumulating arguments on the reduction stack until saturation would avoid the
repeated copy.

Impact: medium, concentrated in higher-order/point-free code.

---

## 8. `Thunk` carries a redundant per-instance name and two frame slices — `value.go`, `machine.go` `MakeThunk`/`StoreLet`

```go
type Thunk struct {
    Forced bool; Value Value      // 1 + 32
    Name string                   // 16  — copied from the template into every instance
    Code PC
    Locals []Value; Upvalues []Value  // 24 + 24
    Read func() Value             // 8
}
```

`Thunk` is ~112 bytes and is the most frequently heap-allocated object
(`MakeThunk`/`StoreLet` on essentially every lazy argument and let binding).
Two issues:

- `Name` (16 bytes) is copied from `ThunkTemplate` into every instance purely for
  traces/`show`. `MakeThunk` even does `name := m.Prog.Names[tmpl.Name]` per
  instantiation. Store a template index (`int32`) instead and resolve the name
  only on the cold trace/show path — saves 12 bytes per thunk and a string copy
  per allocation.
- `Locals` and `Upvalues` are two separate slice headers (48 bytes) on every
  thunk. Whole-frame capture needs the references, but if a thunk could point at
  a single captured *activation* object (frame + upvalues together, shared by all
  thunks of the same body) instead of copying both headers, the per-thunk
  footprint drops further. This dovetails with finding #3 (frames).

Impact: medium; reduces the size and cost of the runtime's most common
allocation.

---

## 9. `StackFrame` is 72 bytes and unions two disjoint payloads — `machine.go`

```go
type StackFrame struct {
    Kind  StackFrameKind   // padded to 8
    Arg   value.Value      // 32   (argFrame only)
    Pos   source.SourcePos // 24   (argFrame only)
    Thunk *value.Thunk     // 8    (updateFrame only)
}
```

An `updateFrame` uses only `Thunk` (8 bytes) but occupies 72; an `argFrame` uses
`Arg`+`Pos` (56) and never `Thunk`. The reduction stack is pushed/popped
constantly, so every push moves 72 bytes and the backing array is 72×depth. The
mutually exclusive layout wastes both bandwidth and cache. Options: split into two
stacks, or shrink — `Pos` (24 bytes, a `*Source`+two ints) is debug-only on the
arg path and could be an interned `int32` index like the bytecode already uses
elsewhere (`Posns`), cutting the frame to ~40 bytes.

Impact: medium; affects every spine operation.

---

## 10. `MakeTuple` is two allocations; minor pool/intern gaps — `machine.go`

```go
fields := make([]value.Value, k)
copy(fields, operands[n-k:])
operands = append(operands[:n-k], value.TupleValue(&value.Tuple{Fields: fields}))
```

A tuple costs two allocations (the `Tuple` struct and its `Fields` slice), versus
one for `Cons` (which is nicely packed). For arities the compiler knows, a
flattened `Tuple` with an inline small-array, or allocating struct+backing in one
make, halves it. Lower priority — tuples are less hot than cons cells, which are
already optimal.

---

## Summary

The dominant theme is **per-operation heap allocation in the reduction core** —
frames (#3), re-entrant reduction stacks (#4), prim arg slices (#6), builtin arg
copies (#7), and the fat, always-heap `Thunk`/`Value`/`StackFrame` objects
(#2, #8, #9). The cleanest high-leverage wins:

1. ~~Resolve module environments to integer-indexed slices (#1)~~ — **done.**
2. Make `equal`/`eval` stop allocating-and-populating a map per node (#5) —
   directly affects the regression suite.
3. Reuse reduction stacks across `WHNF` re-entries (#4) and frames for
   non-escaping calls (#3) — the structural analogue of the memoization fix.
4. Slim `Value` to `unsafe.Pointer` (#2) — biggest single lever, most invasive.

Each should be validated against a before/after benchmark; the same measurement
discipline that caught the 50% memoization regression applies here. Note the
caveat learned from #1: the `bench/cases` programs each import only ~2 modules and
are allocation-bound, so they understate any win concentrated in module
resolution or symbol-table size — a representative large-program benchmark would
be a better yardstick for those.
