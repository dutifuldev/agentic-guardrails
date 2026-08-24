<p align="center">
  <img src="assets/unclebob_hammer.jpg" alt="Uncle Bob with a hammer" width="320">
  <br>
  <sub>*Not affiliated with Uncle Bob; only inspired by him.</sub>
</p>

# Slophammer

A repository quality checker for agent-assisted software projects.

When agents write most of the code, quality cannot depend on someone reading
every diff. Slophammer checks whether a project has the constraints that keep
AI-generated code under control: agent instructions, CI, tests, strict typing,
linting, coverage gates, duplication budgets, and a structure humans can still
review. It reports what is missing, and it exits with a stable code so the
verdict can gate CI.

This repository holds the product spec, four released implementations, the
shared test fixtures, and project templates — all in one place so agents can
copy tested patterns from working code instead of inventing them.

## Install

Each implementation ships under its own name. Pick the one that matches your
toolchain; any of them can check a repository.

```sh
go install github.com/osolmaz/slophammer/go/cmd/slophammer-go@latest
npm install -g slophammer-ts
cargo install slophammer-rs --locked
uv tool install slophammer-py
```

Then point it at a repository:

```sh
slophammer-go check .
slophammer-go check . --format json
slophammer-go rules
```

`check` exits `0` when the repo is clean, `1` when it has findings, and `2` on
usage or runtime errors.

### Pin the Version in CI

