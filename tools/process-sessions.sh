#!/bin/bash
# Process sessions through engram-agent one at a time.
# Waits for each extraction to complete before triggering the next.
#
# Usage:
#   process-sessions.sh                     # process all unprocessed sessions in ~/.claude/projects/
#   process-sessions.sh --dir <path>        # process sessions from a specific directory
#   process-sessions.sh --file <list.txt>   # process session IDs from a file (one per line)
#   process-sessions.sh --reprocess         # reprocess all sessions (reset + force)

AGENT_URL="http://127.0.0.1:7438"
STATE_DB="$HOME/.local/share/engram-agent/state.db"
POLL_INTERVAL=10
DIR=""
FILE=""
REPROCESS=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) DIR="$2"; shift 2 ;;
    --file) FILE="$2"; shift 2 ;;
    --reprocess) REPROCESS=true; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# Collect session IDs to process.
declare -a SESSIONS

if [ -n "$FILE" ]; then
  while IFS= read -r sid; do
    [ -n "$sid" ] && SESSIONS+=("$sid")
  done < "$FILE"
elif [ -n "$DIR" ]; then
  for f in "$DIR"/*.jsonl; do
    [ -f "$f" ] || continue
    sid=$(basename "$f" .jsonl)
    SESSIONS+=("$sid")
  done
else
  # Default: find all sessions in ~/.claude/projects/ not yet processed
  for f in $(find "$HOME/.claude/projects" -name "*.jsonl" -not -path "*/subagents/*"); do
    sid=$(basename "$f" .jsonl)
    processed=$(sqlite3 "$STATE_DB" "SELECT last_turn FROM session_state WHERE session_id='$sid';" 2>/dev/null)
    if [ -z "$processed" ] || [ "$processed" = "0" ] || [ "$REPROCESS" = true ]; then
      SESSIONS+=("$sid")
    fi
  done
fi

TOTAL=${#SESSIONS[@]}
if [ "$TOTAL" -eq 0 ]; then
  echo "No sessions to process."
  exit 0
fi

echo "=== Processing $TOTAL sessions ==="
echo ""

SUCCESS=0
FAILED=0
SKIPPED=0

for i in "${!SESSIONS[@]}"; do
  sid="${SESSIONS[$i]}"
  num=$((i + 1))

  # Check if engram-agent is healthy.
  if ! curl -sf "$AGENT_URL/health" --max-time 2 > /dev/null 2>&1; then
    echo "[$num/$TOTAL] ERROR: engram-agent not responding. Stopping."
    break
  fi

  # Get current state before extraction.
  turn_before=$(sqlite3 "$STATE_DB" "SELECT COALESCE(last_turn, 0) FROM session_state WHERE session_id='$sid';" 2>/dev/null || echo "-1")

  echo -n "[$num/$TOTAL] $sid ... "

  # Ensure session exists in engram server (needed for imported sessions
  # that weren't created by the engram plugin at session start).
  ENGRAM_URL="http://127.0.0.1:7437"
  curl -sf -X POST "$ENGRAM_URL/sessions" \
    -H "Content-Type: application/json" \
    -d "{\"id\":\"$sid\",\"project\":\"import\",\"directory\":\"/tmp\"}" \
    > /dev/null 2>&1

  # Build payload.
  if [ "$REPROCESS" = true ]; then
    payload="{\"session_id\":\"$sid\",\"event\":\"force\",\"reset\":true}"
  else
    payload="{\"session_id\":\"$sid\",\"event\":\"force\"}"
  fi

  # Trigger extraction.
  http_code=$(curl -sf -o /dev/null -w "%{http_code}" -X POST "$AGENT_URL/notify" \
    -H "Content-Type: application/json" \
    -d "$payload" --max-time 5)

  if [ "$http_code" != "202" ]; then
    echo "FAILED (HTTP $http_code)"
    FAILED=$((FAILED + 1))
    continue
  fi

  # Wait for extraction to complete by polling session state.
  # The session's last_turn will increase when extraction finishes.
  waited=0
  max_wait=600  # 10 minutes max per session
  while [ $waited -lt $max_wait ]; do
    sleep $POLL_INTERVAL
    waited=$((waited + POLL_INTERVAL))

    # Check if last_turn has changed (extraction done).
    last_turn=$(sqlite3 "$STATE_DB" "SELECT COALESCE(last_turn, 0) FROM session_state WHERE session_id='$sid';" 2>/dev/null || echo "-1")
    obs_count=$(sqlite3 "$STATE_DB" "SELECT count(*) FROM observations WHERE session_id='$sid' AND status='saved';" 2>/dev/null || echo "0")

    if [ "$last_turn" != "$turn_before" ] && [ "$last_turn" != "-1" ]; then
      echo "done (${last_turn} turns, ${obs_count} observations, ${waited}s)"
      SUCCESS=$((SUCCESS + 1))
      break
    fi
  done

  if [ $waited -ge $max_wait ]; then
    echo "TIMEOUT after ${max_wait}s"
    FAILED=$((FAILED + 1))
  fi
done

echo ""
echo "=== Complete: $SUCCESS succeeded, $FAILED failed, $SKIPPED skipped ==="
