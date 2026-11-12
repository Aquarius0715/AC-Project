#!/bin/sh
# Runs review-loop-prompt.md over and over, one fresh `claude -p` session per round.
# State lives only in docs/05-document-review/review-log.md, so each round starts with an empty context.
#
# Stop:  touch .review-loop-stop   (checked between rounds; the current round finishes first)
# Env:   PERM_MODE   permission mode for claude -p (default: acceptEdits; see note below)
#        SLEEP_SEC   pause between rounds (default: 60)
#        MAX_ROUNDS  stop after this many rounds (default: 0 = no limit)
#        MAX_FAILS   stop after this many failed rounds in a row (default: 3)
#        BUDGET_USD  per-round spend cap passed to claude (default: 15)
#
# Note: in -p mode nobody can answer a permission prompt, so a command that is not allowed fails the round.
# acceptEdits allows file edits only. The prompt runs git, make, npm, gh, docker, so either allow those in
# .claude/settings.json, or set PERM_MODE=bypassPermissions and run this only where that is acceptable.

cd "$(dirname "$0")/.." || exit 1

PERM_MODE=${PERM_MODE:-acceptEdits}
SLEEP_SEC=${SLEEP_SEC:-60}
MAX_ROUNDS=${MAX_ROUNDS:-0}
MAX_FAILS=${MAX_FAILS:-3}
BUDGET_USD=${BUDGET_USD:-15}
STOP_FILE=.review-loop-stop
LOG_DIR=${LOG_DIR:-.review-loop-logs}
PROMPT_FILE=review-loop-prompt.md

[ -f "$PROMPT_FILE" ] || { echo "missing $PROMPT_FILE" >&2; exit 1; }
mkdir -p "$LOG_DIR"

# The file starts with "/loop "; a one-shot session needs the body only, and must not self-schedule.
PROMPT=$(sed '1s|^/loop ||' "$PROMPT_FILE")
PROMPT="$PROMPT

【この実行について】外側のシェルループが回を繰り返す。この会話では 1 回（1 PR）だけ行い、記録を書いたら終了する。ScheduleWakeup は使わない。"

n=0
fails=0
while :; do
  if [ -e "$STOP_FILE" ]; then
    echo "stop file found, exiting"
    exit 0
  fi
  if [ "$MAX_ROUNDS" -gt 0 ] && [ "$n" -ge "$MAX_ROUNDS" ]; then
    echo "reached MAX_ROUNDS=$MAX_ROUNDS"
    exit 0
  fi
  n=$((n + 1))
  log="$LOG_DIR/round-$(date +%Y%m%d-%H%M%S).log"
  echo "[$(date '+%F %T')] round $n -> $log"
  claude -p "$PROMPT" --permission-mode "$PERM_MODE" --max-budget-usd "$BUDGET_USD" >"$log" 2>&1
  rc=$?
  echo "[$(date '+%F %T')] round $n rc=$rc"
  if [ "$rc" -eq 0 ]; then
    fails=0
  else
    fails=$((fails + 1))
    if [ "$fails" -ge "$MAX_FAILS" ]; then
      echo "$fails failed rounds in a row, exiting (see $log)" >&2
      exit 1
    fi
  fi
  sleep "$SLEEP_SEC"
done
