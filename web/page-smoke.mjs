// Headless page test for the built site. web/smoke.mjs checks the wasm binary;
// this checks the *pages* around it, which nothing else did: a top-level
// JavaScript error would render a dead shell while every other check in CI
// stayed green.
//
//   node web/page-smoke.mjs [sitedir]      # default sitedir: _site
//
// It serves the site over HTTP (wasm and fetch both need a real origin), drives
// a headless Chrome over the DevTools protocol, and asserts the things a user
// would notice. No npm dependencies: Node has a global WebSocket, and CDP is
// just JSON over it — the same choice smoke.mjs makes in hand-rolling the wasm
// host instead of pulling a framework.
//
// Set CHROME to pick a specific browser binary.

import { createServer } from "node:http";
import { spawn } from "node:child_process";
import { readFile, mkdtemp, rm } from "node:fs/promises";
import { existsSync, readFileSync } from "node:fs";
import { join, extname, normalize } from "node:path";
import { tmpdir } from "node:os";

const siteDir = process.argv[2] || "_site";
if (!existsSync(join(siteDir, "index.html"))) {
    console.error(`no site at ${siteDir} — run web/build.sh first`);
    process.exit(2);
}

// --- static server -----------------------------------------------------------

const MIME = {
    ".html": "text/html; charset=utf-8",
    ".js": "text/javascript; charset=utf-8",
    ".mjs": "text/javascript; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".json": "application/json; charset=utf-8",
    ".wasm": "application/wasm",
    ".svg": "image/svg+xml",
    ".png": "image/png",
    ".ico": "image/x-icon",
    ".md": "text/markdown; charset=utf-8",
    ".txt": "text/plain; charset=utf-8",
    ".th": "text/plain; charset=utf-8",
    ".þ": "text/plain; charset=utf-8",
};

// Mirrors GitHub Pages: an unknown path serves 404.html with status 404, which
// is what makes the custom 404 page worth having.
function serve(dir) {
    return createServer(async (req, res) => {
        let rel = decodeURIComponent(req.url.split("?")[0].split("#")[0]);
        if (rel.endsWith("/")) rel += "index.html";
        const path = join(dir, normalize(rel).replace(/^(\.\.[/\\])+/, ""));
        try {
            const body = await readFile(path);
            res.writeHead(200, { "content-type": MIME[extname(path).toLowerCase()] || "application/octet-stream" });
            res.end(body);
        } catch {
            try {
                res.writeHead(404, { "content-type": MIME[".html"] });
                res.end(await readFile(join(dir, "404.html")));
            } catch {
                res.writeHead(404).end("not found");
            }
        }
    });
}

// --- browser -----------------------------------------------------------------

function findChrome() {
    if (process.env.CHROME) return process.env.CHROME;
    const candidates = process.platform === "win32"
        ? ["C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
           "C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
           "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
           "C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe"]
        : process.platform === "darwin"
        ? ["/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
           "/Applications/Chromium.app/Contents/MacOS/Chromium"]
        : ["/usr/bin/google-chrome", "/usr/bin/google-chrome-stable",
           "/usr/bin/chromium", "/usr/bin/chromium-browser", "/snap/bin/chromium"];
    for (const c of candidates) if (existsSync(c)) return c;
    return null;
}

const sleep = ms => new Promise(r => setTimeout(r, ms));

// Chrome writes the port it actually bound to into DevToolsActivePort. Asking
// for port 0 and reading it back avoids colliding with anything already on a
// fixed port.
async function readDevToolsPort(userDataDir, timeoutMs = 30000) {
    const file = join(userDataDir, "DevToolsActivePort");
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
        try {
            const [port] = readFileSync(file, "utf8").split("\n");
            if (port && port.trim()) return Number(port.trim());
        } catch { /* not written yet */ }
        await sleep(100);
    }
    throw new Error("Chrome never reported a DevTools port");
}

// A minimal CDP client: request/response by id, plus event listeners.
async function connect(wsUrl) {
    const ws = new WebSocket(wsUrl);
    await new Promise((resolve, reject) => {
        ws.addEventListener("open", resolve, { once: true });
        ws.addEventListener("error", () => reject(new Error("CDP socket failed")), { once: true });
    });

    let nextId = 1;
    const pending = new Map();
    const handlers = [];
    ws.addEventListener("message", ev => {
        const msg = JSON.parse(ev.data);
        if (msg.id !== undefined) {
            const p = pending.get(msg.id);
            pending.delete(msg.id);
            if (!p) return;
            msg.error ? p.reject(new Error(msg.error.message)) : p.resolve(msg.result);
        } else {
            for (const h of handlers) h(msg);
        }
    });

    return {
        send(method, params = {}) {
            const id = nextId++;
            return new Promise((resolve, reject) => {
                pending.set(id, { resolve, reject });
                ws.send(JSON.stringify({ id, method, params }));
            });
        },
        on(fn) { handlers.push(fn); },
        close() { try { ws.close(); } catch { /* already gone */ } },
    };
}

