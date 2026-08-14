package backend

import (
	"fmt"
	"os"

	"thunky/internal/source"
	"thunky/internal/value"
)

// Machine is the STG-style push/enter reducer. It executes the flat bytecode
// produced by compile.go and keeps an explicit, heap-allocated reduction stack.
// Every strict point — forcing a thunk, applying an argument, forcing a builtin's
// operand or a match scrutinee — is a frame on that stack rather than a Go
// recursive call, so it forces values of arbitrary depth in bounded Go stack.
type Machine struct {
	Prog *Program
	// moduleEnvs holds each module's binding thunks, indexed by module index
	// (Prog.ModuleNames position) — the same integer a PushModule instruction
	// carries in its A operand. Resolving module references to a slice index keeps
	// the build loop free of the per-access string-keyed map lookup it used before.
	moduleEnvs [][]value.Value

	// The shared operand buffer runCode builds values on. Exactly one activation
	// runs on it at a time: when a body suspends at a strict point it snapshots its
	// live operands into its contState, so the buffer is free for the forcing it
	// triggers and is restored on resume (see runCode / resume).
	opstack []value.Value

	// The application span and reduction stack of the builtin currently running,
	// so a structural builtin (write, bwrite, …) can locate and trace its error
	// without threading them through every helper.
	builtinPos   source.SourcePos
	builtinStack []StackFrame
}

// StackFrameKind distinguishes the five entries the reduction stack can hold.
type StackFrameKind uint8

const (
	argFrame      StackFrameKind = iota // an argument waiting to be applied to the head
	updateFrame                         // memoise the WHNF result back into a thunk
	runFrame                            // resume a suspended body activation (see contState)
	primArgsFrame                       // force a saturated builtin's args, then run its kernel
	seqFrame                            // seq: once its first arg is WHNF, continue with Arg (its second)
)

// squeezeUpdates collapses the update frames that a thunk entry would stack up
// behind itself.
//
// Entering thunk B while an update frame for thunk A is on top means A is
// waiting for exactly the value B will produce: the frames are adjacent, so
// nothing can come between them and change the result. A can therefore be made
// an indirection to B right now and its frame dropped, instead of being kept
// until B finishes.
//
// Without this, a loop whose recursive call is reached by entering a thunk
// stacks one update frame per iteration and pops none of them until it ends.
// That covers most effectful loops: `seq a b` continues into b, which is a
// thunk, and `if c t f` returns one of two thunks, so an ordinary `seq`/`if`
// loop pushes two per iteration. The frames themselves are small; what makes it
// a leak is that each pins its thunk and each thunk pins the locals its body
// captured, so an animation loop retains every board it has ever drawn.
//
// This is the reduction-stack half of the story. The other half is dropping the
// environment when the update finally happens (see the updateFrame case below):
// squeezing bounds how many thunks are pending, and clearing bounds what a
// finished one holds onto.
//
// GHC does the same collapse, but from threadPaused at a safe point rather than
// on entry. That variant was tried here and does not work: by the time the stack
// is walked, the loop's update frames have prim and run frames interleaved
// between them (40 update frames in 23 separate runs, measured on life.þ), and
// adjacency is a now-or-never property at the moment of entry.
func squeezeUpdates(stack []StackFrame, entering *value.Thunk, enteringValue value.Value) []StackFrame {
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		// A thunk already under evaluation on this stack would become an
		// indirection to itself; leave those alone.
		if top.Kind != updateFrame || top.Thunk == entering {
			break
		}
		// A named binding's frame is what a reduction trace is made of, so it is
		// left in place: the accumulating frames in a loop are the anonymous
		// thunks built for `seq`'s second argument and `if`'s branches.
		if top.Thunk.Name != "" {
			break
		}
		older := top.Thunk
		older.Value = enteringValue
		older.Forced = false
		older.Code = value.NoCode
		older.Locals = nil
		older.Upvalues = nil
		stack = stack[:len(stack)-1]
	}
	return stack
}

// StackFrame is one entry of the reduction stack.
type StackFrame struct {
	Kind  StackFrameKind
	Arg   value.Value      // valid when Kind == argFrame
	Pos   source.SourcePos // valid when Kind == argFrame
	Thunk *value.Thunk     // valid when Kind == updateFrame
	Cont  *contState       // valid when Kind == runFrame
	Prim  *primArgs        // valid when Kind == primArgsFrame
}

