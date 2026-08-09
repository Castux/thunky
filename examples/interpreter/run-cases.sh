#!/usr/bin/env bash
# Runs the conformance corpus through the interpreter and compares each result
# with the output the compiler itself produced.
#
#   run-cases.sh [pattern]
#
# The expectations are tests/cases/*/*.expected, which the native compiler wrote
# — so nothing here is a hand-written answer.
set -uo pipefail
cd "$(dirname "$0")/../.."

pattern="${1:-}"
pass=0; fail=0; failed=()

for case in tests/cases/*/*.þ; do
    name="${case#tests/cases/}"
    [ -n "$pattern" ] && [[ "$name" != *"$pattern"* ]] && continue
    expected="${case%.þ}.expected"
    input="${case%.þ}.in"
    [ -f "$input" ] || input=""

    got=$(bash examples/interpreter/bundle.sh "$case" $input \
        | GOMEMLIMIT=3GiB timeout 300 ./thunky.exe examples/interpreter/interp.þ 2>&1)
    if [ "$got" == "$(cat "$expected")" ]; then
        pass=$((pass+1))
    else
        fail=$((fail+1)); failed+=("$name")
    fi
done

echo "=== $pass passed, $fail failed"
for name in "${failed[@]:-}"; do [ -n "$name" ] && echo "  FAIL $name"; done
exit 0
