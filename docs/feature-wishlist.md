# secret_inject feature wishlist

Status: discussion draft. These are desired capabilities, not an approved design
or a commitment to particular commands, config syntax, or implementation order.

## Direction

Make `secret_inject` a utility knife for secrets, API keys, and environment
variables: gather values from different sources, choose what a workflow needs,
and deliver them to a shell, command, or file with deliberate exposure controls.

## Current baseline

- Doppler, 1Password, and Bitwarden Secrets Manager are already supported together.
- Sources run in a configured sequence. Earlier results are available to later
  provider CLIs, allowing credentials from one provider to unlock another.
- Results merge into one set; later sources overwrite duplicate variable names.
- Configuration currently allows one source entry per provider type.
- Output formats already include shell exports, JSON, and env-file text.
- Fetched values are cached using keyring or development-only plaintext storage.
- Named workflow profiles support star-pattern include/exclude rules, exact key
  renaming, shell overwrite control, and values-free missing-include checks.
- Caches are scoped by source configuration; `--debug` writes a private rotating
  log without secret values or raw provider output.
- Profiles can overlay fresh current-directory dotenv files on remote sources,
  or disable remote sources and caches entirely for local-only output.
- There is no input conversion command or direct command-execution mode today.

## Wishlist

### 1. Compose secrets from multiple providers

**Desired outcome:** One invocation can assemble the secrets needed for a
workflow, regardless of where they are stored.

- Retain existing provider chaining and merged output.
- Decide whether to support multiple instances of the same provider, such as two
  Doppler projects or multiple local env files.
- Make duplicate-name handling predictable and understandable without printing
  values. Decide whether collisions overwrite, fail, or require explicit policy.
- Distinguish a missing required source from an intentionally optional source;
  do not silently produce an incomplete environment.

**Discussion:** Is the existing provider composition sufficient for now, or are
multiple projects/accounts of the same provider an immediate need?

### 2. Produce one environment for shell creation

**Desired outcome:** A shell starts with a single selected set of variables
assembled from the configured sources.

- Continue supporting exports that a shell integration can consume.
- Consider launching a child shell or command directly with the resolved
  environment, avoiding secret-bearing shell text when it is unnecessary.
- Define how selected values interact with variables already in the environment.
- Identify supported shells rather than assuming one export syntax works in all
  of them. A child process cannot modify its parent shell's environment directly.

**Discussion:** Should the primary workflow be loading the current shell,
starting a new shell, or both?

### 3. Omit selected secrets from shell creation

**Desired outcome:** A value can be available for a specific purpose without
being exported into every interactive shell.

- Support explicit inclusion/exclusion of variables for shell use.
- Keep provider bootstrap credentials usable for fetching without automatically
  exporting them to the final shell.
- Consider reusable selections for interactive shells, individual commands, and
  agent tasks; decide whether named profiles are actually needed.
- Make selection inspectable through names and source metadata, not raw values.
- Define whether exclusions apply only to fetched values or also remove matching
  variables inherited from the parent environment.

**Discussion:** Are exclusions attached to individual secrets, to each workflow,
or both? Should shell selection be an allowlist or an exclusion list?

### 4. Safe use by agents

**Desired outcome:** An agent can perform authorized work using a secret without
needing to read or reproduce the raw value in its prompt, tool arguments, or
conversation history.

- Explore a command-execution mode that resolves secrets internally and injects
  only the selected variables into the target process.
- Provide non-interactive behavior and useful failures without exposing values
  through diagnostics, provider errors, or debug logs.
- Separate value-revealing export/conversion operations from agent-oriented use;
  names-only inspection should not require fetching or displaying values when
  the source permits it.
- Decide who authorizes access and where that policy lives. Agent-controlled
  flags or repository config alone are not a meaningful approval boundary.
- Define inheritance, cache access, and child stdout/stderr behavior explicitly.

**Safety boundary:** Environment injection reduces incidental exposure; it is
not a sandbox. A command receiving a secret can print it, write it, or transmit
it. An agent allowed to run arbitrary commands with credentials can therefore
recover them. Output redaction alone cannot guarantee containment. Stronger
restrictions would require a separately defined trusted execution boundary or
narrowly scoped operations, not merely a `run` wrapper.

**Discussion:** Is the first goal avoiding accidental disclosure to agent
context, or enforcing restrictions even against agent-selected commands?

### 5. Local env files as a source

**Desired outcome:** Locally managed env files participate in the same source
composition and selection workflow as remote secret providers.

- Read one or more explicitly selected files and merge them under the same
  duplicate-name policy as other sources.
- Keep env-file input distinct from plaintext cache storage; they serve different
  purposes.
- Define paths, missing-file behavior, and permission expectations.
- Treat file content as data, not executable shell code.
- Define quoting, multiline values, interpolation, and duplicate keys rather
  than assuming every dotenv implementation uses the same grammar.
- Ensure local edits and changes to source configuration cannot reuse unrelated
  or stale cached results unnoticed.

**Discussion:** Start with explicit file paths, or also discover project-local
files? Should local sources bypass caching?

### 6. Convert env files and exported environment variables

**Desired outcome:** Move a selected set of values between env files and shell
exports without manual rewriting.

- Convert an env file to safely quoted shell export statements.
- Convert selected variables from the current process environment to env-file
  text, without dumping the entire inherited environment by default.
- Clarify whether the reverse direction also includes parsing a file containing
  literal `export NAME=...` assignments. If supported, reject executable shell
  constructs rather than evaluating or sourcing arbitrary scripts.
- Define a supported format that preserves values across conversions, including
  empty strings, quotes, dollar signs, backslashes, and newlines. Document
  unsupported inputs; do not promise preservation of comments or formatting.
- Support deliberate output to stdout or a file. Define restrictive file
  permissions and explicit overwrite behavior for secret-bearing files.

**Discussion:** Does “vice versa” mean capturing the current environment,
parsing shell export text, or both?

## Proposed discussion order

This is a starting point, not an implementation plan:

1. Agree on the agent safety boundary and the shell-selection model.
2. Define source composition, collisions, and local env-file input semantics.
3. Define shell/command delivery and conversion behavior.
4. Choose a small first implementation and its compatibility/test requirements.

Preserve existing configurations and shell workflows unless we explicitly agree
on a migration. Implementation work should test value fidelity, selection and
inheritance, provider chaining, cache isolation, and secret-free diagnostics.

## Related existing roadmap

The [README roadmap](../README.md#roadmap) also lists AWS Secrets Manager,
HashiCorp Vault, Azure Key Vault, prefix/pattern filtering, name transformation,
and direct command execution. Those remain candidates, not additional agreed
requirements for this wishlist.
