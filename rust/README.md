# Slophammer Rust

Rust implementation of the Slophammer repository quality checker. The
user-facing product name is `slophammer-rs`.

## Commands

```sh
cargo run -p slophammer-rs -- check ..
cargo run -p slophammer-rs -- check .. --format json
cargo run -p slophammer-rs -- check .. --format sarif
cargo run -p slophammer-rs -- check .. --execute
cargo run -p slophammer-rs -- check .. --only rust.unsafe-policy-required
cargo run -p slophammer-rs -- agents init .. --dry-run
cargo run -p slophammer-rs -- agents check ..
cargo run -p slophammer-rs -- rules --format json
cargo run -p slophammer-rs -- dry ..
cargo run -p slophammer-rs -- boundaries ..
cargo run -p slophammer-rs -- unsafe ..
```

Install the public package:

```sh
cargo install slophammer-rs --locked
slophammer-rs agents init . --dry-run
slophammer-rs check .
```

Install the local package during source-tree development:

```sh
cargo install --path crates/slophammer-cli --locked
slophammer-rs check .
```

## Quality Gate

Run before finishing Rust changes:

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
```

`cargo audit` and `cargo mutants` are declared in CI. They require installing
`cargo-audit` and `cargo-mutants` locally.

## Crates.io Status

`slophammer-rs` is published to crates.io. Install the released checker with:

```sh
cargo install slophammer-rs --locked
```

For local Rust development, install from this source tree:

```sh
cargo install --path crates/slophammer-cli --locked
```

## Publish Prerequisites

The production publish target is a single user-facing Cargo package:
`slophammer-rs`.

The Rust implementation is packaged as one publishable CLI crate. Internal
scanner, config, rule, report, and execution modules live inside the CLI package
instead of separate crates.io packages.

Before a crates.io release, verify that the CLI package can be packaged without
unpublished internal dependencies:

```sh
cargo package -p slophammer-rs --locked
```

The stable public contract should be the CLI behavior, config format, report
formats, exit codes, and rule IDs. Keep scanner, config, rule, report, and
execution internals private unless a deliberate Rust library API is added later.

See
[Rust CLI-only Cargo publish plan](../docs/2026-06-08-rust-cli-only-cargo-publish-plan.md).

## Release Workflow

The crates.io release path is implemented by
`.github/workflows/rust-release.yml`. It runs for `rust/v*` tag pushes and can
also be started manually with `workflow_dispatch`.

Before a release:

1. Create a crates.io API token with publish access.
2. Store it as the repository or `crates-io` environment secret named
   `CARGO_REGISTRY_TOKEN`.
3. Make sure the release commit is on `origin/main`.
4. Tag the commit with the Rust workspace version, for example `rust/v0.1.1`.

The workflow:

1. validates the `rust/vX.Y.Z` tag against the `slophammer-rs` package version,
2. verifies the tagged commit is on `origin/main`,
3. runs the Rust quality gate,
4. runs `cargo package -p slophammer-rs --locked`,
5. runs `cargo test` from Cargo's verified package directory,
6. installs the packaged CLI artifact,
7. runs installed CLI smoke checks and shared conformance,
8. publishes only `slophammer-rs`,
9. creates or updates the GitHub Release for the Rust release tag.

Publishing to crates.io is permanent. The publish helper skips
`slophammer-rs@<version>` if that version is already visible on crates.io.
