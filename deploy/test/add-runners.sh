#!/usr/bin/env bash
# Adds GitHub Actions runners on the lab server (DEC-95): ~/actions-runner-2, ~/actions-runner-3, … beside the first
# runner ~/actions-runner, each with the label ac-test and a service of its own, so the CI jobs of a push run in
# parallel. Run it on the server as the runner's user; sudo is asked for the services. One registration token registers
# them all: the repository's Settings > Actions > Runners > New self-hosted runner shows one, valid for an hour. The
# token is read here, never echoed, and reaches config.sh through its environment, not a command line that other users
# of the server could see.
#
#   bash add-runners.sh [COUNT]   (2 by default: 3 runners in all)
set -euo pipefail

count=${1:-2}
url=https://github.com/Aquarius0715/AC-Project

read -rsp "Runner registration token: " token
echo
[ -n "$token" ] || {
  echo "add-runners: no token" >&2
  exit 1
}

# the runner package: the one the first runner came from, else the latest release
# shellcheck disable=SC2012 # the names are the runner releases, and sort -V orders their versions
tarball=$(ls "$HOME"/actions-runner/actions-runner-linux-x64-*.tar.gz 2>/dev/null | sort -V | tail -n 1 || true)
if [ -z "$tarball" ]; then
  version=$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest |
    python3 -c 'import json, sys; print(json.load(sys.stdin)["tag_name"].lstrip("v"))')
  tarball=$(mktemp --suffix=.tar.gz)
  curl -fsSL -o "$tarball" "https://github.com/actions/runner/releases/download/v$version/actions-runner-linux-x64-$version.tar.gz"
fi
echo "runner package: $tarball"

for i in $(seq 2 $((count + 1))); do
  dir=$HOME/actions-runner-$i
  if [ -f "$dir/.runner" ]; then
    echo "$dir is registered already"
    continue
  fi
  mkdir -p "$dir"
  tar xzf "$tarball" -C "$dir"
  (
    cd "$dir"
    ACTIONS_RUNNER_INPUT_TOKEN=$token ./config.sh --unattended --url "$url" --labels ac-test --name "$(hostname)-$i" \
      --work _work
    sudo ./svc.sh install "$(id -un)"
    sudo ./svc.sh start
  )
  echo "runner $(hostname)-$i is on ($dir)"
done
echo "The repository's Settings > Actions > Runners lists them; each takes one job at a time."
