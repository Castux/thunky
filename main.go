//go:build !js

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"thunky/internal/backend"
	"thunky/internal/core"
	"thunky/internal/source"
	"thunky/internal/syntax"
)

func LoadProgram(path string) *syntax.Program {
	tokens := syntax.Lex(path)
	if tokens == nil {
		os.Exit(exitError)
	}

	prog := syntax.ParseProgram(tokens)
	if prog == nil {
		os.Exit(exitError)
	}

	return prog
}

// moduleDirs are the directories searched for a local module, in order: the
// directory holding the program being run, then the working directory. A
// program can therefore be shipped with its helper modules beside it and run by
// path from anywhere, which is what `thunky some/where/prog.þ` has to do for
// `import helper` to mean `some/where/helper.th`.
var moduleDirs []string

// setModuleDirs records where local modules are searched for, given the path of
// the program being run. The working directory stays in the list, second, so
// that running from inside a directory keeps working as before.
func setModuleDirs(programPath string) {
	dir := filepath.Dir(programPath)
	moduleDirs = []string{dir}
	if dir != "." {
		moduleDirs = append(moduleDirs, ".")
	}
}

// LexModule tries to load a module by name: from each of moduleDirs in order
// (trying .th then .þ), then from the embedded core/ library. Returns nil if
// not found in any location.
func LexModule(name string) []syntax.Token {
	for _, dir := range moduleDirs {
		for _, ext := range []string{".th", ".þ"} {
			path := filepath.Join(dir, name+ext)
			text, err := os.ReadFile(path)
			if err == nil {
				return syntax.LexContent(path, string(text))
			}
			if !errors.Is(err, fs.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "Could not read %s: %v\n", path, err)
				return nil
			}
		}
	}

	for _, ext := range []string{".th", ".þ"} {
		corePath := "core/" + name + ext
		text, err := coreFS.ReadFile(corePath)
		if err == nil {
			return syntax.LexContent(corePath, string(text))
		}
	}

	fmt.Fprintf(os.Stderr, "Module not found: %s (looked for %s.th/%s.þ in %s, and in the embedded library)\n",
		name, name, name, strings.Join(moduleDirs, ", "))
	return nil
}

func LoadModules(imports []*syntax.Name) map[string]*syntax.Module {
	loaded := make(map[string]*syntax.Module)

	var load func(*syntax.Name)
	load = func(name *syntax.Name) {
		if loaded[name.Value] != nil {
			return
		}

		tokens := LexModule(name.Value)
		if tokens == nil {
			source.Log("imported here", name.Pos, source.SeverityInfo)
			os.Exit(exitError)
		}

		module := syntax.ParseModule(tokens)
		if module == nil {
			source.Log("imported here", name.Pos, source.SeverityInfo)
			os.Exit(exitError)
		}
		module.Name = name.Value

		loaded[name.Value] = module
		for _, name := range module.Imports {
			load(name)
		}
	}

	for _, name := range imports {
		load(name)
	}
	return loaded
}

// dumpFlags selects which intermediate representations to emit. Any selection
// switches the compiler into inspection mode: the requested stages are emitted and
// the program is not run (see main).
type dumpFlags struct {
	ast      bool
	core     bool
	bytecode bool
	toFile   bool
}

func (d dumpFlags) any() bool { return d.ast || d.core || d.bytecode }

func main() {
	defer catchInternalError()

	var path string
	var dump dumpFlags
	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--dump-ast":
			dump.ast = true
		case arg == "--dump-core":
			dump.core = true
		case arg == "--dump-bytecode":
			dump.bytecode = true
		case arg == "--to-file":
			dump.toFile = true
		case len(arg) > 0 && arg[0] == '-':
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
			os.Exit(exitUsage)
		default:
			path = arg
		}
	}

	if path == "" {
		fmt.Fprintln(os.Stderr, "Usage: thunky [--dump-ast] [--dump-core] [--dump-bytecode] [--to-file] <path>")
		os.Exit(exitUsage)
	}

	setModuleDirs(path)
	program := LoadProgram(path)
	modules := LoadModules(program.Imports)

	// The AST is available right after parsing, so it can be dumped even for a
	// program that would fail to resolve.
	if dump.ast {
		emitDump(path, "ast", syntax.DumpAST(program, modules), dump.toFile)
	}

	if dump.core || dump.bytecode || !dump.any() {
		resolution := syntax.Resolve(program, modules)
		if resolution.Errors > 0 {
			fmt.Fprintf(os.Stderr, "Analyzer found %d errors\n", resolution.Errors)
			os.Exit(exitError)
		}

		mainCore, moduleCores := core.Lower(program, modules, resolution)
		if dump.core {
			emitDump(path, "ir", core.DumpCore(mainCore, moduleCores), dump.toFile)
		}

		prog := backend.Compile(mainCore, moduleCores, program, modules)
		if dump.bytecode {
			emitDump(path, "bc", backend.DumpBytecode(prog), dump.toFile)
		}

		if !dump.any() {
			machine := backend.NewMachine(prog)
			backend.RunSafe(machine)
		}
	}
}

// emitDump writes one stage's textual representation. With --to-file it goes to a
// sibling file named after the input with the stage's extension (.ast/.ir/.bc);
// otherwise it is printed to stdout.
func emitDump(inputPath, ext, content string, toFile bool) {
	if !toFile {
		fmt.Print(content)
		return
	}
	outPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + "." + ext
	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Could not write %s: %v\n", outPath, err)
		os.Exit(exitError)
	}
	fmt.Printf("wrote %s\n", outPath)
}
