---
title: Enforcement and Adoption Plan
author: Onur Solmaz <onur.solmaz@huggingface.co>
date: 2026-06-11
status: active
---

# Enforcement and Adoption Plan

Slophammer's goal is quality enforced by machinery, not by hoping someone
reads the diff. Testing the released checkers against real-world repositories
and crafted counterexamples shows where the current rules fall short of that
goal. This plan turns those findings into ordered, verifiable tasks.

Two findings motivate the top of the list. First, the command-presence rules
credit declarations that cannot execute: a workflow with a never-matching
branch filter, `if: false` on the job, and `continue-on-error: true` on every
step still passes `check` with exit 0. Second, config scope can quietly carve
real code out of checking: pointing `dry.paths` at an innocuous directory
hides genuine duplication, and nothing in the report says the production tree
went unscanned. Both holes matter because the adversary in Slophammer's
threat model is an agent optimizing to make the check pass.

The plan has two phases. Phase 1 is one implementation pass covering every
enforcement, adoption, and distribution improvement. Phase 2 — the Python
checker and the multi-language dispatcher — comes later.

## Phase 1: Enforcement, Adoption, and Distribution

1. [x] Credit only CI evidence that can run and can fail.

   Every `*-required` command rule accepts any command-file text containing
   the right command. Evidence collection lives in
   `go/internal/rules/go_command_files.go` (`commandFiles`, fed by
   `snapshot.WorkflowFiles()` plus Makefile/Taskfile/justfile and `scripts/`
   trees), `typescript/src/repo/repo.ts` (`commandFiles`, consumed by
   `commandSegmentsWithPackageExpansion` in `typescript/src/rules/rules.ts`),
   and the evidence helpers in
   `rust/crates/slophammer-cli/src/rust_rules/mod.rs`. None of them parse
   workflow structure; they strip comments and split command segments.

   Change: parse workflow YAML before extracting command segments, and drop
   neutralized evidence:

   - steps with a literal `continue-on-error: true`, and all steps of jobs
     with a literal job-level `continue-on-error: true`
   - jobs with a literal-false `if:` (`false`, `${{ false }}`)
   - workflows whose triggers cannot fire for integration. Version 1 keeps a
     workflow when `on` includes `pull_request`, `merge_group`, `schedule`,
     or a `push` with no `branches` filter or a filter containing a wildcard
     or a plausible integration branch (`main`, `master`, `trunk`,
     `develop`). Everything else is non-binding.

   Scripts and Makefiles are credited only when reachable: a script,
   Makefile target, Taskfile task, or justfile recipe counts as evidence
   only if a binding workflow step invokes it (directly, through `make`,
   `task`, or `just`, or through a package script that a binding step runs).
   This closes the standing hole where commands written in a file that
   nothing executes satisfy the rules. Reachability is one level deep:
   scripts invoking other scripts are followed once, not transitively.

   Accepted limitations, documented in `specs/RULES.md`:

   - Expressions are only neutralizing when literal. A conditional
     `continue-on-error: ${{ matrix.experimental }}` or a non-constant
     always-false `if:` keeps the step credited; the checkers stay static
     and ship no expression evaluator.
   - Reusable workflows (`uses:` at job level) stay credited; following
     `uses:` references is out of scope.
   - Unparseable workflow YAML stays credited as raw text (today's
     behavior), so this change can only remove false passes, not add false
     failures on exotic-but-working setups.

   Implementation per checker: a shared "binding evidence" filter applied
   where workflow files enter the command-file set, plus the reachability
   pass over non-workflow command files — new
   `go/internal/rules/go_workflow_binding.go`, a workflow filter in
   `typescript/src/repo/repo.ts` (the `yaml` dependency is already present),
   and a module beside `rust_rules/mod.rs` (`yaml_serde` is already a
   dependency).

   Fixtures: `go-neutralized-ci`, `typescript-neutralized-ci`,
   `rust-neutralized-ci` (neutralized workflows must fail like their
   `missing-*` counterparts) and one `*-unreachable-script` fixture per
   ecosystem (commands present in a script no workflow invokes must fail).
   Register all in `scripts/check-conformance.mjs` and add
   per-neutralization-mechanism unit tests in each implementation.

   Done when: both counterexamples (neutralized workflow, uninvoked script)
   produce findings in every implementation, the fixtures lock the behavior
   into conformance, and the clean fixtures still pass unchanged.

