# Config

Slophammer loads optional project config from the first matching file:

- `slophammer.yml`
- `slophammer.yaml`

Config is parsed after repository scanning and before rule evaluation. Invalid
config fails the check with exit code `2`.

Config keys are strict. Unknown root, rule, language, DRY, copied-block, or
dependency-boundary keys fail instead of being ignored.

Every language section uses the same nested key shape; see the
[Config Dialect Decision](../docs/2026-06-10-config-dialect-decision.md).

## Scope Completeness

Thresholds are validated, and scope is too: findings must not be hideable by
retargeting `paths`, `targets`, or `exclude` instead of weakening thresholds.

The conventional non-production list classifies files that scope may exclude
freely: test files (`*_test.go`, `*.test.ts`, `*.spec.ts`, `*_test.rs`,
`tests/` directories), and paths under `fixtures/`, `templates/`,
`testdata/`, `dist/`, `build/`, `coverage/`, `target/`, `node_modules/`,
`vendor/`, `scripts/`, or containing a `generated` segment. Everything else
is production.

`exclude` entries are plain strings when they match the conventional list.
An exclude that matches production files must use the object form with a
reason, following the `rust.unsafe.allow` precedent:

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

After expansion, every production file must be either in configured scope or
covered by a conventional or reasoned exclude. Uncovered production files
produce a `<lang>.scope-incomplete` finding naming the uncovered directories,
and scoped checks report `scope` coverage counts per
[Report Format](REPORT_FORMAT.md).

## Shape

```yaml
rules:
  go.crap-required:
    severity: warn
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
    exclude:
      - "go/internal/rules/generated/**"
  dependency_boundaries:
    - from: go/internal/rules
      allow:
        - go/internal/config
        - go/internal/gotools
        - go/internal/repo
    - from: go/internal/repo
      allow: []
typescript:
  coverage:
    threshold: 85
  complexity:
    max: 8
  dry:
    max_findings: 0
    paths:
      - typescript/src
    exclude:
      - "**/*.test.ts"
      - "**/*.spec.ts"
      - "typescript/fixtures/**"
      - "typescript/dist/**"
      - "typescript/coverage/**"
    copied_blocks:
      enabled: true
      min_tokens: 100
  mutation:
    targets:
      - typescript/src/rules/rules.ts
  dependency_boundaries:
    - from: typescript/src/app
      allow:
        - typescript/src/config
        - typescript/src/dry
        - typescript/src/repo
        - typescript/src/report
        - typescript/src/rules
        - typescript/src/scan
        - typescript/src/toolchecks
python:
  coverage:
    threshold: 85
    paths:
      - python/src
    exclude: []
  complexity:
    max: 8
  dry:
    max_findings: 0
    paths:
      - python/src
    exclude: []
    copied_blocks:
      enabled: true
      min_tokens: 100
  mutation:
    targets:
      - python/src/slophammer/rules
    exclude: []
  dependency_boundaries:
    - from: python/src/slophammer/rules
      allow:
        - python/src/slophammer/config
  typecheck:
    demotions:
      - rule: deprecated
        reason: upstream false positive on sqlalchemy decorators
rust:
  coverage:
    threshold: 85
    paths:
      - rust/crates
    exclude:
      - "target/**"
      - "fixtures/**"
  complexity:
    cognitive_max: 8
  targets:
    - rust/crates
  exclude:
    - "target/**"
    - "fixtures/**"
  dry:
    max_findings: 0
    paths:
      - rust/crates
    exclude:
      - "**/*_test.rs"
      - "fixtures/**"
      - "target/**"
    copied_blocks:
      enabled: true
      min_tokens: 100
  unsafe:
    policy: forbid
  mutation:
    targets:
      - rust/crates/slophammer-cli/src/rust_rules
  dependency_boundaries:
    - from: rust/crates/slophammer-cli
      allow: []
```

## Rule Config

Rule config supports severity overrides and reasoned disabling:

```yaml
rules:
  repo.readme-required:
    severity: warn
  go.mutation-required:
    disabled: true
    reason: Mutation testing runs in a separate required release gate.
```

