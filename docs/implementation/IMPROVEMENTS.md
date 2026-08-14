# Future improvements

Proposals for further optimization of the Thunky implementation, building on
the current single engine: the flat bytecode
([5.Bytecode and Compiler](5.Bytecode%20and%20Compiler.md)) run by the
spineless-tagless G-machine ([6.The G-machine](6.The%20G-machine.md)).

Runtime performance is the optimization target; clarity comes first, so each of
these is a deliberate trade and listed in roughly descending order of expected
payoff.

This is a **worklog, not a roadmap**. Each proposal carries its own status —
some are implemented, some were measured and rejected, most are untouched — and
that status is the useful part: it records what was tried and what the numbers
said. Nothing here is a commitment.

---

## 1. Eager strict-argument evaluation (the big one)

**Problem:** The dominant remaining allocation in numeric workloads
(`fib`, `ackermann`, `collatz`) is the thunk built for each argument of an
arithmetic builtin. The builtin is strict, so the thunk is forced immediately —
it is pure overhead.

**Status:** The machine-side half is now **implemented**. `runCode` forces a
strict prim's (and match scrutinee's) operands on the *explicit* reduction stack:
on hitting a value that needs reduction it suspends the activation into a
`contState`, pushes a run frame, and lets the single `reduce` loop force it, then
resumes — the continuation/return-frame, interleaved exec/reduce machine this item
called for. This removed the Go-stack recursion that made deep evaluation (a long
`foldr`, `sum`/`length` of a large list, an iterated `hash`) overflow, at roughly
neutral throughput on the numeric benches.

**Remaining:** the *allocation* win. Now that the machine forces operands on the
stack, the compiler could stop building a thunk for a strict prim's arithmetic
operands and instead push the operand expression to be forced in place — skipping
the per-argument thunk entirely. That compiler change is the outstanding part.

- **Saturated direct prim calls.** *(implemented)* The lowerer detects when a
  builtin name is applied to exactly its arity of arguments and emits `CorePrim`
  / `Prim` directly, skipping the `Builtin` value and the spine saturation dance in
  the reducer. `PrimArity` / `PrimNames` are arrays indexed by the dense `PrimOp`
  enum. When a `Prim` forces an operand that needs reduction it suspends, copying
  its live operands into the suspension, so the shared operand buffer stays valid
  across the forcing.

---

## 2. Minimal free-variable capture for thunks

**Problem:** Thunks capture the whole enclosing activation frame by reference
(simple, no slot renumbering, cheap `letrec`) at the cost of retaining the entire
frame for as long as any thunk over it is live.

**Fix:** Compute each thunk's exact free variables — as the lowerer already does
for lambda upvalues — and capture only those. This cuts retention but conflicts
with cheap `letrec` under by-value capture, so it needs care around mutually
recursive `let` groups.

---

## 3. Constructor-tag specialisation and field unboxing

**Problem:** Every constructor field is a `Value` (tagged union); a list of
numbers boxes each element's pointer indirection through `*Cons`.

**Fix:** Specialise common constructor shapes (e.g. a cons whose head is known to
be a number) and/or unbox constructor fields, reducing both allocation and
indirection on list-heavy code.

---

## 4. Pattern-bind without the indirection thunk

**Problem:** A pattern binding wraps the matched subject in a named, memoising
indirection thunk so the bound name appears on traces and in `show`. When the
subject is already a thunk, this is a redundant wrapper.

**Fix:** Behind an analyzer flag (so trace/show parity stays the default), skip the
wrapper when the subject is already a named thunk, or when the binding's name is
never observed.

---

## 5. Fail-fast matcher without frame allocation

**Problem:** A closure application allocates one frame (`make([]Value, Frame)`)
reused across the cases tried; a case that binds slots before a later sub-pattern
fails leaves partial writes (harmless, but wasted work).

**Fix:** Hoist a constructor-tag test to the front of each case so most
non-matching cases reject before any `Bind`, or defer binds to a scratch list and
commit only on the winning case.

---

## 6. Operand and subject stacks as fixed arrays

**Problem:** The operand buffer grows via `append`, paying a slice header and
bounds checks.

**Fix:** Compute a per-body maximum operand/subject depth in a single pass over
the `Code` and size a fixed array from it, eliminating `append` and bounds checks
on the hot path.

---

## 7. Superinstructions / peephole fusion

**Problem:** Common opcode pairs (`PushLocal; PushArg`, `PushConst; PushArg`,
`MatchTuple; Bind`) each cost a dispatch iteration.

**Fix:** Fuse frequent pairs into single opcodes to reduce dispatch overhead.
Measure which pairs dominate first.

---

## 8. Threaded dispatch

**Depends on:** profiling showing the `switch` dispatch as a bottleneck.

**Problem:** Go's `switch` over the opcode compiles to an indexed jump table that
may show up in profiles for the tight `runFrom` loop.

**Fix:** Computed-goto-style dispatch via a function table if measurements show it
wins. Measure first.

---

## 9. Serialized bytecode

**Depends on:** the bytecode being stable.

**Problem:** The bytecode is in-process only — programs re-compile from source on
every run.

**Fix:** A versioned binary encoding of `Program`, keyed by a source digest, so
compilation can be cached across runs. The `Posns` / `Names` pools already define
the source-mapping shape to serialize.

---

## 10. Drop the redundant number check in the prim kernel *(done)*

**Problem:** Before calling `EvalPrim`, `finishBuiltin` already forces every operand
of an arithmetic/comparison prim and verifies each is a `NumberTag`. `EvalPrim` then
read each operand through a `getNumber` closure that re-checked the tag — a branch
that could only fire on a machine bug.

**Fix:** *(implemented)* The kernels are now `EvalPrim1`/`EvalPrim2`, taking raw
`float64`. Measured effect across the bench suite: **within noise** (fib 6.36 s →
6.30 s, collatz 7.45 s → 7.31 s, match-dispatch 5.92 s → 6.00 s) — Go was inlining
the closure and the tag branch predicted perfectly. Kept as a clarity change, not a
speedup; do not expect a win from removing similar defensive checks elsewhere
without measuring first.

---

## Measured and rejected

Two items proposed after an earlier profiling pass turned out to be **already
solved** by "Memoize all thunks: drop the call-by-name Update flag". Both are
recorded here so they are not re-proposed:

- **Structural builtins returning their argument unforced.** The claim was that a
  top-level `… > show` re-runs the whole program, because `show` returns the raw
  pipe thunk and the machine's final `reduce` forces it a second time. That was
  worth ~1.8× when pipe thunks were call-by-name. Now every thunk memoises, so the
  second force is a memo hit: `fib 29 > show` and `let r = fib 29 in show r` are
  1.56 s and 1.52 s — no difference.
- **Forcing each match subject once per application.** `Case` still resets the
  subject stack to the raw argument and each `MatchNumber`/`MatchTuple` re-forces
  it, but re-forcing a memoised thunk just follows the memo. Verified directly:
  `{ 0 -> …, 1 -> …, 2 -> …, n -> … } (peek 9)` prints `9` exactly once, not once
  per case tried.

The general lesson: memoising every thunk subsumed both, so the remaining wins are
in *allocation*, not in repeated evaluation.
