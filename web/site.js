// Documentation shell: fetches the repo's markdown, renders it, and upgrades
// thunky code blocks into editable, runnable snippets backed by the wasm
// runner. index.html?page=<id> selects a page.

"use strict";

// Pages, grouped for the sidebar. `file` is relative to the site root, which
// mirrors the repository layout (see web/build.sh).
const PAGES = [
    { id: "home", title: "Overview", file: "README.md", group: "Thunky" },
    { id: "language", title: "Language reference", file: "docs/LANGUAGE.md", group: "Thunky" },

    { id: "tut", title: "Introduction", file: "docs/tutorial/README.md", group: "Tutorial" },
    { id: "tut1", title: "1. Getting started", file: "docs/tutorial/01-getting-started.md", group: "Tutorial" },
    { id: "tut2", title: "2. Functions", file: "docs/tutorial/02-functions-and-application.md", group: "Tutorial" },
    { id: "tut3", title: "3. Pipes & composition", file: "docs/tutorial/03-pipes-and-composition.md", group: "Tutorial" },
    { id: "tut4", title: "4. Data", file: "docs/tutorial/04-data.md", group: "Tutorial" },
    { id: "tut5", title: "5. Pattern matching", file: "docs/tutorial/05-pattern-matching.md", group: "Tutorial" },
    { id: "tut6", title: "6. Errors & debugging", file: "docs/tutorial/06-errors-and-debugging.md", group: "Tutorial" },
    { id: "tut7", title: "7. Let & recursion", file: "docs/tutorial/07-let-and-recursion.md", group: "Tutorial" },
    { id: "tut8", title: "8. The list library", file: "docs/tutorial/08-list-library.md", group: "Tutorial" },
    { id: "tut9", title: "9. Thinking functionally", file: "docs/tutorial/09-thinking-functionally.md", group: "Tutorial" },
    { id: "tut10", title: "10. Lazy evaluation", file: "docs/tutorial/10-lazy-evaluation.md", group: "Tutorial" },
    { id: "tut11", title: "11. Performance & space", file: "docs/tutorial/11-performance-and-space.md", group: "Tutorial" },
    { id: "tut12", title: "12. Standard library", file: "docs/tutorial/12-standard-library.md", group: "Tutorial" },
    { id: "tut13", title: "13. Point-free style", file: "docs/tutorial/13-point-free-style.md", group: "Tutorial" },
    { id: "tut14", title: "14. Modules", file: "docs/tutorial/14-modules.md", group: "Tutorial" },
    { id: "tut15", title: "15. A program end to end", file: "docs/tutorial/15-program-end-to-end.md", group: "Tutorial" },
];

// The implementation notes (docs/implementation/) are deliberately not part of
// the site: they document the compiler rather than the language, and are read
// alongside the source on GitHub.

// Fenced-block info strings that stay as plain, non-interactive code. Anything
// else (untagged, `thunky`, `th`) becomes an editable, runnable snippet;
// `thunky-static` is Thunky source that is a fragment rather than a whole
// program — highlighted and editable, but with no Run button.
const STATIC_LANGS = new Set(["text", "txt", "output", "ebnf", "bnf", "sh", "bash", "console", "go", "yaml", "json", "none"]);

function currentPage() {
    const id = new URLSearchParams(location.search).get("page") || "home";
    return PAGES.find(p => p.id === id) || PAGES[0];
}

function buildSidebar(active) {
    const nav = document.getElementById("sidebar");
    let currentGroup = null;
    let list = null;
    for (const page of PAGES) {
        if (page.group !== currentGroup) {
            currentGroup = page.group;
            const heading = document.createElement("div");
            heading.className = "nav-group";
            heading.textContent = currentGroup;
            nav.appendChild(heading);
            list = document.createElement("ul");
            nav.appendChild(list);
        }
        const li = document.createElement("li");
        const a = document.createElement("a");
        a.href = page.id === "home" ? "index.html" : "index.html?page=" + page.id;
        a.textContent = page.title;
        if (page.id === active.id) a.classList.add("active");
        li.appendChild(a);
        list.appendChild(li);
    }
}

// GitHub-style slugs so intra-doc #anchors keep working.
function slugify(text) {
    return text.toLowerCase().replace(/[^\w\- ]+/g, "").trim().replace(/ +/g, "-");
}

function addHeadingIds(root) {
    const used = new Set();
    for (const h of root.querySelectorAll("h1, h2, h3, h4, h5, h6")) {
        let slug = slugify(h.textContent);
        let n = 1;
        while (used.has(slug)) slug = slugify(h.textContent) + "-" + n++;
        used.add(slug);
        h.id = slug;
    }
}

