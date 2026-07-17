# treehouse — Wayfinder Map

Label: `wayfinder:map`
Created: 2026-07-17

## What It Is

Git worktree pool manager — acquire/release isolated worktrees for parallel
agent tasks. RETIRED from bridge use: bridge's `internal/worktree` (pure Go
`git worktree add/remove + flock`) replaced it.

Forked from `kunchenguid/treehouse`. Upstream has active community (13 open PRs).

## Current State

- **Branch:** `main` — clean
- **Build:** ✅ go build, go test, go vet all pass
- **CI:** ✅ GitHub Actions (3-OS matrix) + release pipeline
- **Last commit:** `43b06f5` (2026-07-16) AGENTS.md header fix
- **Status:** Frozen for bridge. Follows upstream. No active development from captain.

## Relationship to Bridge

Bridge doc says: "uses `internal/worktree` (pure Go git worktree add/remove, no
external treehouse dependency). The external `treehouse` binary is no longer
required for spawn."

Bridge's `internal/worktree/worktree.go` documents bisimulation equivalence with
treehouse: `treehouse "get --lease"` equals `Acquire()`, etc.

## Tickets

### 🔵 Future

1. **Archive if no upstream need** — the fork exists only for historical continuity. If upstream activity ceases or the captain never uses it directly, archive it.

2. **13 open upstream PRs** — would any of these benefit our fork? (Worktree recovery, bare-repo support, environment variables docs)
