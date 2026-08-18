# Rules

This file defines the shared rule contract. Every implementation should use the
same rule IDs, severities, finding paths, messages, and descriptions for shared
rules.

## Shared Repository Rules

| Rule ID                | Severity | Finding path        | Finding message                                                      |
| ---------------------- | -------- | ------------------- | -------------------------------------------------------------------- |
| `repo.readme-required` | `error`  | `README.md`         | `README.md is required`                                              |
| `repo.agents-required` | `error`  | `AGENTS.md`         | `AGENTS.md is required`                                              |
| `repo.agents-empty` | `error` | `AGENTS.md` | `AGENTS.md must contain useful repository instructions` |
| `repo.agents-commands-required` | `error` | `AGENTS.md` | `AGENTS.md must name a verification command supported by the repository` |
| `repo.agents-command-invalid` | `error` | `AGENTS.md` | `AGENTS.md contains generated commands without repository evidence` |
| `repo.agents-scope-required` | `error` | `<package>/AGENTS.md` | `Package instructions must name a verification command for this package` |
| `repo.agents-stale` | `error` | `AGENTS.md` | `The Slophammer AGENTS.md evidence block is stale` |
| `repo.ci-required`     | `error`  | `.github/workflows` | `.github/workflows must contain at least one .yml or .yaml workflow` |
| `repo.slophammer-ci-required` | `error` | `.github/workflows` | `CI must run a Slophammer checker when slophammer.yml is present` |

## Binding Evidence

Command-presence rules accept evidence only from steps that can run and can
fail. Workflow YAML is parsed structurally: steps and jobs with a literal
`continue-on-error: true` or a literal-false `if:` condition contribute
nothing, and a workflow contributes nothing unless its triggers can fire for
integration — `pull_request`, `pull_request_target`, `merge_group`,
`schedule`, or `push` whose branch filter is absent, wildcarded, or names an
integration branch (`main`, `master`, `trunk`, `develop`). A `push` filter
that defines only `tags` or `tags-ignore` is a release trigger, not
integration CI: it never fires for branch pushes, so it contributes
nothing. A `branches-ignore` filter still fires for branch pushes and
stays binding. Surviving steps
contribute their `run` script and their `uses:` action reference.

Scripts, Makefiles, Taskfiles, and justfiles count as evidence only when
binding workflow evidence invokes them (a script by name; runner files
through `make`, `task`, or `just`), following script-to-script references one
level deep. `package.json` scripts count only when invoked by name from
binding evidence, with one level of chained run-script references.

Mutation evidence must be able to fail: list, scan, check, dry-run, and
manifest-only forms (`cargo mutants --list`, `cargo mutants --check`,
`mutate --scan`, `mutate --update-manifest`, `stryker --dryRunOnly`,
`mutmut results`) enumerate, build, or baseline without executing a single
mutant against the tests, so they are not mutation-testing evidence.
Diff-scoped and incremental executing forms count. Tools that execute
mutants but exit zero when they survive are held to the same bar: bare
`mutate4go` and bare `mutmut run` are not gates, so Go credits only the
`slophammer-go mutate` wrapper and Python credits the kill-rate check
(`--min-kill-rate` with a positive floor) or `cr-rate --fail-over` beside
`cosmic-ray exec`. Stryker likewise exits zero without `thresholds.break`,
so `stryker run` counts only beside a positive breaking threshold.

Accepted limitations: expressions are only neutralizing when literal — the
checkers ship no expression evaluator, so a non-literal always-false
condition stays credited. Reusable workflows (`uses:` at the job level) stay
credited without following the reference. Unparseable workflow YAML stays
credited as raw text, so structural filtering can only remove false passes.

## Go Rules

Go rules apply only when the target appears to be a Go project. A repo appears
to be a Go project when it contains Go source, a `go.mod` file, or declared Go
commands.

