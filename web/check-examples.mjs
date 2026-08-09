// Checks the playground's example catalogue against the built site: every
// program and data file it names must exist, and every program that imports a
// local module must declare it. Optionally runs the quick ones under wasm.
//
//   node web/check-examples.mjs <sitedir> [--run]

import { existsSync, readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";

const siteDir = process.argv[2] || "_site";
const alsoRun = process.argv.includes("--run");

// The catalogue is the source of truth, so it is read out of playground.js
// rather than duplicated here — both constants, evaluated together.
const source = readFileSync("web/playground.js", "utf8");
const sampleStart = source.indexOf("const SAMPLE_TEXT =");
const groupsStart = source.indexOf("const EXAMPLE_GROUPS = [");
const groupsEnd = source.indexOf("\n];", groupsStart) + 3;
const { SAMPLE_TEXT, EXAMPLE_GROUPS } = (0, eval)(
    source.slice(sampleStart, groupsEnd) + "\n({ SAMPLE_TEXT, EXAMPLE_GROUPS })");

let problems = 0;
const fail = message => { console.log("  FAIL " + message); problems++; };

const items = EXAMPLE_GROUPS.flatMap(group => group.items);
console.log(`${items.length} catalogue entries`);

for (const item of items) {
    const programPath = join(siteDir, "examples", item.file);
    if (!existsSync(programPath)) { fail(`${item.file}: not in the site`); continue; }
    if (item.stdin && !existsSync(join(siteDir, "examples", item.stdin)))
        fail(`${item.file}: stdin file ${item.stdin} missing`);

    const text = readFileSync(programPath, "utf8");
    const imports = (text.match(/^import ([^\n]*) in/m) || [, ""])[1];
    const local = imports.split(",").map(s => s.trim())
        .filter(name => name && !["core","list","math","text","maybe","comb","heap","table","hashmap","bit","big","json"].includes(name));
    const declared = Object.keys(item.modules || {});
    for (const name of local)
        if (!declared.includes(name)) fail(`${item.file}: imports ${name}, not declared in modules`);
    for (const name of declared)
        if (!existsSync(join(siteDir, "examples", item.modules[name])))
            fail(`${item.file}: module file ${item.modules[name]} missing`);

    // A program reading stdin with nothing to feed it is a catalogue bug.
    if (/\bstdin\b/.test(text) && !item.stdin && !item.stdinText)
        fail(`${item.file}: reads stdin but the entry pre-fills nothing`);
}

if (alsoRun) {
    const quick = items.filter(item => !item.slow);
    console.log(`running ${quick.length} non-slow entries under wasm`);
    for (const item of quick) {
        const args = [siteDir, join("examples", item.file),
            item.stdin ? join(siteDir, "examples", item.stdin) : "", ""];
        for (const path of Object.values(item.modules || {}))
            args.push(join(siteDir, "examples", path));
        try {
            const out = execFileSync("node", ["web/smoke.mjs", ...args], { encoding: "utf8", timeout: 300000 });
            if (out.trim() === "") fail(`${item.file}: ran but produced no output`);
        } catch (err) {
            fail(`${item.file}: ${String(err.stderr || err.message).split("\n")[0]}`);
        }
    }
}

console.log(problems === 0 ? "all entries check out" : `${problems} problem(s)`);
process.exit(problems === 0 ? 0 : 1);
