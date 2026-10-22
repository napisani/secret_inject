# secret_inject

A CLI tool for fetching secrets from external secret managers, caching them locally, and injecting them as environment variables.

Currently supports:
- **Doppler** secret management (via `doppler` CLI)
- **1Password** secret retrieval (via `op` CLI and service accounts)
- **Bitwarden Secrets Manager** (via `bws` CLI access tokens)
- **Keyring** storage (macOS Keychain, Windows Credential Manager, Linux secret service, etc.)
- **File** storage (development-only plaintext files in your user cache directory)

## Features

- 🔐 Secure secret caching with keyring integration
- ⏱️ Configurable cache TTL (time-to-live)
- 🔄 Force refresh option to bypass cache
- 📤 Multiple output formats (shell, JSON, env file)
- Named workflow profiles with include/exclude patterns and exact key renaming
- Values-free missing-include checks and private rotating debug logs
- Source-scoped caches shared across profiles using the same sources
- ✅ Proper error handling (no panics!)
- 🧪 Unit tested
- 🛠️ Built-in config helper (`secret_inject config`)
- 📦 Clean package structure

## Installation

### Build from source
```bash
make build
```

### Install
```bash
make install
```

## Usage

### Basic Usage
```bash
# Fetch secrets and output as shell exports
./secret_inject --config ~/.config/secret_inject.json

# Force refresh (ignore cache)
./secret_inject --force

# Output as JSON
./secret_inject --output json

# Output as env file format
./secret_inject --output env

# Set custom cache TTL
./secret_inject --ttl 30m

# Clean cached secrets
./secret_inject --clean

# Enable debug logging
./secret_inject --debug

# Show version
./secret_inject --version
```

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `~/.config/.secret_inject.json` | Config file path |
| `--clean` | `false` | Clean all scoped caches in the selected backend; no profile required |
| `--debug` | `false` | Write a private rotating debug log; errors still use stderr |
| `--force` | `false` | Force refresh, ignore cache |
| `--ttl` | `1h` | Cache TTL duration (e.g., '1h', '30m', '24h') |
| `--output` | `shell` | Output format: shell, json, env |
| `--profile` | - | Named workflow profile; required when profiles are configured |
| `--version` | - | Print version information |
 
### Config Helper

Bootstrap or edit your config file without copying templates manually:

```bash
# Create a new config (fails if the file already exists)
secret_inject config init --config ~/.config/.secret_inject.json

# Overwrite an existing file (enforces owner-only POSIX permissions)
secret_inject config init --force --config ~/.config/.secret_inject.json

# Open the config in your preferred editor ($EDITOR or vi)
secret_inject config edit --config ~/.config/.secret_inject.json
```

Use `--editor` to override the editor for a single invocation (for example `--editor "code --wait"`).
Both config commands support `--help` and reject positional arguments. Help and
errors use stderr; config creation confirmations use stdout.

## Configuration


### Config File Format
```json
{
  "source_sequence": ["doppler", "onepassword", "bitwarden"],
  "sources": {
    "doppler": {
      "env": "dev",
      "project": "my-project"
    }
  },
  "storage": {
    "type": "keyring",
    "allowed_backends": ["keychain"],
    "password": "optional-master-password"
  }
}
```

### Source Options

Sources are fetched in `source_sequence` order. Secrets from earlier sources are exported as environment variables when invoking later source CLIs, so you can chain dependencies (for example, `OP_SERVICE_ACCOUNT_TOKEN` coming from Doppler before 1Password runs).

#### Doppler
Configured identically to previous releases. Provide the project and config name to fetch with the `doppler` CLI:

```json
{
  "sources": {
    "doppler": {
      "project": "my-project",
      "env": "dev"
    }
  }
}
```

