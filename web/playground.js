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

const SAMPLE_TEXT = `the quick brown fox jumps over the lazy dog
the quick brown fox is quick and the dog is lazy
a lazy dog sleeps while the quick fox runs
`;

// Every example the site ships, grouped for the picker.
//
//   stdin   a data file to pre-fill the stdin box with, for programs that read
//           their input rather than embedding it
//   modules local modules to fetch and hand to the runtime — the browser has no
//           filesystem, so this stands in for the directory they sit in natively
//   slow    takes tens of seconds natively, and proportionally longer here
const EXAMPLE_GROUPS = [
    {
        label: "Examples",
        items: [
            { file: "examples.þ", name: "A tour of the language" },
            { file: "streams.þ", name: "Infinite lists and self-reference" },
            { file: "wordfreq.þ", name: "Word frequency (reads stdin)", stdinText: SAMPLE_TEXT },
            { file: "dijkstra.þ", name: "Dijkstra shortest paths" },
            { file: "huffman.þ", name: "Huffman coding" },
            { file: "countdown.þ", name: "Countdown numbers game" },
            { file: "sudoku.þ", name: "Sudoku solver" },
            { file: "random-chisquare.þ", name: "Chi-square of a hash stream", slow: true },
            { file: "core_tests.þ", name: "Standard library test suite" },
        ],
    },
    {
        label: "Project Euler",
        items: [
            { file: "euler/p001-multiples.þ", name: "Project Euler 1: Multiples of 3 and 5" },
            { file: "euler/p002-even-fibonacci.þ", name: "Project Euler 2: Even Fibonacci numbers" },
            { file: "euler/p003-largest-prime-factor.þ", name: "Project Euler 3: Largest prime factor" },
            { file: "euler/p004-palindrome-product.þ", name: "Project Euler 4: Largest palindrome product" },
            { file: "euler/p005-smallest-multiple.þ", name: "Project Euler 5: Smallest multiple" },
            { file: "euler/p006-sum-square-difference.þ", name: "Project Euler 6: Sum square difference" },
            { file: "euler/p007-nth-prime.þ", name: "Project Euler 7: 10001st prime" },
            { file: "euler/p008-largest-product.þ", name: "Project Euler 8: Largest product in a series", stdin: "euler/data/p008-number.txt" },
            { file: "euler/p009-pythagorean-triplet.þ", name: "Project Euler 9: Special Pythagorean triplet" },
            { file: "euler/p011-grid-product.þ", name: "Project Euler 11: Largest product in a grid", stdin: "euler/data/p011-grid.txt" },
            { file: "euler/p012-triangle-divisors.þ", name: "Project Euler 12: Highly divisible triangular number", modules: { euler: "euler/euler.th" } },
            { file: "euler/p013-large-sum.þ", name: "Project Euler 13: Large sum", stdin: "euler/data/p013-numbers.txt" },
            { file: "euler/p014-longest-collatz.þ", name: "Project Euler 14: Longest Collatz sequence", slow: true },
            { file: "euler/p016-power-digit-sum.þ", name: "Project Euler 16: Power digit sum" },
            { file: "euler/p017-number-letters.þ", name: "Project Euler 17: Number letter counts" },
            { file: "euler/p018-maximum-path-sum.þ", name: "Project Euler 18: Maximum path sum I  (and 67, unchanged)", stdin: "euler/data/p018-triangle.txt" },
            { file: "euler/p019-counting-sundays.þ", name: "Project Euler 19: Counting Sundays" },
            { file: "euler/p020-factorial-digit-sum.þ", name: "Project Euler 20: Factorial digit sum" },
            { file: "euler/p021-amicable-numbers.þ", name: "Project Euler 21: Amicable numbers", modules: { euler: "euler/euler.th" } },
            { file: "euler/p022-names-scores.þ", name: "Project Euler 22: Names scores", stdin: "euler/data/p022-names.txt" },
            { file: "euler/p024-lexicographic-permutation.þ", name: "Project Euler 24: Lexicographic permutations" },
            { file: "euler/p025-thousand-digit-fibonacci.þ", name: "Project Euler 25: 1000-digit Fibonacci number" },
            { file: "euler/p029-distinct-powers.þ", name: "Project Euler 29: Distinct powers", slow: true },
            { file: "euler/p030-digit-fifth-powers.þ", name: "Project Euler 30: Digit fifth powers", modules: { euler: "euler/euler.th" }, slow: true },
            { file: "euler/p031-coin-sums.þ", name: "Project Euler 31: Coin sums" },
            { file: "euler/p034-digit-factorials.þ", name: "Project Euler 34: Digit factorials" },
            { file: "euler/p036-double-base-palindromes.þ", name: "Project Euler 36: Double-base palindromes" },
            { file: "euler/p037-truncatable-primes.þ", name: "Project Euler 37: Truncatable primes", modules: { euler: "euler/euler.th" }, slow: true },
            { file: "euler/p040-champernowne.þ", name: "Project Euler 40: Champernowne's constant" },
            { file: "euler/p041-pandigital-prime.þ", name: "Project Euler 41: Pandigital prime", modules: { euler: "euler/euler.th" }, slow: true },
            { file: "euler/p042-coded-triangle-words.þ", name: "Project Euler 42: Coded triangle numbers", stdin: "euler/data/p042-words.txt" },
            { file: "euler/p048-self-powers.þ", name: "Project Euler 48: Self powers" },
            { file: "euler/p052-permuted-multiples.þ", name: "Project Euler 52: Permuted multiples", slow: true },
            { file: "euler/p055-lychrel-numbers.þ", name: "Project Euler 55: Lychrel numbers", slow: true },
            { file: "euler/p057-square-root-convergents.þ", name: "Project Euler 57: Square root convergents", slow: true },
            { file: "euler/p059-xor-decryption.þ", name: "Project Euler 59: XOR decryption", stdin: "euler/data/p059-cipher.txt" },
            { file: "euler/p063-powerful-digit-counts.þ", name: "Project Euler 63: Powerful digit counts" },
            { file: "euler/p018-maximum-path-sum.þ", name: "Project Euler 67: Maximum path sum II — the same program, bigger triangle", stdin: "euler/data/p067-triangle.txt" },
            { file: "euler/p097-large-non-mersenne.þ", name: "Project Euler 97: Large non-Mersenne prime" },
        ],
    },
    {
        label: "Rosetta Code",
        items: [
            { file: "rosetta/100-doors.þ", name: "Rosetta Code: 100 doors" },
            { file: "rosetta/99-bottles.þ", name: "Rosetta Code: 99 bottles of beer" },
            { file: "rosetta/ackermann.þ", name: "Rosetta Code: Ackermann function" },
            { file: "rosetta/binary-search.þ", name: "Rosetta Code: Binary search" },
            { file: "rosetta/binary-tree-traversal.þ", name: "Rosetta Code: Tree traversal" },
            { file: "rosetta/caesar-cipher.þ", name: "Rosetta Code: Caesar cipher" },
            { file: "rosetta/crc-32.þ", name: "Rosetta Code: CRC-32" },
            { file: "rosetta/fizzbuzz.þ", name: "Rosetta Code: FizzBuzz" },
            { file: "rosetta/gray-code.þ", name: "Rosetta Code: Gray code" },
            { file: "rosetta/hailstone.þ", name: "Rosetta Code: Hailstone sequence (Collatz)" },
            { file: "rosetta/happy-numbers.þ", name: "Rosetta Code: Happy numbers" },
            { file: "rosetta/json-load-print.þ", name: "Rosetta Code: JSON" },
            { file: "rosetta/levenshtein-distance.þ", name: "Rosetta Code: Levenshtein distance" },
            { file: "rosetta/lzw-compression.þ", name: "Rosetta Code: LZW compression" },
            { file: "rosetta/man-or-boy-test.þ", name: "Rosetta Code: Man or boy test" },
            { file: "rosetta/mandelbrot.þ", name: "Rosetta Code: Mandelbrot set" },
            { file: "rosetta/matrix-multiplication.þ", name: "Rosetta Code: Matrix multiplication" },
            { file: "rosetta/n-queens.þ", name: "Rosetta Code: N-queens problem" },
            { file: "rosetta/pascals-triangle.þ", name: "Rosetta Code: Pascal's triangle" },
            { file: "rosetta/priority-queue.þ", name: "Rosetta Code: Priority queue" },
            { file: "rosetta/roman-numerals.þ", name: "Rosetta Code: Roman numerals (encode and decode)" },
            { file: "rosetta/rsa-code.þ", name: "Rosetta Code: RSA code" },
            { file: "rosetta/sieve-of-eratosthenes.þ", name: "Rosetta Code: Sieve of Eratosthenes" },
            { file: "rosetta/sorting-algorithms.þ", name: "Rosetta Code: Sorting algorithms" },
            { file: "rosetta/towers-of-hanoi.þ", name: "Rosetta Code: Towers of Hanoi" },
            { file: "rosetta/word-wrap.þ", name: "Rosetta Code: Word wrap" },
            { file: "rosetta/y-combinator.þ", name: "Rosetta Code: Y combinator" },
            { file: "rosetta/zebra-puzzle.þ", name: "Rosetta Code: Zebra puzzle" },
        ],
    },
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

// Options are keyed by index rather than by filename, because one program can
// appear twice with different input — Euler 18 and 67 are the same file.
const examplesByKey = new Map();
for (const group of EXAMPLE_GROUPS) {
    const optgroup = document.createElement("optgroup");
    optgroup.label = group.label;
    for (const item of group.items) {
        const key = String(examplesByKey.size);
        examplesByKey.set(key, item);
        const opt = document.createElement("option");
        opt.value = key;
        opt.textContent = item.name + (item.slow ? "  ·  slow" : "");
        optgroup.appendChild(opt);
    }
    exampleSelect.appendChild(optgroup);
}

// Local modules for the loaded example, passed to the runtime on every run.
let currentModules = {};

async function fetchExampleFile(path) {
    const resp = await fetch("examples/" + path);
    if (!resp.ok) throw new Error(path + " (" + resp.status + ")");
    return await resp.text();
}

exampleSelect.addEventListener("change", async () => {
    const item = examplesByKey.get(exampleSelect.value);
    exampleSelect.value = "";
    if (!item) return;
    try {
        editor.setValue(await fetchExampleFile(item.file));
        stdinBox.value = item.stdin
            ? await fetchExampleFile(item.stdin)
            : (item.stdinText || "");
        const modules = {};
        for (const [name, path] of Object.entries(item.modules || {})) {
            modules[name] = await fetchExampleFile(path);
        }
        currentModules = modules;
    } catch (err) {
        output.textContent = "Could not load example: " + err.message;
    }
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
        modules: currentModules,
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
