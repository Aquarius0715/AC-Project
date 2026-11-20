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

step "SwiftLint"
.tools/swiftlint/swiftlint lint --strict --quiet
step "swift-format"
# shellcheck disable=SC2086 # the source list is split on purpose
swift format lint --strict -r $sources
step "unit tests and coverage"
sh scripts/coverage.sh
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
for scheme in CustomerApp TechnicianApp; do
  if ! xcodebuild -project AC.xcodeproj -scheme "$scheme" -destination "platform=iOS Simulator,id=$device_id" test \
    -resultBundlePath "results/$scheme.xcresult" >"$log" 2>&1; then
    grep -E "error:|Test Case .* failed|accessibility audit|^(Contrast|Dynamic Type|Hit area|Text clipped|Element)" "$log" || tail -n 60 "$log"
    echo "$scheme: UI tests failed on $device" >&2
    exit 1
  fi
  grep -E "Executed [0-9]+ tests" "$log" | tail -n 1
done
echo "all iOS checks passed"