2. [x] Add a rule that the checker itself must run in CI.

   A repository can carry `slophammer.yml` and `AGENTS.md` while no CI step
   ever executes a Slophammer checker; the config is decoration and nothing
   reports it. Surveyed adopters include exactly this state.

   New shared rule `repo.slophammer-ci-required`, severity `error`,
   implemented by all checkers. It fires only when `slophammer.yml` or
   `slophammer.yaml` is present (config-gated, like dependency boundaries).
   It passes when binding command evidence (task 1 semantics) invokes any
   released checker. Accepted invocation shapes, mirroring the regex style
   of `hasTypeScriptDryCommand` in `typescript/src/rules/rules.ts`:

   - `slophammer-go|slophammer-ts|slophammer-rs|slophammer-py ... check`
   - `go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@<v>`
   - `npx|pnpm dlx|npm exec slophammer-ts[@<v>]`
   - package-script expansion, so `npm run <script>` counts when the script
     invokes the checker

   Spec and fixture changes:

   - `specs/rules.json` and `specs/RULES.md` gain the rule.
   - Every existing `*-clean` fixture that carries a `slophammer.yml` must
     gain a checker invocation in its workflow, or the rule makes it fail.
     This is an intentional cutover: config without enforcement becomes a
     finding. Fixture expected files regenerate accordingly.
   - New fixtures: `unenforced-config` (config present, CI present, no
     checker invocation — fails with exactly this rule) and the updated
     clean fixtures as the passing case.

   Done when: `unenforced-config` fails with only
   `repo.slophammer-ci-required` across all implementations and conformance
   passes with the regenerated expected reports.

3. [x] Enforce scope completeness so carve-outs cannot hide code.

   Config can restrict `paths`, `targets`, and `exclude` without limit, so
   findings can be hidden by retargeting scope instead of weakening
   thresholds — thresholds are validated, scope is not. Making the gap
   merely visible is not enough; an unread report does not stop anything.

   Design: scoped checks (native DRY, coverage, CRAP, mutation,
   boundaries-bearing targets) must account for every production file. A
   production file is one left after the conventional non-production list
   (test globs such as `**/*_test.go` and `**/*.test.ts`, and `fixtures/`,
   `templates/`, `testdata/`, `dist/`, `build/`, `coverage/`, `target/`,
   `node_modules/`, `vendor/`, `generated` segments). Three parts:

   - Justified production excludes. An `exclude` entry stays a plain string
     when it matches the conventional list. Any exclude that matches
     production files must use the object form, following the
     `rust.unsafe.allow` precedent:

     ```yaml
     dry:
       paths:
         - src
       exclude:
         - "**/*.test.ts"
         - pattern: "src/vendored_parser/**"
           reason: vendored upstream code, synced verbatim
     ```

     Strict validation rejects a production-matching string exclude with
     `<section>.exclude requires a reason for production paths`.

   - Scope-completeness finding. After expansion, every production file
     must be either in scope or covered by a reasoned or conventional
     exclude. Uncovered production files produce a finding
     (`<lang>.scope-incomplete`, severity `error`) naming the uncovered
     directories, so narrowing `paths` to an innocuous corner fails the
     check instead of passing with a quiet report. This closes the
     demonstrated carve-out fully — exit code, not visibility.

   - Scope coverage in the report. Scoped checks also gain an additive
     report block, `"scope": { "scanned": 42, "production_files": 45 }`,
     and the text renderer prints `DRY scanned 42 of 45 production files`.
     `specs/REPORT_FORMAT.md` documents the field; expected fixture reports
     regenerate.

   The classification list is shared, spelled out in `specs/CONFIG.md`, and
   identical in all three validators (`go/internal/config/config.go`,
   `typescript/src/config/config.ts`,
   `rust/crates/slophammer-cli/src/config.rs`). `paths` pointing at a
   nonexistent directory already fails loudly and keeps doing so.

   Fixtures: a `*-carved-scope` fixture per ecosystem (real duplication in
   `src/`, scope pointed elsewhere, no reasoned exclude — must fail with
   `scope-incomplete`) and a passing variant where the same carve-out
   carries a reason.

   Done when: the carve-out counterexample exits 1 with the new finding in
   every implementation, the reasoned variant passes, and conformance covers
   the object-form exclude, the new rule, and the `scope` report field.