| Rule ID                             | Severity | Finding path                   | Finding message                                                |
| ----------------------------------- | -------- | ------------------------------ | -------------------------------------------------------------- |
| `go.module-required`                | `error`  | `go.mod`                       | `Go projects must include a go.mod file`                       |
| `go.tests-required`                 | `error`  | `.github/workflows`            | `Go projects must declare go test ./... in CI or scripts`      |
| `go.vet-required`                   | `error`  | `.github/workflows`            | `Go projects must declare go vet ./... in CI or scripts`       |
| `go.lint-required`                  | `error`  | `.golangci.yml`                | `Go projects must configure and declare golangci-lint`         |
| `go.coverage-required`              | `error`  | `scripts/check-go-coverage.sh` | `Go projects must declare a coverage gate`                     |
| `go.complexity-required`            | `error`  | `.golangci.yml`                | `Go projects must enable a complexity linter`                  |
| `go.dry-required`                   | `error`  | `.github/workflows`            | `Go projects must declare a DRY check`                         |
| `go.crap-required`                  | `error`  | `.github/workflows`            | `Go projects must declare crap4go with a threshold`            |
| `go.mutation-required`              | `error`  | `.github/workflows`            | `Go projects must declare mutate4go`                           |
| `go.dependency-boundaries-required` | `error`  | `slophammer.yml`               | `Go projects must respect configured dependency boundaries`    |
| `go.scope-incomplete`               | `error`  | `slophammer.yml`               | `Configured Go scope must cover all production files or exclude them with reasons` |
| `go.suppressions-justified`         | `error`  | `.`                            | `nolint directives in production Go code must carry a reason`  |

## TypeScript Rules

TypeScript rules apply only when the target appears to be a TypeScript project.
A repo appears to be a TypeScript project when it contains `tsconfig.json`,
production `.ts` or `.tsx` source, or `package.json` TypeScript signals such as
the `typescript` package, `@typescript-eslint/*`, `tsc`, or a `typecheck`
script. `@types/*` packages alone do not make a JavaScript package a
TypeScript project.

| Rule ID                               | Severity | Finding path        | Finding message                                                |
| ------------------------------------- | -------- | ------------------- | -------------------------------------------------------------- |
| `ts.package-required`                 | `error`  | `package.json`      | `TypeScript projects must include package.json`                |
| `ts.typecheck-required`               | `error`  | `.github/workflows` | `TypeScript projects must declare a no-emit typecheck in CI or scripts` |
| `ts.strict-required`                  | `error`  | `tsconfig.json`     | `TypeScript projects must enable strict mode`                  |
| `ts.no-explicit-any`                  | `error`  | `eslint.config.mjs` | `TypeScript projects must reject explicit any`                 |
| `ts.no-unsafe-types`                  | `error`  | `eslint.config.mjs` | `TypeScript projects must reject unsafe type operations`       |
| `ts.lint-required`                    | `error`  | `.github/workflows` | `TypeScript projects must declare a linter in CI or scripts`   |
| `ts.format-required`                  | `error`  | `.github/workflows` | `TypeScript projects must declare a formatter check`           |
| `ts.test-required`                    | `error`  | `.github/workflows` | `TypeScript projects must declare tests in CI or scripts`      |
| `ts.coverage-required`                | `error`  | `.github/workflows` | `TypeScript projects must declare a coverage gate`             |
| `ts.complexity-required`              | `error`  | `eslint.config.mjs` | `TypeScript projects must enforce complexity limits`           |
| `ts.dry-required`                     | `error`  | `.github/workflows` | `TypeScript projects must declare a DRY check`                 |
| `ts.mutation-required`                | `error`  | `.github/workflows` | `TypeScript projects must declare mutation testing`            |
| `ts.dependency-boundaries-required`   | `error`  | `slophammer.yml`    | `TypeScript projects must respect configured dependency boundaries` |
| `ts.scope-incomplete`                 | `error`  | `slophammer.yml`    | `Configured TypeScript scope must cover all production files or exclude them with reasons` |
| `ts.suppressions-justified`           | `error`  | `.`                 | `lint and type suppressions in production TypeScript code must carry a description` |

## Python Rules

Python rules apply only when the target appears to be a Python project. A
repo appears to be a Python project when it contains a non-conventional
`pyproject.toml`, a `setup.py`, or production `.py` source. For Python paths,
`migrations/` joins the conventional non-production list.

The strict-typing rule judges tool configuration as evidence: ty severities
in `ty.toml` or `[tool.ty]` plus invocation flags (no stable default-error
rule demoted without a reasoned `python.typecheck.demotions` override,
`error-on-warning` enabled, `missing-type-argument` and the `possibly-*`
correctness rules promoted to error, `respect-type-ignore-comments`
disabled), mypy `strict`/`disallow_untyped_defs` (with the pydantic plugin
when pydantic is a dependency), or pyright strict mode — and in every case
Ruff's ANN selection, so every production signature is annotated.