// --- assertions --------------------------------------------------------------

class Failure extends Error {}
const fail = msg => { throw new Failure(msg); };

function main(cdp, origin, requests) {
    // Evaluate an expression in the page and return its value, turning a page-side
    // throw into a test failure rather than a silent undefined.
    async function evalIn(expression) {
        const r = await cdp.send("Runtime.evaluate", {
            expression, returnByValue: true, awaitPromise: true,
        });
        if (r.exceptionDetails) {
            fail("page threw: " + (r.exceptionDetails.exception?.description || r.exceptionDetails.text));
        }
        return r.result.value;
    }

    async function waitFor(expression, what, timeoutMs = 60000) {
        const deadline = Date.now() + timeoutMs;
        let last;
        while (Date.now() < deadline) {
            last = await evalIn(expression);
            if (last) return last;
            await sleep(150);
        }
        fail(`timed out waiting for ${what} (last value: ${JSON.stringify(last)})`);
    }

    // Always via about:blank. Chrome treats a navigation that differs only in the
    // fragment as *same-document*: the page's scripts do not re-run. Without the
    // blank hop, the share-link checks below assert against the state the previous
    // check left behind — which they did, and passed, until the stdin-drawer
    // assertion exposed it.
    async function goto(path) {
        await cdp.send("Page.navigate", { url: "about:blank" });
        await waitFor("location.href === 'about:blank'", "a blank page", 10000);
        requests.length = 0;
        const pathname = path.split("#")[0];
        await cdp.send("Page.navigate", { url: origin + path });
        await waitFor(
            `location.pathname === ${JSON.stringify(pathname)} && document.readyState === 'complete'`,
            `${path} to load`, 30000);
    }

    async function noPageErrors() {
        const errs = await evalIn("JSON.stringify(window.__pageErrors || [])");
        const list = JSON.parse(errs);
        if (list.length) fail("page reported errors: " + list.join(" | "));
    }

    // Every request must be same-origin: the vendoring work is only real if
    // nothing reaches for a CDN at run time.
    function noExternalRequests() {
        const external = requests.filter(u => !u.startsWith(origin) && !u.startsWith("data:") && !u.startsWith("blob:"));
        if (external.length) fail("external requests: " + external.join(", "));
    }

    const PRIMES = "[2; 3; 5; 7; 11; 13; 17; 19; 23; 29]";
    const editor = "document.querySelector('#editor .CodeMirror').CodeMirror";

    return [
        ["docs page renders its markdown", async () => {
            await goto("/index.html");
            await waitFor("!!document.querySelector('#content h1')", "the README to render");
            const h1 = await evalIn("document.querySelector('#content h1').textContent");
            if (!/Thunky/.test(h1)) fail(`unexpected first heading: ${h1}`);
            if (await evalIn("!!document.querySelector('#content .loading')")) {
                fail("the Loading… placeholder is still present");
            }
            const navLinks = await evalIn("document.querySelectorAll('#sidebar a').length");
            if (navLinks < 16) fail(`sidebar has only ${navLinks} links; expected the tutorial's 15 chapters plus more`);
            await noPageErrors();
            noExternalRequests();
        }],

        ["a doc snippet compiles and runs in place", async () => {
            await waitFor("!!document.querySelector('.snippet .snippet-bar button')", "a snippet Run button");
            await evalIn(`(() => {
                const box = [...document.querySelectorAll('.snippet')]
                    .find(b => [...b.querySelectorAll('button')].some(x => x.textContent.trim() === 'Run'));
                if (!box) throw new Error('no snippet has a Run button');
                [...box.querySelectorAll('button')].find(x => x.textContent.trim() === 'Run').click();
                window.__box = box;
                return true;
            })()`);
            const out = await waitFor(
                "window.__box.querySelector('.snippet-output') && !window.__box.querySelector('.snippet-output').hidden" +
                " && window.__box.querySelector('.snippet-output').textContent.trim() || false",
                "snippet output");
            if (!out.includes(PRIMES)) fail(`snippet output missing the primes: ${JSON.stringify(out.slice(0, 200))}`);
            await noPageErrors();
        }],

        ["playground runs the default program", async () => {
            await goto("/playground.html");
            await waitFor(`!!${editor}`, "the editor");
            await evalIn("document.getElementById('run-btn').click(), true");
            await waitFor("document.getElementById('status').classList.contains('ok')", "a successful run");
            const out = await evalIn("document.getElementById('output').textContent");
            if (!out.includes(PRIMES)) fail(`playground output missing the primes: ${JSON.stringify(out.slice(0, 200))}`);
            await noPageErrors();
            noExternalRequests();
        }],

        ["share link round-trips the program AND the stdin", async () => {
            await goto("/playground.html");
            await waitFor(`!!${editor}`, "the editor");
            const program = "import list in\n\tstdin > length > show\n";
            const stdin = "alpha\nbeta\ngamma\n";
            await evalIn(`(() => {
                ${editor}.setValue(${JSON.stringify(program)});
                document.getElementById('stdin').value = ${JSON.stringify(stdin)};
                document.getElementById('share-btn').click();
                return true;
            })()`);
            const hash = await waitFor("location.hash.startsWith('#p=') && location.hash", "a compressed share hash");

            await goto("/playground.html" + hash);
            await waitFor(`${editor}.getValue().length > 0`, "the shared program to load");
            const gotProgram = await evalIn(`${editor}.getValue()`);
            const gotStdin = await evalIn("document.getElementById('stdin').value");
            if (gotProgram !== program) fail(`program not restored: ${JSON.stringify(gotProgram)}`);
            if (gotStdin !== stdin) fail(`stdin not restored: ${JSON.stringify(gotStdin)}`);
            if (!await evalIn("document.querySelector('.pg-stdin').open")) {
                fail("the stdin drawer stayed closed, so the recipient cannot see the input");
            }
            // And it still runs, against that input — which is the whole point: a
            // shared program that arrives without its stdin is useless.
            await evalIn("document.getElementById('run-btn').click(), true");
            await waitFor("document.getElementById('status').classList.contains('ok')", "the shared program to run");
            const out = await evalIn("document.getElementById('output').textContent");
            if (out.trim() !== String(stdin.length)) {
                fail(`expected the ${stdin.length} characters of the shared stdin, got ${JSON.stringify(out)}`);
            }
            await noPageErrors();
        }],

        ["a legacy #code= link still loads", async () => {
            const program = "import core in\n\tshow (add 1 2)\n";
            const legacy = Buffer.from(program, "utf8").toString("base64");
            await goto("/playground.html#code=" + legacy);
            await waitFor(`${editor}.getValue().length > 0`, "the legacy program to load");
            const got = await evalIn(`${editor}.getValue()`);
            if (got !== program) {
                const hash = await evalIn("location.hash");
                fail(`legacy link not honoured.\n        sent hash: ${JSON.stringify("#code=" + legacy)}` +
                     `\n        page saw:  ${JSON.stringify(hash)}` +
                     `\n        editor:    ${JSON.stringify(got.slice(0, 60))}`);
            }
            await noPageErrors();
        }],

        ["an uncompressed #u= link loads (the no-CompressionStream fallback)", async () => {
            // What a browser without CompressionStream writes, and what such a
            // browser — or any browser, receiving that link — has to read back.
            const payload = { c: "import core in\n\tshow (mul 6 7)\n", i: "unused\n" };
            const b64url = Buffer.from(JSON.stringify(payload), "utf8").toString("base64")
                .replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
            await goto("/playground.html#u=" + b64url);
            await waitFor(`${editor}.getValue().length > 0`, "the uncompressed program to load");
            if (await evalIn(`${editor}.getValue()`) !== payload.c) fail("program not restored from #u=");
            if (await evalIn("document.getElementById('stdin').value") !== payload.i) fail("stdin not restored from #u=");
            await evalIn("document.getElementById('run-btn').click(), true");
            await waitFor("document.getElementById('status').classList.contains('ok')", "the run to finish");
            const out = await evalIn("document.getElementById('output').textContent");
            if (out.trim() !== "42") fail(`expected 42, got ${JSON.stringify(out)}`);
            await noPageErrors();
        }],

        ["a corrupt share link says so instead of failing silently", async () => {
            await goto("/playground.html#p=this-is-not-a-valid-payload");
            await waitFor("document.getElementById('output').textContent.includes('could not be read')",
                "the bad-link message");
            const status = await evalIn("document.getElementById('status').textContent");
            if (!/bad link/.test(status)) fail(`status did not report a bad link: ${JSON.stringify(status)}`);
            // It must still be usable: the default program is loaded and runnable.
            if (!(await evalIn(`${editor}.getValue().includes('isPrime')`))) {
                fail("the editor was left empty rather than falling back to the default program");
            }
        }],

        ["Esc escapes the editor (WCAG 2.1.2)", async () => {
            await goto("/playground.html");
            await waitFor(`!!${editor}`, "the editor");
            await evalIn(`${editor}.focus(), true`);
            if (!await evalIn("document.activeElement.closest('.CodeMirror') !== null")) {
                fail("could not focus the editor to begin with");
            }
            for (const type of ["keyDown", "keyUp"]) {
                await cdp.send("Input.dispatchKeyEvent", {
                    type, key: "Escape", code: "Escape", windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27,
                });
            }
            await waitFor("document.activeElement.closest('.CodeMirror') === null",
                "focus to leave the editor after Escape", 5000);
        }],

        ["an unknown path serves the custom 404", async () => {
            await goto("/no-such-page");
            const body = await evalIn("document.body.textContent");
            if (!body.includes("There is no page at that address")) {
                fail(`404 page not served: ${JSON.stringify(body.slice(0, 160))}`);
            }
        }],
    ];
}

