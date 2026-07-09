# Pruning & Safety

`treehouse prune` removes stale idle worktrees that are safe to delete. It is **dry-run by default** — pass `--yes` to actually delete.

---

## Staleness criteria

A worktree is considered stale (a prune candidate) when **all** of the following are true:

1. **Clean** — `git status --porcelain --untracked-files=all` produces no output.
2. **Unreserved** — no alive owner reservation in the state file.
3. **No running processes** — `process.IsWorktreeInUse` returns `false`.
4. **HEAD is merged into the default branch** — the HEAD commit is an ancestor of `{defaultBranchMergeRef}` (see below).

If any criterion fails, the worktree is reported in the "skipped" output with a stable category label.

---

## Default branch merge safety (the key safety check)

`git.DefaultBranchMergeRef` (`/internal/git/git.go`) determines the reference to compare against:

- If the repo has a remote named `origin`:
  1. Fetch `origin` to ensure latest state.
  2. Resolve `origin/HEAD` to find the remote default branch.
  3. Verify the **local** tracking ref (e.g., `refs/remotes/origin/main`) matches the **remote** HEAD SHA. If they diverge (e.g., the local tracking ref is stale), prune refuses to merge-check against it — this prevents falsely claiming a worktree is merged when it's only merged into a stale local tracking branch.
- If no `origin` remote: use the local default branch ref (`HEAD` resolved via `git rev-parse`).
- If `origin` exists but is unreachable: prune reports `origin unreachable (cannot verify)` and **skips** the worktree. This is not treated as a deletable orphan even with `--prune-orphans`.

> **Why this matters**: if the tracking ref is stale (local says `origin/main=abc`, remote's actual HEAD is `def`), a commit merged into `abc` might not be merged into `def`. Prune refuses to delete in this case.

---

## Orphan detection

A worktree is an **orphan** when its backing git directory (`.git` file content pointing to a gitdir) no longer exists. This can happen when the main repo was deleted or moved.

- Normal prune (`treehouse prune`, `treehouse prune --all`): orphans are reported as skipped with `orphaned (backing repository missing)`.
- With `--prune-orphans`: orphans are included as unverified prune candidates. Each orphan candidate shows `content could not be verified` because treehouse cannot check dirtiness or merge status without the backing repo.
- `--yes` is still required to delete.
- An orphan whose backing repository reappears is reported as `backing repository is available again`.

---

## Global prune (`--all` / `--global`)

`treehouse prune --all` sweeps every managed pool under the user-level treehouse root. It can be run from **any directory** — no repo context needed.

### How it works

1. Enumerate directories under `~/.treehouse/` that contain a `treehouse-state.json` file.
2. For each pool, read the state and resolve each worktree's owning repository from the worktree's `.git` file (which points back to the main repo's gitdir).
3. Use `config.LoadGlobal()` to read only the user-level config (no repo-level config — there's no single repo to read from).
4. Fetch each owning repository's origin and check merge safety.
5. Plan all pools **before** executing any deletions (two-phase: plan all, then execute all).

### Two-phase execution

Global prune plans all pools first, then deletes. This prevents partial deletion if a later pool errors during planning. The phases:

1. **Plan**: For each pool, call `planPrunePool` (reads state, resolves contexts, classifies candidates). Aggregate all candidates and skipped worktrees.
2. **Execute**: If not dry-run and there are candidates, iterate the pre-computed plans and execute deletions per pool.

---

## Skip categories

| Category | Meaning |
|---|---|
| `uncommitted changes` | Worktree has tracked or untracked changes |
| `unmerged` | HEAD is not an ancestor of the default branch merge ref |
| `orphaned (backing repository missing)` | Backing gitdir is gone (reported by default, not deleted) |
| `origin unreachable (cannot verify)` | Could not reach origin to verify merge safety |
| `in use` | Has running processes or an alive owner reservation |
| `cannot check processes` | Failed to enumerate processes via gopsutil |
| `cannot verify worktree` | Generic verification failure |
| `cannot measure size` | Could not calculate reclaimable bytes |
| `cleanup failed` | Reset/clean failed before deletion |
| `remove failed` | `git worktree remove` failed |
| `content could not be verified` | Orphan with no backing repo (shown only with `--prune-orphans`) |

---

## Prune CLI flags

| Flag | Effect |
|---|---|
| `--yes` | Actually delete candidates instead of dry-run |
| `--all`, `--global` | Sweep every pool under the user-level treehouse root |
| `--prune-orphans` | Include backing-repository-missing orphans as candidates |
| `--verbose`, `-v` | Show detailed git diagnostics for skipped worktrees |

---

## Source map

| File | Role |
|---|---|
| `/cmd/prune.go` | CLI command, formatting, flag handling |
| `/internal/pool/prune.go` | Core prune logic: Prune, PruneWithOptions, PruneAll, PruneAllWithOptions, planPrune, executePrune, analyzePruneCandidate, finalPruneSafetyCheck |
| `/internal/git/git.go` | DefaultBranchMergeRef, IsHeadMergedIntoRef, Fetch, GetDefaultBranch |
| `/internal/config/config.go` | LoadGlobal (user-level config without repo context), ResolvePoolDir, ResolvePoolRoot |
| `/internal/process/detect.go` | IsWorktreeInUse (for skip check) |
| `/internal/hooks/hooks.go` | PreDestroy hook execution |
| `/internal/pool/pool_test.go` | Prune tests |
| `/internal/pool/fuzz_test.go` | Concurrency fuzz tests for acquire/release contention |