// Where to send repository links the site does not host itself — the
// implementation notes, LICENSE, and any source file referenced from prose.
//
// web/build.sh substitutes the two placeholders; it is the single place the
// repository URL and branch are configured, so a rename or a transfer does not
// leave every doc link 404ing. The guards keep this file working when it is
// read straight off disk, unbuilt.
const REPO = "__REPO_URL__".startsWith("__") ? "https://github.com/Castux/thunky" : "__REPO_URL__";
const BRANCH = "__REPO_BRANCH__".startsWith("__") ? "master" : "__REPO_BRANCH__";
const REPO_BLOB = REPO + "/blob/" + BRANCH + "/";

// Rewrite relative links between the markdown files to ?page= URLs. Links are
// resolved against the linking page's own directory, so `../LANGUAGE.md` from a
// tutorial chapter and `docs/LANGUAGE.md` from the README both find the same
// page. A repo-relative link the site does not host goes to GitHub rather than
// 404ing.
function rewriteLinks(root, fromFile) {
    const baseDir = fromFile.includes("/") ? fromFile.replace(/\/[^/]*$/, "") : "";
    for (const a of root.querySelectorAll("a[href]")) {
        const href = a.getAttribute("href");
        if (/^[a-z]+:/i.test(href) || href.startsWith("#")) continue;
        const [pathPart, fragment] = href.split("#");
        if (!pathPart) continue;

        // Normalise "<baseDir>/<href>" by resolving . and .. segments.
        const segments = [];
        for (const seg of (baseDir ? baseDir + "/" : "").concat(decodeURIComponent(pathPart)).split("/")) {
            if (seg === "" || seg === ".") continue;
            if (seg === "..") segments.pop();
            else segments.push(seg);
        }
        const resolved = segments.join("/");

        const target = PAGES.find(p => p.file === resolved);
        if (target) {
            a.setAttribute("href", "index.html?page=" + target.id + (fragment ? "#" + fragment : ""));
        } else if (resolved) {
            a.setAttribute("href", REPO_BLOB + resolved.split("/").map(encodeURIComponent).join("/") +
                (fragment ? "#" + fragment : ""));
            a.setAttribute("target", "_blank");
            a.setAttribute("rel", "noopener");
        }
    }
}

// --- runnable snippets ---

// Shared editor settings. Thunky sources are indented with tabs, displayed
// four columns wide; Tab inserts a real tab rather than spaces.
//
// Binding Tab to indent means Tab no longer moves focus, which on a tutorial
// page holding dozens of these is a keyboard trap (WCAG 2.1.2). Esc blurs, so
// there is always a way out with the keyboard alone; the placeholder text under
// each editor says so.
const EDITOR_INDENT = {
    indentUnit: 4,
    tabSize: 4,
    indentWithTabs: true,
    extraKeys: {
        Tab: cm => cm.execCommand(cm.somethingSelected() ? "indentMore" : "insertTab"),
        "Shift-Tab": cm => cm.execCommand("indentLess"),
        Esc: cm => cm.getInputField().blur(),
    },
};

const OUTPUT_BUILTINS = /\b(show|peek|write|bwrite)\b/;

