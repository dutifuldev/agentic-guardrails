---
title: Reasoned Rule Disabling Implementation Plan
author: Onur Solmaz <2453968+osolmaz@users.noreply.github.com>
date: 2026-08-24
status: active
---

# Reasoned Rule Disabling Implementation Plan

## Goal

Make the existing `rules.<rule-id>.disabled` field work in every Slophammer
checker. A reasoned disabled rule must not run, start a rule-specific tool,
produce a finding, enter baseline matching, or appear in a report.

This is a hard cutover in the existing config shape. The work does not add a
new config version, alias, compatibility reader, fallback, or feature flag.

## Shared Contract

A rule is active only when all three conditions are true:

1. The checker implements the rule.
2. `--only` selects the rule, or `--only` has no restriction.
3. The rule does not have `disabled: true` in config.

An unrestricted selection and a restricted selection with no active rules are
different states. If `--only` selects only known disabled rules, Slophammer runs
no rule or rule-specific tool and exits `0` when no independent error exists.

A disabled rule must have a non-empty `reason` after trimming whitespace.
Invalid config exits `2` before static evaluation or tool execution. Missing
`disabled` and `disabled: false` keep current behavior.

Rule processing follows this order:

1. Validate config and `--only` rule IDs.
2. Build one immutable rule policy.
3. Admit static evaluators before they run.
4. Admit `check --execute` tasks before a runner starts.
5. Apply severity overrides to enabled findings.
6. Reject disabled findings at the common finding boundary as a safeguard.
7. Apply the baseline.
8. Sort and render text, JSON, or SARIF output.

Disabling one rule does not stop work required by another enabled rule. For
example, an enabled coverage rule can run tests even when the separate test rule
is disabled. Explicit direct commands such as `dry`, `coverage`, `crap`,
`mutate`, `boundaries`, and `unsafe` still run when a user calls them.

## Central Rule Policy

Each Go, TypeScript, Python, and Rust implementation gets one small native
policy object. It owns three decisions:

- whether a rule ID is active;
- the effective severity of an enabled finding;
- whether a produced finding can continue to baseline and report processing.

The policy is built once from validated config and the explicit `--only`
selection state. Rule implementations do not add their own disable checks.
Static runners and executed-tool schedulers use the same policy object.

The final finding check is defensive. It does not replace the checks before
static work and tool launch.

## TypeScript Changes

Add the policy under `typescript/src/rules/` and build it in the app after
config and `--only` validation.

Update the ordered static rule runner to admit each definition before calling
its evaluator. Replace the separate severity helper with the policy decision.

Update `typescript/src/toolchecks/toolchecks.ts` so policy admission happens
before package-script discovery and grouping. An empty active selection must
not restore all checks. An aggregate script must not run a disabled
rule-specific command. Work that an enabled rule needs remains allowed and any
failure stays attributed to that enabled rule.

Use recording or fail-if-called runners to prove that disabled format, lint,
typecheck, test, coverage, complexity, DRY, and mutation checks do not start.
Include a mutation script case so Stryker cannot run through an aggregate
script when the mutation rule is disabled.

## Python Changes

Add the policy under `python/src/slophammer/` and use it in the app, static rule
runner, and executable checks.

Filter gate descriptors before coverage and test de-duplication, command
construction, or runner calls. Apply the same policy to the internal executed
DRY path. Python mutation and audit checks remain declaration-only, but their
static rules can be disabled.

Use a recording runner to prove that disabled coverage and typecheck gates make
no call, an all-disabled execute request makes no call, enabled gates still run,
and an enabled coverage command can still run tests. Invalid or blank reasons
must fail before any runner call.

## Go Changes

Add the policy under `go/internal/rules/` and use it from the static runner and
`go/internal/app/`.

Put the policy in the existing Go tool environment. Replace the current
`ruleSelected` checks for DRY, coverage, CRAP, and mutation with policy
admission before target resolution, option construction, or runner invocation.
Pass tool findings through policy for severity and defensive admission. Remove
the superseded config-only severity path.

Use the injected runner to prove that reasoned disabled mutation with configured
targets never calls mutate4go. Add the same no-call coverage for metrics and
DRY, plus mixed enabled and disabled checks.

