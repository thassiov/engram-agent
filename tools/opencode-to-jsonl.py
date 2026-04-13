#!/usr/bin/env python3
"""Convert OpenCode sessions from SQLite to Claude-compatible JSONL files.

Reads sessions and messages from opencode.db, reconstructs conversation turns,
and writes JSONL files that engram-agent can process.

Usage:
    opencode-to-jsonl.py [--db PATH] [--output DIR] [--min-messages N]
"""

import argparse
import json
import os
import sqlite3
import sys


def get_sessions(db, min_messages):
    """Get sessions that have enough text content to be worth processing."""
    cur = db.execute("""
        SELECT s.id, s.title, s.directory, s.time_created
        FROM session s
        WHERE (
            SELECT count(DISTINCT m.id)
            FROM message m
            JOIN part p ON p.message_id = m.id
            WHERE m.session_id = s.id
            AND json_extract(p.data, '$.type') = 'text'
        ) >= ?
        ORDER BY s.time_created
    """, (min_messages,))
    return cur.fetchall()


def get_messages_with_text(db, session_id):
    """Get messages for a session, ordered by time, with their text parts."""
    cur = db.execute("""
        SELECT m.id, json_extract(m.data, '$.role') as role, m.time_created
        FROM message m
        WHERE m.session_id = ?
        ORDER BY m.time_created
    """, (session_id,))
    messages = cur.fetchall()

    result = []
    for msg_id, role, time_created in messages:
        if role not in ('user', 'assistant'):
            continue

        # Get text parts for this message.
        pcur = db.execute("""
            SELECT json_extract(data, '$.text')
            FROM part
            WHERE message_id = ?
            AND json_extract(data, '$.type') = 'text'
            ORDER BY time_created
        """, (msg_id,))
        texts = [row[0] for row in pcur if row[0] and row[0].strip()]

        if not texts:
            continue

        content = "\n".join(texts)
        result.append({
            "role": role,
            "content": content,
            "time_created": time_created,
        })

    return result


def to_jsonl(session_id, messages):
    """Convert messages to Claude-compatible JSONL lines.

    Merges consecutive messages of the same role into one line,
    since OpenCode creates separate message rows for each assistant step.
    """
    # Merge consecutive same-role messages.
    merged = []
    for msg in messages:
        if merged and merged[-1]["role"] == msg["role"]:
            merged[-1]["content"] += "\n" + msg["content"]
        else:
            merged.append(dict(msg))

    lines = []
    for msg in merged:
        if msg["role"] == "user":
            line = {
                "type": "user",
                "message": {"role": "user", "content": msg["content"]},
                "sessionId": session_id,
            }
        else:
            line = {
                "type": "assistant",
                "message": {
                    "role": "assistant",
                    "content": [{"type": "text", "text": msg["content"]}],
                },
                "sessionId": session_id,
            }
        lines.append(json.dumps(line, ensure_ascii=False))
    return lines


def main():
    parser = argparse.ArgumentParser(description="Convert OpenCode sessions to JSONL")
    parser.add_argument("--db", default=os.path.expanduser("~/.local/share/opencode/opencode.db"),
                        help="Path to opencode.db")
    parser.add_argument("--output", default=os.path.expanduser("~/.claude/projects/-opencode-import"),
                        help="Output directory for JSONL files")
    parser.add_argument("--min-messages", type=int, default=4,
                        help="Minimum text messages per session (default: 4)")
    args = parser.parse_args()

    if not os.path.exists(args.db):
        print(f"Database not found: {args.db}", file=sys.stderr)
        sys.exit(1)

    os.makedirs(args.output, exist_ok=True)

    db = sqlite3.connect(f"file:{args.db}?mode=ro", uri=True)
    sessions = get_sessions(db, args.min_messages)
    print(f"Found {len(sessions)} sessions with >= {args.min_messages} text messages")

    converted = 0
    skipped = 0
    for session_id, title, directory, time_created in sessions:
        messages = get_messages_with_text(db, session_id)
        if len(messages) < args.min_messages:
            skipped += 1
            continue

        lines = to_jsonl(session_id, messages)
        outpath = os.path.join(args.output, f"{session_id}.jsonl")
        with open(outpath, "w") as f:
            f.write("\n".join(lines) + "\n")

        converted += 1

    db.close()
    print(f"Converted: {converted}, Skipped: {skipped}")
    print(f"Output: {args.output}")


if __name__ == "__main__":
    main()
