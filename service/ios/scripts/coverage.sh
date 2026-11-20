#!/bin/sh
# The unit-test coverage of the package (CLAUDE.md, the iOS apps): each module by its own tests (ACKit by ACKitTests,
# ACShared by ACSharedTests, …), every line and function of its hand-written code. Not counted: the generated code
# (Generated/), the screens (*View.swift, *Root.swift: the UI tests drive them) and the UI tests' stub (Debug/).
#   sh scripts/coverage.sh     runs swift test with coverage, prints each module's report, exits 1 below 100 %
set -eu
cd "$(dirname "$0")/../ACKit"
log=$(mktemp)
trap 'rm -f "$log"' EXIT
swift test --enable-code-coverage >"$log" 2>&1 || { cat "$log"; exit 1; }
products=$(swift build --show-bin-path)
profile="$products/codecov/default.profdata"
status=0
for module in ACKit ACShared CustomerFeature TechnicianFeature; do
  binary="$products/${module}Tests.xctest/Contents/MacOS/${module}Tests"
  report=$(xcrun llvm-cov report "$binary" -instr-profile "$profile" -ignore-filename-regex='(Generated/|View\.swift|Root\.swift|Debug/)' \
    "$PWD/Sources/$module")
  echo "== $module"
  echo "$report" | tail -n 1
  # TOTAL … Functions Missed … Lines Missed …: the missed functions (column 6) and missed lines (column 9) must be 0
  missed=$(echo "$report" | awk '/^TOTAL/ { print $6 + $9 }')
  if [ "$missed" != 0 ]; then
    echo "$report" | awk 'NR > 2 && !/^TOTAL/ && !/^-/ && ($6 + $9) > 0'
    status=1
  fi
done
exit "$status"
