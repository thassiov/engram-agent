# engram-agent

[![CI](https://github.com/thassiov/engram-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/thassiov/engram-agent/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/thassiov/engram-agent)](https://goreportcard.com/report/github.com/thassiov/engram-agent)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Observation extraction, embedding, and sync agent for [engram](https://github.com/Gentleman-Programming/engram).

A single Go daemon that automatically extracts structured observations from AI coding sessions, embeds them as vectors for semantic deduplication, and syncs them across machines via PostgreSQL. Works alongside the engram MCP server to provide persistent, searchable memory for AI coding agents.

## How It Works

```
                          YOU ──type──▶ Claude Code
                                            │
                                     (every message)
                                            │
                                    Hook: POST /notify
                                            │
                                            ▼
┌────────────────────────────────────────────────────────────────────┐
│                        engram-agent                                │
│                                                                    │
│   1. WATCH       Count turns. Enough new ones? Extract.            │
│         │                                                          │
│         ▼                                                          │
│   2. CHUNK       Split session into 8-turn chunks (3 overlap)      │
│         │                                                          │
│         ▼                                                          │
│   3. COMPRESS    Send each chunk to ollama ──▶ observations        │
│         │        (semantic compression via LLM)                    │
│         ▼                                                          │
│   4. EMBED       Generate 768-dim vector per observation           │
│         │        (fastembed service, remote or local)              │
│         ▼                                                          │
│   5. DEDUP       Compare vectors against existing observations     │
│         │        cosine similarity > 0.85 = duplicate, skip        │
│         ▼                                                          │
│   6. SAVE        POST unique observations to engram API            │
│         │                                                          │
│         ▼                                                          │
│   7. SYNC        Push to PostgreSQL, pull from other machines      │
│                  (LISTEN/NOTIFY for real-time, polling fallback)   │
└────────────────────────────────────────────────────────────────────┘
                                            │
                                            ▼
                                   engram serve (:7437)
                                   SQLite + FTS5 + MCP
```

## Architecture

```
                         ┌──────────────┐
                         │  Claude Code │
                         └──────┬───────┘
                                │ hook → POST /notify
                                ▼
        ┌───────────────────────────────────────────────┐
        │              engram-agent :7438               │
        │                                               │
        │   hook listener  →  pipeline  →  sync engine  │
        │                         │                     │
        │                         ▼                     │
        │                 internal SQLite               │
        │   sessions · chunks · observations · vectors  │
        └──┬───────────┬────────────┬─────────────┬─────┘
           │           │            │             │
           ▼           ▼            ▼             ▼
       ┌───────┐  ┌─────────┐  ┌─────────┐  ┌───────────┐
       │ ollama│  │fastembed│  │ engram  │  │PostgreSQL │
       │  LLM  │  │  :8491  │  │  :7437  │  │    hub    │
       └───────┘  └─────────┘  └─────────┘  └───────────┘
       compress      embed        save          sync
```

The agent is a single Go binary, but talks to four external systems: ollama for LLM-based extraction, fastembed for vectors, the engram HTTP API for persistence, and PostgreSQL for cross-machine sync. All four are optional in degraded modes (see [Optional Components](#optional-components)).

## Observation Types

The extraction pipeline produces structured observations with these types:

| Type | What it captures |
|------|-----------------|
| `decision` | A choice made between alternatives (what and why) |
| `config` | A configuration file created or changed |
| `preference` | How the user wants things done |
| `discovery` | Something non-obvious learned about the system |
| `bugfix` | A problem identified and fixed (root cause + fix) |
| `architecture` | A system design choice (design + tradeoffs) |
| `pattern` | A convention or approach established |

## Features

- **Automatic extraction** — hooks into Claude Code session lifecycle, extracts every 15 turns
- **Semantic compression** — LLM-based extraction via ollama (configurable model)
- **Vector dedup** — cosine similarity check prevents duplicate observations
- **Cross-machine sync** — CDC-based push/pull via PostgreSQL with LISTEN/NOTIFY
- **Scope filtering** — work machines only get preferences and configs, not personal project data
- **Offline resilient** — extraction works without PG or embedding service, syncs catch up later
- **Force extraction** — on-demand processing with optional full reset
- **Event tracking** — administrative actions (resets, force extracts) are recorded
- **Systemd integration** — watchdog, ready notification, graceful shutdown

## Install

```bash
git clone https://github.com/thassiov/engram-agent.git
cd engram-agent

# Build
make build

# Install binary to ~/.local/bin
make install-local

# Install + create systemd service + start
make install-service
```

## Configuration

Create `~/.config/engram/agent.json`:

```json
{
  "machine_id": "my-machine",
  "scope": "personal",
  "engram_db": "~/.engram/engram.db",
  "engram_api": "http://127.0.0.1:7437",
  "listen_addr": "127.0.0.1:7438",
  "ollama_url": "http://localhost:11434",
  "ollama_model": "gemma3n:e4b",
  "embed_url": "http://localhost:8491",
  "embed_dims": 768,
  "dedup_threshold": 0.85,
  "pull_filter": "all",
  "postgres": {
    "host": "postgres.example.com",
    "port": 5432,
    "database": "knowledge",
    "user": "engram",
    "password": "secret",
    "sslmode": "disable"
  }
}
```

### Configuration Reference

| Field | Default | Description |
|-------|---------|-------------|
| `machine_id` | *required* | Unique identifier for this machine |
| `scope` | `personal` | `personal` or `work` — determines what gets synced |
| `engram_db` | `~/.engram/engram.db` | Path to engram's SQLite database (read-only, for sync) |
| `engram_api` | `http://127.0.0.1:7437` | Engram HTTP API URL |
| `listen_addr` | `127.0.0.1:7438` | Address for the hook notification listener |
| `ollama_url` | `http://127.0.0.1:11434` | Ollama API URL for observation extraction |
| `ollama_model` | `gemma3n:e4b` | Model name for extraction |
| `embed_url` | *(empty)* | Fastembed service URL. Omit to skip embedding/dedup |
| `embed_dims` | `768` | Embedding dimensions (must match the model) |
| `dedup_threshold` | `0.85` | Cosine similarity threshold for dedup (0.0-1.0) |
| `pull_filter` | `all` | `"all"` or `{"types": ["preference", "config"]}` |
| `postgres` | *(empty)* | PG connection. Omit to disable cross-machine sync |

### Optional Components

Everything except the core extraction is optional:

```
                   ┌─────────────────────────────────────────┐
                   │         What works without it           │
                   ├─────────────────────────────────────────┤
No embed_url    →  │ Extraction works, no dedup              │
No postgres     →  │ Extraction + dedup work, no sync        │
No ollama       →  │ Chunks queued, processed when available │
                   └─────────────────────────────────────────┘
```

## Usage

### Daemon

```bash
# Run with default config (~/.config/engram/agent.json)
engram-agent daemon

# Run with custom config
engram-agent daemon -c /path/to/config.json
```

### Status

```bash
engram-agent status
# Machine:     portus
# Scope:       personal
# Engram DB:   /home/user/.engram/engram.db
# Engram API:  http://127.0.0.1:7437
# Listen:      127.0.0.1:7438
# Ollama:      http://10.0.0.105:11434 (gemma3n:e4b)
# Pull filter: all
# Push cursor: 463
# Pull cursor: 0
# PG mutations: 463
```

### Force Extraction

```bash
# Extract now (skip the 15-turn threshold)
curl -X POST http://localhost:7438/notify \
  -H "Content-Type: application/json" \
  -d '{"session_id":"<SESSION_ID>", "event":"force"}'

# Full reset + re-extract (wipe all data for this session)
curl -X POST http://localhost:7438/notify \
  -H "Content-Type: application/json" \
  -d '{"session_id":"<SESSION_ID>", "event":"force", "reset":true}'
```

### Health Check

```bash
curl http://localhost:7438/health
# {"status":"ok"}
```

## Claude Code Hook Setup

Add to `~/.claude/settings.local.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [{
      "hooks": [{
        "type": "command",
        "command": "curl -sf -X POST http://localhost:7438/notify -H 'Content-Type: application/json' -d \"$(cat)\" > /dev/null 2>&1 &",
        "timeout": 2
      }]
    }],
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "curl -sf -X POST http://localhost:7438/notify -H 'Content-Type: application/json' -d \"$(cat | jq -c '. + {event: \"stop\"}')\" > /dev/null 2>&1 &",
        "timeout": 5
      }]
    }]
  }
}
```

## Embedding Service

engram-agent talks to a fastembed HTTP service for vector generation. Deploy it with Docker:

```bash
# Dockerfile
FROM python:3.11-slim
WORKDIR /app
RUN pip install --no-cache-dir fastembed flask
COPY server.py .
EXPOSE 8491
CMD ["python", "server.py"]
```

```python
# server.py
import os
from flask import Flask, request, jsonify
from fastembed import TextEmbedding

model_name = os.environ.get("FASTEMBED_MODEL", "BAAI/bge-base-en-v1.5")
model = TextEmbedding(model_name=model_name)
app = Flask(__name__)

@app.route("/embeddings", methods=["POST"])
def embeddings():
    texts = request.get_json().get("texts", [])
    vectors = list(model.embed(texts))
    return jsonify({"embeddings": [v.tolist() for v in vectors]})

@app.route("/health")
def health():
    return jsonify({"status": "ok"})

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8491)
```

```bash
docker build -t fastembed-service .
docker run -d --name fastembed --restart unless-stopped \
  -p 8491:8491 \
  -e FASTEMBED_MODEL=BAAI/bge-base-en-v1.5 \
  fastembed-service
```

## Internal State

All state is stored in a single SQLite database at `~/.local/share/engram-agent/state.db`:

```
state.db
├── session_state    # Per-session tracking (turns, status)
├── chunks           # Processed chunks with full text content
├── observations     # Extracted observations (pending/saved/duplicate)
├── vectors          # 768-dim embeddings for dedup
├── events           # Administrative actions audit trail
└── logs             # Structured operation logs
```

## Sync Architecture

```
         ┌─────────┐                         ┌─────────┐
         │ Machine │                         │ Machine │
         │    A    │                         │    B    │
         │engram.db│                         │engram.db│
         └────┬────┘                         └────┬────┘
              │                                   │
              │  push (30s)             push (30s)│
              ▼                                   ▼
         ┌─────────────────────────────────────────────┐
         │          PostgreSQL (central hub)           │
         │                                             │
         │   engram_sync_mutations  (CDC log)          │
         │   engram_sync_cursors    (watermarks)       │
         │   engram_machines        (scope registry)   │
         │                                             │
         │   NOTIFY 'engram_sync' on INSERT            │
         └──────────────────────┬──────────────────────┘
                                │
              ┌─────────────────┴─────────────────┐
              │ LISTEN + poll (60s fallback)      │
              ▼                                   ▼
    A pulls B's mutations               B pulls A's mutations
      (scope-filtered)                    (scope-filtered)
```

### Scope Filtering

| Machine scope | What it pulls |
|--------------|---------------|
| `personal` | All observation types from all machines |
| `work` | Only `preference` and `config` types from personal machines |

## Development

```bash
# Install dev tools (golangci-lint, gosec, goimports)
make tools

# Build
make build

# Run tests
make test

# Full quality check (fmt, tidy, vet, test, build)
make check

# Full CI pipeline (includes lint + coverage)
make ci

# Rebuild on changes (requires entr)
make watch

# All available targets
make help
```

## Systemd Service

```bash
# Install binary + create + enable + start service
make install-service

# Check status
systemctl --user status engram-agent.service

# View logs
journalctl --user -u engram-agent.service -f

# Stop
systemctl --user stop engram-agent.service

# Remove service completely
make uninstall-service
```

## Project Status

Under active development. Current state:

- [x] Phase 1: Sync engine (push/pull/LISTEN) + HTTP hook listener
- [x] Phase 2: Observation extraction pipeline (chunk, compress, save)
- [x] Phase 3: Vector embedding + cosine similarity dedup
- [x] Phase 4: Fastembed service deployment + model selection
- [ ] Include assistant responses in extraction chunks
- [ ] Improve extraction selectivity (skip trivial observations)

## License

MIT
