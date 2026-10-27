# secret_inject

Compose secrets and local dotenv files into shell exports, JSON, env-file text,
or the environment of a command. Supported platforms are Linux and macOS.

## Development status: breaking migration

This branch implements private credential storage and unattended resolution.
**Fresh remote fetches are currently refused:** no exact provider release has
completed the required conformance matrix. Source-audited candidates are not
certified. There is no runtime bypass, personal-session fallback, or automatic
login. The release workflow refuses to publish until all three providers have
certified releases. See [conformance requirements](docs/unattended-conformance.md).
Local-only profiles, inspection, and local-only `run` work independently.

Native adapters are implemented for macOS Keychain and Linux Secret Service;
compiling them does not establish native conformance.
Do not activate the managed configuration migration before completing its setup
and release gates.

## Build and check

Go 1.25 or newer:

```sh
make build
make test
make install
# From the monorepo root:
make check FILTER=secret-inject
```

macOS builds that use native storage require CGO and the Apple SDK. Release
archives build both macOS architectures natively with CGO enabled. Linux builds
do not require CGO. Installed skill examples are a separate `agent/go.mod`
module and are not compiled as part of the CLI.

## Configuration

The default path is `~/.config/.secret_inject.json`. `config init` writes a
profile-free template with private bindings and exactly one platform backend;
`config edit` uses `$EDITOR` or `--editor` without executing a shell command.

```sh
secret_inject config init --config ./secrets.json
secret_inject config edit --config ./secrets.json
```

Example with synthetic identifiers, portable between macOS and Linux:

```json
{
  "storage": {"type":"keyring", "allowed_backends":["darwin-keychain","linux-secret-service"]},
  "source_sequence": ["doppler", "onepassword"],
  "sources": {
    "doppler": {
      "project":"example", "env":"dev",
      "credential":{"kind":"stored", "name":"work-root"}
    },
    "onepassword": {
      "credential":{
        "kind":"from_source", "source":"doppler",
        "key":"OP_SERVICE_ACCOUNT_TOKEN"
      },
      "secrets":{"SERVICE_TOKEN":"op://example/item/field"}
    }
  },
  "profiles": {
    "shell": {"exclude":[], "overwrite":false},
    "task": {
      "include":["SERVICE_*"],
      "rename":{"SERVICE_TOKEN":"API_KEY"}
    },
    "project-only": {
      "exclude":[], "remote_sources":false,
      "env_files":["*.env", "*.env.local"]
    }
  }
}
```

`secret_inject` selects `darwin-keychain` on macOS and `linux-secret-service` on Linux.
List both in `allowed_backends` to share a config across systems, or list only
the intended host's backend. List order does not affect selection. A missing
host backend, unknown backend, duplicate entry, or configured storage password
is rejected. Native access failure never falls back to another backend.
Local-only profiles need no storage or provider configuration.

### Private authentication

All remote export/check/run operations require explicit bindings. Ambient
`DOPPLER_TOKEN`, personal 1Password sessions, and ambient `BWS_ACCESS_TOKEN` are
not authentication fallbacks.

```sh
secret_inject store prepare --config ./secrets.json
secret_inject credential set work-root --provider doppler --config ./secrets.json
secret_inject credential list --config ./secrets.json
secret_inject credential remove work-root --config ./secrets.json
```

- Only `store prepare` and enrollment may request OS interaction. Normal exports,
  checks, `run`, credential metadata/removal, and cache operations do not create
  or unlock native containers or invoke a native prompt.
- `set` requires terminal input, reads a single-line token without echo, and
  upserts by default. No token argument, stdin-pipe mode, environment-value
  enrollment, or raw-value read/export command exists.
- Credential names have 1–128 ASCII alphanumeric/underscore/hyphen/dot characters,
  starting with an alphanumeric character. They are identifiers, not paths.
- Existing names cannot change provider kind through replacement. Metadata and
  receipts contain only name/provider fields. Removal deletes the local record,
  not the vendor's token.
- `store prepare` can create/unlock/request OS authorization without replacing
  credentials or changing their revision merely for preparation. Authorization
  may still be item-specific or change later.
