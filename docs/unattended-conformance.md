# Unattended conformance and release gates

## Status

The private-record engine, native adapters, credential commands, private resolver,
inspection, POSIX exec path, migration, and native-capable build configuration are
implemented. This is **not** a native/provider conformance result.

Certification gates releases, not ordinary local setup or fetching. Runtime
provider calls retain private configurations, token-free version probes, version
output validation, timeouts, and sanitized failures. Version syntax is not proof
that a particular CLI supports all flags or honors the unattended recipe.

`certifiedProviderReleases` exists only in the `releasegate` tests and is
intentionally empty. Do not populate that evidence table from source inspection
or a successful fake-provider test. The migration wizard runs ordinary synthetic
checks and then asks the operator before native setup and enrollment.

The release workflow requires
`go test -tags releasegate ./internal/source ./internal/storage`.
It must fail until all three providers have an exact certified release and all
shipped native platforms/architectures have reviewed runtime conformance evidence.
Normal `make check` is an implementation gate, not authorization to release.

## Release certification authority and isolation

- Do not use a developer's live keyrings, accounts, token environment, vendor
  sessions, or production secrets. Never print credentials, provider references,
  raw provider output, native error strings, or environment dumps.
- Native runtime conformance needs a disposable OS account/VM with a private
  app namespace. Use synthetic values only. Capture exit/status/operation metadata,
  not raw data. Delete only the fixture resources the test created.
- Authenticated provider conformance additionally requires explicit authority and
  dedicated nonproduction credentials/accounts for each exact candidate release.
  Enrollment stays terminal-only; never place tokens in test argv, fixture files,
  CI logs, a generic environment dump, or this document.
- Neither source edits nor the setup instructions authorize activation, vendor
  rotation, live cache deletion, credential enrollment, commits, or publication.

## Guarded fixture entrypoints

The opt-in fixture tags refuse to run unless the OS username starts with
`secret-inject-conformance` and
`SECRET_INJECT_CONFORMANCE_AUTHORIZED=disposable-account-only` is explicitly set.
They are test binaries, not runtime CLI bypasses. Do not use them in a developer
account. Prepare the disposable account's app store explicitly before native tests.

```sh
# In the authorized disposable account/VM only:
secret_inject store prepare --config ./fixture-config.json
SECRET_INJECT_CONFORMANCE_AUTHORIZED=disposable-account-only \
  go test -tags nativeconformance -timeout 2m ./internal/storage
```

The native entrypoint exercises real prepared-store CRUD, bounded-size credential
and workload objects, role-safe cleanup, and revision fencing. Complete the
remaining negative/race/UI matrix below separately before recording evidence.

The provider entrypoint additionally requires `SECRET_INJECT_CONFORMANCE_CONFIG`
to name a private configuration containing three direct stored bindings. Enroll
those dedicated nonproduction roots through the CLI terminal first. Each provider
must expose `CONFORMANCE_OK=synthetic-fixture-ok` through its configured fixture.
`SECRET_INJECT_CONFORMANCE_VERSIONS` is non-secret JSON mapping each provider kind
to its exact candidate version output. It must not contain credentials. Then run
`go test -tags providerconformance -timeout 5m ./internal/source` under the same
explicit disposable-account guard. The tagged fixture checks each installed
candidate against the supplied version data before acquiring credentials. It does
not populate the release evidence table or constitute certification. The entrypoint
tests successful synthetic reads and invalid-token failure through the controlled
context. Complete all remaining matrix cells.

## Native matrix

Run every ordinary operation with UI/terminal input unavailable. A test timing
out is a failure, not evidence of no prompting. Test interactive preparation
separately through the same adapter/engine implementation.

| Surface | Required cases |
|---|---|
| Open/discovery | Prepared and missing container; unavailable service; locked container; wrong backend; duplicate/ambiguous Linux collections; cancellation. |
| Read/metadata | Present/missing values, locked items, permission failures, lock races, malformed manifests/chunks, native query/enumeration errors. |
| Write/delete | Create/replace/remove, locked/raced items, native size boundary, failed and post-commit/uncertain replies, per-item authorization changes. |
| Prompt policy | macOS save/disable/restore interaction on all paths; Linux raw mutation prompts and IsLocked errors never lead to Prompt unattended. |
| Setup | Explicit prepare/enrollment can create/unlock/authorize; normal operations cannot. Preparation alone preserves credentials/revision/cache eligibility. |
| Ownership | Private same-store locks coordinate independent processes/config files; symlink/nonregular/wrong-owner files fail; connection/session/handle cleanup precedes handoff. |
| Storage roles | Credentials and snapshots share bounded native byte/object CRUD. Snapshot/legacy cleanup and orphan collection preserve credential/revision roles. |
| Fencing | Mutation advances revision before payload; failures conservatively invalidate; acquisition is atomic; network work holds no store lock; stale publication and final handoff fail without retry. |

