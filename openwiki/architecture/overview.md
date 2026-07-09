# Architecture Overview

Treehouse is a small, dependency-light Go CLI. It has **no daemon and no server**: every command is a short-lived process that reads/writes a per-pool JSON state file under a file lock, shells out to `git`, inspects running processes, and exits. This page explains how the pieces fit together and the rules to follow when changing them.

Related pages: [Worktree Lifecycle](worktree-lifecycle.md) · [Pruning & Safety](pruning-and-safety.md) · [Build, Release & Contributing](../operations/build-release-contributing.md).

## Entry point and command dispatch

- [`/main.go`](../../main.go) resolves the build version (from `-ldflags -X main.version=...` or `debug.ReadBuildInfo`), then:
  - Special-cases `treehouse --update-check ...`, which is the **background child process** spawned by the updater and bypasses Cobra entirely (`updater.RunBackgroundCheck`).
  - Otherwise calls `cmd.SetVersion` and `cmd.Execute()`.
- [`/cmd/root.go`](../../cmd/root.go) defines the Cobra `rootCmd`. Running `treehouse` with no subcommand is an alias for `get` (`RunE` calls `getRunE`). A `PersistentPreRun` shows a cached update notice and spawns a background update check when the cache is stale (skipped for `dev` builds, the `update` command, or when `TREEHOUSE_NO_UPDATE_CHECK=1`).
- Each command registers itself with `rootCmd.AddCommand(...)` in its own `init()` (`get`, `return`, `status`, `prune`, `destroy`, `init`, `update`).

Most commands follow the same preamble: find the repo root (`git.FindRepoRoot`), load config (`config.Load`), and resolve the pool directory (`config.ResolvePoolDir`). `prune --all` is the exception — it can run without a repository (see [Pruning & Safety](pruning-and-safety.md)).

## Package responsibilities

| Package | Responsibility | Key files |
| --- | --- | --- |
| `cmd` | Cobra command definitions and user-facing output/prompts | [`root.go`](../../cmd/root.go), [`get.go`](../../cmd/get.go), [`prune.go`](../../cmd/prune.go) |
| `internal/pool` | Pool state, acquire/release/destroy/list/prune, locking, reservations | [`pool.go`](../../internal/pool/pool.go), [`state.go`](../../internal/pool/state.go), [`prune.go`](../../internal/pool/prune.go) |
| `internal/git` | All git interactions (default branch, worktree add/remove, reset, dirty/merge checks) | [`git.go`](../../internal/git/git.go) |
| `internal/process` | Find/terminate processes rooted in a worktree; process identity for reservations | [`detect.go`](../../internal/process/detect.go), [`terminate.go`](../../internal/process/terminate.go) |
| `internal/config` | Config load/merge, pool dir + root resolution, `.gitignore` handling | [`config.go`](../../internal/config/config.go), [`gitignore.go`](../../internal/config/gitignore.go) |
| `internal/hooks` | Run `post_create` / `pre_destroy` shell commands | [`hooks.go`](../../internal/hooks/hooks.go) |
| `internal/shell` | Spawn an interactive subshell in the worktree | [`shell.go`](../../internal/shell/shell.go) |
| `internal/ui` | Y/n confirmation prompts and pretty path rendering | [`prompt.go`](../../internal/ui/prompt.go), [`path.go`](../../internal/ui/path.go) |
| `internal/updater` | GitHub release check, download, checksum verify, atomic replace | [`updater.go`](../../internal/updater/updater.go) |

## Dependencies

From [`/go.mod`](../../go.mod) (Go 1.25.x):

- `spf13/cobra` — command framework.
- `BurntSushi/toml` — config parsing.
- `fatih/color` — colored terminal output.
- `shirou/gopsutil/v4` — cross-platform process listing (cwd, name, start time, parent).
- `golang.org/x/sys` — low-level syscalls behind build tags.

**Design note:** git is invoked by shelling out to the `git` binary rather than using a Go git library. Per [`/AGENTS.md`](../../AGENTS.md), this is deliberate — go-git had incomplete worktree support. Treat `git` as a required runtime dependency.

## Data and state

There is no database. The only persistent state is a per-pool JSON file:

