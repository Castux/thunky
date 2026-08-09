// Main-thread interface to the wasm worker. A single shared runner executes
// one program at a time: starting a new run cancels the one in flight, and a
// run that exceeds its time limit gets the worker terminated out from under
// it (a fresh worker is spawned lazily for the next run).

"use strict";

const ThunkyRunner = (() => {
    const TIMEOUT_MS = 20000;

    let worker = null;
    let current = null; // { id, onOutput, resolve, timer }
    let nextId = 1;

    function spawnWorker() {
        worker = new Worker("worker.js");
        worker.onmessage = event => {
            const msg = event.data;
            if (!current || (msg.id !== undefined && msg.id !== current.id)) return;
            switch (msg.type) {
                case "output":
                    current.onOutput(msg.text, msg.fd);
                    break;
                case "done":
                    finish({ exitCode: msg.exitCode });
                    break;
                case "error":
                    finish({ hostError: msg.message });
                    break;
            }
        };
        worker.onerror = event => {
            // The worker claims Go's own exit-path throws (see worker.js), so
            // anything reaching here is a genuine host failure — a missing or
            // corrupt wasm binary, or a bug in the harness.
            event.preventDefault();
            if (current) finish({ hostError: event.message || "the worker stopped unexpectedly" });
        };
    }

    function finish(result) {
        const run = current;
        current = null;
        clearTimeout(run.timer);
        run.resolve({ ...result, elapsedMs: performance.now() - run.startedAt });
    }

    function stop(reason) {
        if (!current) return;
        // The run may be stuck inside wasm; the only way out is to kill the worker.
        worker.terminate();
        worker = null;
        finish(reason);
    }

    // run(source, opts) -> Promise<{exitCode} | {timedOut} | {cancelled} | {hostError}>
    // opts: { path, stdin, dump, modules, onOutput(text, fd), timeoutMs }
    function run(source, opts = {}) {
        stop({ cancelled: true });
        if (!worker) spawnWorker();

        return new Promise(resolve => {
            const id = nextId++;
            current = {
                id,
                onOutput: opts.onOutput || (() => {}),
                resolve,
                startedAt: performance.now(),
                timer: setTimeout(() => stop({ timedOut: true }), opts.timeoutMs || TIMEOUT_MS),
            };
            worker.postMessage({
                id,
                source,
                path: opts.path || "playground.þ",
                stdin: opts.stdin || "",
                dump: opts.dump || "",
                modules: opts.modules || {},
            });
        });
    }

    return {
        run,
        stop: () => stop({ cancelled: true }),
        get running() { return current !== null; },
    };
})();
