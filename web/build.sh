#!/usr/bin/env bash
# Assembles the static site (docs + playground) into a deployable directory.
#
#   web/build.sh [outdir]     # default outdir: _site
#
# Requires Go. The result is self-contained: serve it with any static file
# server, e.g. `python -m http.server -d _site`.

set -euo pipefail
cd "$(dirname "$0")/.."

out="${1:-_site}"
rm -rf "$out"
mkdir -p "$out/docs" "$out/examples"

# The site is Thunky-branded; the wasm binary keeps the module name.

# The wasm build of the compiler+runtime, and the Go-version-matched JS shim.
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o "$out/thunky.wasm" .
goroot="$(go env GOROOT)"
if [ -f "$goroot/lib/wasm/wasm_exec.js" ]; then
    cp "$goroot/lib/wasm/wasm_exec.js" "$out/"
else
    cp "$goroot/misc/wasm/wasm_exec.js" "$out/"   # Go < 1.24 layout
fi

# Static assets. logo.svg lives at the repository root rather than in web/ so
# that the same relative path works from README.md on GitHub and from the
# rendered README at the root of this site; it is both the favicon and the mark
# at the top of the overview page.
cp web/index.html web/playground.html web/style.css \
   web/site.js web/playground.js web/runner.js web/worker.js web/thunky-mode.js \
   logo.svg "$out/"

# The markdown the site renders, and the example programs the playground loads.
# docs/implementation/ is intentionally excluded — see the note in web/site.js.
mkdir -p "$out/docs/tutorial"
cp README.md LICENSE.md "$out/"
cp docs/*.md "$out/docs/"
cp docs/tutorial/*.md "$out/docs/tutorial/"
cp examples/*.þ "$out/examples/"

# The exercise sets, their data files, and the helper module a few of them
# import. The playground fetches modules and hands them to the runtime, since
# the browser has no filesystem to resolve `import euler` against.
mkdir -p "$out/examples/euler/data" "$out/examples/rosetta"
cp examples/euler/*.þ examples/euler/*.th "$out/examples/euler/"
cp examples/euler/data/*.txt "$out/examples/euler/data/"
cp examples/rosetta/*.þ "$out/examples/rosetta/"

echo "site assembled in $out/"