## Rust Changes

Add the policy under `rust/crates/slophammer-cli/src/` and register the module in
the crate.

Make the static rule runner admit definitions through policy. Filter
`ExecutableCheck` descriptors before workspace iteration can invoke the runner,
including the `cargo mutants` descriptor. Replace the post-run config severity
pass with policy finding admission before report creation and baseline handling.

Use a recording runner to prove that configured disabled mutation never starts
`cargo mutants`. Cover another executed check, mixed policy, multiple
workspaces, an all-disabled selection, and severity on enabled tool findings.

## Findings, Reports, and Baselines

All four apps apply defensive finding admission before report construction and
baseline handling. Disabled rules are absent from text, JSON, and SARIF. The
report schema, stable rule IDs, sorting, severity mapping, and exit-code meaning
do not change for enabled rules.

Baselines remain a separate finding-level adoption tool. A disabled rule needs
no baseline entry. If a project already has an entry for a finding that becomes
disabled, the entry is stale and `check --baseline` keeps exit code `2` until a
reviewed `check --baseline-write` shrinks the file.

## Shared Fixtures and Tests

Add one shared repository fixture that lacks a README but disables
`repo.readme-required` with a reason. Every checker must return a clean report.
Add one shared invalid fixture with `disabled: true` and no reason. Every
checker must return exit code `2`.

Add native disabled-mutation fixtures where needed. Extend
`scripts/check-conformance.mjs` to cover the shared fixture, invalid config,
disabled-only `--only`, and clean text, JSON, and SARIF output.

Native tests must cover:

- missing, false, and reasoned true values;
- missing, empty, and whitespace-only reasons;
- unrestricted and restricted selection;
- a restricted selection with zero active rules;
- static evaluator no-call behavior;
- executed-tool no-call behavior;
- mixed enabled and disabled work;
- severity overrides on enabled and disabled rules;
- baseline matching and stale-entry migration;
- deterministic text, JSON, and SARIF output;
- unchanged enabled-rule exit codes and ordering.

Run focused language checks while implementing. Finish with `make conformance`
and `make check`. Run `pi-reviewer --base main` after the latest branch commit is
pushed. Fix P0 and P1 findings until none remain, then require green pull-request
CI before merge.

## Release 0.5.0

This change activates a public config field that the current specification says
is reserved. Prepare a coordinated pre-1.0 minor release, version `0.5.0`.

Update and lock matching metadata for:

- `typescript/package.json` and `typescript/package-lock.json`;
- `python/pyproject.toml` and `python/uv.lock`;
- the Rust workspace metadata and `rust/Cargo.lock`.

Go has no package version field; its release version comes from the module tag.
All release metadata and release checks must agree on `0.5.0` before merge.

After the pull request is reviewed, green, and merged into `main`, create the
four release tags from the merged main commit:

- `go/v0.5.0`;
- `typescript/v0.5.0`;
- `python/v0.5.0`;
- `rust/v0.5.0`.

Use only the existing tag-triggered release workflows and their configured
trusted publishing. Do not create release infrastructure or move credentials.
Wait for every release workflow and verify:

- the four GitHub Releases exist and point to the merged commit;
- `slophammer-ts@0.5.0` is available from npm;
- `slophammer-py==0.5.0` is available from PyPI;
- `slophammer-rs 0.5.0` is available from crates.io;
- the Go tag supports the documented install path.

Do not retry by deleting, rewriting, or republishing an immutable version. Stop
and report the exact failed workflow or missing artifact if publication cannot
be verified.

## Scope Boundary

This implementation and release run owns only the Slophammer repository and its
existing package release targets. It does not edit consumer repositories,
deploy services, change repository policy, create credentials, or add release
infrastructure.

Harbor-HF adoption is a separate authorized follow-up. It starts only after
`slophammer-py==0.5.0` is verified on PyPI.

## Completion

The work is complete when the documented contract matches all four checkers,
focused and shared tests pass, pi-reviewer has no P0 or P1 findings, pull-request
CI is green, the pull request is merged, and all four `0.5.0` release artifacts
are live and verified.
