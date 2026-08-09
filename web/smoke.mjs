// Headless smoke test for the wasm build: runs a .þ program under Node the
// same way the browser worker does (source via global, output captured via
// the fs polyfill) and prints its output. Non-zero program exit becomes a
// non-zero process exit.
//
//   node web/smoke.mjs <sitedir> <program.þ> [stdin-file] [dump-stage] [module.th ...]
//
// A stdin argument naming an existing file is read from it, otherwise it is
// used as the text itself. Trailing module paths are supplied to the runtime
// the way the playground supplies a program's local modules.

import { existsSync, readFileSync } from "node:fs";
import { basename } from "node:path";
import { join } from "node:path";

const [siteDir, programPath, stdinArg, dumpStage, ...modulePaths] = process.argv.slice(2);
if (!siteDir || !programPath) {
    console.error("usage: node web/smoke.mjs <sitedir> <program.þ> [stdin-text]");
    process.exit(2);
}

// wasm_exec.js is a plain script; evaluate it in global scope so that it
// installs globalThis.Go and (since globalThis.fs is undefined here) its
// browser-style fs polyfill, which we then hook exactly like worker.js does.
(0, eval)(readFileSync(join(siteDir, "wasm_exec.js"), "utf8"));

let captured = "";
const decoder = new TextDecoder();
globalThis.fs.writeSync = (fd, buf) => {
    captured += decoder.decode(buf, { stream: true });
    return buf.length;
};

let stdinText = stdinArg || "";
if (stdinArg && existsSync(stdinArg)) stdinText = readFileSync(stdinArg, "utf8");
const stdinBytes = new TextEncoder().encode(stdinText);
let stdinPos = 0;
globalThis.fs.read = (fd, buffer, offset, length, position, callback) => {
    const n = Math.min(length, stdinBytes.length - stdinPos);
    buffer.set(stdinBytes.subarray(stdinPos, stdinPos + n), offset);
    stdinPos += n;
    callback(null, n);
};

globalThis.__thunky_source = readFileSync(programPath, "utf8");
globalThis.__thunky_path = programPath;
globalThis.__thunky_dump = dumpStage || "";
globalThis.__thunky_modules = Object.fromEntries(modulePaths.map(
    path => [basename(path).replace(/\.(th|þ)$/, ""), readFileSync(path, "utf8")]));

const go = new Go();
let exitCode = 0;
go.exit = code => { exitCode = code; };

const wasm = readFileSync(join(siteDir, "thunky.wasm"));
const { instance } = await WebAssembly.instantiate(wasm, go.importObject);
await go.run(instance);

process.stdout.write(captured);
process.exit(exitCode);