// --- driver ------------------------------------------------------------------

const chrome = findChrome();
if (!chrome) {
    console.error("no Chrome or Edge found. Set CHROME=/path/to/chrome.");
    console.error("(Failing rather than skipping: a page test that quietly does nothing is worse than none.)");
    process.exit(2);
}

const server = serve(siteDir);
await new Promise(r => server.listen(0, "127.0.0.1", r));
const origin = `http://127.0.0.1:${server.address().port}`;

const userDataDir = await mkdtemp(join(tmpdir(), "thunky-page-smoke-"));
const child = spawn(chrome, [
    "--headless=new",
    "--disable-gpu",
    "--no-sandbox",                 // required in CI containers
    "--disable-dev-shm-usage",
    "--no-first-run",
    "--no-default-browser-check",
    "--remote-debugging-port=0",
    `--user-data-dir=${userDataDir}`,
    "about:blank",
], { stdio: ["ignore", "ignore", "pipe"] });
let chromeStderr = "";
child.stderr.on("data", d => { chromeStderr += d.toString(); });

let failed = 0;
let cdp;
try {
    const port = await readDevToolsPort(userDataDir);
    const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
    const page = targets.find(t => t.type === "page");
    if (!page) throw new Error("Chrome exposed no page target");
    cdp = await connect(page.webSocketDebuggerUrl);

    const requests = [];
    cdp.on(msg => {
        if (msg.method === "Network.requestWillBeSent") requests.push(msg.params.request.url);
    });
    await cdp.send("Page.enable");
    await cdp.send("Runtime.enable");
    await cdp.send("Network.enable");

    // Installed before any page script runs, so it catches a top-level throw —
    // exactly the failure this whole file exists to detect.
    await cdp.send("Page.addScriptToEvaluateOnNewDocument", {
        source: `
            window.__pageErrors = [];
            window.addEventListener('error', e => window.__pageErrors.push('error: ' + (e.message || e)));
            window.addEventListener('unhandledrejection', e =>
                window.__pageErrors.push('unhandled rejection: ' + ((e.reason && e.reason.message) || e.reason)));
            const _err = console.error;
            console.error = (...a) => { window.__pageErrors.push('console.error: ' + a.map(String).join(' ')); _err(...a); };
        `,
    });

    console.log(`page smoke: ${siteDir} via ${origin}\n`);
    for (const [name, run] of main(cdp, origin, requests)) {
        try {
            await run();
            console.log(`  PASS  ${name}`);
        } catch (err) {
            failed++;
            console.log(`  FAIL  ${name}`);
            console.log(`        ${err instanceof Failure ? err.message : (err.stack || err)}`);
        }
    }
} catch (err) {
    failed++;
    console.log(`  FAIL  harness: ${err.message}`);
    if (chromeStderr.trim()) console.log("        chrome stderr: " + chromeStderr.trim().split("\n").slice(-3).join(" / "));
} finally {
    cdp?.close();
    child.kill();
    await new Promise(r => server.close(r));
    await rm(userDataDir, { recursive: true, force: true }).catch(() => {});
}

console.log(`\n=== ${failed === 0 ? "all page checks passed" : failed + " failed"} ===`);
process.exit(failed === 0 ? 0 : 1);