4. [x] Add a baseline ratchet mode for existing repositories.

   Thresholds are absolute and weakening is forbidden, so a repository that
   is not already clean has no incremental path in: surveyed non-adopted
   repositories fail with anywhere from one to over seventy findings, and
   the only options today are "fix everything first" or "do not adopt".
   Baselines are how mypy, ESLint bulk suppressions, and similar gates
   solved the same problem.

   Contract, to be specified in a new `specs/BASELINE.md`:

   - The baseline lives in a checked-in `slophammer-baseline.json`:

     ```json
     {
       "version": 1,
       "findings": [
         { "rule_id": "go.coverage-required", "path": ".github/workflows" }
       ]
     }
     ```

     Matching is on `rule_id` plus `path` (not `message`, so message
     rewording across checker versions does not invalidate baselines).
     Duplicate keys collapse.

   - `check --baseline` reads the file and exits `0` when current findings
     are a subset of the baseline, `1` when any non-baselined finding
     exists. Baselined findings still appear in the report, marked
     `"baselined": true`, and the text renderer always prints the debt:
     `12 findings baselined; 0 new`. Debt is never silent.
   - Stale entries (baselined findings that no longer occur) are an error
     (exit `2`, `baseline contains resolved findings; rewrite it`), which is
     what makes the ratchet shrink-only.
   - `check --baseline-write` writes the file from current findings, refuses
     to write a superset of an existing baseline (exit `2`), and prints the
     full added/removed diff when it writes.
   - The flag is explicit; the file is never auto-read. An agent must not be
     able to silence findings by committing a baseline that the standard
     `check` invocation then honors invisibly.
   - Composition: `--baseline` combines with `--only` and `--execute`
     (matching applies after filtering). SARIF output maps baselined
     findings to SARIF `suppressions`, which code scanning already
     understands.

   Accepted limitation, stated in `specs/BASELINE.md`: deleting the
   baseline file and writing a fresh one can launder new findings into
   "initial" debt. A static checker cannot distinguish a first adoption
   from a rewrite, so the spec requires the mitigation instead: baseline
   files must be review-protected (CODEOWNERS or equivalent), and
   `--baseline-write` output is designed to make a laundering diff obvious.

   Implementation: baseline read/write and matching live beside report
   assembly in each `app` layer (`go/internal/app/app.go`,
   `typescript/src/app/app.ts`, `rust/crates/slophammer-cli/src/app.rs`);
   the finding model gains the optional `baselined` field. Fixtures:
   `adoption-baseline` (failing repo plus baseline file → exit 0), a
   regression case (one finding not in baseline → exit 1), and a stale case
   (baseline lists a resolved finding → exit 2), registered for all
   implementations.

   Done when: the three fixture cases hold in every implementation and the
   format is specified well enough that a baseline written by one checker is
   readable by the others.