- A `from_source` binding names an exact original output key in an earlier enabled
  source. That entry is extracted into a private in-memory channel **before**
  workload merge, cache publication, filtering, renaming, or collision logging.
  Missing required private entries fail; profile include tolerance is unrelated.
- Only the consuming provider receives its private credential. Unmarked copies
  or equal values under different keys are not detected automatically.

One source instance per provider is supported. Doppler uses `project`/`env`;
1Password maps names to `op://` references. Bitwarden's `secrets` values accept
an ID string, or an object containing `id`, or `key` with optional `project_id`.
Later sources overwrite duplicate ordinary workload names.

### Provider process isolation

Fresh fetches use a private, token-free provider configuration before even a
version probe. Personal CLI files are not imported. Competing ambient auth
selectors are removed from the provider child only; target inheritance is not
modified. Credentials are supplied only to the selected provider's fetch.

Per-command provider execution retains a 30-second limit, captures output
internally, and never reads target stdin. Caller cancellation propagates; there
is no additional aggregate fetch-timeout flag or vendor login recovery.

Documented proxy/CA/endpoint environment controls remain available. In particular,
`BWS_SERVER_URL` is validated into the private TOML profile's `server_base` so it
cannot bypass the state-opt-out profile. No new endpoint source-schema fields
exist. Changing external endpoint/provider context still requires `--force`.

## Profiles and local env files

- Each profile has exactly one `include` or `exclude` array. Empty includes select
  nothing; empty exclusions select everything. Matching is case-sensitive and
  supports only `*` (including an empty match), not regex or bracket patterns.
- Exact renames use original names and do not chain. Explicit renames beat
  unchanged destinations; duplicate explicit destinations are invalid.
- Missing includes are tolerated. `profile check --profile NAME` resolves values
  and returns `{"profile":"NAME","missing_includes":[...]}` without values:
  status 0 complete, 2 missing includes, 1 operational failure.
- Shell `overwrite` defaults true. False preserves existing shell variables,
  including empty/unexported ones; JSON/env cannot enforce that consumer policy.
  Excluded/absent names never unset inherited variables.
- If profiles exist, exports require `--profile`. Profile-free export remains
  available with the same private-auth requirements. `run` always requires an
  explicit existing profile.
- `remote_sources:false` skips providers and remote storage completely.
- `env_files` patterns are basename-only and evaluated in the current directory.
  Files are sorted within each pattern, processed once, and overlaid in pattern
  order. `*.env` then `*.env.local` puts local overrides last. No subdirectory
  search, symlink/nonregular-file input, or shell execution is allowed.
- Dotenv files are always read fresh and never cached. Local values override
  ordinary remote values before projection, but cannot bootstrap remote auth.

## Run a command

Linux/macOS only:

```sh
secret_inject run --profile task -- executable --literal-target-flag
secret_inject run --config ./secrets.json --profile project-only -- ./tool
```

The literal `--` separator is mandatory. No task registry, inheritance flags,
restricted mode, child supervisor, or shell fallback exists.

`run` inherits the **entire parent environment** and overlays selected final
values, regardless of shell `overwrite`. Missing includes can therefore retain
ambient fallback values; exclusions do not remove parent variables.

The initial executable is resolved with ordinary Go parent-PATH/explicit-path
lookup **before** resolution. A selected `PATH` reaches the target but cannot
reselect the initial executable. Go's implicit-current-directory refusal remains.
This is not inode binding or executable authentication.

Preparation does not consume target stdin. The CLI finalizes native sessions,
locks, provider temporary contexts, diagnostics, and signal handling, then uses
POSIX exec. Target stdin/stdout/stderr/status/signals are native and unchanged.

New-command wrapper errors use versioned JSON on stderr with fixed messages:
usage 2, target missing 127, cannot execute 126, other preparation failures 1,
SIGINT/SIGTERM preparation cancellation 130/143. Once exec succeeds, target output
is arbitrary bytes and is not a wrapper receipt or a redacted stream.

## Inspect without resolving

```sh
secret_inject profile list
secret_inject profile describe --profile task
```