// contState is a suspended body activation: the state runCode needs to resume
// after a strict point (a Prim or a Match instruction) had to force a value that
// was not yet in WHNF. Rather than forcing it on the Go call stack — the recursion
// that made deep evaluation overflow — runCode snapshots itself into a contState,
// pushes a runFrame, and hands the value to the single reduce loop. When that value
// reaches WHNF, reduce writes it into the recorded slot and re-enters runCode at pc,
// which re-executes the suspending instruction (now finding the slot in WHNF).
//
// pc points at the suspending instruction itself, so resumption re-runs it. operands
// is a snapshot (the live operand buffer is scratch, reused by the forcing); subjects
// is owned outright by the activation, so it is kept by reference. Exactly one
// activation is ever running on the shared operand buffer: all suspended ancestors
// hold their operands safely in their contState.
type contState struct {
	pc       PC
	locals   []value.Value
	upvalues []value.Value
	operands []value.Value
	subjects []value.Value
	arg      value.Value
	noMatch  source.SourcePos

	// On resume, the forced value is written here before runCode re-executes the
	// suspending instruction: into subjects[injectIdx] for a Match, operands[injectIdx]
	// for a Prim.
	injectSubjects bool
	injectIdx      int
}

// primArgs drives the forcing of a saturated numeric builtin's arguments through the
// reduce loop (the spine-application path, where a Builtin value gathers its last
// argument). idx is the next argument to force; when all are forced the kernel runs.
type primArgs struct {
	op   value.PrimOp
	args []value.Value
	idx  int
	pos  source.SourcePos
}

func NewMachine(prog *Program) *Machine {
	m := &Machine{Prog: prog}
	// WHNF re-enters the machine through this package-level handle (show, equal,
	// the prim kernels, and the stdin streams all force values without a Machine
	// receiver). A program runs one Machine, so a single handle suffices.
	globalMachine = m
	// Install the runtime hooks the value package calls but cannot implement: forcing
	// requires running compiled code, and structural-builtin errors are located and
	// traced against the live reduction stack. The value package declares these as
	// nil function variables; nothing forces before a Machine exists.
	value.Force = WHNF
	value.RaiseBuiltinError = m.raiseBuiltinError
	return m
}

// Run initialises module environments and reduces the main body to WHNF.
func (m *Machine) Run() value.Value {
	// Create module environments so modules can refer to each other. They are
	// indexed by module index (Prog.ModuleNames position) so PushModule reaches one
	// with a direct slice index; a module that was never referenced has no name in
	// ModuleNames and so no env, but it is also never accessed.
	m.moduleEnvs = make([][]value.Value, len(m.Prog.ModuleNames))
	for i, modName := range m.Prog.ModuleNames {
		mbs := m.Prog.Modules[modName]
		env := make([]value.Value, len(mbs))
		for j, mb := range mbs {
			env[j] = value.ThunkValue(&value.Thunk{
				Code:   mb.Code,
				Locals: make([]value.Value, mb.Frame),
				Name:   mb.Name,
			})
		}
		m.moduleEnvs[i] = env
	}

	// Reduce the main body. runFrom may itself suspend (its head needs forcing);
	// reduce drives the returned stack to completion either way.
	mainLocals := make([]value.Value, m.Prog.EntryFrame)
	control, stack, _ := m.runFrom(m.Prog.Entry, mainLocals, nil, nil)
	return m.reduce(control, stack)
}

// RunSafe wraps Run with RuntimeError recovery.
func RunSafe(m *Machine) (result value.Value) {
	defer func() {
		if r := recover(); r != nil {
			if rerr, ok := r.(*source.RuntimeError); ok {
				source.ReportRuntimeError(rerr)
				os.Exit(1)
			}
			panic(r)
		}
	}()
	return m.Run()
}

// --- WHNF (the re-entrant entry point for show/equal/prims) ---

// WHNF forces a value to weak head normal form. An already-forced thunk is
// followed cheaply; anything that still needs work — an unforced thunk (code
// thunk, stdin cell, or pattern-binding indirection) or a pending application
// from a composition — is handed to the full reducer, which pushes the right
// update frames and memoises. It is the re-entrant entry point used by show,
// DeepEqual, FullNormalForm, and the primitive kernels (installed as value.Force);
// routing every force through the same reducer keeps update-frame handling and
// memoisation in one place.
func WHNF(v value.Value) value.Value {
	for {
		switch v.Tag {
		case value.ThunkTag:
			thunk := v.Thunk()
			if !thunk.Forced {
				return globalMachine.reduce(v, nil)
			}
			v = thunk.Value // forced thunks always hold a WHNF value
		case value.ApplyTag:
			return globalMachine.reduce(v, nil)
		default:
			return v
		}
	}
}

