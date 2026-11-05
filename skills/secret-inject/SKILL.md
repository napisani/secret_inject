---
name: secret-inject
description: Use secret_inject from an agent session without leaking secret values into the chat context. Use when running a command that needs secrets, API keys, or tokens from Doppler, 1Password, Bitwarden, or local .env files; checking which secrets a profile provides; or diagnosing a secret_inject error.
---

# secret_inject for agents

`secret_inject` resolves secrets from configured providers and local dotenv
files and hands them to a command's environment. Everything a tool call prints
lands in your context window and is sent to the model provider, so the rule for
this whole skill is: **values stay in the child process**. You handle profile
names, variable names, exit codes, and value-free JSON. You never handle values.

## Pick the command by what it prints

Value-free, safe to run and read:

| Command | Prints |
| --- | --- |
| `secret_inject profile list` | profile names |
| `secret_inject profile describe --profile P` | selection/rename policy, env-file patterns, provider kinds; no bindings or values |
| `secret_inject profile check --profile P` | `{"profile":"P","missing_includes":[...]}`; exit 0 complete, 2 missing, 1 failure |
| `secret_inject credential list` | enrolled credential names and provider kinds |
| `secret_inject run --profile P -- CMD ...` | only what `CMD` prints |
| `secret_inject --clean` | nothing secret; drops cached snapshots, keeps credentials |

These print values to stdout, so keep them out of your tool calls unless the
output is captured and never echoed: the bare export form
`secret_inject --profile P`, plus `--output json` and `--output env`.

Pass `--config PATH` when the project uses a config other than
`~/.config/.secret_inject.json`.

## Run a command with secrets

1. Find the profile: `profile list`, then `profile describe --profile P` to
   confirm it selects what the task needs.
2. Run the target through `run`. The `--` separator is mandatory:

   ```sh
   secret_inject run --profile P -- npm run deploy
   ```

3. When the command line itself references a secret, wrap it in `sh -c` with
   **single quotes** so the variable expands inside the child:

   ```sh
   secret_inject run --profile P -- sh -c 'curl -sf -H "Authorization: Bearer $API_KEY" https://api.example.com/health'
   ```

   With double quotes your own shell expands `$API_KEY` before secret_inject
   runs, so the child receives an empty string or an ambient value, and a
   non-empty expansion is written into the transcript.

4. Each tool call is a fresh shell, so exported variables do not carry over
   between calls. Put a multi-step flow inside one `sh -c '...'` or a script
   file and run that under `secret_inject run`.

`run` inherits the whole parent environment and overlays the profile's values.
A variable can therefore be present from the ambient environment even when the
profile missed it; `profile check` is the authoritative completeness test.

## Keep target output value-free

`run` passes the target's stdout and stderr through unredacted. Choose
commands and flags whose output contains no credentials:

- Check presence, not content:
  `secret_inject run --profile P -- sh -c 'test -n "${API_KEY-}" && echo API_KEY=set || echo API_KEY=missing'`.
- Leave out environment dumps and tracing: `env`, `printenv`, `echo $VAR`,
  `set -x`, `bash -x`, `curl -v`, HTTP-client debug modes, `docker inspect`,
  and framework "print config" commands all echo values or auth headers.
- When a target's output might include secrets (an unfamiliar CLI, a failing
  deploy that dumps its config), redirect it to a file outside the repo and
  report only the exit status. Read back only lines you have filtered to known
  safe patterns, and tell the user where the full log is.
- Scripts you write for `run` log variable names and outcomes, never values.

## Files that hold secrets

- **Dotenv files** matched by a profile's `env_files` (often `*.env`,
  `*.env.local`) contain raw values. To see which keys a file defines, print
  only the names: `grep -oE '^[A-Za-z_][A-Za-z0-9_]*=' file.env`.
- **The config file** holds no values, but its provider references (Doppler
  project names, `op://` paths, Bitwarden IDs) are private. Use
  `profile describe` for questions about a profile; open the raw config only
  when the user asks you to edit it.
- **Exported files**: if a tool truly needs an env file, tell the user and let
  them decide. Writing `--output env` to disk creates a plaintext secret file
  that must stay gitignored and unread.

## Steps that belong to the user

These need a real terminal, masked input, or an OS dialog, so ask the user to
run them in their own terminal, outside the agent session:

- `secret_inject credential set NAME --provider KIND` enrolls a provider token
  through a masked prompt. It accepts no token argument, pipe, or env var by
  design. Never ask the user to paste a token into the chat.
- `secret_inject store prepare` creates or unlocks the native store and may
  show OS authorization dialogs.
- `secret_inject config edit` opens `$EDITOR`.

## Diagnose failures

Wrapper errors are value-free JSON on stderr, for example
`{"schema_version":1,"error":{"code":"credential_missing","message":"credential is unavailable"}}`.
Once the target has started, the exit code and output are the target's own.

| Code / exit | Meaning and next step |
| --- | --- |
| `profile_selection` | Profile name is wrong; run `profile list`. |
| `configuration` | Config is unreadable or invalid; ask the user to fix it. |
| `credential_missing` | No enrolled credential for a source; the user runs `credential set`. |
| `interaction_required`, `store_unavailable` | Store not prepared on this host; the user runs `store prepare`. |
| `credential_context_changed` | Credentials changed mid-resolution; retry once. |
| `provider_failure`, `provider_timeout` | Provider auth or network failed; retry with `--force --debug`, then read the log. |
| `provider_version_unsupported` | Provider CLI (`doppler`, `op`, `bws`) is missing or too old. |
| exit 127 / 126 | Target executable not found / not executable. |
| `usage`, exit 2 | Bad invocation, often a missing `--` or `--profile`. |

`--debug` writes a value-free log to
`${XDG_STATE_HOME:-~/.local/state}/secret_inject/debug.log`, which is safe to
`tail`. `--force` bypasses the cached snapshot; `--ttl 30m` shortens cache
lifetime.

## Limits of the protection

secret_inject reduces accidental exposure. It does not contain you or the
target: any process holding the values can print, store, or send them. The
protections above hold only while every command you run keeps values inside
the child.