5. [x] Add suppression-discipline rules.

   Generated code accumulates lint and type suppressions, and nothing
   counts them: an agent silencing a linter with `eslint-disable`,
   `nolint`, or equivalent passes every current rule. Slophammer already
   holds its own Go code to a justified-`nolint` standard through lint
   config; this makes the same discipline a product rule.

   New rules, one per ecosystem: `go.suppressions-justified`,
   `ts.suppressions-justified`, `rust.suppressions-justified` (and
   `py.suppressions-justified` when Phase 2 lands). A suppression directive
   in production scope must carry a justification:

   - Go: `//nolint:<linter>` requires a trailing `// reason` comment
     (golangci's `nolintlint` convention).
   - TypeScript: `eslint-disable`/`eslint-disable-next-line`,
     `@ts-ignore`/`@ts-expect-error`, and `biome-ignore`/`oxlint-disable`
     require a description on the directive line (the formats all support
     one).
   - Rust: `#[allow(...)]` in production scope requires either an adjacent
     `// reason` comment or use of `#[expect(...)]` with a `reason`
     attribute.

   Bare suppressions are findings naming the file and line. Test scope is
   exempt by the shared non-production classification from task 3.
   Detection is line-based and deterministic — no semantic analysis.

   Deliberately deferred, recorded here so the omission is a decision:
   swallowed-error detection (empty catch blocks, discarded error values),
   dead-export detection, and assertion-free test detection all need real
   language analysis and earn their own plan if the suppression rules prove
   the category.

   Fixtures: `*-bare-suppression` per ecosystem (fails) and justified
   variants inside the clean fixtures (pass). Specs `rules.json`/`RULES.md`
   gain the three rules.

   Done when: a bare suppression in production code fails in every
   implementation and a justified one passes.

6. [x] Ship the distribution surface: GitHub Action and pre-commit hook.

   Today the only adoption path is a human pasting a prompt block; the
   checkers reach CI only when someone hand-writes the workflow steps.

   - Composite GitHub Action at the repo root (`action.yml`), usable as
     `uses: osolmaz/slophammer@<tag>` with inputs `checker`
     (`go|ts|rs`), `version` (exact, required — the action refuses
     `latest`), and `args` (default `check .`). It installs the pinned
     checker and runs it; SARIF upload stays the consumer's step. The
     action is validated in this repo's CI by running it against the clean
     fixtures.
   - `.pre-commit-hooks.yaml` exposing `slophammer-go-check`,
     `slophammer-ts-check`, and `slophammer-rs-check` hooks for pre-commit
     users, each invoking an installed checker.
   - README and `docs/AGENT_ENTRYPOINT.md` document both, and the
     Agent Entrypoint instructs agents to prefer the action over
     hand-written install steps.

   Done when: a consumer workflow of three lines runs a pinned checker via
   the action, this repo's CI exercises the action, and the pre-commit
   hooks run against a fixture in CI.

7. [x] Document version pinning as the supported consumption pattern.

   Slophammer makes breaking releases deliberately and ships no
   compatibility shims, but neither the README nor the Agent Entrypoint says
   how to consume it safely; CI integrations that install `@latest` absorb
   breaking changes mid-pipeline, and surveyed integrations do exactly that.

   - Add a short "Pinning" note to the README install section and a matching
     instruction in `docs/AGENT_ENTRYPOINT.md`: pin an exact version in CI
     (`go run .../slophammer-go@v0.2.0`, `npx slophammer-ts@0.2.0`,
     `cargo install slophammer-rs --version 0.2.0 --locked`), ideally behind
     one env var so upgrades are single-line, and upgrade deliberately —
     strict config validation fails loudly across breaking releases by
     design. The task 6 action enforces this by requiring an exact version.
   - Update the copy-paste agent block to tell agents to pin the version
     they verified against.
   - State the outer boundary explicitly in the README: Slophammer cannot
     defend against being removed from a repository. Branch protection and
     required status checks are the layer that makes the gate mandatory;
     the docs should say so instead of implying the checker is
     self-protecting.

   Done when: both documents show pinned commands for all released
   checkers, the paste block mentions pinning, and the README names the
   branch-protection boundary.

## Follow-up: Mutation Reality

10. [x] Make mutation testing real in every ecosystem.

   Three of the four implementations never execute a mutant: Go CI runs
   `mutate --scan` (site counting, with a threshold warning nothing acts
   on), TypeScript runs `stryker --dryRunOnly` (config validation), and
   Rust runs `cargo mutants --list`. Only Python executes mutants, and it
   exits 0 regardless of survivors. The rules permit this because
   `*.mutation-required` accepts any declared invocation — the same
   cannot-fail evidence the binding-CI work eliminated elsewhere.

   Change, applying the binding-evidence principle to mutation:

   - Rule matchers in all four implementations stop crediting forms that
     cannot fail on a surviving mutant: `--list`, `--scan`,
     `--dryRunOnly`, and equivalents are rejected; only executing
     invocations count. Diff-scoped and incremental forms
     (`cargo mutants --in-diff`, Stryker `--incremental`) count, because
     they can fail.
   - Own gates run real mutants per PR, fast form per tool: full mutmut
     for Python behind a kill-rate floor, `cargo mutants --in-diff` for
     Rust, a full Stryker run with a `thresholds.break` floor for
     TypeScript (the whole run takes about five minutes, so incremental
     caching is unnecessary), and the executing `mutate` mode for Go,
     which is natively incremental: committed in-source manifests record
     per-function tested states, only changed functions re-mutate, and
     any survivor fails the gate.
   - Full-sweep mutation runs are `workflow_dispatch` workflows per
     implementation, dispatched on demand — correctly non-binding under
     the shared trigger semantics, since a dispatched diagnostic is not a
     gate; it may run red until the baselined debt is triaged. Release
     workflows gate on changes instead: Rust mutates the diff since the
     previous release tag, Go re-tests functions changed since their
     manifest stamps, and Python and TypeScript run their full gates,
     which already pass their floors.
   - Initial floors record today's honest state and only rise: Python at
     74 (74.8% measured), TypeScript break at 44 (45.4% measured), Rust
     and Go gating new code at zero survivors while existing debt (184
     missed Rust mutants, the stamped Go baseline) is triaged over time.
     A configurable `mutation.minimum_kill_rate` ratchet remains a
     follow-up.

   Done when: every implementation's CI executes mutants on PRs and fails
   on the tool's native gate, the matchers reject cannot-fail forms with
   fixtures locking the behavior, and each release workflow runs a full
   sweep.

## Phase 2: Cover More Languages (Later)

8. [x] Ship `slophammer-py`.

   Python is the largest agent-coding ecosystem and the one planned
   implementation that does not exist; polyglot repositories with Python
   components have no gate for them today.

   Scope, mirroring the TypeScript checker's shape:

   - New `python/` implementation directory (src layout, `pyproject.toml`,
     same internal boundaries: CLI → app → scan → config → rules → report).
     The existing `templates/python` stays a template; the checker is new
     code held to Slophammer's own gates (coverage ≥ 85, complexity ≤ 8,
     DRY 0, mutation declared). Runtime decisions, fixed here: Python
     3.12 minimum, uv as the dev and build tool with a committed frozen
     lockfile, and PyYAML as the single runtime dependency (safe_load
     only), matching the one-YAML-library rule the other implementations
     follow.
   - Full report-contract parity, explicitly including the baseline
     ratchet: `--baseline` and `--baseline-write` with identical
     subset/superset/stale semantics and exit codes, since conformance
     runs the three baseline cases against every implementation.
   - The checker's own typechecker is `ty` (Astral), configured to the
     same contract it enforces (below). ty has no annotation-coverage rule
     (verified against ty source at 0.0.49): it checks the annotations you
     wrote but never requires you to write them. That gap closes with
     Ruff's ANN rule selection riding on the already-required Ruff gate
     instead of a second typechecker: every production function signature
     fully annotated — parameters (ANN001-ANN003) and returns
     (ANN201-ANN206), private helpers included — and no `Any` in
     signatures (ANN401). Locals are never required (inference is checked
     against the annotated signature), `self`/`cls` are exempt (Ruff
     deprecated ANN101/ANN102), tests are exempt via per-file-ignores that
     count as conventional, and genuinely dynamic boundaries escape with
     `# noqa: ANN401 -- reason`, which `py.suppressions-justified` keeps
     honest. Signatures always, locals never, tests exempt, escapes
     reasoned.
   - The ty strictness contract `py.types-strict-required` enforces, in
     full: no stable default-error rule demoted below error (in `ty.toml`,
     `[tool.ty]`, or `--warn`/`--ignore` invocation flags) except through a
     reasoned severity override in `slophammer.yml`;
     `error-on-warning = true` (or the flag) so warn-tier rules block;
     these ignore-default rules promoted to error: `missing-type-argument`,
     `possibly-missing-attribute`, `possibly-unresolved-reference`,
     `possibly-missing-import`; and
     `respect-type-ignore-comments = false`, so blanket `# type: ignore`
     comments silence nothing and the only working suppression is the
     rule-coded `# ty: ignore[rule]` form. Preview rules are never
     required, and unknown rule names are tolerated in both directions so
     checker upgrades do not break gates. The default-severity table ships as
     generated spec data (`specs/ty-rules.json`: rule, default level,
     stability), extracted from ty's source by a script; refreshing it is
     part of bumping the supported ty version.
   - Tool configuration is evidence, not just CI commands: the checker
     parses `ty.toml`/`[tool.ty]`, mypy config, and pyright config, plus
     severity flags on the CI invocation, because that is where quiet
     weakening happens. Evidence matchers are uv-native (`uv run`, `uvx`,
     `uv sync --frozen`/`--locked`), and a frozen lockfile counts as the
     toolchain pinning evidence for pre-1.0 tools like ty.
   - Rule set `py.*` at TypeScript parity: `py.project-required`
     (`pyproject.toml`), `py.typecheck-required` (ty, mypy, or pyright —
     ty recognized from day one, including its `# ty: ignore[rule]`
     suppression form), `py.types-strict-required` (the configuration that
     makes annotations mandatory: ty rule severities, mypy
     `disallow_untyped_defs`/`strict`, or pyright strict; strictness
     carve-outs on production modules need reasons, mirroring scope
     excludes; when pydantic is a dependency and mypy is the checker, the
     pydantic mypy plugin is required so strict mode is not lying at the
     boundary), `py.lint-required` and `py.format-required` (Ruff check and
     format, tool-agnostic where reasonable), `py.test-required` (pytest
     and friends), `py.coverage-required` (`fail_under` or
     `--cov-fail-under` ≥ 85), `py.complexity-required` (Ruff `C901` ≤ 8 or
     radon), `py.dry-required` plus a native copied-block port,
     `py.mutation-required` (mutmut or cosmic-ray declared),
     `py.suppressions-justified` (`# noqa`, `# type: ignore`, and
     `# ty: ignore` need reasons; bare `# type: ignore` without an error
     code is itself a finding), `py.dependency-audit-required` (pip-audit
     or uv audit), `py.dependency-boundaries-required` (import parsing
     against the shared boundary config), and `py.typed-marker-required`
     (a project that builds a published package must ship the `py.typed`
     marker, or its checked types degrade to `Any` for every consumer;
     does not apply to applications). Deliberately out of scope, recorded
     so they are not relitigated: mandatory pydantic strict mode
     (boundary coercion is often correct; a semantic-phase candidate) and
     runtime annotation verification via typeguard/beartype in tests
     (real value, but it mandates a dependency; the Python template
     demonstrates it instead).
   - Config: a `python:` section in the shared nested shape; validators in
     the other three checkers learn its allowed keys, exactly as they
     cross-validate each other today.
   - The conventional non-production list gains `migrations/` for Python
     (Alembic and Django migrations are as standard a carve-out as
     `testdata/` is for Go); other production-matching excludes keep
     needing reasons.
   - Conformance: `python-clean`, per-rule `python-missing-*` fixtures, a
     `python-bad-dependency` fixture, plus weakened-typechecking negatives
     (`python-demoted-rule`: a default-error ty rule set to ignore;
     `python-soft-warnings`: no `error-on-warning`), all registered in
     `scripts/check-conformance.mjs`, which invokes the checker
     deterministically via `uv run --directory python slophammer-py`
     against the frozen lockfile; specs `rules.json`/`RULES.md` grow the
     `py.*` rows.
   - Release: PyPI package `slophammer-py` via trusted publishing, a
     `python-release.yml` workflow cloned from the TypeScript one (tag
     shape decided then, avoiding collision with `templates/python`
     history), GitHub release created by the workflow.

   Gets its own dated implementation plan when scheduled; this entry fixes
   scope and contract.

   Done when: `slophammer-py check` passes shared conformance with a fixture
   set at parity with the other ecosystems and the checker passes its own
   gates in CI.

