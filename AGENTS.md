# AGENTS: secret_inject (Go)

CLI for fetching secrets from external secret managers (Doppler, 1Password,
Bitwarden), caching them locally, and injecting them as environment variables.
See `README.md` for user-facing usage and supported backends.

## Build & Test Commands

Prefer the `make` targets over invoking `go` directly.

- Build: `make build` (binary `secret_inject` via `./cmd/secret_inject`; embeds version/commit/date ldflags).
- Test: `make test` (`go test -v ./...`).
- Install: `make install` (`go install` into `$GOBIN`).
- Clean: `make clean`.
- Nix dev shell: `nix develop` (also `shell.nix`/`default.nix` for legacy nix).

## Layout

- `cmd/secret_inject/` — CLI entrypoint and command wiring.
- `internal/` — implementation: secret-manager clients, caching, keyring, output formats.
- Module path: `github.com/napisani/secret_inject`.

## Code Style & Conventions

- **Language**: Go. Standard `gofmt` formatting.
- **Errors**: return explicit errors; no panics (proper error handling is a stated project goal — see README).
- **Secret backends**: each manager (Doppler `doppler`, 1Password `op`, Bitwarden `bws`) is invoked via its own CLI; keep backend logic isolated behind a common interface in `internal/`.
- **Caching**: secrets cache via keyring (macOS Keychain, Windows Credential Manager, Linux secret service) with a configurable TTL; file storage is a dev-only fallback.
- **Output formats**: support shell, JSON, and env-file output — keep these consistent when adding fields.

## Release

- `.goreleaser.yaml` drives release builds; `.drone.yml` is the upstream CI (in the standalone repo).
- This subproject is auto-published from the monorepo to
  [`napisani/secret_inject`](https://github.com/napisani/secret_inject) — work in the
  monorepo, never commit to the published repo directly (see root `AGENTS.md`).