// globalMachine is set during NewMachine so WHNF can re-enter the machine.
var globalMachine *Machine

// --- the reducer ---

// reduce is the main reduction loop and the only place evaluation depth is kept.
// control is the current value in hand; stack is the explicit, heap-allocated
// reduction stack. Every strict point — forcing a thunk, applying an argument,
// forcing a builtin's operand, forcing a match subject — is expressed as a frame on
// this stack rather than a Go recursive call, so the loop forces values of arbitrary
// depth in bounded Go stack. (Structural builtins — show/eval/equal/hash — still
// recurse over a value's *structure* in Go; that is a separate, shallow-by-data path.)
func (m *Machine) reduce(control value.Value, stack []StackFrame) value.Value {
	for {
		switch control.Tag {
		case value.ThunkTag:
			thunk := control.Thunk()
			if thunk.Forced {
				control = thunk.Value // forced thunks always hold a WHNF value
				continue
			}
			if thunk.Read != nil {
				thunk.Value = thunk.Read()
				thunk.Forced = true
				control = thunk.Value
				continue
			}
			if thunk.Code >= 0 {
				// Code thunk: push an update frame to memoise, then run its body. The
				// body may run to a head (control := head) or suspend at a strict point
				// (control := the value it needs forced); either way the loop continues.
				stack = squeezeUpdates(stack, thunk, control)
				stack = append(stack, StackFrame{Kind: updateFrame, Thunk: thunk})
				control, stack, _ = m.runFrom(thunk.Code, thunk.Locals, thunk.Upvalues, stack)
				continue
			}
			// Graph-style indirection (NoCode): push an update frame to memoise the
			// reduced value, then continue with the value it stands for.
			stack = squeezeUpdates(stack, thunk, control)
			stack = append(stack, StackFrame{Kind: updateFrame, Thunk: thunk})
			control = thunk.Value
			continue

		case value.ApplyTag:
			// Unwind Apply nodes (only created by composition reduction).
			ap := control.Apply()
			stack = append(stack, StackFrame{Kind: argFrame, Arg: ap.Arg, Pos: ap.Pos})
			control = ap.Fn
			continue
		}

		// control is WHNF. Interact with the stack.
		if len(stack) == 0 {
			return control
		}

		frame := stack[len(stack)-1]

		switch frame.Kind {
		case updateFrame:
			// Write back the WHNF result into the thunk, and drop the environment
			// that produced it. Nothing reads Locals/Upvalues once Forced is set
			// (the check above short-circuits before the Code path), and keeping
			// them alive keeps every value the body captured alive with them —
			// for a value derived from its predecessor, that retains the whole
			// ancestry and turns an iterative program into a space leak.
			frame.Thunk.Value = control
			frame.Thunk.Forced = true
			frame.Thunk.Locals = nil
			frame.Thunk.Upvalues = nil
			stack = stack[:len(stack)-1]

		case runFrame:
			// A suspended activation whose forced value is now in hand: inject it into
			// the recorded slot and resume the body (which re-executes the instruction
			// that suspended). Resumption may run to a head or suspend again.
			stack = stack[:len(stack)-1]
			cs := frame.Cont
			if cs.injectSubjects {
				cs.subjects[cs.injectIdx] = control
			} else {
				cs.operands[cs.injectIdx] = control
			}
			control, stack, _ = m.resume(cs, stack)

		case primArgsFrame:
			// A saturated numeric builtin gathering forced operands. Store the operand
			// just forced; force the next, or run the kernel once all are in WHNF.
			pa := frame.Prim
			pa.args[pa.idx] = control
			pa.idx++
			if pa.idx < len(pa.args) {
				control = pa.args[pa.idx]
			} else {
				stack = stack[:len(stack)-1]
				control = m.finishBuiltin(pa.op, pa.args, pa.pos, stack)
			}

		case seqFrame:
			// seq's first argument is now in WHNF; its result is the second argument
			// (held in Arg), continued unforced.
			stack = stack[:len(stack)-1]
			control = frame.Arg

		case argFrame:
			// Apply control to the argument.
			switch control.Tag {
			case value.ClosureTag:
				closure := control.Closure()
				stack = stack[:len(stack)-1] // consume the argument
				// Run the closure's match+body code. The Case instruction resets the
				// subject; on match failure the match instructions jump to the next Case
				// (or NoMatch). This may run to a head or suspend at a strict point.
				locals := make([]value.Value, closure.Frame)
				control, stack, _ = m.runCode(closure.Code, locals, closure.Env, m.opstack[:0], nil, frame.Arg, closure.NoMatch, stack)

			case value.BuiltinTag:
				b := control.Builtin()
				stack = stack[:len(stack)-1]
				newArgs := make([]value.Value, len(b.Args)+1)
				copy(newArgs, b.Args)
				newArgs[len(b.Args)] = frame.Arg
				if len(newArgs) == b.Arity {
					if b.Prim == value.PrimSeq {
						// seq a b: force a to WHNF (continue with it as control), then
						// resume with b via a seqFrame. seq must not force b.
						stack = append(stack, StackFrame{Kind: seqFrame, Arg: newArgs[1]})
						control = newArgs[0]
					} else {
						// Saturated: force every argument to WHNF through the loop via a
						// primArgsFrame, then run the kernel (finishBuiltin). Forcing the
						// arguments here — rather than letting a structural kernel force them
						// inline through Go-recursive WHNF — is what keeps a deep argument
						// chain (e.g. iterated hash) off the Go stack.
						stack = append(stack, StackFrame{Kind: primArgsFrame, Prim: &primArgs{op: b.Prim, args: newArgs, pos: frame.Pos}})
						control = newArgs[0]
					}
				} else {
					// Partial application: return a new Builtin with one more arg.
					control = value.BuiltinValue(&value.Builtin{
						Prim:  b.Prim,
						Arity: b.Arity,
						Args:  newArgs,
						Name:  b.Name,
					})
				}

			case value.CompositionTag:
				// (first *> second) arg → first (second arg)
				comp := control.Composition()
				stack[len(stack)-1] = StackFrame{
					Kind: argFrame,
					Arg:  value.ApplyValue(comp.Second, frame.Arg, frame.Pos),
					Pos:  frame.Pos,
				}
				control = comp.First

			default:
				m.raiseRuntimeError(
					"cannot apply "+value.StringifyValue(control)+", it is not a function",
					frame.Pos, stack)
			}
		}
	}
}

