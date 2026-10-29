# AGENTS: secret_inject (Go)

CLI for fetching secrets from external secret managers (Doppler, 1Password,
Bitwarden), caching them locally, and injecting them as environment variables.
Supported platforms are Linux and macOS. See `README.md` for usage and backends.

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
- **Storage**: credentials and workload snapshots use one `RecordStore` engine.
  Select Keychain, Secret Service, systemd credentials, or age explicitly; never
  fall back between them. Age uses a user-readable key, not OS-managed key custody.
  See `README.md` for backend selection and security trade-offs. The old plaintext
  file cache is retained only for explicit legacy cleanup.
- **Output formats**: support shell, JSON, and env-file output — keep these consistent when adding fields.

## Release

- `.goreleaser.yaml` drives release builds; `.drone.yml` is the upstream CI (in the standalone repo).
- This subproject is auto-published from the monorepo to
  [`napisani/secret_inject`](https://github.com/napisani/secret_inject) — work in the
  monorepo, never commit to the published repo directly (see root `AGENTS.md`).