| Rule ID | Severity | Finding path | Finding message |
| ------- | -------- | ------------ | --------------- |
| `py.project-required` | `error` | `pyproject.toml` | `Python projects must declare a pyproject.toml` |
| `py.typecheck-required` | `error` | `.github/workflows` | `Python projects must run a typechecker (ty, mypy, or pyright) in CI` |
| `py.types-strict-required` | `error` | `pyproject.toml` | `Python typechecking must be strict` |
| `py.lint-required` | `error` | `.github/workflows` | `Python projects must run a linter (ruff check) in CI` |
| `py.format-required` | `error` | `.github/workflows` | `Python projects must verify formatting (ruff format --check or black --check) in CI` |
| `py.test-required` | `error` | `.github/workflows` | `Python projects must run tests (pytest) in CI` |
| `py.coverage-required` | `error` | `.github/workflows` | `Python projects must enforce a coverage gate of at least 85` |
| `py.complexity-required` | `error` | `pyproject.toml` | `Python projects must enforce complexity at most 8 (Ruff C901 or radon)` |
| `py.dry-required` | `error` | `.github/workflows` | `Python projects must declare a DRY check` |
| `py.mutation-required` | `error` | `.github/workflows` | `Python projects must declare mutation testing (mutmut or cosmic-ray)` |
| `py.suppressions-justified` | `error` | `.` | `Python suppressions must carry a reason: bare # noqa, # type: ignore without an error code, or uncommented # ty: ignore` |
| `py.dependency-audit-required` | `error` | `.github/workflows` | `Python projects must audit dependencies (pip-audit or uv audit) in CI` |
| `py.dependency-boundaries-required` | `error` | `.` | `Python imports must respect the configured dependency boundaries` |
| `py.typed-marker-required` | `error` | `.` | `Published Python packages must ship a py.typed marker` |
| `py.absolute-imports-required` | `error` | `.` | `Python imports must be absolute; replace relative imports (ruff check --select TID252 --fix)` |
| `py.scope-incomplete` | `error` | `slophammer.yml` | `Configured Python scope must cover all production files or exclude them with reasons` |

## Rust Rules

Rust rules apply only when the target appears to be a Rust project. A repo
appears to be a Rust project when it contains `Cargo.toml`, production `.rs`
source, or inspectable Cargo commands.

| Rule ID                               | Severity | Finding path        | Finding message                                                |
| ------------------------------------- | -------- | ------------------- | -------------------------------------------------------------- |
| `rust.manifest-required`              | `error`  | `Cargo.toml`        | `Rust projects must include Cargo.toml`                        |
| `rust.msrv-required`                  | `error`  | `Cargo.toml`        | `Rust projects must declare a minimum supported Rust version`  |
| `rust.check-required`                 | `error`  | `.github/workflows` | `Rust projects must declare cargo check in CI or scripts`      |
| `rust.fmt-required`                   | `error`  | `.github/workflows` | `Rust projects must declare cargo fmt --check in CI or scripts` |
| `rust.clippy-required`                | `error`  | `.github/workflows` | `Rust projects must declare cargo clippy in CI or scripts`     |
| `rust.test-required`                  | `error`  | `.github/workflows` | `Rust projects must declare cargo test in CI or scripts`       |
| `rust.coverage-required`              | `error`  | `.github/workflows` | `Rust projects must declare a coverage gate`                   |
| `rust.complexity-required`            | `error`  | `.github/workflows` | `Rust projects must enforce complexity limits`                 |
| `rust.dry-required`                   | `error`  | `.github/workflows` | `Rust projects must declare a DRY check`                       |
| `rust.mutation-required`              | `error`  | `.github/workflows` | `Rust projects must declare mutation testing`                  |
| `rust.unsafe-policy-required`         | `error`  | `slophammer.yml`    | `Rust projects must declare and respect an unsafe-code policy` |
| `rust.dependency-audit-required`      | `error`  | `.github/workflows` | `Rust projects must declare dependency audit checks`           |
| `rust.dependency-boundaries-required` | `error`  | `slophammer.yml`    | `Rust projects must respect configured dependency boundaries`  |
| `rust.scope-incomplete`               | `error`  | `slophammer.yml`    | `Configured Rust scope must cover all production files or exclude them with reasons` |
| `rust.suppressions-justified`         | `error`  | `.`                 | `allow attributes in production Rust code must carry a reason` |

## Rule Descriptions

### `repo.readme-required`

The target repo should have a `README.md`.

The filename comparison is case-insensitive.

