# secret_inject

A CLI tool for fetching secrets from external secret managers, caching them locally, and injecting them as environment variables.

Currently supports:
- **Doppler** secret management (via `doppler` CLI)
- **1Password** secret retrieval (via `op` CLI and service accounts)
- **Bitwarden Secrets Manager** (via `bws` CLI access tokens)
- **Keyring** storage (macOS Keychain, Windows Credential Manager, Linux secret service, etc.)
- **File** storage (for development, stores in temp directory)

## Features

- 🔐 Secure secret caching with keyring integration
- ⏱️ Configurable cache TTL (time-to-live)
- 🔄 Force refresh option to bypass cache
- 📤 Multiple output formats (shell, JSON, env file)
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
| `--clean` | `false` | Clean cached secrets |
| `--debug` | `false` | Enable debug logging |
| `--force` | `false` | Force refresh, ignore cache |
| `--ttl` | `1h` | Cache TTL duration (e.g., '1h', '30m', '24h') |
| `--output` | `shell` | Output format: shell, json, env |
| `--version` | - | Print version information |
 
### Config Helper

Bootstrap or edit your config file without copying templates manually:

```bash
# Create a new config (fails if the file already exists)
secret_inject config init --config ~/.config/.secret_inject.json

# Overwrite an existing file
secret_inject config init --force --config ~/.config/.secret_inject.json

# Open the config in your preferred editor ($EDITOR or vi)
secret_inject config edit --config ~/.config/.secret_inject.json
```

Use `--editor` to override the editor for a single invocation (for example `--editor "code --wait"`).

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

> Each source verifies the required CLI is installed before enabling itself. Missing CLIs leave the source disabled so other providers can still run.

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
⚠️ **Not recommended for production** - stores secrets in plaintext in temp directory

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

### Fish Shell
Add to your `~/.config/fish/config.fish`:

```fish
function load_secrets
    set output (secret_inject)
    if test $status -eq 0
        eval $output
        echo "✅ Secrets loaded successfully"
    else
        echo "❌ Failed to load secrets:" >&2
        echo $output >&2
        return 1
    end
end
```

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
- **Config files**: Monitored for insecure permissions (world-readable/group-readable)
- **Directory permissions**: Monitored for insecure cache directory permissions

**Note**: Security information (file permissions, storage type) is only logged in debug mode (`--debug` flag) to keep stdout/stderr clean for production use.

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
5. **Enable debug mode** during troubleshooting to see security information: `--debug`
6. **Environment isolation** - use separate config files for different environments (dev/staging/prod)
7. **Review security info** - use `--debug` to check file permissions and storage configuration

### Limitations

Due to Go's string immutability and memory management:
- Secrets are stored in memory as regular strings (cannot be securely zeroed)
- Use the shortest practical TTL to minimize memory exposure
- Restart the process periodically if handling highly sensitive secrets

## Roadmap

Future enhancements:
- [ ] AWS Secrets Manager support
- [ ] HashiCorp Vault support
- [ ] Azure Key Vault support
- [ ] Secret filtering by prefix/pattern
- [ ] Secret name transformation
- [ ] Direct command execution mode (`secret_inject run -- command`)

## Contributing

Contributions welcome! Please ensure:
- Tests pass (`make test`)
- Code builds cleanly (`make build`)
- Follow existing code style

## License

MIT License - see LICENSE file for details

