package backend

import (
	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/value"
)

// The bytecode is flat: one Instr slice for the whole program (see Program.Code),
// not a tree of nested blocks. Every body — the program, each module binding,
// each lambda case, each thunk — is a contiguous span of that one array, reached
// by its start PC. A thunk or closure value therefore holds an integer PC, not a
// pointer to a sub-block. This "assembly, not a tree" layout — one dense array —
// is friendlier to the instruction cache and gives a single uniform notion of
// "run instructions from PC P".
//
// One instruction set covers both building values and matching patterns. A lambda
// case compiles to its match instructions —
// each tag test jumping to the next case on failure — followed immediately by its
// body instructions; there is no separate matcher IR. Building never forces; the
// match and Prim instructions are the only ones that force, and they do so by
// suspending the activation onto the machine's explicit stack rather than recursing
// (see runCode in machine.go).

// PC is re-exported from the value package, where Thunk and Closure store entry
// points; the two definitions are the same int32 alias.
type PC = value.PC

// Instr is one instruction. The two operands are small integers (pool indices,
// slots, arities, jump targets), never pointers, so Code stays dense. Each
// opcode's use of A and B is documented on the constant.
type Instr struct {
	Op Op
	A  int32
	B  int32
}

type Op uint8

const (
	// Build (push a value; never force). The head of a body is whatever these
	// leave on the operand stack when Enter is reached.
	PushConst   Op = iota // push Consts[A]
	PushLocal             // push Locals[A]
	PushUpvalue           // push Upvalues[A]
	PushModule            // push ModuleEnvironments[ModuleNames[A]][B]
	PushStdin             // push the lazy stdin code-point stream
	PushBstdin            // push the lazy stdin byte stream
	MakeCons              // pop tail, head; push Cons{head, tail}
	MakeTuple             // pop A operands; push Tuple of arity A (A ≠ 2)
	MakeCompose           // pop second, first; push Composition{first, second}
	MakeClosure           // push a closure from Closures[A], capturing the current frames
	MakeThunk             // push a (memoising) thunk from Thunks[A] over the current frames
	StoreLet              // Locals[A] = a (memoising) thunk from Thunks[B] over the current frames

	// Apply / tail position (push/enter: push arguments, then enter the head).
	PushArg // pop an operand; push it as an argument frame (with Posns[A]) onto the reduction stack
	Enter   // terminator: the operand stack's top is this body's head — hand control back to the reducer

	// Match (force the subject, test, and bind). These run before a case's body;
	// a failed test jumps to B, the next case's Case instruction.
	Case        // reset the subject stack to hold only the closure's argument (case entry / retry point)
	MatchNumber // pop subject; force to a number; jump B unless it equals Consts[A]
	MatchTuple  // pop subject; force; if a tuple/cons of arity A push its elements, else jump B
	MatchString // pop subject; match the code-point spine against Consts[A]; jump B on mismatch
	Bind        // pop subject; Locals[A] = a named thunk over it (no force)
	NoMatch     // no case matched: raise the located "no pattern matched" error

	// Saturated primitive call (the strict arithmetic/comparison builtins). The
	// kernel forces and pops its operands and pushes the result; see prims.go.
	Prim // run primitive PrimOp(A); B = Posns index for error location
)

// A ClosureTemplate is the compile-time description of a lambda. MakeClosure
// instantiates it into a *Closure, reading Capture to copy the minimal set of free
// variables from the enclosing activation's frames. Frame is the largest case
// frame, allocated once per application and reused across the cases tried.
type ClosureTemplate struct {
	Code    PC               // entry point: the first case's Case instruction
	Capture []Capture        // minimal free-variable capture (closures escape, so this is worth computing)
	Frame   int              // slots to allocate for the matched case's bindings and lets
	NoMatch source.SourcePos // span of the whole pattern set, for "no pattern matched"
}

// A ThunkTemplate is the compile-time description of a thunk body. The thunk
// captures the whole enclosing activation by reference (Locals + Upvalues), so no
// capture list is needed; it addresses them with the enclosing slot numbers.
type ThunkTemplate struct {
	Code PC
	Name int              // index into Names; the binding name for let/module thunks, -1 (→ "") for anonymous
	Pos  source.SourcePos // debug: definition site; never read by the machine
}

// A Capture says where MakeClosure reads one captured free variable: from the
// enclosing activation's own captured env (FromUpvalue) or from its local frame.
type Capture struct {
	FromUpvalue bool
	Slot        int
	Name        string // debug: source name of the captured variable; never read by the machine
}

// A ModuleBinding is one public binding of a module, compiled as its own
// activation (its right-hand side run in a fresh frame, memoised in the module
// environment). Order matches the module's PublicBindings so environment slots
// line up.
type ModuleBinding struct {
	Code  PC
	Frame int
	Name  string
}

// Program is the whole compiled unit: one flat instruction array plus the pools
// its instructions index, the closure/thunk templates, the program-body entry,
// and the per-module binding entries.
type Program struct {
	Code        []Instr
	Consts      []value.Value
	Posns       []source.SourcePos
	Names       []string
	ModuleNames []string
	Closures    []ClosureTemplate
	Thunks      []ThunkTemplate

	Entry      PC
	EntryFrame int

	Modules     map[string][]ModuleBinding
	ModuleOrder []string // stable iteration order for startup and disassembly
}
