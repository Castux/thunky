#!/usr/bin/env bash
# Bundles a program, the standard library, and optional input into the one
# stream the interpreter can read.
#
#   bundle.sh program.þ [input-file] | thunky examples/interpreter/interp.þ
#
# Every core module is included; the interpreter parses only the ones the
# program actually imports, so the unused ones cost nothing.
set -euo pipefail
cd "$(dirname "$0")/../.."

program="$1"
input="${2:-}"

for module in core/*.þ; do
    name="$(basename "$module" .þ)"
    echo "@@@ module $name"
    cat "$module"
done

echo "@@@ input"
# A section is delimited by the next marker line, so an input file that does not
# end in a newline would run into it. Add one only when it is missing, so an
# input that does end in a newline keeps exactly one.
if [ -n "$input" ]; then
    cat "$input"
    [ -n "$(tail -c1 "$input")" ] && echo
fi

echo "@@@ main"
cat "$program"