Prove the actual configured native value limits and both architecture artifacts,
not only the synthetic 512-byte adapter limit used by unit tests. Checksums are
corruption detection, not an application-exclusive security boundary.

## Provider matrix

Source-backed candidates are Doppler 3.75.1 and bws 1.0.0. No exact op release is
selected/certified yet. A candidate is not a supported matrix entry.

For each exact release, retain non-secret evidence of binary version/digest,
platform/architecture, recipe, assertions, and completion status:

1. Token-free private configuration exists **before** the version command. Verify
   complete startup/hook behavior and the exact safe output parser for the probe.
2. Unsupported/empty/malformed version results refuse fetching. Probe failures
   never echo raw output; inspection/cache hits do not execute probes.
3. Valid dedicated automation credentials fetch synthetic values as expected.
   Invalid/missing tokens fail with no personal-session, Connect, biometric,
   browser, login, signin, or ambient-token recovery.
4. Competing auth selectors and old personal configuration cannot redirect auth.
   The private credential reaches only its consuming provider, never another
   provider, ordinary snapshot, export, selected target variable, or diagnostic.
5. Validate documented endpoint/proxy/CA controls. For bws, prove that consuming
   `BWS_SERVER_URL` into `server_base` preserves the selected profile and
   `state_opt_out = "true"`; no URL flag/env bypass may reactivate state files.
6. Prove bounded subprocess execution, caller cancellation, target stdin isolation,
   private config cleanup, native close/finalization, and sanitized failures.
7. Verify exact source-order capture, missing private dependencies, unchanged
   ordinary merge/projection behavior, and cache/source/credential isolation.

Doppler must have a preexisting token-free `.doppler.yaml` containing `{}` to
avoid personal-config migration. Do not invent an ENCLAVE_TOKEN alias. op must
use a private config directory, disabled biometric/cache integration, and no
competing Connect/session selectors. bws configuration is TOML, not JSON.

## Artifact gate

Use Go 1.25 for compatibility checks. Both macOS archives require native SDK/CGO
builds; non-CGO macOS builds intentionally cannot open the native store. Before
publishing, inspect both archives' architecture, `go version -m` CGO setting,
Security/CoreFoundation linkage, and run the relevant native fixture matrix on
matching architecture hardware/VMs. Linux compilation alone does not replace its
runtime fixtures. Keep releases blocked on any unexecuted required
matrix cell.

## Headless systemd smoke test

The `linux-systemd-credentials` adapter has a separate opt-in synthetic probe:

```sh
SECRET_INJECT_SYSTEMD_SMOKE=synthetic-only \
  go test -tags systemdsmoke ./internal/storage \
  -run '^TestSystemdCredentialsSyntheticHost$' -count=1 -v
```

It uses a private temporary directory and synthetic bootstrap/workload values,
not the production store. It exercises native user-scoped encryption, reopening,
multi-chunk snapshots, cleanup, and credential preservation. It may initialize
systemd's root-protected host key, so obtain operator approval before running it.
The encrypted temporary records are removed by the test. It never requests an
authentication prompt or enrolls a provider token.

The implementation was exercised over SSH on Supermicro with systemd 261.3 and
no graphical session or TPM. That is host smoke evidence, not reviewed release
certification for either Linux architecture or the Secret Service adapter.
Release gates continue to require separately reviewed backend/platform evidence.

## Managed migration

The personal/work JSON assets now declare separate named Doppler roots and no
file-cache password. The shared configs allow `darwin-keychain` and
`linux-systemd-credentials`; `secret_inject` selects the host's backend without
Home Manager rewriting the JSON. `linux-secret-service` remains an explicit
alternative for an existing Secret Service session.
Human preparation/enrollment and host-local smoke tests do not require release
certification. Activate the managed config only after that host's fresh-fetch and
warm-cache checks pass; distributing a certified release still requires the full
matrix. Explicitly remove old exports/caches/ambient duplicates and
consider vendor token rotation; neither migration nor local deletion revokes a
vendor credential automatically.
