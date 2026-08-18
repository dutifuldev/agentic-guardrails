# Product

Slophammer is a repository quality standard for agent-assisted software
projects.

Each language implementation exposes the same product behavior, even
though each implementation uses its language's normal project structure and
tooling. Installable implementations use short language-specific product
names:

| Implementation | Public command  |
| -------------- | --------------- |
| Go             | `slophammer-go` |
| TypeScript     | `slophammer-ts` |
| Rust           | `slophammer-rs` |
| Python         | `slophammer-py` |

The product name identifies the implementation package. It does not forbid
cross-language checking. An implementation is native-first, and it can also
support other languages when those checks follow the shared Slophammer rules,
config, report, and exit-code contracts.

## Commands

The public command surface is the same for each implementation, using that
implementation's executable name:

```sh
slophammer-go check <path>
slophammer-go check <path> --format json
slophammer-go check <path> --format sarif
slophammer-go check <path> --execute
slophammer-go check <path> --only <rule-id>
slophammer-go agents init [path] [--dry-run] [--force]
slophammer-go agents check [path] [--format text|json|sarif]
slophammer-go explain <rule-id>
slophammer-go rules [--format text|json]
```

During the rename from the early `slophammer ...` command shape,
implementations keep compatibility aliases. Documentation and help text point
users to the language-specific public names.

## Check

`check` scans the target repository, evaluates the default rule set, writes a
report, and exits with a stable exit code.

The default report format is text. JSON and SARIF report formats are selected
with `--format json` and `--format sarif`.

`--execute` is opt-in. It runs configured tool checks and adds tool failures to
the same report model as static findings.

`--only <rule-id>` evaluates a focused rule without requiring every other rule
to pass first. The flag repeats and also accepts comma-separated rule IDs.
Unknown rule IDs are command errors. All implementations support it.

Implementations may add flags when a check needs extra inputs.
`slophammer-go check` accepts `--coverage-profile <file>` to reuse an existing
Go coverage profile during `--execute` runs.

## Agent Instructions

`agents init` scans the target repository and renders a deterministic root
`AGENTS.md` starter from package manifests and repository runner commands. It
writes the file when one does not exist. `--dry-run` prints the proposed file
without writing it. An existing file is never replaced unless `--force` is
present. Forced replacement rejects symbolic links and other nonregular entries,
then replaces a regular file through a temporary file in the same directory.
Package paths in generated
shell commands use POSIX single-quote escaping. Initialization rejects package
paths that contain newlines because they cannot form one safe command line in
the generated Markdown block.

The generated file contains a bounded evidence block between
`<!-- slophammer:agents:start -->` and `<!-- slophammer:agents:end -->`.
Slophammer owns this block and checks it for stale package and command facts.
Maintainers own the prose outside it. Hand-written files do not need an evidence
block.

`agents check` evaluates only the shared `repo.agents-*` rules and uses the
normal report and exit-code contracts. The default `check` command also runs
these rules.

Direct language commands may exist for checks that Slophammer owns natively.
All direct commands preserve the normal finding and exit-code model:

- `slophammer-go dry <path>`, `slophammer-go coverage <path>`,
  `slophammer-go crap <path>`, and `slophammer-go mutate <path> [--scan]` run
  the Go DRY, coverage, CRAP, and mutation checks directly.
- `slophammer-ts dry <path>` and `slophammer-ts boundaries <path>` run the
  TypeScript DRY and dependency boundary rules directly.
- `slophammer-rs dry <path>`, `slophammer-rs boundaries <path>`, and
  `slophammer-rs unsafe <path>` expose Rust DRY, dependency-boundary, and
  unsafe policy checks directly.

## Explain

`explain` prints rule metadata for a known rule ID.

Unknown rule IDs are command errors.

## Rules

`rules` prints the implemented rule catalog for the current implementation.
The catalog includes rule ID, category, severity, implementation status, and
the backing tool when one exists.

The default `rules` format is text. `--format json` writes the same rule
definitions as structured data for agents and automation.

`rules` is an inspection command. It does not scan a repository and it exits
successfully when the catalog is written.

## Release Policy

The Go, TypeScript, and Rust implementations are releasable products.

Go releases use the `go/` submodule tag shape:

```sh
git tag go/v0.1.0
git push origin go/v0.1.0
```

Users install a tagged Go release with:

```sh
go install github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.0
```

The TypeScript checker is released as the `slophammer-ts` npm package. The
TypeScript release workflow validates `typescript/vX.Y.Z` tags, runs the full
TypeScript package gate, runs shared conformance, publishes with npm trusted
publishing, and creates the GitHub Release.

The Go release workflow validates `go/vX.Y.Z` tags, runs the Go release
checks, runs shared conformance, verifies tagged `go install` on tag push, and
creates the GitHub Release.

The Rust checker is released to crates.io as the `slophammer-rs` Cargo package
built from `rust/crates/slophammer-cli`. Users install a release with
`cargo install slophammer-rs --locked`. For local development, install from
the source tree with `cargo install --path rust/crates/slophammer-cli
--locked`.

The production Cargo release target is one user-facing package:
`slophammer-rs`. Internal Rust implementation modules are not published as
separate crates unless there is a deliberate library API to support.

The Rust release workflow validates the release tag, runs the Rust quality gate,
packages `slophammer-rs`, runs package tests from Cargo's verified package
directory, installs the packaged CLI artifact, runs installed CLI smoke tests
and shared conformance, and publishes only `slophammer-rs`. See the
[Rust CLI-only Cargo publish plan](../docs/2026-06-08-rust-cli-only-cargo-publish-plan.md).

## Implementation Boundary

Implementations keep this dependency direction:

```text
CLI
-> app orchestration
-> scanner
-> typed repository snapshot
-> pure rule evaluation
-> reporter
```

Rules do not walk the filesystem, parse command-line arguments, or write
reports.
