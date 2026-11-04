#!/bin/sh
# The integration suite (test/integration) in SHARDS parallel processes, each on a fresh database of its own:
#   SHARDS=3 sh scripts/test-shards.sh             one server (ac_test_s1 ... ac_test_s3)
#   SHARDS=3 CLUSTER=1 sh scripts/test-shards.sh   the split services (ac_test_cluster_s1 ...)
# Every test builds its own server on AC_TEST_DB, so tests on different databases never meet; within a shard they run
# one after the other in the suite's order, as in one process. The tests go to the shards in turn. The suite is compiled
# once. Each shard's output follows the others' at the end, and the exit status is 1 when any shard failed. Needs what
# scripts/resetdb.sh needs (PG_ADMIN_URL in CI). RESET=0 keeps the shards' databases as they are, RESET=only rebuilds
# them and stops: make test-all rebuilds every database first, one after the other, then runs both suites at once.
set -eu
cd "$(dirname "$0")/.."
SHARDS=${SHARDS:-3}
TIMEOUT=${TIMEOUT:-30m}
base=ac_test
if [ "${CLUSTER:-}" = 1 ]; then base=ac_test_cluster; fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

RESET=${RESET:-1}
if [ "$RESET" != 0 ]; then
  # the databases one after the other: their migrations update the cluster-wide login roles
  i=1
  while [ "$i" -le "$SHARDS" ]; do
    DB=${base}_s$i sh scripts/resetdb.sh >"$work/resetdb.log" 2>&1 || { cat "$work/resetdb.log"; exit 1; }
    i=$((i + 1))
  done
  if [ "$RESET" = only ]; then exit 0; fi
fi

GOTOOLCHAIN=${GOTOOLCHAIN:-auto} go test -c -o "$work/integration.test" ./test/integration
tests=$(cd test/integration && "$work/integration.test" -test.list '.*' | grep '^Test')

i=1
while [ "$i" -le "$SHARDS" ]; do
  echo "$tests" | awk -v n="$SHARDS" -v i="$i" '(NR - 1) % n == i - 1' | paste -sd '|' - >"$work/run-$i"
  i=$((i + 1))
done

i=1
while [ "$i" -le "$SHARDS" ]; do
  (
    cd test/integration
    if AC_TEST_DB=${base}_s$i AC_TEST_CLUSTER=${CLUSTER:-} "$work/integration.test" -test.count=1 \
      -test.timeout "$TIMEOUT" -test.run "^($(cat "$work/run-$i"))\$" >"$work/shard-$i.log" 2>&1; then
      echo 0 >"$work/status-$i"
    else
      echo 1 >"$work/status-$i"
    fi
  ) &
  i=$((i + 1))
done
wait

failed=0
i=1
while [ "$i" -le "$SHARDS" ]; do
  status=$(cat "$work/status-$i")
  count=$(tr '|' '\n' <"$work/run-$i" | wc -l | tr -d ' ')
  echo "=== shard $i/$SHARDS on ${base}_s$i: $count tests, exit $status"
  cat "$work/shard-$i.log"
  [ "$status" = 0 ] || failed=1
  i=$((i + 1))
done
exit "$failed"