- `treehouse-state.json` in the pool directory, holding the list of `WorktreeEntry` records (name, path, created time, `Destroying` flag, and the transient owner reservation `OwnerPID` / `OwnerStartedAt`). See [`state.go`](../../internal/pool/state.go).
- `treehouse-state.lock` guards all reads/writes via `WithStateLock` (advisory file lock; `flock` on unix, `LockFileEx` on windows through the `lock_unix.go` / `lock_windows.go` build-tag pair).

The updater keeps a separate cache at `~/.treehouse/update-check.json`.

Two robustness properties are baked into the pool logic:
- **Self-healing** — `healState` drops entries whose paths no longer exist and clears stale owner reservations whose owning process is gone.
- **Reservations are short-lived** — an owner reservation is proven "alive" only if a process with that PID *and* the recorded start time still exists (`ownerAlive`), so recycled PIDs cannot masquerade as owners.

## Control flow: `treehouse get`

```
main.go → cmd.Execute → rootCmd.RunE → getRunE (cmd/get.go)
  git.FindRepoRoot
  config.Load → config.ResolvePoolDir → config.EnsureGitignore
  pool.Acquire            # under state lock: reuse or create worktree, reserve owner
    git.GetDefaultBranch / git.Fetch
    process.IsWorktreeInUse / git.IsDirty     # skip unavailable
    git.ResetWorktree  OR  git.AddWorktree
    hooks.Run(post_create)
  shell.Spawn(wtPath, TREEHOUSE_DIR=...)      # blocks until subshell exits
  git.DetachWorktree
  git.IsDirty → ui.Confirm (prompt to clean if dirty)
  process.TerminateWorktreeProcesses          # kill lingering agent processes
  pool.Release                                # reset + clear reservation
```

The full lifecycle, including `return`, `destroy`, and `status`, is documented in [Worktree Lifecycle](worktree-lifecycle.md).

## Cross-platform rules

Treehouse targets **Linux, macOS, and Windows**, and CI enforces this. When editing code, follow the conventions from [`/AGENTS.md`](../../AGENTS.md):

- **Paths:** never hardcode `/`. Use `filepath.Join`, `filepath.Separator`, `filepath.ToSlash`.
- **Shell:** don't assume `/bin/sh` or `$SHELL`. On Windows use `%COMSPEC%` (`cmd.exe`). See [`shell.go`](../../internal/shell/shell.go) and the hook shell wrappers ([`command_unix.go`](../../internal/hooks/command_unix.go) / [`command_windows.go`](../../internal/hooks/command_windows.go)).
- **Syscalls:** isolate Unix-only calls (e.g. `flock`) behind build tags. Follow the `_unix.go` / `_windows.go` naming pattern — examples: [`pool/lock_unix.go`](../../internal/pool/lock_unix.go) & [`lock_windows.go`](../../internal/pool/lock_windows.go), [`process/terminate_unix.go`](../../internal/process/terminate_unix.go) & [`terminate_windows.go`](../../internal/process/terminate_windows.go), and the updater's `sysproc_*.go` / `quarantine_*.go` files.
- **Process detection:** `gopsutil` is cross-platform; avoid importing platform-specific process APIs directly.
- **Verify locally:** `GOOS=windows go build ./...` catches most portability issues before CI.

## Where to start when changing things

- **Adding a CLI command or flag:** add a file under `cmd/`, register in `init()`, reuse the repo-root/config/pool-dir preamble. Update the tables in [`/README.md`](../../README.md) and this wiki.
- **Changing pool/reservation behavior:** work in `internal/pool`. Keep all mutations inside `WithStateLock` and preserve `healState` / `ownerAlive` invariants. Tests: [`pool_test.go`](../../internal/pool/pool_test.go), [`fuzz_test.go`](../../internal/pool/fuzz_test.go).
- **Changing git behavior:** work in `internal/git`; add tests to [`git_test.go`](../../internal/git/git_test.go). Remember worktrees are detached HEAD and default-branch selection prefers `origin`.
- **End-to-end behavior:** the primary integration suite is [`cmd/e2e_test.go`](../../cmd/e2e_test.go).