// shallowWHNF returns v in WHNF if that needs no reduction — v is already a
// constructor/number/function, or a chain of already-forced thunks — reporting ok.
// It reports ok == false for anything that needs the reducer (an unforced code or
// stdin thunk, or a pending application); the caller then suspends and lets the
// reduce loop force it. This is the fast, allocation-free path that keeps matching
// an already-evaluated value off the suspension machinery.
func shallowWHNF(v value.Value) (value.Value, bool) {
	for {
		switch v.Tag {
		case value.ThunkTag:
			t := v.Thunk()
			if t.Forced {
				v = t.Value
				continue
			}
			return v, false
		case value.ApplyTag:
			return v, false
		default:
			return v, true
		}
	}
}

// isStructural reports whether a primitive forces its own arguments (and may recurse
// over their structure) rather than expecting the machine to force them first.
func isStructural(op value.PrimOp) bool {
	switch op {
	case value.PrimEqual, value.PrimEval, value.PrimPeek, value.PrimShow,
		value.PrimWrite, value.PrimBwrite, value.PrimString, value.PrimHash:
		return true
	}
	return false
}

// finishBuiltin runs a saturated builtin once every argument is in WHNF. A structural
// builtin's kernel may still walk its arguments' structure (and re-enter the reducer
// for deeper sub-values), but its top-level arguments are already forced, so a deep
// argument *chain* was reduced on the explicit stack rather than the Go stack. A
// numeric builtin first checks every argument is a number, naming the operation on
// failure, then evaluates the kernel.
func (m *Machine) finishBuiltin(op value.PrimOp, args []value.Value, pos source.SourcePos, stack []StackFrame) value.Value {
	if isStructural(op) {
		m.builtinPos = pos
		m.builtinStack = stack
		return value.EvalStructuralBuiltin(op, args)
	}
	for i := range args {
		if args[i].Tag != value.NumberTag {
			m.raiseRuntimeError("argument to "+value.PrimNames[op]+" is not a number", pos, stack)
		}
	}
	// The integer kernels convert to int and divide, which the Go runtime
	// panics on for a zero divisor. Both are threshold-first, so the divisor is
	// the first argument. (fdiv and fmod stay unguarded: float division by zero
	// is IEEE-defined and yields an infinity or a NaN.)
	if op == value.PrimDiv || op == value.PrimMod {
		if args[0].Num == 0 {
			m.raiseRuntimeError(value.PrimNames[op]+" by zero", pos, stack)
		}
	}
	return value.EvalPrim(op, args)
}

