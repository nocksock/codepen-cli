# ADR-0001: Secrets are a user concern

Status: accepted — 2026-10-27 (amended: `pen init` no longer writes `.envrc`
either). Supersedes the secret-handling parts of
`docs/specs/2026-10-24-go-rewrite-design.md`.

## Context

`pen` used to accept `CODEPEN_API_KEY=op://…` and resolve it by running
`op read`, and `pen init` wrote a `.env` containing a 1Password reference by
default. That made 1Password an implicit dependency, and users without it (or
users who already inject the key through their environment, direnv, or another
secret manager) had to work around the default.

Reading `CODEPEN_API_KEY` from the environment and the two env files is ordinary
configuration loading. Resolving a secret-manager reference is not: it couples
the CLI to one vendor, spawns a subprocess per command, and can prompt
interactively.

## Decision

1. `pen` never resolves references and never runs a secret-manager program.
   `CODEPEN_API_KEY` is used exactly as given.
2. The precedence stays: process environment, then `.env.local`, then `.env`
   (empty values count as unset). `.env` is read-only; the user owns it.
3. `pen init` adds `.env` and `.env.local` to `.gitignore`. It does not create
   or edit `.env` or `.envrc`, and it never runs direnv.
4. A key that still starts with `op://` is rejected with guidance to resolve it
   outside the CLI, e.g. `op run -- pen push`. This is validation, not
   resolution: it turns a confusing 401 after upgrading into a clear error.

## Consequences

- `pen init --key` is gone; setting a key is the user's job.
- 1Password users run `op run -- pen …` (or let their own direnv setup inject
  the key).
- Docker/CI and existing direnv setups keep working unchanged.
- `internal/secret` is deleted; `setup.DefaultKey` is removed.
