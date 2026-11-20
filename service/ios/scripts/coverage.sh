#!/bin/sh
# The unit-test coverage of the package (CLAUDE.md, the iOS apps): each module by its own tests (ACKit by ACKitTests,
# ACShared by ACSharedTests, …), every line and function of its hand-written code. Not counted: the generated code
# (Generated/), the screens (*View.swift, *Root.swift: the UI tests drive them), the UI tests' stub (Debug/) and the
# adapters to the phone's services (*Platform.swift: Core Location needs a phone; the models use them through protocols).
#   sh scripts/coverage.sh     runs swift test with coverage, prints each module's report, exits 1 below 100 %
set -eu
cd "$(dirname "$0")/../ACKit"
log=$(mktemp)
trap 'rm -f "$log"' EXIT
swift test --enable-code-coverage >"$log" 2>&1 || { cat "$log"; exit 1; }
products=$(swift build --show-bin-path)
profile="$products/codecov/default.profdata"
status=0
# SwiftPM 6.3 and later build one test bundle per test target (each module is measured by its own tests); earlier ones
# (the CI runner's Xcode 26) build one bundle of all of them, which then measures every module
combined="$products/ACKitPackageTests.xctest/Contents/MacOS/ACKitPackageTests"
for module in ACKit ACShared CustomerFeature TechnicianFeature; do
  binary="$products/${module}Tests.xctest/Contents/MacOS/${module}Tests"
  [ -f "$binary" ] || binary=$combined
  report=$(xcrun llvm-cov report "$binary" -instr-profile "$profile" -ignore-filename-regex='(Generated/|View\.swift|Root\.swift|Debug/|Platform\.swift)' \
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
