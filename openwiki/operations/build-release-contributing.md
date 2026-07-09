# Build, Release & Contributing

This page covers the build system, CI/CD pipeline, self-update mechanism, and development workflow.

---

## Build system

### Makefile (`/Makefile`)

| Target | Description |
|---|---|
| `build` | `go build -ldflags "-X main.version=$(VERSION)" -o treehouse .` |
| `test` | `go test ./...` |
| `fmt` | `gofmt -w .` |
| `lint` | `gofmt -l .` + `go vet ./...` |
| `dist` | Cross-compile for darwin/arm64, darwin/amd64, linux/arm64, linux/amd64, windows/arm64, windows/amd64 |
| `install` | Copy binary to `$GOPATH/bin` (fallback: `/usr/local/bin`) |
| `clean` | Remove build artifacts |
| `demo` | Generate demo GIF via `vhs demo.tape` |

### Nix flake (`/flake.nix`)

A `buildGoModule` flake that includes `git` as a native check input (needed by integration tests). Version is maintained by release-please (marker comment `# x-release-please-version`). The `vendorHash` must be updated when dependencies change — see the [update-vendor-hash workflow](#ci--github-actions).

```sh
nix run github:kunchenguid/treehouse
```

### Version injection

Version is set via `-ldflags "-X main.version=<version>"`. In CI/release builds it's the release tag; for local builds the fallback is `dev`. See `/main.go` lines 13-21:

```go
if version == "" {
    if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
        version = info.Main.Version
    } else {
        version = "dev"
    }
}
```

---

## CI / GitHub Actions

Three workflows under `/.github/workflows/`:

### `ci.yml` — Pull request & push to `main`

| Job | Runs on | Steps |
|---|---|---|
| `check` | ubuntu-latest | `gofmt -l .` check, `go vet ./...` |
| `test` | 3-OS matrix (ubuntu, macos, windows) | `go test ./...`, `go build` |

The `test` job matrix runs on all three operating systems. **All new code must compile and pass tests on Windows** — use `GOOS=windows go build ./...` locally to verify.

### `release.yml` — Push to `main`

Uses release-please to automate versioning and release. The workflow:

1. **release-please** job: Runs `googleapis/release-please-action@v4` with `release-please-config.json` and `.release-please-manifest.json`. Outputs `release_created`, `tag_name`, and `version`.
2. **build-and-upload** job (conditional on release being created): Cross-compiles for 6 platform/arch combinations, archives (`.tar.gz` for Unix, `.zip` for Windows), and uploads to both the GitHub release and as workflow artifacts.
3. **notify-issues** job: Posts a comment to issues mentioned in the release notes.

Cross-compile targets:

| GOOS | GOARCH |
|---|---|
| darwin | arm64, amd64 |
| linux | arm64, amd64 |
| windows | arm64, amd64 |

### `update-vendor-hash.yml` — Manual trigger

Updates the `vendorHash` in `flake.nix` when Go dependencies change. Triggered manually via GitHub UI or `gh workflow run`.

### Release process

1. PRs merged to `main` are collected by `release-please` into a release PR.
2. When the release PR is merged, `release.yml` builds and uploads binaries.
3. The self-updater (`treehouse update`) fetches the latest release from GitHub API and applies it.

---

## Self-updater

Package: `/internal/updater/updater.go`

### How it works

1. **Background check**: Every `treehouse` command (except `update` and dev builds) checks the cached update check result. If the cache is stale (>24h or the user has updated past the cached version), it spawns a **detached child process** (`treehouse --update-check <version>`) that hits the GitHub API and writes `~/.treehouse/update-check.json`.
2. **Cache display**: If the cached result shows an update, a yellow notice is printed before the command runs.
3. **Apply** (`treehouse update`): Calls `CheckLatest` → finds the right archive for the current platform → downloads to temp → verifies checksum → extracts → atomically replaces the current binary.

### Key design decisions

- The background check is fire-and-forget via a child process; it never blocks the main command.
- The child process sets `TREEHOUSE_NO_UPDATE_CHECK=1` to prevent recursive spawning.
- HTTPS is enforced on all download URLs (`enforceHTTPS`), disabled only in tests.
- macOS quarantine is removed post-extraction.
- Version comparison handles semver (including pre-release sorting).

### Test setup

The updater's `githubAPIURL` and `enforceHTTPS` vars are overridable in tests to point at local HTTP servers.

---

## Contribution workflow

From `/CONTRIBUTING.md`:

1. Fork the repo, create a branch from `main`.
2. Make changes.
3. `make lint` + `make test` locally.
4. Open a PR.

### Guidelines

- One feature/fix per PR.
- Follow existing code style (`gofmt` enforced in CI).
- Add tests for new functionality.
- All new code must work on Windows — follow the [cross-platform rules](../architecture/overview.md#cross-platform-rules).

### What to watch out for

- **Version bumps** across files: `flake.nix` and `.release-please-manifest.json` must stay in sync. Release-please manages this automatically.
- **vendorHash** in `flake.nix` must be updated when deps change — run the `update-vendor-hash` workflow or compute locally.
- **Windows compatibility**: test with `GOOS=windows go build ./...` and `GOOS=windows go vet ./...`. Watch for path separators, shell paths, and platform-specific syscalls.
- **Update check cache** (`~/.treehouse/update-check.json`) may need to be cleared during local development to force re-check.

---

## Source map

| File | Role |
|---|---|
| `/Makefile` | Build, test, lint, dist, install, clean |
| `/flake.nix` | Nix flake for build + test |
| `/flake.lock` | Nix lockfile |
| `/main.go` | Entry point with version resolution + `--update-check` intercept |
| `/cmd/root.go` | Cobra root command with persistent update check |
| `/cmd/update.go` | `treehouse update` command |
| `/internal/updater/updater.go` | CheckLatest, Apply, background spawn, cache, checksum verify |
| `/.github/workflows/ci.yml` | CI: lint + test matrix |
| `/.github/workflows/release.yml` | Release build + upload |
| `/.github/workflows/update-vendor-hash.yml` | Nix vendor hash updater |
| `/release-please-config.json` | Release-please configuration |
| `/.release-please-manifest.json` | Release-please manifest |
| `/CONTRIBUTING.md` | Contribution guidelines |
