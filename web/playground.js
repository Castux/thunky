// The playground page: a full-screen editor wired to the wasm runner, with
// example loading, stage dumps, stdin, and shareable URLs (#code=base64).

"use strict";

const DEFAULT_PROGRAM = `import list, math, core in

let
\t-- Primality test: n is prime when no divisor exists in [2, sqrt n]
\tdivides = d -> n -> eq 0 (mod n d),
\tisPrime = n -> rangeIncl 2 (floor (sqrt n)) > noneMatch (divides n) > and (gte 2 n),
\tprimes  = upFrom 2 > filter isPrime,    -- lazy infinite stream of primes

\t-- Fibonacci as a self-referential lazy stream
\tfibs = prepend [1;1] (zipWith add fibs (tail fibs))
in

[take 10 primes; take 10 fibs] > map show > eval
`;

// The ceiling on a single run. It is a backstop against a runaway program, not
// a budget: the examples menu ships programs that legitimately run for minutes
// under wasm (sudoku ~90 s, countdown ~60 s, random-chisquare ~200 s measured on
// an Intel Core i7-6700K, and slower hardware in proportion), and the Stop
// button is always available, so the limit is set well above the slowest.
const TIMEOUT_MS = 300000;
const TIMEOUT_LABEL = TIMEOUT_MS >= 60000
    ? TIMEOUT_MS / 60000 + " min"
    : TIMEOUT_MS / 1000 + " s";

const EXAMPLES = [
    "examples.þ", "streams.þ", "wordfreq.þ", "dijkstra.þ", "huffman.þ",
    "countdown.þ", "sudoku.þ", "random-chisquare.þ", "core_tests.þ",
];

const output = document.getElementById("output");
const runBtn = document.getElementById("run-btn");
const stopBtn = document.getElementById("stop-btn");
const shareBtn = document.getElementById("share-btn");
const dumpSelect = document.getElementById("dump-select");
const exampleSelect = document.getElementById("example-select");
const stdinBox = document.getElementById("stdin");
const status = document.getElementById("status");

// Thunky sources are indented with tabs, shown four columns wide.
const editor = CodeMirror(document.getElementById("editor"), {
    value: initialProgram(),
    mode: "thunky",
    theme: "thunky",
    lineNumbers: true,
    lineWrapping: true,
    indentUnit: 4,
    tabSize: 4,
    indentWithTabs: true,
    extraKeys: {
        "Ctrl-Enter": run,
        "Cmd-Enter": run,
        Tab: cm => cm.execCommand(cm.somethingSelected() ? "indentMore" : "insertTab"),
        "Shift-Tab": cm => cm.execCommand("indentLess"),
    },
});

function initialProgram() {
    const match = location.hash.match(/#code=(.+)/);
    if (match) {
        try {
            return decodeURIComponent(escape(atob(match[1])));
        } catch (err) { /* fall through to default */ }
    }
    return DEFAULT_PROGRAM;
}

for (const name of EXAMPLES) {
    const opt = document.createElement("option");
    opt.value = name;
    opt.textContent = name;
    exampleSelect.appendChild(opt);
}

exampleSelect.addEventListener("change", async () => {
    const name = exampleSelect.value;
    if (!name) return;
    try {
        const resp = await fetch("examples/" + name);
        if (!resp.ok) throw new Error(resp.status);
        editor.setValue(await resp.text());
    } catch (err) {
        output.textContent = "Could not load example: " + err.message;
    }
    exampleSelect.value = "";
});

function formatElapsed(ms) {
    if (ms < 1000) return Math.round(ms) + " ms";
    return (ms / 1000).toFixed(2) + " s";
}

async function run() {
    runBtn.disabled = true;
    stopBtn.disabled = false;
    output.textContent = "";
    status.textContent = "running…";
    status.className = "pg-status running";

    let got = "";
    const result = await ThunkyRunner.run(editor.getValue(), {
        stdin: stdinBox.value,
        dump: dumpSelect.value,
        timeoutMs: TIMEOUT_MS,
        onOutput: text => { got += text; output.textContent = got; },
    });

    runBtn.disabled = false;
    stopBtn.disabled = true;
    if (result.cancelled) {
        status.textContent = "stopped";
        status.className = "pg-status failed";
        return;
    }

    // The compiler and runtime print their own located diagnostics, which the
    // worker forwards to `got`; the status line only summarises the outcome.
    const elapsed = formatElapsed(result.elapsedMs);
    if (result.timedOut) {
        output.textContent = got + (got ? "\n" : "") + "[stopped: exceeded the " + TIMEOUT_LABEL + " time limit]";
        status.textContent = "timed out after " + elapsed;
        status.className = "pg-status failed";
    } else if (result.hostError) {
        output.textContent = got + (got ? "\n" : "") + "[could not run: " + result.hostError + "]";
        status.textContent = "host error";
        status.className = "pg-status failed";
    } else if (result.exitCode) {
        if (got === "") output.textContent = "[exited with status " + result.exitCode + "]";
        status.textContent = "failed (status " + result.exitCode + ") in " + elapsed;
        status.className = "pg-status failed";
    } else {
        if (got === "") output.textContent = "(no output — use show, peek or write to print)";
        status.textContent = "finished in " + elapsed;
        status.className = "pg-status ok";
    }
}

runBtn.addEventListener("click", run);
stopBtn.addEventListener("click", () => ThunkyRunner.stop());

shareBtn.addEventListener("click", async () => {
    const encoded = btoa(unescape(encodeURIComponent(editor.getValue())));
    const url = location.origin + location.pathname + "#code=" + encoded;
    history.replaceState(null, "", "#code=" + encoded);
    try {
        await navigator.clipboard.writeText(url);
        shareBtn.textContent = "Copied!";
    } catch (err) {
        shareBtn.textContent = "URL updated";
    }
    setTimeout(() => { shareBtn.textContent = "Share"; }, 1500);
});
