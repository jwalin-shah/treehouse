# Dead Code Review — treehouse — 2026-07-06

## Summary

A tldr dead-code scan reported 74 dead functions (24.5% of the repo). After thorough cross-checking with call-graph analysis, ripgrep searches, and manual code review, **73 of the 74 are false positives**. Only **1 function** was genuinely dead and removed.

## Scope of Removals

| File | Lines Removed | Symbol | Type |
|------|---------------|--------|------|
| `internal/process/detect.go` | 28-31 (4 lines) | `func Exists(pid int32) bool` | Exported function |

### Rationale for Removal

- `Exists()` wrapped `process.PidExists(pid)` from gopsutil.
- A full-project ripgrep search (`rg -n "\.Exists\b"`) found **zero callers** — no production code, no test code, no reflection, no build-tag variants.
- The function was exported but had no callers inside or outside the package.
- No CGo, FFI, or unsafe references.
- The underlying import (`github.com/shirou/gopsutil/v4/process`) remains in use by `StartedAt()` and `FindProcessesInWorktree()`, so no import cleanup was needed.

## Near-Miss Symbols Considered but Kept

These were flagged by tldr as dead but are **false positives** after verification:

| Symbol | File | Why Kept |
|--------|------|----------|
| `init()` (7 functions) | `cmd/*.go` | Go runtime calls `init()` to register cobra commands. tldr cannot see this. |
| All `Test*` functions (36) | Various `*_test.go` | Go testing framework discovers and runs `Test*` functions. tldr's default entry-point pattern `test_` does not match Go's `Test` prefix. |
| All `Fuzz*` functions (5) | `internal/pool/fuzz_test.go` | Go 1.18+ fuzz framework runs `Fuzz*` functions via `go test -fuzz`. |
| `SetVersion` | `cmd/root.go` | Called from `main.go:14`. |
| `PrettyPath` | `internal/ui/path.go` | Called from `cmd/get.go`, `cmd/status.go`, `cmd/prune.go`, `cmd/destroy.go`. |
| `Confirm` | `internal/ui/prompt.go` | Called from `cmd/get.go`, `cmd/return_cmd.go`, `cmd/destroy.go`. |
| `resolveWorktreePath` | `cmd/return_cmd.go` | Called within the same file. |
| `SpawnBackgroundCheck` | `internal/updater/updater.go` | Called from `cmd/root.go:50`. |
| `AssetNameForVersion` | `internal/updater/updater.go` | Only called from test code (`updater_test.go`). Kept because it is tested and may be useful for future production use. |
| `windowsShellCommandLine` | `internal/hooks/hooks.go` | Called from `command_windows.go`. |
| `FindByPath` | `internal/pool/pool.go` | Called from `cmd/return_cmd.go`. |
| `StartedAt` | `internal/process/detect.go` | Called from `internal/pool/pool.go:385,391`. |
| `parentPID` | `internal/process/terminate.go` | Called from `filterProtectedProcesses` in the same file. |
| `(p ProcessInfo) String()` | `internal/process/detect.go` | Implements `fmt.Stringer`. No current callers found, but kept for debugging utility. |
| `lockFile`/`unlockFile` (unix+windows) | `internal/pool/lock_*.go` | Called from pool state management via build-tagged platform files. tldr cannot trace cross-build-tag callers. |
| `newHookCommand` (unix+windows) | `internal/hooks/command_*.go` | Called from `internal/hooks/hooks.go` via build-tagged platform files. |
| `setSysProcAttr` (unix+windows) | `internal/updater/sysproc_*.go` | Called from `internal/updater/updater.go:191`. |
| `removeQuarantine` (darwin+other) | `internal/updater/quarantine_*.go` | Called from `internal/updater/updater.go:251`. |
| `terminate` (windows) | `internal/process/terminate_windows.go` | Called from `TerminateWorktreeProcesses` via build-tagged platform file. |

## Build and Test Verification

| Check | Result |
|-------|--------|
| `go build -o treehouse .` | PASS (exit 0) |
| `go test ./...` (all 7 packages) | ALL PASS |
| `go vet ./...` | PASS (no warnings) |

Test results per package:
- `cmd` — ok (23.058s)
- `internal/config` — ok (2.043s)
- `internal/git` — ok (1.188s)
- `internal/hooks` — ok (0.937s)
- `internal/pool` — ok (27.201s)
- `internal/process` — ok (4.689s)
- `internal/updater` — ok (1.491s)

## Risks and Follow-Ups

- **Low risk.** The removed function had zero callers. No behavior change.
- **tldr tool limitation:** The default entry-point patterns (`main`, `test_`, `cli`) do not match Go's `Test*` convention or `init()` runtime hooks. For Go projects, `tldr dead` should be invoked with `--entry-points "main,Test,init"` to reduce false positives. The same applies to fuzz functions (`Fuzz*`), which could be added as `Fuzz`.
- **`AssetNameForVersion`** is technically dead in production (only used by tests). If the updater code is ever removed, this function and its tests should go together.
- **`(p ProcessInfo) String()`** has no callers but implements a standard interface. It is harmless and may be useful for future debugging/logging.