// snapshotOperands copies the live operand buffer, which is scratch reused by the
// forcing a suspension triggers, into a slice the contState owns.
func snapshotOperands(operands []value.Value) []value.Value {
	s := make([]value.Value, len(operands))
	copy(s, operands)
	return s
}

// --- runCode: body execution (build + match), suspendable at strict points ---

// runFrom starts a fresh thunk body. A thunk body never matches, so it has no
// subject stack, argument, or no-match span.
func (m *Machine) runFrom(pc PC, locals, upvalues []value.Value, stack []StackFrame) (value.Value, []StackFrame, bool) {
	return m.runCode(pc, locals, upvalues, m.opstack[:0], nil, value.Value{}, source.SourcePos{}, stack)
}

// resume continues a suspended activation: it restores the snapshot operands into the
// shared scratch buffer (free at this point — no other activation is running) and
// re-enters runCode at the suspending instruction, whose strict slot is now in WHNF.
func (m *Machine) resume(cs *contState, stack []StackFrame) (value.Value, []StackFrame, bool) {
	operands := append(m.opstack[:0], cs.operands...)
	return m.runCode(cs.pc, cs.locals, cs.upvalues, operands, cs.subjects, cs.arg, cs.noMatch, stack)
}

// runCode executes a contiguous body span — the match instructions of a closure case
// (if any), then its build instructions — over the given frames and operand buffer.
// It returns one of:
//   - (head, stack, false): the body reached Enter; head is its head value.
//   - (toForce, stack, true): the body hit a strict point and suspended; it pushed a
//     runFrame and returns the value the reduce loop must force. On resume the forced
//     value is in the recorded slot and execution re-runs the suspending instruction.
//
// Building never forces; only the Prim and Match instructions do, and they do so by
// suspending rather than calling the reducer recursively.
func (m *Machine) runCode(pc PC, locals, upvalues, operands, subjects []value.Value, arg value.Value, noMatch source.SourcePos, stack []StackFrame) (value.Value, []StackFrame, bool) {
	for {
		in := m.Prog.Code[pc]
		pc++

		switch in.Op {
		// --- Match instructions ---
		case Case:
			// Reset the subject stack to just the closure's argument.
			subjects = subjects[:0]
			subjects = append(subjects, arg)

		case MatchNumber:
			n := len(subjects)
			subject := subjects[n-1]
			fv, ok := shallowWHNF(subject)
			if !ok {
				cs := &contState{pc: pc - 1, locals: locals, upvalues: upvalues,
					operands: snapshotOperands(operands), subjects: subjects, arg: arg, noMatch: noMatch,
					injectSubjects: true, injectIdx: n - 1}
				stack = append(stack, StackFrame{Kind: runFrame, Cont: cs})
				return subject, stack, true
			}
			subjects = subjects[:n-1]
			if fv.Tag != value.NumberTag || fv.Num != m.Prog.Consts[in.A].Num {
				pc = PC(in.B) // jump to fail target
			}

		case MatchTuple:
			n := len(subjects)
			subject := subjects[n-1]
			fv, ok := shallowWHNF(subject)
			if !ok {
				cs := &contState{pc: pc - 1, locals: locals, upvalues: upvalues,
					operands: snapshotOperands(operands), subjects: subjects, arg: arg, noMatch: noMatch,
					injectSubjects: true, injectIdx: n - 1}
				stack = append(stack, StackFrame{Kind: runFrame, Cont: cs})
				return subject, stack, true
			}
			subjects = subjects[:n-1]
			arity := int(in.A)
			if arity == 2 {
				if fv.Tag == value.ConsTag {
					c := fv.Cons()
					subjects = append(subjects, c.Tail, c.Head) // Head on top → matched first
				} else {
					pc = PC(in.B)
				}
			} else if fv.Tag == value.TupleTag {
				t := fv.Tuple()
				if len(t.Fields) == arity {
					for j := len(t.Fields) - 1; j >= 0; j-- {
						subjects = append(subjects, t.Fields[j])
					}
				} else {
					pc = PC(in.B)
				}
			} else {
				pc = PC(in.B)
			}

		case MatchString:
			n := len(subjects)
			subject := subjects[n-1]
			subjects = subjects[:n-1]
			// matchStringSpine forces the subject's spine through WHNF; the reducer it
			// re-enters is itself iterative, so a deep subject does not recurse in Go.
			target := m.Prog.Consts[in.A]
			if !matchStringSpine(subject, target) {
				pc = PC(in.B)
			}

		case Bind:
			n := len(subjects)
			subject := subjects[n-1]
			subjects = subjects[:n-1]
			// Bind the subject behind a named, memoising indirection thunk, so the
			// bound name appears on traces and in show.
			name := ""
			if in.B >= 0 {
				name = m.Prog.Names[in.B]
			}
			locals[in.A] = value.ThunkValue(&value.Thunk{
				Code:  value.NoCode,
				Value: subject,
				Name:  name,
			})

		case NoMatch:
			m.raiseRuntimeError(
				"no pattern matched value "+value.StringifyValue(arg),
				noMatch, stack)

		// --- Build instructions ---
		case PushConst:
			operands = append(operands, m.Prog.Consts[in.A])

		case PushLocal:
			operands = append(operands, locals[in.A])

		case PushUpvalue:
			operands = append(operands, upvalues[in.A])

		case PushModule:
			operands = append(operands, m.moduleEnvs[in.A][in.B])

		case PushStdin:
			operands = append(operands, value.StdinCodePoints())

		case PushBstdin:
			operands = append(operands, value.StdinBytes())

		case MakeCons:
			n := len(operands)
			operands[n-2] = value.ConsValue(operands[n-2], operands[n-1])
			operands = operands[:n-1]

		case MakeTuple:
			n := len(operands)
			k := int(in.A)
			fields := make([]value.Value, k)
			copy(fields, operands[n-k:])
			operands = append(operands[:n-k], value.TupleValue(&value.Tuple{Fields: fields}))

		case MakeCompose:
			n := len(operands)
			operands[n-2] = value.Value{Tag: value.CompositionTag, Ref: &value.Composition{
				First: operands[n-2], Second: operands[n-1],
			}}
			operands = operands[:n-1]

		case MakeClosure:
			tmpl := &m.Prog.Closures[in.A]
			env := m.captureEnv(tmpl.Capture, locals, upvalues)
			operands = append(operands, value.ClosureValue(&value.Closure{
				Code:    tmpl.Code,
				Env:     env,
				Frame:   tmpl.Frame,
				NoMatch: tmpl.NoMatch,
			}))

		case MakeThunk:
			tmpl := &m.Prog.Thunks[in.A]
			name := ""
			if tmpl.Name >= 0 {
				name = m.Prog.Names[tmpl.Name]
			}
			operands = append(operands, value.ThunkValue(&value.Thunk{
				Code:     tmpl.Code,
				Locals:   locals,
				Upvalues: upvalues,
				Name:     name,
			}))

		case StoreLet:
			tmpl := &m.Prog.Thunks[in.B]
			name := ""
			if tmpl.Name >= 0 {
				name = m.Prog.Names[tmpl.Name]
			}
			locals[in.A] = value.ThunkValue(&value.Thunk{
				Code:     tmpl.Code,
				Locals:   locals,
				Upvalues: upvalues,
				Name:     name,
			})

		case PushArg:
			n := len(operands)
			stack = append(stack, StackFrame{
				Kind: argFrame,
				Arg:  operands[n-1],
				Pos:  m.Prog.Posns[in.A],
			})
			operands = operands[:n-1]

		case Prim:
			op := value.PrimOp(in.A)
			arity := value.PrimArity[op]
			base := len(operands) - arity
			if op == value.PrimSeq {
				// seq a b: force only a (operands[base]); the result is b, unforced.
				a := operands[base]
				if fv, ok := shallowWHNF(a); ok {
					operands[base] = fv
				} else {
					cs := &contState{pc: pc - 1, locals: locals, upvalues: upvalues,
						operands: snapshotOperands(operands), subjects: subjects, arg: arg, noMatch: noMatch,
						injectSubjects: false, injectIdx: base}
					stack = append(stack, StackFrame{Kind: runFrame, Cont: cs})
					return a, stack, true
				}
				b := operands[base+1]
				operands = operands[:base]
				operands = append(operands, b)
				break
			}
			// Force each operand to WHNF, suspending on any that needs reduction.
			// Re-execution resumes here with that operand forced in place, so the scan
			// advances rather than repeating work. Forcing the operands here — for
			// structural builtins too — keeps a deep argument chain off the Go stack;
			// the kernel then runs in finishBuiltin with its top-level arguments in WHNF.
			for j := base; j < len(operands); j++ {
				fv, ok := shallowWHNF(operands[j])
				if !ok {
					cs := &contState{pc: pc - 1, locals: locals, upvalues: upvalues,
						operands: snapshotOperands(operands), subjects: subjects, arg: arg, noMatch: noMatch,
						injectSubjects: false, injectIdx: j}
					stack = append(stack, StackFrame{Kind: runFrame, Cont: cs})
					return operands[j], stack, true
				}
				operands[j] = fv
			}
			// Copy the (now forced) arguments out and pop them before running the
			// kernel: a structural kernel may re-enter the reducer, which reuses the
			// shared operand buffer this slice aliases.
			args := make([]value.Value, arity)
			copy(args, operands[base:])
			operands = operands[:base]
			operands = append(operands, m.finishBuiltin(op, args, m.Prog.Posns[in.B], stack))

		case Enter:
			head := operands[len(operands)-1]
			m.opstack = operands[:0] // keep the backing array
			return head, stack, false

		default:
			panic(fmt.Sprintf("machine: unexpected opcode %d in runCode", in.Op))
		}
	}
}

