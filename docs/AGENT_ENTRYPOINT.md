# Agent Entrypoint

Attention agent: start here when you are asked to apply Slophammer standards to
an existing repository.

Only add files, scripts, and checks that are enforced locally or in CI. Prefer
small, reviewable changes that leave maintainers with commands they can run
locally.

## Operating Rules

- Do not weaken existing tests, linters, type checks, or CI to make your work
  pass.
- Do not add broad type escapes such as `any`, `Any`, unchecked casts,
  reflection, or dynamic maps unless they are isolated at an external boundary.
- Do not hide failures behind warnings unless the repo owner explicitly asks for
  a staged rollout.
- Do not add a dependency when the language standard library or existing project
  tooling is enough.
- Keep domain behavior separate from IO, frameworks, databases, queues, clocks,
  random values, networks, and process state.
- Add or update tests for every behavior change.
- Run the checks before you finish.

## First Pass

Start by reading the repository before changing it:

```sh
find . -maxdepth 3 -type f | sort | head -200
```

Identify:

- the primary language or languages
- the package manager and build tool
- existing test, lint, type-check, and coverage commands
- CI workflow files
- existing agent instruction files
- whether generated, fixture, vendor, or template code should be excluded from
  production quality budgets

Then make the smallest plan that turns the existing project into an enforceable
project. Do not rewrite the project structure unless the current structure blocks
quality checks.

If the repository has no useful root `AGENTS.md`, preview a starter with the
selected checker:

```sh
slophammer-<lang> agents init . --dry-run
```

Review the derived commands and package areas before writing the file. Run
`slophammer-<lang> agents check .` after editing it. Existing files are never
replaced unless you pass `--force`.

## Choose The Implementation

Choose the Slophammer implementation from the target repository's primary
language. Do not use an unrelated implementation just because it is available.

Use this selection model:

| Target repo language | Slophammer command | Current availability                                                             |
| -------------------- | ------------------ | -------------------------------------------------------------------------------- |
| Go                   | `slophammer-go`    | Public Go install                                                                |
| TypeScript           | `slophammer-ts`    | Available from npm and this repo's TypeScript implementation |
| Rust                 | `slophammer-rs`    | Available from crates.io and this repo's Rust implementation |
| Python               | `slophammer-py`    | Released to PyPI                                                                 |

Pin the exact checker version you verify against. Local one-off runs may use
`@latest`, but anything you write into the target repository's CI must pin an
exact version, ideally behind one variable so upgrades are a single line.
Slophammer makes breaking releases deliberately, and an unpinned CI install
absorbs them mid-pipeline. The simplest CI integration is the bundled GitHub
Action, which requires an exact version:

```yaml
- uses: osolmaz/slophammer@main
  with:
    checker: go
    version: 0.4.0
```

For a Go target outside this source tree, install the current released checker:

```sh
go install github.com/osolmaz/slophammer/go/cmd/slophammer-go@latest
```

For this repository's Go implementation, use the source-tree command:

```sh
cd go
go run ./cmd/slophammer-go help
```

For TypeScript, use `slophammer-ts` only when the command is available from a
local Slophammer checkout or package artifact. If there is no installable
matching implementation for the target language, say that clearly and do not
claim Slophammer passed. You may still apply the documented standards manually,
but report that the language-specific Slophammer checker could not run.

For Rust targets outside this source tree, install the current released checker:

```sh
cargo install slophammer-rs --locked
slophammer-rs help
```

For this repository's Rust implementation, use the source-tree command:

```sh
cd rust
cargo run -p slophammer-rs -- help
```

For Rust release work, follow the
[Rust CLI-only Cargo publish plan](2026-06-08-rust-cli-only-cargo-publish-plan.md).
Do not publish internal implementation modules as separate crates just to
satisfy Cargo dependency resolution. The Rust release workflow should publish
only the user-facing `slophammer-rs` package.

Before changing files, inspect the implemented rule catalog for the selected
checker:

```sh
slophammer-go rules --format json
slophammer-ts rules --format json
slophammer-rs rules --format json
```

Then run the selected checker against the target repo:

```sh
<slophammer-command> check . --format json
```

## Required Files

Slophammer expects these files:

```text
README.md
AGENTS.md
slophammer.yml
.github/workflows/ci.yml
```

If the repo already has equivalent CI, update it instead of adding a parallel
workflow.

## Agent Instructions

Create or update `AGENTS.md` with repository-specific instructions. Keep it
short and enforceable.

At minimum, include:

- the local commands agents must run before finishing
- language-specific typing rules
- testing expectations
- dependency rules
- architecture boundaries that should not be crossed
- a pointer back to this entrypoint if Slophammer standards are being applied

Do not fill `AGENTS.md` with generic advice that no tool checks.

## Slophammer Config

Add `slophammer.yml` at the repository root.

For Go projects, start with this policy:

```yaml
go:
  coverage:
    threshold: 85
  targets:
    - go
  exclude:
    - "fixtures/**"
    - "templates/**"
  dry:
    max_findings: 0
    paths:
      - go/cmd
      - go/internal
    exclude:
      - "**/*_test.go"
      - "fixtures/**"
      - "templates/**"
    structural:
      enabled: true
      threshold: 0.82
      min_lines: 4
      min_nodes: 20
    copied_blocks:
      enabled: true
      min_tokens: 100
  crap:
    max_score: 8
  mutation:
    targets:
      - go/internal/rules
```