Valid severities are:

- `error`
- `warn`

`disabled` defaults to `false`. A rule with `disabled: true` must have a
non-empty `reason` after trimming whitespace. Invalid rule config fails before
any rule or tool runs.

For `check` and `check --execute`, a rule runs only when the checker implements
it, `--only` selects it or has no restriction, and it is not disabled. A known
disabled rule selected by `--only` runs no rule or rule-specific tool and
produces a clean result when no other error exists. A severity override has no
effect when the same rule is disabled.

Each checker applies rule config in this order:

1. Validate config and `--only` rule IDs.
2. Select enabled static rules and executed tool checks before they run.
3. Apply severity overrides to enabled findings.
4. Remove any disabled finding at the shared finding boundary as a safeguard.
5. Apply the baseline and render the report.

Disabling one rule does not stop work that another enabled rule needs. For
example, disabling the test rule does not stop an enabled coverage command from
running tests to measure coverage.

Rule disabling does not block a direct command such as `dry`, `coverage`,
`crap`, `mutate`, `boundaries`, or `unsafe`. Calling a direct command is an
explicit request to run that command.

## Go Config

`go.coverage`, `go.targets`, `go.exclude`, `go.dry`, `go.crap`, `go.mutation`,
and `go.dependency_boundaries` are parsed as typed policy fields.
`go.coverage` holds `threshold` and an optional `profile` path for a
pre-generated coverage artifact. `go.crap` holds `max_score`.

The Go policy values have hard recommended bounds. Slophammer rejects config
that weakens them:

- `go.coverage.threshold` must be at least `85`.
- `go.crap.max_score` must be at most `8`.

Projects may choose stricter values, such as higher coverage or lower CRAP
limits, but they cannot configure weaker values through `slophammer.yml`.

`go.targets` and `go.exclude` define the default production Go files in scope
for Go checks. Targets may be `.go` files or directories. Slophammer expands
directory targets recursively, excludes test files and fixture directories by
default, applies configured excludes, sorts the result, and fails commands that
resolve to zero files.

`go.dry` configures production-only DRY enforcement:

- `go.dry.max_findings` sets the finding budget.
- `go.dry.paths` selects paths to scan.
- `go.dry.exclude` excludes tests, fixtures, templates, or generated code.
- `go.dry.structural` configures function and method similarity.
- `go.dry.copied_blocks` configures CPD-style copied-block detection.

Slophammer expands the configured paths to Go source files before running its
native DRY engine.

The direct Go commands use these values as defaults:

- `slophammer-go dry` uses `go.dry.max_findings` unless
  `--max-candidates` is passed.
- `slophammer-go crap` uses `go.crap.max_score` unless `--max-score` is
  passed.
- `slophammer-go mutate` uses `go.mutation.targets` when present, otherwise
  `go.targets`, unless `--target` is passed.

`slophammer-go check --execute` runs configured Go tool checks and adds failures
to the normal report. Go tool execution runs from discovered Go module roots,
so a repo-level config can drive a nested module such as `go/`. Embedded
`fixtures/`, `templates/`, and `vendor/` modules are not execution targets.

The configured DRY budget is zero for production code. Tests are reviewed
selectively, fixtures are excluded, and templates are checked as independent
reference projects.

`go.dependency_boundaries` is active now. Each boundary declares:

| Field   | Meaning                                                |
| ------- | ------------------------------------------------------ |
| `from`  | Repository-root or module-root-relative package path.  |
| `allow` | Local package paths that code under `from` may import. |

External imports are ignored. Local imports are resolved through the nearest
`go.mod` module path.

## TypeScript Config

`typescript.coverage`, `typescript.complexity`, `typescript.dry`,
`typescript.mutation`, and `typescript.dependency_boundaries` are parsed as
typed policy fields. `typescript.complexity` holds `max`, and
`typescript.mutation` holds `targets`.

The TypeScript policy values have hard recommended bounds. Slophammer rejects
config that weakens them:

- `typescript.coverage.threshold` must be at least `85`.
- `typescript.complexity.max` must be at most `8`.

