# engram-agent

[![CI](https://github.com/thassiov/engram-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/thassiov/engram-agent/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/thassiov/engram-agent)](https://goreportcard.com/report/github.com/thassiov/engram-agent)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Observation extraction, embedding, and sync agent for [engram](https://github.com/Gentleman-Programming/engram).

A single Go daemon that consolidates session observation extraction (via semantic compression), vector embedding, cross-machine sync, and deduplication into one binary. Works alongside the engram MCP server to provide persistent, searchable memory for AI coding agents.

## What it does

- **Listens** for Claude Code session hooks (UserPromptSubmit, Stop) via HTTP
- **Extracts** observations from session transcripts using a local LLM (ollama)
- **Embeds** observations as vectors for semantic search and deduplication (FastEmbed)
- **Deduplicates** across sessions using vector cosine similarity
- **Syncs** observations across machines via PostgreSQL (CDC with LISTEN/NOTIFY)
- **Falls back** gracefully when remote services are unreachable

## Architecture

```
Claude Code ──hook──> engram-agent ──ollama──> observations
                           │                       │
                           │                  fastembed
                           │                       │
                           │                  384-dim vectors
                           │                       │
                           ├── dedup (cosine similarity)
                           ├── save to engram API (:7437)
                           └── sync to PostgreSQL
```

A single daemon that handles the full lifecycle from session capture to cross-machine availability.

## Install

```bash
# From source
make build
make install-local

# Or directly
go install github.com/thassiov/engram-agent/cmd/engram-agent@latest
```

## Usage

```bash
# Run the agent daemon
engram-agent daemon

# Check version
engram-agent version
```

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
```

## Project status

Under active development. Currently implementing Phase 1 (sync engine absorption + HTTP listener).

## License

MIT
