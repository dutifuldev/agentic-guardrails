---
title: Add AGENTS.md Initialization and Checks
author: Onur Solmaz <2453968+osolmaz@users.noreply.github.com>
date: 2026-08-18
---

# Add AGENTS.md Initialization and Checks

Slophammer currently checks only that a root `AGENTS.md` exists. A repository can pass with an empty or generic file that gives an agent no useful instructions. Slophammer must be able to create a concise starter file from repository evidence and check that agent instructions remain useful as the repository changes.

## Requirements

- Keep `AGENTS.md` at the repository root as the required entry point.
- Add the same `agents init` and `agents check` command surface to every Slophammer implementation.
- Derive starter content from files already in the repository. Do not invent commands.
- Keep generated instructions concise and focused on commands, package areas, and working rules.
- Refuse to replace an existing `AGENTS.md` unless the caller passes an explicit replacement flag.
- Report stable findings through the existing text, JSON, and SARIF report contract.
- Check independent package areas without requiring one local file for every manifest. An ancestor file can cover a package when it names a command that applies to that package.
- Keep the implementation dependency-free beyond each package's current dependencies.

## Design

Each implementation will add an `agents` module with the same pure operations:

1. Inspect a repository snapshot.
2. Detect package boundaries from supported manifests.
3. Derive a small set of verification commands from root runner targets, package scripts, and language manifests.
4. Render a deterministic root `AGENTS.md` starter.
5. Evaluate the AGENTS.md rule set.

The public commands will be:

```sh
slophammer-<lang> agents init [path] [--dry-run] [--force]
slophammer-<lang> agents check [path] [--format text|json|sarif]
```

`agents init` writes `AGENTS.md` when it is absent. `--dry-run` prints the proposed file without writing it. `--force` permits replacement and cannot be combined with `--dry-run` only if an implementation cannot make that combination useful. The command exits with code `2` for invalid input, scan failures, and refused replacement.

`agents check` runs only the shared `repo.agents-*` rules. The normal `check` command also runs these rules by default and supports them through `--only`.

## Rules

The existing `repo.agents-required` rule remains the root presence rule. The following shared rules are added:

| Rule | Purpose |
| ---- | ------- |
| `repo.agents-empty` | Reject a file with no useful prose or commands. |
| `repo.agents-commands-required` | Require at least one verification command backed by repository evidence. |
| `repo.agents-command-invalid` | Reject generated command entries that no longer have backing repository evidence. |
| `repo.agents-scope-required` | Require package-specific instructions only when no governing AGENTS.md names a command that applies to that package. |
| `repo.agents-stale` | Report a generated evidence block whose recorded package or command facts no longer match the repository. |

The generated file will contain a bounded Slophammer evidence block. The block has explicit start and end comments and lists the package paths and commands used to create the starter. Maintainers can edit all prose outside the block. Slophammer compares only the bounded evidence block when it checks for stale or invalid generated facts.

A hand-written `AGENTS.md` does not need the generated block. It passes when it contains useful text and at least one detected verification command. This keeps the format compatible with the open AGENTS.md convention and avoids making Slophammer metadata mandatory.

## Package Scope

Package boundaries come from manifests such as `go.mod`, `package.json`, `pyproject.toml`, and `Cargo.toml`. Build output, dependencies, fixtures, and hidden tool caches remain outside the scan.

A package is governed by the nearest ancestor `AGENTS.md`. A separate local file is required only when that governing file does not name any detected command for the package. Root umbrella commands such as `make check` can govern all package areas when the repository defines them.

## Non-goals

- Grade repositories with a weighted score or letter grade.
- Judge writing style or use a language model during checks.
- Require a local `AGENTS.md` at every package boundary.
- Convert or synchronize `CLAUDE.md` files.
- Treat AGENTS.md text as an enforcement mechanism. CI and local quality gates remain the enforcement layer.

## Acceptance Criteria

- All four public CLIs expose `agents init` and `agents check` with equivalent behavior.
- `agents init --dry-run` produces the same normalized document for shared fixtures.
- Initialization refuses to replace an existing file unless `--force` is present.
- Empty, command-free, invalid generated, stale generated, and uncovered package fixtures produce the specified rule IDs and paths.
- Existing clean fixtures remain clean after their AGENTS.md files are hydrated.
- `rules`, `explain`, `check --only`, baselines, JSON, and SARIF support the new rules.
- Slophammer's own root `AGENTS.md` passes the new checks.
- Shared conformance and all language test suites pass.

## Verification

```sh
npx -y @simpledoc/simpledoc check
make check-go
make check-typescript
make check-rust
make check-python
node scripts/check-conformance.mjs
make check
```

Run focused CLI smoke tests for each implementation against a temporary repository. Verify creation, dry-run output, replacement refusal, forced replacement, JSON findings, and a clean generated file.
