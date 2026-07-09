# Worktree Lifecycle

This page describes the core business logic of treehouse: how worktrees move through the pool as agents acquire, use, return, and remove them. If you are changing pool behavior, read this together with [Architecture Overview](overview.md) and [Pruning & Safety](pruning-and-safety.md).

All state mutations happen inside `WithStateLock` (see [`state.go`](../../internal/pool/state.go)); the pool logic lives in [`internal/pool/pool.go`](../../internal/pool/pool.go).

## Pool layout and state

Each repository gets its own pool directory, resolved by `config.ResolvePoolDir` ([`config.go`](../../internal/config/config.go)):

```
<root>/<repoName>-<shortHash>/        # pool dir; shortHash is sha256(remote URL or repo path)[:3]
  treehouse-state.json                # pool membership + owner reservations
  treehouse-state.lock                # advisory lock file
  1/<repoName>/                        # worktree #1 (a git worktree)
  2/<repoName>/                        # worktree #2
  ...
```

- Default `<root>` is `~/.treehouse`. It can be overridden with `root` in config; relative roots resolve from the repo root, absolute roots are required for global prune (`ResolvePoolRoot`).
- Worktrees live under a numbered directory (`nextName` picks the next free integer) so the parent dir can be removed cleanly on destroy.
- `EnsureGitignore` writes a `.gitignore` entry for the pool's parent so the worktree pool never pollutes the user's repo.

Each `WorktreeEntry` records `Name`, `Path`, `CreatedAt`, an optional `Destroying` flag, and the transient owner reservation `OwnerPID` / `OwnerStartedAt`.

## Status model

`pool.List` (used by `treehouse status`, [`cmd/status.go`](../../cmd/status.go)) reports one of:

| Status | Meaning |
| --- | --- |
| `available` | Not reserved, no processes, clean — free to acquire |
| `in-use` | Owner reservation alive, or a process has cwd inside the worktree |
| `you're here` | In-use and your current directory is inside it |
| `dirty` | No owner/process, but has tracked or untracked changes |

`Destroying` entries are hidden from status. `List` also calls `healState` and rewrites the file, so listing self-heals stale entries.

## Acquire (`treehouse get`)

Driven by `getRunE` ([`cmd/get.go`](../../cmd/get.go)) → `pool.Acquire`:

1. Resolve the default branch (`git.GetDefaultBranch`) and `git.Fetch` if `origin` exists.
2. Under the state lock, `healState`, then scan existing worktrees for the first that is **not** `Destroying`, **not** owner-alive, **not** process-in-use, and **not** dirty. Reset it to the default branch (`git.ResetWorktree`) and reserve it.
3. If none are reusable and `len(worktrees) < max_trees`, create a new detached worktree with `git.AddWorktree` and add it to the pool. If the pool is full, return an actionable error suggesting `treehouse status` or a larger `max_trees`.
4. Reserve ownership (`reserveOwner` records this process's PID + start time), persist state, then run `post_create` hooks **outside** the lock.
5. Spawn a subshell (`shell.Spawn`) with `TREEHOUSE_DIR` set to the worktree path. This blocks until the user types `exit`.

### Detached HEAD and default-branch selection

Worktrees are always **detached HEAD**, which sidesteps git's rule that a branch can only be checked out in one worktree. The ref they reset to is chosen by `branchRef` in [`git.go`](../../internal/git/git.go):

- If both local `refs/heads/<branch>` and `refs/remotes/origin/<branch>` exist, pick whichever is further ahead (ancestor test via `merge-base --is-ancestor`); on divergence, **prefer origin**.
- Otherwise use whichever ref exists.

`ResetWorktree` does `checkout --detach --force <ref>` + `reset --hard <ref>` + `clean -fd`, guaranteeing a pristine tree while preserving untracked build caches only when they are outside git's clean scope. `GetDefaultBranch` prefers `origin/HEAD`, falling back to local `HEAD` symbolic ref or `init.defaultBranch`.

## Return / release

Both the automatic return on subshell exit (in `getRunE`) and the explicit `treehouse return [path]` ([`cmd/return_cmd.go`](../../cmd/return_cmd.go)) converge on the same steps:

1. `git.DetachWorktree` — detach HEAD before reuse (fixes a class of "branch already checked out" reuse bugs).
2. Dirty check: if the worktree has uncommitted or untracked changes, prompt to clean (`ui.Confirm`). If declined, the worktree is left dirty and the user is told to run `treehouse return --force` later. `--force` skips the prompt.
3. `killLingeringProcesses` → `process.TerminateWorktreeProcesses` — terminate every process whose cwd is inside the worktree, so detached tools (e.g. agent servers that ignore SIGHUP) don't keep holding it.
4. `pool.Release` — reset the worktree to the default branch and clear the owner reservation.

### Lingering-process termination

`TerminateWorktreeProcesses` ([`process/terminate.go`](../../internal/process/terminate.go)):

- Finds processes whose resolved cwd is the worktree or a descendant (`FindProcessesInWorktree` in [`detect.go`](../../internal/process/detect.go), using absolute-path + symlink resolution).
- **Protects the current process and its ancestor chain** (`filterProtectedProcesses`) so treehouse never kills the shell it is running under.
- On unix: SIGTERM, wait up to a grace period (2s from `get`), then SIGKILL survivors (`terminate_unix.go`). On windows: `TerminateProcess` (`terminate_windows.go`).

## Destroy (`treehouse destroy`)

[`cmd/destroy.go`](../../cmd/destroy.go) → `pool.Destroy` / `pool.DestroyAll`:

1. Under lock, mark the entry `Destroying = true` and reserve it (prevents concurrent reuse). `DestroyAll` marks every entry; both refuse in-use worktrees unless `--force`.
2. Run `pre_destroy` hooks outside the lock.
3. Re-acquire the lock, confirm the destroy reservation still matches (`sameDestroyReservation` — guards against a racing operation), then `git.RemoveWorktree` (`worktree remove --force`), remove the numbered parent directory, and drop the entry from state.

The two-phase (reserve → hook → delete) pattern with reservation re-validation is the general concurrency-safety idiom across pool operations; preserve it when adding new lifecycle actions.

## Hooks

User-configured lifecycle commands run via [`internal/hooks/hooks.go`](../../internal/hooks/hooks.go):

- `post_create` runs after a worktree is provisioned/reset, just before `get` hands it over.
- `pre_destroy` runs before a worktree is removed by `destroy`, `destroy --all`, or prune deletion.
- Commands run **sequentially** in the worktree directory through the OS shell (`/bin/sh -c` unix, `%COMSPEC% /d /s /c` windows). A non-zero exit is logged (command, exit code) and **does not fail** the overall operation.
- **Hooks only load from user-level config** (`~/.config/treehouse/config.toml`). Repo-level `treehouse.toml` hooks are ignored — a deliberate safety measure so cloning a repo cannot run arbitrary commands. Enforced in `config.Load` and `LoadGlobal` (see [`config.go`](../../internal/config/config.go)).

## What to watch out for when changing this area

- Keep every state read/write inside `WithStateLock`; never mutate `treehouse-state.json` outside it.
- Preserve `ownerAlive`'s PID + start-time identity check — dropping the start time reintroduces recycled-PID bugs.
- Preserve process-protection in termination; a regression here can kill the user's shell.
- Run hooks **outside** the lock (they can be slow / interactive) but reserve first so the worktree can't be stolen mid-hook.
- Tests: unit/behavioral in [`pool_test.go`](../../internal/pool/pool_test.go) and [`fuzz_test.go`](../../internal/pool/fuzz_test.go) (acquire/release contention), hook behavior in [`config/hooks_test.go`](../../internal/config/hooks_test.go) and [`hooks/hooks_test.go`](../../internal/hooks/hooks_test.go), process logic in [`process/*_test.go`](../../internal/process), and end-to-end flows in [`cmd/e2e_test.go`](../../cmd/e2e_test.go).