// --- helpers ---

func (m *Machine) captureEnv(captures []Capture, locals, upvalues []value.Value) []value.Value {
	if len(captures) == 0 {
		return nil
	}
	env := make([]value.Value, len(captures))
	for i, cap := range captures {
		if cap.FromUpvalue {
			env[i] = upvalues[cap.Slot]
		} else {
			env[i] = locals[cap.Slot]
		}
	}
	return env
}

// raiseRuntimeError panics with a RuntimeError carrying the reduction trace.
func (m *Machine) raiseRuntimeError(message string, pos source.SourcePos, stack []StackFrame) {
	// Walk the stack from the outermost frame inward, collecting the names of the
	// bindings being forced — this is the reduction-trace skeleton.
	var trace []string
	for i := 0; i < len(stack); i++ {
		if stack[i].Kind == updateFrame && stack[i].Thunk.Name != "" {
			trace = append(trace, stack[i].Thunk.Name)
		}
	}
	panic(&source.RuntimeError{
		Message: message,
		Pos:     pos,
		HasPos:  pos.File != nil,
		Trace:   trace,
	})
}

// raiseBuiltinError reports an error from inside a structural builtin, locating it
// at the builtin's application span and tracing it through the reduction stack
// active when the builtin was entered. It is installed as value.RaiseBuiltinError.
func (m *Machine) raiseBuiltinError(message string) {
	m.raiseRuntimeError(message, m.builtinPos, m.builtinStack)
}

// matchStringSpine compares a value against a pre-built string constant (a cons
// list of code points). Both sides are forced spine-only.
func matchStringSpine(subject value.Value, target value.Value) bool {
	a := subject
	b := target
	for {
		fa := WHNF(a)
		fb := WHNF(b)

		if fa.Tag == value.TupleTag && fb.Tag == value.TupleTag {
			// Both empty → match.
			return len(fa.Tuple().Fields) == 0 && len(fb.Tuple().Fields) == 0
		}
		if fa.Tag == value.ConsTag && fb.Tag == value.ConsTag {
			ca := fa.Cons()
			cb := fb.Cons()
			ha := WHNF(ca.Head)
			hb := WHNF(cb.Head)
			if ha.Tag != value.NumberTag || hb.Tag != value.NumberTag || ha.Num != hb.Num {
				return false
			}
			a = ca.Tail
			b = cb.Tail
			continue
		}
		return false
	}
}