Slophammer makes breaking releases deliberately and ships no compatibility
shims; strict config validation fails loudly across breaking releases by
design. Installing `@latest` in CI absorbs those breaks mid-pipeline. Choose an
exact version from the [releases page](https://github.com/osolmaz/slophammer/releases),
ideally behind one variable so upgrades are a single line:

```sh
go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v<version> check .
npx slophammer-ts@<version> check .
cargo install slophammer-rs --version <version> --locked
uvx slophammer-py@<version> check .
```

The simplest CI integration is the bundled GitHub Action, which requires an
exact version by construction:

```yaml
- uses: osolmaz/slophammer@main
  with:
    checker: go
    version: <version>
```

Pre-commit users can wire the hooks from `.pre-commit-hooks.yaml`
(`slophammer-go-check`, `slophammer-ts-check`, `slophammer-rs-check`), which
run the installed checkers.

One boundary to be explicit about: Slophammer cannot defend against being
removed from a repository. Branch protection and required status checks are
the layer that makes the gate mandatory; the checker reports, your repository
settings enforce.

## Quick Setup: Tell Your Agent About Slophammer

If you are an agent applying Slophammer standards to an existing repository,
start with [Agent Entrypoint](docs/AGENT_ENTRYPOINT.md). It explains how to
choose the right implementation, what to add, which targets to enforce, how to
wire CI, and how to report the result.

Copy the block below and paste it into your coding agent when you want it to
apply Slophammer standards to a repository.

```text
Use Slophammer to enforce repository quality for this project.

Attention agent: start with this file before changing code:
https://raw.githubusercontent.com/osolmaz/slophammer/refs/heads/main/docs/AGENT_ENTRYPOINT.md

Follow it exactly. Detect the target repo's language, use the matching Slophammer
implementation, and say clearly if no matching implementation exists. Pin the
exact checker version you verified against in CI; do not install latest.
```

## What This Is

- A small product spec for a repo quality checker.
- Four released implementations of that spec: `slophammer-go`,
  `slophammer-ts` on npm, `slophammer-rs` on crates.io, and `slophammer-py`
  on PyPI.
- Go, TypeScript, and Python project templates with strict local checks.
- A reference for project structure, testing, errors, reporting, and CI.
- A source of patterns for agents working in different language ecosystems.

## What This Is Not

- A generic starter template collection.
- A framework.
- A replacement for architecture review.
- A claim that code is good because generated checks pass.

## The Checkers

Slophammer is the standard; implementations carry short language-specific
names. The Python checker ships as `slophammer-py` and owns the
`import slophammer` namespace.

| Language   | Command         | Status                                  |
| ---------- | --------------- | --------------------------------------- |
| Go         | `slophammer-go` | Released from this repository's tags     |
| TypeScript | `slophammer-ts` | Released to npm                          |
| Rust       | `slophammer-rs` | Released to crates.io                    |
| Python     | `slophammer-py` | Released to PyPI                         |

The language suffix names the implementation and packaging target, not a hard
limit on what the checker inspects. Each implementation is best at its native
ecosystem first, and it can carry selected checks for other languages when
those checks reuse the shared contract.

### Shared Commands

Every checker supports the same command surface under its own executable name:

```sh
slophammer-go check <path>
slophammer-go check <path> --format json
slophammer-go check <path> --format sarif
slophammer-go check <path> --execute
slophammer-go check <path> --only <rule-id>
slophammer-go explain <rule-id>
slophammer-go rules [--format text|json]
```

Static `check` reads the target repo and reports missing guardrails.
`check --execute` is opt-in and also runs the configured local tool commands,
folding tool failures into the same report. `--only` evaluates focused rules;
it repeats and accepts comma-separated rule IDs. `rules` prints the
implemented rule catalog with text or JSON output so agents can inspect it
without reading source. SARIF output lets GitHub code scanning consume
findings.

### Direct Commands

Each implementation also exposes its native checks directly:

```sh
slophammer-go dry [path] [--max-candidates n] [--show-report] [--format json|text]
slophammer-go coverage [path] [--threshold n] [--profile file]
slophammer-go crap [path] [--max-score n]
slophammer-go mutate [path] [--target file] [--scan]

slophammer-ts dry [path] [--max-findings n] [--show-report] [--format json|text]
slophammer-ts boundaries <path> [--format text|json|sarif]

slophammer-rs dry [path] [--max-findings n] [--format json|text]
slophammer-rs boundaries [path] [--format json|text|sarif]
slophammer-rs unsafe [path] [--format json|text|sarif]
```

When `slophammer.yml` defines policy values, the direct commands use them as
defaults. Explicit CLI flags still win.

### The Report

The checker reports findings such as a missing `README.md`, `AGENTS.md`, or CI
workflow; a missing test command, coverage gate, or linting setup; weak typing
configuration; missing complexity, duplication, or mutation checks; and
dependency imports that cross configured boundaries. The shared report model
stays simple:

```json
{
  "ok": false,
  "findings": [
    {
      "rule_id": "repo.agents-required",
      "severity": "error",
      "path": "AGENTS.md",
      "message": "AGENTS.md is required"
    }
  ]
}
```

## Configuration

`slophammer.yml` at the repository root is the policy file. Every language
section uses the same nested key shape, validation is strict — unknown keys
fail instead of being ignored — and projects may configure stricter thresholds
but not weaker ones. This repo sets these targets for itself:

| Policy                | Target                                          |
| --------------------- | ----------------------------------------------- |
| Coverage              | at least `85`                                   |
| Go CRAP               | at most `8`                                     |
| TypeScript complexity | at most `8`                                     |
| Production DRY        | `0` findings                                    |
| Copied-block tokens   | `100` minimum token window                      |
| Go structural DRY     | `0.82` similarity, `4` lines, `20` nodes        |
| Dependency rules      | declared in `go`, `typescript`, and `rust`      |

Scope is validated like thresholds: excludes that carve out production code
need a `pattern` plus `reason` object form, and configured scope must account
for every production file or the check fails with `scope-incomplete`. For
existing repositories, `check --baseline` grandfathers current findings into a
checked-in, shrink-only `slophammer-baseline.json`; see
[Baseline](specs/BASELINE.md).

A project can disable a rule for `check` and `check --execute` only when it
records a non-empty reason:

```yaml
rules:
  py.mutation-required:
    disabled: true
    reason: Mutation testing runs in a separate required release gate.
```

A disabled rule does not run or produce a finding. Direct commands still run
when called explicitly. See [Config](specs/CONFIG.md) for the full rule order
and boundaries.

The intended DRY policy is production-only: implementation code should trend
toward a zero-candidate budget, while tests are reviewed selectively, fixtures
are excluded, and templates run their own checks. See [Config](specs/CONFIG.md)
for the full shape.

## Rule Set

The shared registry contains 62 implemented rules: 4 repository rules, 12 Go
rules, 15 TypeScript rules, 16 Python rules, and 15 Rust rules. Each
executable prints the rules it implements: repo rules plus its native
language rules.

| Rule ID                             | Meaning                                                       |
| ----------------------------------- | ------------------------------------------------------------- |
| `repo.readme-required`              | The target repo should have a `README.md`.                    |
| `repo.agents-required`              | The target repo should have an `AGENTS.md`.                   |
| `repo.ci-required`                  | The target repo should have a CI workflow.                    |
| `repo.slophammer-ci-required`       | Repos with slophammer.yml must run a checker in CI.           |
| `go.module-required`                | Go projects should include a `go.mod`.                        |
| `go.tests-required`                 | Go projects should run `go test ./...`.                       |
| `go.vet-required`                   | Go projects should run `go vet ./...`.                        |
| `go.lint-required`                  | Go projects should run `golangci-lint`.                       |
| `go.coverage-required`              | Go projects should enforce coverage.                          |
| `go.complexity-required`            | Go projects should check complexity.                          |
| `go.dry-required`                   | Go projects should declare a DRY check.                       |
| `go.crap-required`                  | Go projects should gate `crap4go`.                            |
| `go.mutation-required`              | Go projects should declare `mutate4go`.                       |
| `go.dependency-boundaries-required` | Go projects should obey configured import boundaries.         |
| `go.scope-incomplete`               | Configured Go scope must cover all production files.          |
| `go.suppressions-justified`         | nolint directives in production Go code need a reason.        |
| `ts.package-required`               | TypeScript projects should include `package.json`.            |
| `ts.typecheck-required`             | TypeScript projects should run a no-emit typecheck.           |
| `ts.strict-required`                | TypeScript projects should use strict mode.                   |
| `ts.no-explicit-any`                | TypeScript projects should reject `any`.                      |
| `ts.no-unsafe-types`                | TypeScript projects should reject unsafe type operations.     |
| `ts.lint-required`                  | TypeScript projects should run a configured linter.           |
| `ts.format-required`                | TypeScript projects should run a formatter check.             |
| `ts.test-required`                  | TypeScript projects should run tests.                         |
| `ts.coverage-required`              | TypeScript projects should enforce coverage.                  |
| `ts.complexity-required`            | TypeScript projects should enforce complexity limits.         |
| `ts.dry-required`                   | TypeScript projects should run duplication detection.         |
| `ts.mutation-required`              | TypeScript projects should declare mutation testing.          |
| `ts.dependency-boundaries-required` | TypeScript projects should obey configured import boundaries. |
| `ts.scope-incomplete`               | Configured TypeScript scope must cover all production files.  |
| `ts.suppressions-justified`         | Lint and type suppressions need a description.                |
| `py.project-required` | Production Python code needs project metadata in pyproject.toml. |
| `py.typecheck-required` | Binding CI evidence must invoke a Python typechecker. |
| `py.types-strict-required` | The typechecker configuration must make annotations mandatory and must not be quietly weakened: no unreasoned demotion of stable default-error ty rules, error-on-warning enabled, the ignore-default correctness rules promoted, coded suppressions only, and Ruff ANN annotation coverage. |
| `py.lint-required` | Binding CI evidence must invoke a Python linter. |
| `py.format-required` | Binding CI evidence must verify formatting without mutating files. |
| `py.test-required` | Binding CI evidence must run the Python test suite. |
| `py.coverage-required` | Binding CI evidence must enforce coverage, via --cov-fail-under or a fail_under coverage configuration of at least the configured threshold. |
| `py.complexity-required` | Complexity must be capped at the configured maximum. |
| `py.dry-required` | Binding CI evidence must run a duplication check (slophammer-py dry). |
| `py.mutation-required` | Binding CI evidence must declare a mutation testing tool. |
| `py.suppressions-justified` | Suppression directives in production Python code need a stated reason; bare # type: ignore without an error code is itself a finding. |
| `py.dependency-audit-required` | Binding CI evidence must audit Python dependencies. |
| `py.dependency-boundaries-required` | When python.dependency_boundaries is configured, imports crossing a boundary outside its allow list are findings. |
| `py.typed-marker-required` | A project that builds a published package must ship the py.typed marker, or its checked types degrade to Any for every consumer. |
| `py.absolute-imports-required` | Relative imports defeat grep, break on file moves, and read as dot-counting at depth; production imports must name the package. |
| `py.scope-incomplete` | Every production Python file must be inside a configured scope or covered by a conventional or reasoned exclude, so narrowing scope cannot hide code. |
| `rust.manifest-required`            | Rust projects should include `Cargo.toml`.                    |
| `rust.msrv-required`                | Rust projects should declare an MSRV.                         |
| `rust.check-required`               | Rust projects should run `cargo check`.                       |
| `rust.fmt-required`                 | Rust projects should run `cargo fmt --check`.                 |
| `rust.clippy-required`              | Rust projects should run `cargo clippy` with warnings denied. |
| `rust.test-required`                | Rust projects should run `cargo test`.                        |
| `rust.coverage-required`            | Rust projects should enforce coverage.                        |
| `rust.complexity-required`          | Rust projects should enforce complexity limits.               |
| `rust.dry-required`                 | Rust projects should run duplication detection.               |
| `rust.mutation-required`            | Rust projects should declare mutation testing.                |
| `rust.unsafe-policy-required`       | Rust projects should declare and respect unsafe policy.       |
| `rust.dependency-audit-required`    | Rust projects should run dependency audit checks.             |
| `rust.dependency-boundaries-required` | Rust projects should obey configured dependency boundaries. |
| `rust.scope-incomplete`               | Configured Rust scope must cover all production files.      |
| `rust.suppressions-justified`         | allow attributes in production Rust code need a reason.     |

The TypeScript rules are tool-agnostic: they accept `tsc` or `tsgo` type
checks, ESLint/Oxlint/Biome linting, Prettier/Oxfmt/Dprint/Biome formatting,
common Node test runners, and `c8`/`nyc`/Vitest/Jest coverage gates. The exact
rule behavior lives in [Rules](specs/RULES.md) so every implementation shares
the same contract.

## Development

`fixtures/` is the acceptance contract: pairs of small repositories and the
exact reports they must produce. `scripts/check-conformance.mjs` runs every
implementation against its fixture set and verifies report shape, findings,
and exit codes, which is how three codebases stay one product.

Run `make check` from the repo root to run the same gates CI runs, or a
per-language target such as `make check-go`. CI also runs every checker
against this repository itself, so Slophammer must pass Slophammer before
anything merges. Release tags (`go/vX.Y.Z`, `typescript/vX.Y.Z`,
`rust/vX.Y.Z`, and `python/vX.Y.Z`) drive workflows that validate, publish,
and create the [GitHub releases](https://github.com/osolmaz/slophammer/releases);
the release policy and shared architecture live in [Product](specs/PRODUCT.md)
and [Implementation Model](docs/IMPLEMENTATION_MODEL.md).

## Guardrail Principles

1. Keep the core isolated.
   Business rules should be ordinary code with direct tests. Frameworks,
   databases, queues, HTTP, file systems, and external APIs belong at the edges.

2. Make weak types fail early.
   Avoid broad escape hatches. If a boundary is dynamic, validate it and convert
   it to a typed shape.

3. Prefer fast local checks.
   Formatting, linting, type checking, and unit tests should be cheap enough to
   run constantly.

4. Separate policy from plumbing.
   The important rules should not depend on CLIs, web servers, ORMs, or cloud
   SDKs.

5. Apply checks to generated code.
   Generated code must pass the same formatter, linter, type, test, and boundary
   checks unless it is explicitly excluded in config.

## Concept Docs

Start with [Uncle Bob Concepts](docs/UNCLE_BOB_CONCEPTS.md) for the wiki-style
notes behind the guardrails.

See [Implementation Model](docs/IMPLEMENTATION_MODEL.md) for the shared
architecture and the detailed per-language policy.

See [Product](specs/PRODUCT.md), [Report Format](specs/REPORT_FORMAT.md), and
[Exit Codes](specs/EXIT_CODES.md) for the shared compatibility contract.

See [DRY](docs/DRY.md) for the duplication policy informed by Uncle Bob's
structural `dry4clj`, `dry4java`, and `dry4go` tools.
