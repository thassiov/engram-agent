#!/bin/bash
# Convert KB vault sessions to Claude-compatible JSONL files.
# Wraps each markdown session as a single user message so engram-agent
# can extract observations from it.
#
# Usage:
#   kb-sessions-to-jsonl.sh [--before DATE] [--output DIR]
#   kb-sessions-to-jsonl.sh --before 2026-01  # only pre-Jan 2026 sessions
#   kb-sessions-to-jsonl.sh                    # all sessions

OUTPUT_DIR="$HOME/.claude/projects/-kb-import"
BEFORE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --before) BEFORE="$2"; shift 2 ;;
    --output) OUTPUT_DIR="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

mkdir -p "$OUTPUT_DIR"

# Get all session IDs from KB.
# kb list outputs: "session  DATE  SCOPE  TITLE"
# kb search returns IDs. We need to get IDs for all sessions.
# Use kb search with a broad query per date range.

echo "Fetching session list from KB vault..."

# Get session IDs by searching. We iterate through known date prefixes.
TMPFILE=$(mktemp)
for year in 2024 2025 2026; do
  for month in $(seq -w 1 12); do
    kb search "$year-$month" --type session --limit 100 2>/dev/null | grep "ID:" | awk '{print $2}' >> "$TMPFILE"
  done
done

# Also try a broad search to catch any we missed.
kb search "Claude Code Session" --type session --limit 300 2>/dev/null | grep "ID:" | awk '{print $2}' >> "$TMPFILE"
kb search "session" --type session --limit 300 2>/dev/null | grep "ID:" | awk '{print $2}' >> "$TMPFILE"

# Deduplicate.
sort -u "$TMPFILE" > "${TMPFILE}.dedup"
mv "${TMPFILE}.dedup" "$TMPFILE"

TOTAL=$(wc -l < "$TMPFILE")
echo "Found $TOTAL unique session IDs"

converted=0
skipped=0
filtered=0

while IFS= read -r session_id; do
  [ -z "$session_id" ] && continue

  # Get the session content.
  content=$(kb show "$session_id" 2>/dev/null)
  if [ -z "$content" ]; then
    skipped=$((skipped + 1))
    continue
  fi

  # Extract date from the header (line starting with "Date:")
  session_date=$(echo "$content" | grep "^Date:" | head -1 | awk '{print $2}')

  # Filter by date if --before specified.
  if [ -n "$BEFORE" ] && [ -n "$session_date" ]; then
    if [[ "$session_date" > "$BEFORE" ]] || [[ "$session_date" == "$BEFORE"* ]]; then
      filtered=$((filtered + 1))
      continue
    fi
  fi

  # Skip if already converted.
  outfile="$OUTPUT_DIR/kb-${session_id}.jsonl"
  if [ -f "$outfile" ]; then
    skipped=$((skipped + 1))
    continue
  fi

  # Strip the KB metadata header (everything before the === line).
  body=$(echo "$content" | sed '1,/^====/d')
  if [ -z "$body" ]; then
    body="$content"
  fi

  # Generate a stable session ID for engram-agent (prefix with kb- to avoid collisions).
  sid="kb-${session_id}"

  # Write as a single user message JSONL.
  python3 -c "
import json, sys
text = sys.stdin.read()
line = {
    'type': 'user',
    'message': {'role': 'user', 'content': text},
    'sessionId': '$sid'
}
print(json.dumps(line, ensure_ascii=False))
" <<< "$body" > "$outfile"

  converted=$((converted + 1))
  if [ $((converted % 10)) -eq 0 ]; then
    echo "  progress: $converted converted..."
  fi
done < "$TMPFILE"

rm -f "$TMPFILE"

echo ""
echo "Done: $converted converted, $skipped skipped, $filtered filtered out by date"
echo "Output: $OUTPUT_DIR"