// A doc example is often a bare expression annotated with `-- result`
// comments. Running it verbatim would print nothing, so when a program never
// mentions an output builtin we run `show (<program>)` instead (keeping any
// import clause in front).
//
// `raw` opts out: a block tagged ```thunky-raw runs exactly as written, for
// the examples whose point is that they print nothing.
function autoShow(src, raw) {
    if (raw) return { src, wrapped: false };
    const stripped = src.replace(/('[^']*'|"[^"]*")/g, "''").replace(/--[^\n]*/g, "");
    if (OUTPUT_BUILTINS.test(stripped)) return { src, wrapped: false };
    if (/^\s*module\b|\bmodule\b/.test(stripped)) return { src, wrapped: false };

    const importClause = src.match(/^\s*import[\w\s,]*?\bin\b/);
    if (importClause) {
        return { src: importClause[0] + "\nshow (\n" + src.slice(importClause[0].length) + "\n)", wrapped: true };
    }
    return { src: "show (\n" + src + "\n)", wrapped: true };
}

// buildSnippet renders one editor box into `box`. opts:
//   value    initial source
//   runnable false for a read-only fragment (no Run button)
//   raw      run verbatim, without the show(...) wrapper
//   scratch  an empty try-it-yourself box: no Reset, a placeholder label
function buildSnippet(box, opts) {
    const original = opts.value;
    box.className = "snippet" + (opts.scratch ? " snippet-scratch" : "");

    if (opts.scratch) {
        const label = document.createElement("div");
        label.className = "snippet-label";
        label.textContent = "Try it yourself";
        box.appendChild(label);
    }

    const editorHost = document.createElement("div");
    editorHost.className = "snippet-editor";
    box.appendChild(editorHost);

    const editor = CodeMirror(editorHost, {
        value: original,
        mode: "thunky",
        theme: "thunky",
        readOnly: !opts.runnable,
        viewportMargin: Infinity,
        lineWrapping: true,
        ...EDITOR_INDENT,
    });
    editor.getWrapperElement().setAttribute("aria-label",
        opts.runnable
            ? "Editable Thunky snippet. Press Escape to leave the editor."
            : "Thunky code");

    if (!opts.runnable) return;

    const bar = document.createElement("div");
    bar.className = "snippet-bar";
    const runBtn = document.createElement("button");
    runBtn.textContent = "Run";
    bar.appendChild(runBtn);
    if (!opts.scratch) {
        const resetBtn = document.createElement("button");
        resetBtn.textContent = "Reset";
        resetBtn.addEventListener("click", () => {
            editor.setValue(original);
            output.hidden = true;
            note.textContent = "";
        });
        bar.appendChild(resetBtn);
    }
    const note = document.createElement("span");
    note.className = "snippet-note";
    bar.appendChild(note);
    box.appendChild(bar);

    const output = document.createElement("pre");
    output.className = "snippet-output";
    output.hidden = true;
    // Output appears without the focus moving, so announce it.
    output.setAttribute("role", "log");
    output.setAttribute("aria-live", "polite");
    box.appendChild(output);

    runBtn.addEventListener("click", async () => {
        if (editor.getValue().trim() === "") return;
        const { src, wrapped } = autoShow(editor.getValue(), opts.raw);
        note.textContent = wrapped ? "showing final value" : "";
        output.hidden = false;
        output.textContent = "";
        output.classList.add("running");
        let got = "";
        const result = await ThunkyRunner.run(src, {
            path: "snippet.þ",
            onOutput: text => { got += text; output.textContent = got; },
        });
        output.classList.remove("running");
        if (result.cancelled) return;
        output.textContent = describeRun(got, result) || "(no output)";
    });

    editor.setOption("extraKeys", {
        ...EDITOR_INDENT.extraKeys,
        "Ctrl-Enter": () => runBtn.click(),
        "Cmd-Enter": () => runBtn.click(),
    });
}

// describeRun renders a finished run: whatever the program printed, plus a
// note only when something went wrong that the output does not already
// explain. The compiler and runtime write their own diagnostics to
// stdout/stderr, which the worker forwards, so a failing program's error text
// is already in `got` — appending "[host error]" on top of it would be noise.
function describeRun(got, result) {
    if (result.timedOut) {
        return got + (got ? "\n" : "") + "[stopped: exceeded the time limit]";
    }
    if (result.hostError) {
        return got + (got ? "\n" : "") + "[could not run: " + result.hostError + "]";
    }
    if (result.exitCode) {
        // A located diagnostic was already printed; only say so if it was not.
        return got || "[exited with status " + result.exitCode + "]";
    }
    return got;
}

function upgradeCodeBlocks(root) {
    for (const pre of [...root.querySelectorAll("pre")]) {
        const code = pre.querySelector("code");
        if (!code) continue;
        const langMatch = (code.className || "").match(/language-([\w-]+)/);
        const lang = langMatch ? langMatch[1] : "";
        if (STATIC_LANGS.has(lang)) continue;
        const box = document.createElement("div");
        pre.replaceWith(box);
        buildSnippet(box, {
            value: code.textContent.replace(/\n$/, ""),
            runnable: lang !== "thunky-static",
            raw: lang === "thunky-raw",
        });
    }
}

// Give every exercise a blank editor to attempt it in, placed just before the
// folded solution so the reader writes an answer before revealing one. The
// markdown stays free of empty code blocks, which would render as noise on
// GitHub.
function addScratchEditors(root) {
    for (const details of [...root.querySelectorAll("details")]) {
        const box = document.createElement("div");
        details.parentNode.insertBefore(box, details);
        buildSnippet(box, { value: "", runnable: true, scratch: true });
    }
}

async function main() {
    const page = currentPage();
    document.title = page.id === "home" ? "Thunky (Þunky)" : page.title + " — Thunky";
    buildSidebar(page);

    const content = document.getElementById("content");
    let md;
    try {
        const resp = await fetch(encodeURI(page.file));
        if (!resp.ok) throw new Error(resp.status);
        md = await resp.text();
    } catch (err) {
        content.textContent = "Could not load " + page.file + " (" + err.message + ")";
        return;
    }

    content.innerHTML = marked.parse(md);
    addHeadingIds(content);
    rewriteLinks(content, page.file);
    upgradeCodeBlocks(content);
    addScratchEditors(content);

    if (location.hash) {
        const target = document.getElementById(location.hash.slice(1));
        if (target) target.scrollIntoView();
    }
}

main();