Projects may choose stricter values, such as higher coverage or lower
complexity limits, but they cannot configure weaker values through
`slophammer.yml`.

`typescript.coverage` keeps the threshold and scope together:

```yaml
typescript:
  coverage:
    threshold: 85
    paths:
      - src/runtime
      - src/flows
    exclude:
      - "**/*.test.ts"
      - "dist/**"
```

Omit `paths` and `exclude` for a whole-project coverage threshold.

The nested `typescript.dry` shape configures native copied-block duplicate
detection:

- `typescript.dry.max_findings` sets the finding budget.
- `typescript.dry.paths` selects paths to scan.
- `typescript.dry.exclude` excludes tests, fixtures, build output, or generated
  code.
- `typescript.dry.copied_blocks` configures CPD-style copied-block detection.

The configured DRY budget is zero for production code.

`typescript.dependency_boundaries` declares import boundaries:

| Field   | Meaning                                               |
| ------- | ----------------------------------------------------- |
| `from`  | Repository-root-relative source path.                 |
| `allow` | Local source paths that code under `from` may import. |

External package imports are ignored. Relative imports are resolved against the
importing source file.

## Python Config

`python.coverage`, `python.complexity`, `python.dry`, `python.mutation`,
`python.dependency_boundaries`, and `python.typecheck` are validated by every
checker; the Python checker enforces them. The shared bounds apply:

- `python.coverage.threshold` must be at least `85`.
- `python.complexity.max` must be at most `8`.
- `python.dry.max_findings` must be `0`.

`python.typecheck.demotions` lists reasoned ty rule demotions: each entry
names the demoted rule and the reason. Demoting a stable default-error ty
rule anywhere else (`ty.toml`, `[tool.ty]`, or invocation flags) is a
`py.types-strict-required` finding. For Python paths, `migrations/` joins
the conventional non-production list.

## Rust Config

`rust.coverage`, `rust.complexity`, `rust.targets`, `rust.exclude`,
`rust.dry`, `rust.unsafe`, `rust.mutation`, and `rust.dependency_boundaries`
are parsed as typed policy fields by `slophammer-rs`.

The Rust policy values have hard recommended bounds. Slophammer rejects config
that weakens them:

- `rust.coverage.threshold` must be at least `85`.
- `rust.complexity.cognitive_max` must be at most `8`.
- `rust.dry.max_findings` must be `0` for production code.

Projects may choose stricter values, such as higher coverage or lower
complexity limits, but they cannot configure weaker values through
`slophammer.yml`.

`rust.targets` and `rust.exclude` define default Rust production source scope.
Targets may be `.rs` files or directories. Slophammer excludes fixture,
template, target, and node_modules paths by default, applies configured
excludes, sorts the result, and uses that set for source-backed Rust rules.

The nested `rust.dry` shape configures native copied-block duplicate detection:

- `rust.dry.max_findings` sets the finding budget.
- `rust.dry.paths` selects paths to scan.
- `rust.dry.exclude` excludes tests, fixtures, build output, or generated code.
- `rust.dry.copied_blocks` configures copied-block detection.

The configured DRY budget is zero for production code.

`rust.unsafe` declares unsafe-code policy:

| Field    | Meaning                                                        |
| -------- | -------------------------------------------------------------- |
| `policy` | `forbid` or `allow_documented`.                                |
| `allow`  | Optional path and reason entries for narrow unsafe boundaries. |

Under `policy: forbid`, unsafe blocks, unsafe functions, unsafe traits, unsafe
impls, and unsafe extern blocks are findings unless covered by a specific allow
entry with a non-empty reason.

`rust.mutation.targets` declares mutation-testing scope. Static checks accept
`cargo mutants` in normal CI, nightly CI, manual CI, or scripts.

`rust.dependency_boundaries` declares local Cargo path dependency boundaries:

| Field   | Meaning                                               |
| ------- | ----------------------------------------------------- |
| `from`  | Repository-root-relative Cargo package path.          |
| `allow` | Local Cargo package paths that `from` may depend on.  |

External crates are ignored by the boundary rule.
