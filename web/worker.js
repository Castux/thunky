// Web Worker that hosts the thunky wasm module. One worker handles runs
// sequentially; each run instantiates a fresh module so the runtime starts
// clean (and so a program that calls os.Exit doesn't poison later runs).
// A run that never terminates is the main thread's problem: it terminates
// the whole worker and spawns a new one (see runner.js).

importScripts("wasm_exec.js");

let modulePromise = null;

function getModule() {
    if (!modulePromise) {
        modulePromise = fetch("thunky.wasm").then(resp => {
            if (!resp.ok) throw new Error("could not fetch thunky.wasm (" + resp.status + ")");
            return resp.arrayBuffer();
        }).then(bytes => WebAssembly.compile(bytes));
    }
    return modulePromise;
}

// Stdin for the current run, served byte-by-byte to Go through fs.read.
let stdinBytes = new Uint8Array(0);
let stdinPos = 0;

// Route the Go program's stdout/stderr to the page. Output can arrive one
// byte at a time (the `write` builtin), so each stream gets a stateful UTF-8
// decoder that holds partial sequences between chunks.
function installFS() {
    const decoders = { 1: new TextDecoder(), 2: new TextDecoder() };
    const fs = globalThis.fs; // the wasm_exec.js polyfill

    fs.writeSync = (fd, buf) => {
        const dec = decoders[fd] || decoders[1];
        const text = dec.decode(buf, { stream: true });
        if (text !== "") postMessage({ type: "output", fd, text });
        return buf.length;
    };
    fs.write = (fd, buf, offset, length, position, callback) => {
        if (offset !== 0 || length !== buf.length || position !== null) {
            callback(new Error("unsupported partial write"));
            return;
        }
        callback(null, fs.writeSync(fd, buf));
    };
    fs.read = (fd, buffer, offset, length, position, callback) => {
        if (fd !== 0 || position !== null) {
            const err = new Error("bad file descriptor");
            err.code = "EBADF";
            callback(err);
            return;
        }
        const n = Math.min(length, stdinBytes.length - stdinPos);
        buffer.set(stdinBytes.subarray(stdinPos, stdinPos + n), offset);
        stdinPos += n;
        callback(null, n);
    };
}

// A program that fails to compile, or that hits a runtime error, calls
// os.Exit. wasm_exec.js implements that by throwing out of the Go runtime once
// it has unwound, which surfaces here (and, if left unhandled, as a bare
// "worker error" on the page). These are normal terminations, not host
// failures: the located diagnostic has already been printed through writeSync,
// so they must be reported as a completed run carrying the exit code.
function isNormalExit(err) {
    const message = String((err && err.message) || err);
    return message.includes("Go program has already exited") ||
        message.includes("exit status") ||
        err instanceof ExitSignal;
}

class ExitSignal extends Error {
    constructor(code) {
        super("exit status " + code);
        this.code = code;
    }
}

onmessage = async event => {
    const { id, source, path, stdin, dump, modules } = event.data;

    // An uncaught async throw from inside the Go runtime would otherwise reach
    // worker.onerror as an unlabelled "worker error"; claim it for this run.
    const onUnhandled = ev => {
        ev.preventDefault();
        postMessage({ type: "done", id, exitCode: 1 });
    };
    self.addEventListener("unhandledrejection", onUnhandled, { once: true });

    let exitCode = 0;
    try {
        const module = await getModule();

        stdinBytes = new TextEncoder().encode(stdin || "");
        stdinPos = 0;
        installFS();

        globalThis.__thunky_source = source;
        globalThis.__thunky_path = path || "playground.þ";
        globalThis.__thunky_dump = dump || "";
        // Local modules the page fetched for this program: the browser has no
        // filesystem, so this stands in for the directory a program's helper
        // modules would sit in natively.
        globalThis.__thunky_modules = modules || {};

        const go = new Go();
        go.exit = code => {
            exitCode = code;
            // Stop the Go runtime here rather than letting wasm_exec continue
            // into its "already exited" path.
            if (code !== 0) throw new ExitSignal(code);
        };

        const instance = await WebAssembly.instantiate(module, go.importObject);
        await go.run(instance);

        postMessage({ type: "done", id, exitCode });
    } catch (err) {
        if (isNormalExit(err)) {
            postMessage({ type: "done", id, exitCode: exitCode || 1 });
        } else {
            postMessage({ type: "error", id, message: String((err && err.message) || err) });
        }
    } finally {
        self.removeEventListener("unhandledrejection", onUnhandled);
    }
};

postMessage({ type: "ready" });