#### 1Password (`op` CLI)
Requirements:
- Install the [1Password CLI](https://developer.1password.com/docs/cli) and ensure `op` is on your `PATH`.
- Authenticate non-interactively using a service account token (`OP_SERVICE_ACCOUNT_TOKEN`) or an existing desktop session, per the [1Password docs](https://developer.1password.com/docs/cli/sign-in-overview).

Define a mapping from environment variable names to secret references:

```json
{
  "sources": {
    "onepassword": {
      "secrets": {
        "API_TOKEN": "op://Production/API/token",
        "DB_USER": "op://Production/Database/username"
      }
    }
  }
}
```

Each value must be a valid secret reference (vault/item[/section]/field). The CLI resolves the references at runtime, so secrets remain outside the config file.

#### Bitwarden Secrets Manager (`bws` CLI)
Requirements:
- Install the [Bitwarden Secrets Manager CLI](https://bitwarden.com/help/cli/secrets-manager-cli/) (`bws`).
- Export a machine account access token (`BWS_ACCESS_TOKEN`) before running `secret_inject`.

Secrets can be referenced directly by ID or resolved by key within an optional project:

```json
{
  "sources": {
    "bitwarden": {
      "secrets": {
        "DB_PASSWORD": "382580ab-1368-4e85-bfa3-b02e01400c9f",
        "API_TOKEN": {
          "key": "api-token",
          "project_id": "e325ea69-a3ab-4dff-836f-b02e013fe530"
        }
      }
    }
  }
}
```

Use the string form when you know the secret's UUID. The object form lets you locate a secret by its `key` inside an optional Bitwarden project. If a project is not specified, all accessible secrets are searched.

> Each configured source checks that its CLI is installed when `secret_inject` starts. A missing CLI is an error: `secret_inject` exits without fetching secrets from any source. To run without a provider, remove its entry from `sources` or set it to `null`.

### Workflow profiles

Add `profiles` alongside `sources` and `storage` in your config:

```json
{
  "profiles": {
    "shell": {
      "exclude": ["OP_SERVICE_ACCOUNT_TOKEN", "BWS_*"],
      "overwrite": false
    },
    "deploy": {
      "include": ["SERVICE_*", "OPTIONAL_TOKEN"],
      "rename": { "SERVICE_TOKEN": "API_KEY" }
    }
  }
}
```

```bash
secret_inject --profile shell
secret_inject --profile deploy --output json
secret_inject profile check --profile deploy
secret_inject profile check --profile deploy --force --debug
```

- Each profile has exactly one `include` or `exclude` array. Empty includes select
  nothing; empty exclusions select everything. Profiles reject unknown/duplicate
  fields, nulls, invalid patterns, and duplicate explicit rename destinations.
- Rules match original names case-sensitively; `*` matches any sequence, including
  empty. No `?`, bracket classes, regex, or pattern-based renaming.
- Missing rules are tolerated during export. `--debug` records unmatched include
  and exclude rules by name. `profile check` reports unmatched include rules as
  JSON: `{"profile":"deploy","missing_includes":["OPTIONAL_TOKEN"]}`. A pattern
  is missing only when it matches nothing. Exclude profiles report `[]`.
- Check exits **0** for no missing includes, **2** for missing includes, and **1**
  for operational/configuration errors. It uses the normal TTL; `--force` checks
  live providers and updates the full cache. It never prints secret values.
- Selection happens before exact renaming, with no chained renames. An explicit
  rename wins over an unchanged selected destination; the debug log records both
  original names. Absent or excluded rename origins are skipped and debug-logged.
- `overwrite` defaults to `true`. When false, generated POSIX shell code preserves
  any variable already set in the consuming shell, including empty or unexported
  variables. JSON/env still contain fetched values: they cannot enforce a
  consumer's overwrite policy. Excluded variables are **not unset**.
- All output formats require portable destination identifiers, such as `API_KEY`.
- If at least one profile exists, exports require an explicit `--profile`.
  Without profiles, existing full-set export behavior remains available.
  `config init` deliberately keeps generating the profile-free template.

### Project-local env files

Profiles can add local dotenv files and choose whether to use configured remote
sources:

```json
{
  "profiles": {
    "shell": { "exclude": [] },
    "project-only": {
      "exclude": [],
      "remote_sources": false,
      "env_files": ["*.env", "*.env.local"]
    },
    "project": {
      "exclude": [],
      "env_files": ["*.env", "*.env.local"]
    }
  }
}
```

```bash
secret_inject --profile project-only
secret_inject --profile project --output json
secret_inject profile check --profile project-only
```

- `remote_sources` defaults to `true`. When false, no remote providers are
  initialized or fetched, and no remote cache or keyring is opened. A local-only
  invocation does not require a `storage` block.
- `env_files` defaults to an empty array. Patterns match filenames in the process's
  current directory, not the config directory. Parent paths and subdirectory
  patterns are rejected; subdirectories are not searched and symlinks are refused.
- Patterns are processed in configured order, with filenames sorted within each
  pattern; a file is read only once. The example loads `.env` and `app.env` before
  `.env.local` and `app.env.local`. It does not match `.env.production` or
  `.env.example`. No matches are allowed and produce no local values.
- Local files are always read fresh and never cached. Their values override remote
  values; later files override earlier files. Selection and renaming happen after
  composition. `--force` affects only remote fetching.
- Files are parsed as dotenv data, never sourced or executed. Dotenv quoting,
  comments, multiline values, and references to earlier variables in the same file
  are supported. These references do not use shell variables or other files.
  Unreadable or malformed matched files fail without printing their contents.

**Profiles are not access control.** Remote-enabled profiles still fetch and cache
all configured remote sources, including bootstrap credentials, before output
selection. Local files do not supply bootstrap credentials to remote providers.
A local-only export emits only file values but does not unset variables already
present in the consuming shell. This is not an agent execution or sandboxing
interface.

### Cache identity and versioning

Caches are keyed by source definitions and effective source order, not by profile,
config path, output format, or TTL. Later sources still overwrite duplicate names.
Remote-enabled profiles reuse the full remote cache; changing sources creates a
new scope. Local file values are never part of this cache.
External CLI login/token/account changes cannot be detected: use `--force` after
changing them. Provider-side edits otherwise become visible after TTL expiry.

Each backend stores a `cache-version` sentinel. If it is missing, unreadable, or
different from the current version, the tool discards its scoped and old unscoped
cache entries without decoding them, then writes the current version. No cached
data is migrated. Stop old binaries before upgrading so they cannot recreate old
entries. A failed cache reset stops the operation.

`--clean` discards cache entries directly without checking their version, then
writes the current sentinel. It preserves unrelated data and does not require
installed provider CLIs. It cannot be
combined with `--profile`. It does not clean unopened keyring backends or prevent
another process from repopulating a cache concurrently.

### Debug logging

`--debug` writes structured name-only diagnostics to
`$XDG_STATE_HOME/secret_inject/debug.log`, falling back to
`~/.local/state/secret_inject/debug.log`. The active file rotates at 5 MiB with
one `.1` backup. Directories are private (`0700`), files are `0600`, with equivalent
current-user access restrictions on Windows. Symlink/nonregular targets are refused.

Only one debug invocation may hold the log lock; another fails immediately.
Open/write/rotation/flush/close failures also stop result output rather than
silently losing the requested diagnostics. Non-debug runs create no log files.
Logs cover cache/provider state, missing rules, skipped renames and collisions;
values, provider arguments, references, raw provider output and environment dumps
are never logged. Provider failures report operation/status instead of upstream
output. Stdout remains result data only; terminal password prompts use stderr.

### Storage Options

#### Keyring Storage (Recommended)
Uses OS-native secure storage:

```json
{
  "storage": {
    "type": "keyring",
    "allowed_backends": ["keychain", "secret-service", "wincred"],
    "password": "optional-master-password"
  }
}
```

**Available backends:**
- `keychain` - macOS Keychain
- `secret-service` - Linux Secret Service (GNOME/KDE)
- `wincred` - Windows Credential Manager
- `kwallet` - KDE Wallet
- `keyctl` - Linux kernel keyring
- `pass` - Pass password manager
- `file` - Encrypted file (requires password)

#### File Storage (Development Only)
⚠️ **Not recommended for production** — stores complete remote snapshots in plaintext
under `os.UserCacheDir()/secret_inject/` (for example `~/.cache/secret_inject/`
on Linux or `~/Library/Caches/secret_inject/` on macOS). This is cache storage;
profile `env_files` inputs are separate and are always read fresh.

```json
{
  "storage": {
    "type": "file"
  }
}
```

## Integration with Shell

### Bash/Zsh
Add to your `~/.bashrc` or `~/.zshrc`:

```bash
# Load secrets from secret_inject
load_secrets() {
  OUTPUT=$(secret_inject)
  RESULT=$?
  if [ $RESULT -eq 0 ]; then
    eval "$OUTPUT"
    echo "✅ Secrets loaded successfully"
  else
    echo "❌ Failed to load secrets:" >&2
    echo "$OUTPUT" >&2
    return 1
  fi
}

# Auto-load on shell start (optional)
# load_secrets
```

If your config declares profiles, change the invocation above to
`secret_inject --profile shell` (using your chosen profile name).

### Other shells

Shell output is POSIX syntax: do not evaluate it in fish, PowerShell, or cmd.exe.
JSON and env output are available for integrations, but native export syntax for
those shells is not implemented.

## Development

### Project Structure
```
secret_inject/
├── cmd/
│   └── secret_inject/     # Main CLI entry point
├── internal/
│   ├── config/           # Configuration parsing
│   ├── secret/           # Secret data structures
│   ├── source/           # Secret source implementations
│   ├── storage/          # Cache storage backends
│   └── output/           # Output formatters
├── go.mod
├── Makefile
└── README.md
```

### Running Tests
```bash
make test
```

Installed skill reference examples are isolated by `agent/go.mod`; they contain
placeholder imports and are not part of the CLI module. Run checks from this
project's root (or `make check FILTER=secret-inject` from the monorepo root).
The installed skill files do not need modification to keep Go checks working.

To exercise the profile matcher against an independent pattern implementation:

```bash
go test ./internal/profile -run='^$' -fuzz=FuzzProfileStarMatching -fuzztime=10s
```

### Building
```bash
# Build binary
make build

# Clean build artifacts
make clean

# Install to $GOPATH/bin
make install
```

## Security Considerations

### Storage Security

- **Keyring storage (Recommended)**: Uses OS-native secure storage (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- **File storage**: ⚠️ **WARNING** - Stores secrets in **plaintext** on disk. Use **only for development**.

### File Permissions

The tool automatically enforces secure file permissions:

- **Cache files**: Created with `0600` (read/write for owner only)
- **Private cache/debug directories**: Restricted to the current user
- **Config files**: Set permissions yourself (`chmod 600` on POSIX systems)

`--debug` records values-free selection, provider and cache diagnostics in the
private log file, not stderr.

### Special Character Handling

The tool properly escapes secrets containing special characters to prevent injection vulnerabilities:

#### Shell Export Format
Uses POSIX-compliant single-quote escaping. Secrets are wrapped in single quotes with embedded quotes escaped as `'\''`:
```bash
export API_KEY='abc'\''def'  # For secret: abc'def
```

#### Env File Format  
Escapes special characters in `.env` file format:
- Backslashes: `\` → `\\`
- Quotes: `"` → `\"`
- Newlines: `\n` → `\\n`
- Tabs: `\t` → `\\t`
- Dollar signs: `$` → `\$`

Example:
```bash
# Secret with newline and quote
API_KEY="line1\\nline2\\\"quoted\\\""
```

### Best Practices

1. **Use keyring storage** for any production or sensitive environments
2. **Secure your config file** with appropriate permissions: `chmod 600 ~/.config/.secret_inject.json`
3. **Rotate secrets regularly** and use `--force` to refresh immediately
4. **Monitor cache TTL** - shorter TTLs reduce exposure window but increase API calls
5. **Enable debug mode** during troubleshooting to inspect names and cache/provider state: `--debug`
6. **Environment isolation** - use separate config files for different environments (dev/staging/prod)
7. **Review selection** - use `profile check --profile NAME` for missing includes, and remember profiles do not restrict fetching or caching

### Limitations

Due to Go's string immutability and memory management:
- Secrets are stored in memory as regular strings (cannot be securely zeroed)
- Use the shortest practical TTL to minimize memory exposure
- Restart the process periodically if handling highly sensitive secrets

## Roadmap

See the [feature wishlist](docs/feature-wishlist.md) for the broader direction
and open design questions around source composition, shell environments,
agent use, local env files, and format conversion.

Future enhancements:
- [ ] AWS Secrets Manager support
- [ ] HashiCorp Vault support
- [ ] Azure Key Vault support
- [x] Secret filtering by star pattern in named profiles
- [x] Exact secret name transformation in named profiles
- [ ] Direct command execution mode (`secret_inject run -- command`)

## Contributing

Contributions welcome! Please ensure:
- Tests pass (`make test`)
- Code builds cleanly (`make build`)
- Follow existing code style

## License

MIT License - see LICENSE file for details