Adjust `go.targets`, `go.exclude`, `go.dry.paths`, `go.dry.exclude`, and
`go.mutation.targets` to match the target repository.

These are hard quality targets:

- coverage must be at least `85`
- CRAP must be at most `8`
- production DRY candidates should be `0`

Use stricter values when the repo already supports them. Do not use weaker
values.

For Rust projects, start with this policy:

```yaml
rust:
  coverage:
    threshold: 85
  complexity:
    cognitive_max: 8
  targets:
    - src
  dry:
    max_findings: 0
    paths:
      - src
    copied_blocks:
      enabled: true
      min_tokens: 100
  unsafe:
    policy: forbid
  mutation:
    targets:
      - src
  dependency_boundaries:
    - from: .
      allow: []
```

Adjust `rust.targets`, `rust.dry.paths`, `rust.mutation.targets`, and
`rust.dependency_boundaries` to match the target repository.

## CI Contract

CI must run the same checks a maintainer can run locally.

For a Go project, the CI job should include:

```sh
go vet ./...
go test ./...
golangci-lint run
go build ./cmd/<binary>
slophammer-go dry .
slophammer-go crap .
slophammer-go mutate . --scan
slophammer-go check .
slophammer-go check . --execute
```

Adapt the build path to the target repo. When working inside this repository's
source tree, `go run ./cmd/slophammer-go ...` is the local development
equivalent of the installed `slophammer-go` command.

If the repo does not implement Slophammer itself, install or run the matching
Slophammer tool, such as `slophammer-go`, instead of assuming
`./cmd/slophammer-go` exists.

Use the rule catalog command when a finding needs context:

```sh
slophammer-go rules --format json
slophammer-ts rules --format json
slophammer-rs rules --format json
```

For a Rust project, the CI job should include:

```sh
cargo check --workspace
cargo fmt --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets
cargo llvm-cov --workspace --fail-under-lines 85
cargo audit
slophammer-rs dry .
slophammer-rs boundaries .
slophammer-rs unsafe .
slophammer-rs check .
```

Use `cargo deny` instead of or in addition to `cargo audit` when the repository
needs license, source, or banned-dependency policy.

## Language Baselines

For TypeScript:

- enable `strict: true`
- reject explicit `any`
- run formatting, linting, type checking, and tests in CI
- validate unknown external input at the boundary before converting it to typed
  domain data

For Python:

- run Ruff
- run mypy
- use type annotations on public functions and meaningful helpers
- avoid `Any` except at narrow dynamic boundaries

For Rust:

- declare `rust-version` or an equivalent pinned Rust toolchain version
- run `cargo check`, `cargo fmt --check`, `cargo clippy`, and `cargo test`
- deny Clippy warnings in CI
- keep unsafe code forbidden unless a narrow allow entry has a reason
- declare dependency audit, mutation, DRY, and dependency-boundary gates
- run tests in CI

For Go:

- run `go test ./...`
- run `go vet ./...`
- run `golangci-lint` v2
- enforce coverage
- enforce CRAP
- run DRY detection on production code
- run mutation checks on risky target files
- keep interfaces near consumers
- keep package APIs small and explicit

## Refactoring Rules

When a new quality gate fails, fix the code. Do not lower the gate.

Use this order:

1. Add missing tests for important behavior.
2. Split high-complexity functions by responsibility.
3. Move IO and framework code away from domain logic.
4. Replace repeated code with a small helper only when the helper has a clear
   name and stable purpose.
5. Remove dead or unused paths.
6. Rerun the failing quality gate.

Do not refactor unrelated areas only because you noticed them.

## Validation

Before finishing, run the relevant local suite.

For this repository's Go implementation, the expected validation set is:

```sh
go test ./...
go vet ./...
golangci-lint run
./scripts/check-go-coverage.sh
go run ./cmd/slophammer-go dry ..
go run ./cmd/slophammer-go crap ..
go run ./cmd/slophammer-go mutate .. --scan
go run ./cmd/slophammer-go check .. --execute
./scripts/check-go-install.sh
node ../scripts/check-conformance.mjs
npx -y @simpledoc/simpledoc check
git diff --check
```

For this repository's Rust implementation, the expected validation set is:

```sh
cargo fmt --check
cargo check --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets
cargo llvm-cov --workspace --fail-under-lines 85
scripts/publish-crate.sh --tag rust/v0.1.1 --dry-run
scripts/test-packaged-crate.sh
scripts/install-packaged-cli.sh
slophammer-rs --version
slophammer-rs dry .. --format json
slophammer-rs boundaries .. --format json
slophammer-rs unsafe .. --format json
slophammer-rs check .. --format json
node ../scripts/check-conformance.mjs
npx -y @simpledoc/simpledoc check
git diff --check
```

If a command cannot run locally, say exactly why and what still needs CI,
staging, or production verification.

## Final Report

When you finish, report:

- files added or changed
- policy values enforced
- checks that passed
- checks that could not run
- any remaining risks

Do not call the repo ready until the changes are committed, pushed, and CI is
green.
