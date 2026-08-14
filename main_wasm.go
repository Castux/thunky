//go:build js && wasm

package main

import (
	"fmt"
	"os"
	"syscall/js"

	"github.com/Castux/thunky/internal/backend"
	"github.com/Castux/thunky/internal/core"
	"github.com/Castux/thunky/internal/source"
	"github.com/Castux/thunky/internal/syntax"
)

// Browser entry point. The JS host stores the program in globals before
// instantiating the module:
//
//	__thunky_source — the program text (required)
//	__thunky_path   — a display name for diagnostics (default "playground.th")
//	__thunky_dump   — "", "ast", "core" or "bytecode": emit that stage instead
//	                    of running
//	__thunky_modules — an object of local modules, { name: source }, searched
//	                    before the embedded library (see LoadEmbeddedModules)
//
// One instantiation runs one program: all package-level runtime state (the
// machine handle, the stdin streams) is per-instance, so the host gets a fresh
// runtime for every run simply by re-instantiating. Output goes through
// os.Stdout/os.Stderr, which wasm_exec.js routes to the host's fs.writeSync
// hook; stdin arrives through fs.read the same way.
func main() {
	defer catchInternalError()

	global := js.Global()

	src := global.Get("__thunky_source")
	if src.Type() != js.TypeString {
		fmt.Fprintln(os.Stderr, "host error: __thunky_source is not set")
		os.Exit(exitUsage)
	}
	path := "playground.th"
	if p := global.Get("__thunky_path"); p.Type() == js.TypeString && p.String() != "" {
		path = p.String()
	}
	dump := ""
	if d := global.Get("__thunky_dump"); d.Type() == js.TypeString {
		dump = d.String()
	}

	tokens := syntax.LexContent(path, src.String())
	if tokens == nil {
		os.Exit(exitError)
	}
	program := syntax.ParseProgram(tokens)
	if program == nil {
		os.Exit(exitError)
	}
	modules := LoadEmbeddedModules(program.Imports)

	if dump == "ast" {
		fmt.Print(syntax.DumpAST(program, modules))
		return
	}

	resolution := syntax.Resolve(program, modules)
	if resolution.Errors > 0 {
		fmt.Fprintf(os.Stderr, "Analyzer found %d errors\n", resolution.Errors)
		os.Exit(exitError)
	}

	mainCore, moduleCores := core.Lower(program, modules, resolution)
	if dump == "core" {
		fmt.Print(core.DumpCore(mainCore, moduleCores))
		return
	}

	prog := backend.Compile(mainCore, moduleCores, program, modules)
	if dump == "bytecode" {
		fmt.Print(backend.DumpBytecode(prog))
		return
	}

	machine := backend.NewMachine(prog)
	backend.RunSafe(machine)
}

// hostModules reads the local modules the host supplied in __thunky_modules,
// an object of { name: source }. The browser has no filesystem, so this is how
// a program that ships beside a helper module — examples/euler/euler.th, say —
// can run in the playground at all: the page fetches the module and hands it
// over with the program.
func hostModules() map[string]string {
	supplied := map[string]string{}
	value := js.Global().Get("__thunky_modules")
	if value.Type() != js.TypeObject {
		return supplied
	}
	names := js.Global().Get("Object").Call("keys", value)
	for i := 0; i < names.Length(); i++ {
		name := names.Index(i).String()
		if text := value.Get(name); text.Type() == js.TypeString {
			supplied[name] = text.String()
		}
	}
	return supplied
}

// LoadEmbeddedModules resolves imports against the modules the host supplied
// and then the embedded core library. That mirrors the native loader's order —
// local first, then the standard library — with the host's map standing in for
// the filesystem the browser does not have.
func LoadEmbeddedModules(imports []*syntax.Name) map[string]*syntax.Module {
	loaded := make(map[string]*syntax.Module)
	supplied := hostModules()

	var load func(*syntax.Name)
	load = func(name *syntax.Name) {
		if loaded[name.Value] != nil {
			return
		}

		// Same extension order as the native loader (main.go), with the host's
		// modules in place of the working directory it cannot search.
		var corePath string
		var text []byte
		if local, ok := supplied[name.Value]; ok {
			corePath = name.Value + ".th"
			text = []byte(local)
		} else {
			for _, ext := range []string{".th", ".þ"} {
				corePath = "core/" + name.Value + ext
				if b, err := coreFS.ReadFile(corePath); err == nil {
					text = b
					break
				}
			}
		}
		if text == nil {
			fmt.Fprintf(os.Stderr, "Module not found: %s (the browser has the embedded standard library, plus any modules the page supplied)\n", name.Value)
			source.Log("imported here", name.Pos, source.SeverityInfo)
			os.Exit(exitError)
		}

		tokens := syntax.LexContent(corePath, string(text))
		if tokens == nil {
			os.Exit(exitError)
		}
		module := syntax.ParseModule(tokens)
		if module == nil {
			os.Exit(exitError)
		}
		module.Name = name.Value

		loaded[name.Value] = module
		for _, imp := range module.Imports {
			load(imp)
		}
	}

	for _, name := range imports {
		load(name)
	}
	return loaded
}
