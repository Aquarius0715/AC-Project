#!/bin/sh
# The checks of the iOS apps (CLAUDE.md, the iOS apps), the same in CI (ci.yml, job ios) and in scripts/check.sh:
# SwiftLint (--strict), swift-format (lint --strict), the package's unit tests with 100 % of lines and functions
# (scripts/coverage.sh), then the Xcode project from project.yml and both apps' UI tests against the stub (AC_UI_TEST).
#   sh service/ios/scripts/check.sh     exits 0 only when every check passed; needs Xcode
set -eu
cd "$(dirname "$0")/.."
step() { printf '\n== %s\n' "$1"; }
sh scripts/tools.sh
sources="ACKit/Sources ACKit/Tests ACKit/Package.swift CustomerApp TechnicianApp CustomerUITests TechnicianUITests UITestsShared"

part=${1:-all}
case "$part" in all | lint | unit | ui) ;; *) echo "usage: check.sh [all | lint | unit | ui <scheme> [k/n]]" >&2; exit 2 ;; esac

if [ "$part" = all ] || [ "$part" = lint ]; then
  step "SwiftLint"
  .tools/swiftlint/swiftlint lint --strict --quiet
  step "swift-format"
  # shellcheck disable=SC2086 # the source list is split on purpose
  swift format lint --strict -r $sources
fi
if [ "$part" = all ] || [ "$part" = unit ]; then
  step "unit tests and coverage"
  sh scripts/coverage.sh
fi
[ "$part" = all ] || [ "$part" = ui ] || { echo "iOS $part checks passed"; exit 0; }
# the UI tests: both apps, or one (CI runs each, and the Technician's in two halves, as parallel jobs)
schemes="CustomerApp TechnicianApp"
shard=""
if [ "$part" = ui ]; then
  schemes=${2:?"check.sh ui <scheme> [k/n]"}
  shard=${3:-}
fi
# one of n groups of an app's tests (k/n): its test methods in file order, every n-th from the k-th
only=""
if [ -n "$shard" ]; then
  # a malformed group would pass no -only-testing option, that is run every test under a group's name
  printf '%s\n' "$shard" | grep -qxE '[1-9][0-9]*/[1-9][0-9]*' && [ "${shard%/*}" -le "${shard#*/}" ] ||
    { echo "check.sh ui: the group is k/n with 1 <= k <= n, not $shard" >&2; exit 2; }
  k=${shard%/*}
  n=${shard#*/}
  target=$(echo "$schemes" | sed 's/App$//')UITests
  # the groups are read from the one test file: a test in another file of the target would be in no group
  files=$(find "$target" -name '*.swift' | wc -l | tr -d ' ')
  [ "$files" = 1 ] || { echo "check.sh ui: $target has $files Swift files; the groups read only $target/$target.swift" >&2; exit 2; }
  only=$(grep -o 'func test[A-Za-z0-9]*' "$target/$target.swift" | sed 's/func //' |
    awk -v k="$k" -v n="$n" -v t="$target" '(NR - 1) % n == k - 1 { printf "-only-testing:%s/%s/%s ", t, t, $0 }')
  [ -n "$only" ] || { echo "check.sh ui: group $shard of $target has no tests" >&2; exit 2; }
fi
step "UI tests (stub)"
.tools/xcodegen/xcodegen/bin/xcodegen generate --spec project.yml --quiet
# the first available iPhone simulator of the installed runtimes (the runner's and a Mac's differ) that Xcode also offers
# for the apps: CoreSimulator may list one Xcode cannot run on. It is addressed by its id.
offered=$(xcodebuild -project AC.xcodeproj -scheme CustomerApp -showdestinations 2>/dev/null | sed -n 's/.*platform:iOS Simulator, arch:[^,]*, id:\([^,]*\),.*/\1/p')
destination=$(xcrun simctl list devices available | sed -n 's/^ *\(iPhone [^(]*[^ (]\) *(\([0-9A-F-]*\)).*/\2|\1/p' | while IFS='|' read -r id name; do
  if printf '%s\n' "$offered" | grep -qx "$id"; then echo "$id|$name"; break; fi
done)
[ -n "$destination" ] || { echo "no iPhone simulator is installed" >&2; exit 1; }
device_id=${destination%%|*}
device=${destination#*|}
# booted and left a moment first: a fresh simulator's first-boot banners ("Ready for Apple Intelligence") would cover
# the app in the first test's accessibility audit
xcrun simctl boot "$device_id" 2>/dev/null || true
xcrun simctl bootstatus "$device_id" -b >/dev/null
sleep 20
# each app's log stays out of the way unless its tests fail: then its failures are printed (xcodebuild -quiet prints none)
log=$(mktemp)
trap 'rm -f "$log"' EXIT
# each app's result bundle (screenshots, the screen recording, the element tree of a failure) is kept in results/, which
# CI uploads when a test fails: a runner's iOS may differ from this Mac's, and its log says only which check failed
rm -rf results && mkdir -p results
for scheme in $schemes; do
  # shellcheck disable=SC2086 # the -only-testing options are split on purpose
  if ! xcodebuild -project AC.xcodeproj -scheme "$scheme" -destination "platform=iOS Simulator,id=$device_id" test $only \
    -resultBundlePath "results/$scheme.xcresult" >"$log" 2>&1; then
    grep -E "error:|Test Case .* failed|accessibility audit|^(Contrast|Dynamic Type|Hit area|Text clipped|Element)" "$log" || tail -n 60 "$log"
    echo "$scheme: UI tests failed on $device" >&2
    exit 1
  fi
  grep -E "Executed [0-9]+ tests" "$log" | tail -n 1
done
echo "all iOS checks passed"