### `repo.agents-required`

The target repo should have an `AGENTS.md`.

The filename comparison is case-insensitive.

### `repo.agents-empty`

A root `AGENTS.md` must contain useful text after Markdown headings, comments,
and whitespace are removed, or it must name a supported verification command.
Slophammer does not grade the writing.

### `repo.agents-commands-required`

When Slophammer can derive verification commands from repository evidence, the
governing `AGENTS.md` must name at least one of them. Supported evidence includes
a root `check` target in a Makefile, Taskfile, or justfile; package scripts in
`package.json`; and standard checks backed by Go, Rust, and Python manifests.
The command must appear as inline code or as its own command line in the
Markdown file. For nested JavaScript packages, Slophammer resolves pnpm, Yarn,
Bun, and npm from package-manager metadata and lockfiles in the package or its
ancestor workspace directories.

### `repo.agents-command-invalid`

Commands inside a Slophammer-managed evidence block must still be derivable from
the current repository. Hand-written prose and commands outside the block are
not evaluated by this rule.

### `repo.agents-scope-required`

A package boundary is governed by the nearest ancestor `AGENTS.md`. The
governing file must name either a root umbrella command or a verification
command derived for that package. The finding path is the local `AGENTS.md` that
would be needed when the ancestor does not cover the package.

Package boundaries are directories containing `go.mod`, `package.json`,
`pyproject.toml`, or `Cargo.toml`. Dependency, build, test, script, fixture,
template, and tool cache directories are ignored. Git ignore patterns do not
hide other package manifests from these rules. A maintained package path that
contains a newline produces this finding because Slophammer cannot put its
shell command on one safe Markdown line.

### `repo.agents-stale`

A Slophammer-managed evidence block must exactly match the block that current
repository evidence produces. Re-run `agents init --force` to replace a stale
generated file, then restore any maintainer prose that belongs outside the
managed block.

Hand-written `AGENTS.md` files without the block do not produce this finding.

### `repo.ci-required`

The target repo should have a CI workflow under `.github/workflows`.

Any regular file directly under `.github/workflows` with a `.yml` or `.yaml`
extension satisfies the rule.

### `go.module-required`

Go projects should include a `go.mod` file.

The file may live at the repository root or inside a language implementation
directory such as `go/`.

### `go.tests-required`

Go projects should declare a `go test` command against `./...` in an
inspectable workflow or script. Common test flags before the package pattern are
accepted.

### `go.vet-required`

Go projects should declare `go vet ./...` in an inspectable workflow or script.

### `go.lint-required`

Go projects should configure `golangci-lint` and declare a lint check in CI or
scripts.

The rule accepts `.golangci.yml` or `.golangci.yaml`.

### `go.coverage-required`

Go projects should declare a coverage gate using coverage output from `go test`,
`go tool cover`, and a minimum threshold in a workflow or script that Slophammer
can inspect.

### `go.complexity-required`

Go projects should enable a complexity linter through `golangci-lint`.
Comments, settings, and disabled linter entries do not satisfy this rule.

The accepted linter names are:

- `cyclop`
- `gocognit`
- `gocyclo`

`linters.default: all` is also accepted because it enables the complexity
linters through golangci-lint.

### `go.dry-required`

Go projects should declare `slophammer-go dry` for structural and copied-block
duplicate detection.

Slophammer checks for an inspectable declaration. It does not run the DRY
engine in static mode. Existing `dry4go` and pre-rename `slophammer go dry`
declarations remain accepted as legacy evidence, but new repos should declare
the `slophammer-go` command.

### `go.crap-required`

Go projects should declare `crap4go` with a threshold for complexity and
coverage risk scoring.

Slophammer checks for an inspectable declaration and threshold. It does not run
`crap4go` in static mode.

### `go.mutation-required`

Go projects should declare `mutate4go` in an inspectable workflow or script.

The mutation command may live in a normal CI workflow, nightly workflow, manual
workflow, or script. Slophammer checks for a declaration in static mode.

### `go.dependency-boundaries-required`

Go projects should keep imports inside the dependency boundaries declared in
`slophammer.yml`.

The rule is active when `go.dependency_boundaries` contains at least one
boundary. External imports are ignored. Local imports are checked by resolving
the import path through the nearest `go.mod` module path.

Boundary paths may be written relative to the repository root or relative to the
Go module root. For example, both `go/internal/rules` and `internal/rules` can
describe the same package when the module root is `go/`.