9. [ ] Build the `slophammer` umbrella dispatcher.

   Polyglot repositories need every per-language checker installed and wired
   by hand; in practice one language gets gated and the rest go unchecked.
   The reserved `slophammer` name (npm, and on PyPI as a reserved
   distribution) becomes a thin dispatcher. Naming decision, fixed here:
   the Python checker's distribution is `slophammer-py` but its import
   package is `slophammer`, so the dispatcher must stay command-only — its
   command is `slophammer`, and it never claims the Python import
   namespace. The first bare PyPI release is a `0.0.0` placeholder to reserve
   the package name; the full dispatcher can replace that implementation in a
   later release:

   - Detection by ecosystem markers, reusing the spec's project-detection
     semantics: `go.mod` → `slophammer-go`, TypeScript signals →
     `slophammer-ts`, `Cargo.toml` → `slophammer-rs`, `pyproject.toml` →
     `slophammer-py`.
   - For each detected ecosystem, resolve the checker on `PATH` and run
     `check <path> --format json`, forwarding `--execute`, `--only`,
     `--baseline`, and `--format`. The dispatcher never installs checkers
     itself.
   - A detected ecosystem with no resolvable checker is itself a finding
     (`repo.checker-missing`, severity error), not a silent skip — the
     dispatcher must be unable to under-report.
   - Reports merge into one finding list (rule IDs are already namespaced);
     exit code is the maximum of the children's. SARIF output emits one run
     per tool, which is the format's native shape for multi-tool results.
   - Repo-level rules (`repo.*`) run once, not once per child; the
     dispatcher dedups identical findings from multiple checkers.

   Done when: one command checks a mixed Go/TypeScript/Rust/Python fixture
   end to end, a missing-checker case produces the new finding, and
   `specs/PRODUCT.md` documents the dispatcher as the default entry point
   for polyglot repositories.

## Out of Scope

- Evaluating workflow expressions. Non-literal always-false conditions
  remain credited; the checkers stay static and ship no expression engine.
- Verifying actual CI run history through the GitHub API. Binding-evidence
  analysis (task 1) is the static approximation.
- Transitive script reachability beyond one level (task 1 follows one hop).
- Semantic slop analysis: swallowed errors, dead exports, assertion-free
  tests (deferred from task 5 pending its results).
- Auto-installing checkers from the dispatcher.
- Trend dashboards or history beyond the shrink-only baseline.

## Sequencing Notes

- Phase 1 is one implementation pass. Within it, task 1 lands first and
  task 2 builds on its binding-evidence test; tasks 2, 3, and 5 all
  regenerate expected fixture reports, so batch their conformance updates;
  task 6 should land after 1–3 so the action ships with binding semantics;
  tasks 4 and 7 are independent.
- Phase 2 follows Phase 1. Task 9 gains most of its value from task 8 and
  follows it; task 8 gets its own dated implementation plan when scheduled.
