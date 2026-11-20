#!/bin/sh
# The iOS build tools at fixed releases, checked by their SHA-256, in service/ios/.tools (not committed): XcodeGen writes
# the Xcode project from project.yml, SwiftLint checks the sources. The same files locally and in CI.
#   sh scripts/tools.sh     downloads what is missing; prints nothing when both are there
set -eu
cd "$(dirname "$0")/.."
XCODEGEN_VERSION=2.46.0
XCODEGEN_SHA256=4d9e34b62172d645eed6457cac13fc222569974098ef4ee9c3368bedf0196806
SWIFTLINT_VERSION=0.65.1
SWIFTLINT_SHA256=c1e429b0599cf1b516f369a2d9ec04eaf0e436f3c12b637df8851fa52ff694d0

# fetch NAME URL SHA256 DIR: downloads the zip, refuses it unless its digest matches, unpacks it into .tools/DIR
fetch() {
  tmp=$(mktemp -d)
  curl -fsSL -o "$tmp/$1.zip" "$2"
  echo "$3  $tmp/$1.zip" | shasum -a 256 -c - >/dev/null || { echo "$1: the download's SHA-256 is not $3" >&2; rm -rf "$tmp"; exit 1; }
  rm -rf ".tools/$4" && mkdir -p ".tools/$4" && unzip -q "$tmp/$1.zip" -d ".tools/$4"
  rm -rf "$tmp"
}
[ "$(cat .tools/xcodegen/VERSION 2>/dev/null)" = "$XCODEGEN_VERSION" ] || {
  fetch xcodegen "https://github.com/yonaskolb/XcodeGen/releases/download/$XCODEGEN_VERSION/xcodegen.zip" "$XCODEGEN_SHA256" xcodegen
  echo "$XCODEGEN_VERSION" >.tools/xcodegen/VERSION
}
[ "$(cat .tools/swiftlint/VERSION 2>/dev/null)" = "$SWIFTLINT_VERSION" ] || {
  fetch swiftlint "https://github.com/realm/SwiftLint/releases/download/$SWIFTLINT_VERSION/portable_swiftlint.zip" "$SWIFTLINT_SHA256" swiftlint
  echo "$SWIFTLINT_VERSION" >.tools/swiftlint/VERSION
}