### `ts.package-required`

TypeScript projects should include `package.json`.

The file may live at the repository root or inside a language implementation
directory such as `typescript/`.

### `ts.typecheck-required`

TypeScript projects should declare `tsc --noEmit`, `tsgo --noEmit`, or an
equivalent no-emit typecheck in an inspectable workflow, script, or
`package.json`. A script named `typecheck` does not satisfy this rule unless it
actually runs a TypeScript-compatible checker with `--noEmit`.

### `ts.strict-required`

TypeScript projects should enable `strict` compiler mode in the project
`tsconfig.json`.

### `ts.no-explicit-any`

TypeScript projects should configure ESLint, Oxlint, or an equivalent typed lint
rule to reject explicit `any`.

### `ts.no-unsafe-types`

TypeScript projects should configure ESLint, Oxlint, or an equivalent typed lint
rule to reject unsafe assignments, calls, member access, and returns.

### `ts.lint-required`

TypeScript projects should declare ESLint, Oxlint, Biome, or an equivalent
linter in CI, scripts, or `package.json`.

### `ts.format-required`

TypeScript projects should declare a formatter check through Prettier, Oxfmt,
Biome, Dprint, or an equivalent tool.

### `ts.test-required`

TypeScript projects should declare a real test command, such as Node's built-in
test runner, Vitest, Jest, or an equivalent runner.

### `ts.coverage-required`

TypeScript projects should declare a coverage gate. The recommended minimum is
`85`.

### `ts.complexity-required`

TypeScript projects should enforce complexity limits through a configured
linter. The recommended cyclomatic complexity maximum is `8`.

### `ts.dry-required`

TypeScript projects should declare `slophammer-ts dry` for native
copied-block duplicate detection.

Pre-rename `slophammer typescript dry` declarations remain accepted as legacy
evidence, but new repos should declare the `slophammer-ts` command.

### `rust.manifest-required`

Rust projects should include a `Cargo.toml` manifest.

### `rust.msrv-required`

Rust projects should declare a minimum supported Rust version using
`rust-version` in `Cargo.toml` or an equivalent pinned Rust toolchain version.

### `rust.check-required`

Rust projects should declare `cargo check` in an inspectable workflow or
script.

### `rust.fmt-required`

Rust projects should declare `cargo fmt --check` in an inspectable workflow or
script.

### `rust.clippy-required`

Rust projects should declare `cargo clippy` with warnings denied in an
inspectable workflow or script.

### `rust.test-required`

Rust projects should declare `cargo test` in an inspectable workflow or script.

### `rust.coverage-required`

Rust projects should declare an enforceable coverage gate. The preferred tool
is `cargo llvm-cov` with a visible threshold of at least `85`.

### `rust.complexity-required`

Rust projects should enforce complexity limits through Clippy configuration or
Slophammer-owned Rust complexity policy. The recommended cognitive complexity
maximum is `8`.

### `rust.dry-required`

Rust projects should declare `slophammer-rs dry` for native copied-block
duplicate detection.

### `rust.mutation-required`

Rust projects should declare mutation testing, normally through `cargo-mutants`.
The mutation command may live in normal CI, nightly CI, manual CI, or scripts.

### `rust.unsafe-policy-required`

Rust projects should declare an unsafe-code policy in `slophammer.yml`.
`policy: forbid` fails unsafe blocks, unsafe functions, unsafe traits, unsafe
impls, and unsafe extern blocks unless covered by a specific allow entry with a
reason.

### `rust.dependency-audit-required`

Rust projects should declare dependency audit checks through `cargo audit`,
`cargo deny`, or an equivalent inspectable command.

### `rust.dependency-boundaries-required`

Rust projects should declare dependency boundaries in `slophammer.yml`.
Slophammer checks local path dependencies in `Cargo.toml`; external crates are
ignored by this rule.

### `ts.mutation-required`

TypeScript projects should declare TypeScript mutation testing, normally through
StrykerJS. Go mutation commands do not satisfy this rule.

### `ts.dependency-boundaries-required`

TypeScript projects should declare dependency boundaries in `slophammer.yml`
and keep imports inside those boundaries.

If no TypeScript boundaries are declared, the rule reports a finding. External
package imports are ignored. Relative imports are resolved against the importing
file.

## Finding Order

Reports sort findings by `rule_id`, then by `path`.

## Machine-Readable Registry

The machine-readable rule registry lives in [`rules.json`](rules.json).