These return versioned JSON. List names are sorted, with no invented default.
Describe returns selection/rename policy, `shell_overwrite`, remote policy,
declared env-file patterns, provider kinds/order, and
`resolution:{"kind":"not_evaluated"}`. It omits bindings, private dependency keys,
provider references/arguments, storage settings, actual resolved names, and values.
Neither command initializes storage/providers, probes binaries, reads/matches
local dotenv files, or checks credential readiness. Remote-enabled describe
rejects invalid applicable provider names/order; list/local-only describe ignore
unused remote declarations.

## Exports, cache, and debug

```sh
secret_inject --profile shell
secret_inject --profile task --output json
secret_inject --profile task --output env --force --ttl 30m
secret_inject --clean
secret_inject --profile task --debug
```

Shell output is POSIX syntax; use a compatible shell such as Bash or Zsh.
For an intentional current-shell integration, capture exports, check the status,
then evaluate only a successful result. Prefer `run` when exported shell text is
unnecessary. Config helper confirmations are not secret exports.

Complete ordinary remote snapshots are scoped by source definitions, effective
order, and binding/extraction declarations—not profile, config path, output, TTL,
or token bytes. One native engine stores snapshots, bootstrap records, and
metadata with generation-named chunks and manifest publication. Credential
mutation advances the store-wide revision before payload mutation. Cache reads,
publication, and final handoff are fenced; changed contexts fail without retry.
No native multi-record transaction or retroactive process revocation is claimed.

`--clean` discards workload snapshots, including owned legacy cache entries,
without deleting credentials/revision. It does not need provider executables.
Legacy file-cache cleanup remains available solely for migration, without
payload decoding, credential import, or interactive password recovery.

Debug logs are private and value-free at `$XDG_STATE_HOME/secret_inject/debug.log`
(or `~/.local/state/secret_inject/debug.log`), rotating at 5 MiB with one backup.
They contain selection/provider/cache metadata, never values, provider arguments,
references, raw native/provider errors, or environment dumps. Requested diagnostic
or output failures stop delivery. Non-debug invocations create no debug logs.

## Breaking migration checklist

1. Stop old binaries and keep the old configuration available for explicit cleanup.
2. Allow the host's native backend, or both for a portable config. Remove storage
   passwords and unsupported backends.
3. Explicitly prepare the store and enroll named automation credentials through
   the terminal. No ambient credentials are imported automatically.
4. Bind every enabled source and classify downstream private bootstrap keys.
5. Add/select profiles. Review config-only descriptions; these are not access
   control or a preview of resolved availability.
6. Explicitly clean old caches/exports and remove duplicated ambient tokens.
   Consider vendor rotation; local deletion does not revoke a vendor token.
7. Complete conformance/release gates before activating the managed migration.

The managed personal/work configs use separate Doppler record names and allow
both native backends. Home Manager links the selected config without rewriting it;
`secret_inject` selects the backend for the running system. Source edits do not
enroll credentials, clear live data, or activate the configuration.

## Security boundary

This reduces accidental exposure, not same-user access or agent containment.
Same-user programs can retrieve/debug credentials or substitute provider binaries.
A target receiving workload secrets can print, store, or transmit them. Full
inheritance retains any legacy ambient token copies. Profiles are projections,
not provider access-control boundaries. CLI providers necessarily receive their
own private credentials. Go strings are not securely zeroized.

Config/reference names may themselves be sensitive: keep configuration private.
Persisted secret text must be representable without JSON rewriting; invalid UTF-8
is rejected. Use explicit vendor rotation and the shortest practical workload TTL.

See the [feature wishlist](docs/feature-wishlist.md) for unimplemented directions
such as conversion, additional providers, and multiple instances per provider.

## Temporary managed-host migration wizard

Run the one-time personal/work host migration from a full monorepo checkout:

```sh
bash pub/secret_inject/scripts/migrate-private-credentials.sh
```

This host-specific helper requires Go, Python 3, and the certified Doppler CLI.
It stops before live setup unless the native/provider release gates pass.
It guides terminal-only enrollment and checks warm-cache reuse without provider
executables on PATH. Merge and Nix activation remain manual.

Private choices and config backups stay outside the checkout at
`$XDG_STATE_HOME/secret_inject/migration`, defaulting to
`~/.local/state/secret_inject/migration`. No provider token enters the wizard's
dotenv state. Remove this temporary helper after the hosts have migrated.

## License

MIT; see LICENSE.
