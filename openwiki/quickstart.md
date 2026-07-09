# Treehouse — OpenWiki Quickstart

> "Manage worktrees without managing worktrees."

Treehouse is a single-binary **Go CLI** that maintains a **pool of reusable, pre-warmed git worktrees** per repository. It exists to make parallel AI-coding-agent workflows fast and conflict-free: instead of cloning a repo (and losing dependencies and build cache) for every agent session, an agent asks treehouse for a worktree, works in an isolated environment, and returns it to the pool when done — dependencies and build caches intact.

This wiki is the entry point for humans and future coding agents. Read this page first, then follow the links to the section that matches your task.

## What treehouse does

- **Instant isolation** — `treehouse` drops you into a clean, detached-HEAD worktree in a subshell.
- **Reusable worktrees** — on exit the worktree is reset and returned to a pool, keeping build artifacts and installed dependencies for the next agent.
- **Conflict-free** — treehouse detects in-use worktrees (by scanning running processes and short-lived owner reservations) so agents never collide.
- **No daemon** — every operation is an inline CLI command. There is no background service and no long-lived mutable state beyond a small JSON state file per pool.

The canonical user-facing overview lives in [`/README.md`](../README.md); the maintainer/agent design notes live in [`/AGENTS.md`](../AGENTS.md).

## How it works (30-second version)

1. Find the repo root, `git fetch origin` if a remote exists.
2. Under a state lock, scan the pool for a worktree that is not in use, not reserved, and not dirty.
3. If found, `git reset --hard` + `clean` it to the latest default branch (detached HEAD). Otherwise create a new detached worktree, if `max_trees` allows.
4. Run `post_create` hooks, then spawn a subshell in the worktree with `TREEHOUSE_DIR` set.
5. When the subshell exits, terminate lingering processes rooted in the worktree, reset it, and clear the owner reservation — the worktree is available again.

Detached HEAD is intentional: worktrees reset to whichever of the local or `origin` default branch is further ahead (preferring `origin` on divergence), which avoids git's "branch already checked out" conflicts entirely. See [Worktree Lifecycle](architecture/worktree-lifecycle.md).

## Repository map

| Path | Role |
| --- | --- |
| [`/main.go`](../main.go) | Entry point; intercepts `--update-check`, otherwise calls `cmd.Execute()` |
| [`/cmd/`](../cmd) | Cobra CLI commands: `get`, `return`, `status`, `prune`, `destroy`, `init`, `update` |
| [`/internal/pool/`](../internal/pool) | Pool manager: acquire, release, list, destroy, prune, state file + locking |
| [`/internal/git/`](../internal/git) | Git operations (shells out to the `git` binary) |
| [`/internal/process/`](../internal/process) | In-use detection and lingering-process termination |
| [`/internal/config/`](../internal/config) | `treehouse.toml` / user config loading, pool dir resolution, `.gitignore` handling |
| [`/internal/hooks/`](../internal/hooks) | `post_create` / `pre_destroy` lifecycle hook execution |
| [`/internal/shell/`](../internal/shell) | Subshell spawning |
| [`/internal/ui/`](../internal/ui) | Y/n prompts and pretty path formatting |
| [`/internal/updater/`](../internal/updater) | Self-update: GitHub release check, download, verify, apply |
| [`/.github/workflows/`](../.github/workflows) | CI (lint/test/build matrix), release-please, Nix vendor-hash |
| [`/flake.nix`](../flake.nix), [`/Makefile`](../Makefile) | Build and packaging |
| [`/docs/agents/`](../docs/agents) | Maintainer agent skills (issue tracker, triage labels, domain layout) |

## CLI reference (summary)

| Command | Description |
| --- | --- |
| `treehouse` / `treehouse get` | Acquire a worktree and open a subshell |
| `treehouse status` | Show pool status (highlights your current worktree) |
| `treehouse return [path]` | Terminate lingering processes and return a worktree |
| `treehouse prune [--all] [--yes] [--prune-orphans] [-v]` | Remove stale idle worktrees (dry run by default) |
| `treehouse destroy [path] [--all] [--force]` | Remove worktrees from the pool |
| `treehouse init` | Create a default `treehouse.toml` |
| `treehouse update` | Self-update to the latest release |

Full flags and behavior: [`/README.md`](../README.md#cli-reference) and [Worktree Lifecycle](architecture/worktree-lifecycle.md) / [Pruning & Safety](architecture/pruning-and-safety.md).

## Build & test (fast path)

```sh
make build   # go build -ldflags "-X main.version=$(VERSION)" -o treehouse .
make test    # go test ./...
make lint    # gofmt -l . && go vet ./...
```

The CI matrix (`ubuntu`, `macOS`, `windows`) runs `gofmt` check, `go vet`, `go test ./...`, and a build. **All new code must compile and pass on Windows** — see the cross-platform rules in [Architecture Overview](architecture/overview.md#cross-platform-rules). More detail: [Build, Release & Contributing](operations/build-release-contributing.md).

## Where to go next

- [Architecture Overview](architecture/overview.md) — package layout, control flow, key design decisions, cross-platform rules.
- [Worktree Lifecycle](architecture/worktree-lifecycle.md) — acquire/release/destroy, pool state, reservations, in-use/dirty detection, hooks.
- [Pruning & Safety](architecture/pruning-and-safety.md) — the prune safety model, orphans, global prune, and config/pool resolution.
- [Build, Release & Contributing](operations/build-release-contributing.md) — Makefile, CI, release-please, Nix, the self-updater, and contribution workflow.

## Notes and caveats

- **Configuration** is layered: repo-level `treehouse.toml` (repo-safe settings only) and user-level `~/.config/treehouse/config.toml`. **Hooks in repo-level config are ignored for safety**; only user-level hooks run. See [config.go](../internal/config/config.go).
- `docs/agents/domain.md` references a repo-root `CONTEXT.md` and `docs/adr/` that do not yet exist; they are created lazily by maintainer skills. Do not assume they are present.
- Module path is `github.com/kunchenguid/treehouse` (see [`/go.mod`](../go.mod)); the repository is mirrored at `github.com/jwalin-shah/treehouse`.
