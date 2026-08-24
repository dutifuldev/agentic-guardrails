# Report Format

Slophammer currently supports text, JSON, and SARIF reports.

The JSON report is the shared compatibility contract for fixtures and future
language implementations.

## JSON Shape

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

## Fields

`ok` is `true` when there are no findings and `false` when at least one finding
exists.

`findings` is always present. Clean reports use an empty array.

Each finding contains:

| Field       | Meaning                                                        |
| ----------- | -------------------------------------------------------------- |
| `rule_id`   | Stable public rule identifier.                                 |
| `severity`  | Rule severity. Current shared value: `error`.                  |
| `path`      | Repository-relative path for the finding.                      |
| `message`   | Human-readable finding summary.                                |
| `baselined` | Optional. `true` when `check --baseline` matched the finding against the checked-in baseline. Absent otherwise. |

Findings are sorted by `rule_id`, then by `path`.

## Disabled Rules

A rule disabled in `slophammer.yml` produces no finding. It is absent from text,
JSON, and SARIF output and does not affect `ok` or the process exit code. Its
required reason stays in project config; it is not copied into the report.

Disablement happens before baseline matching. A disabled rule is not a
baselined finding or a SARIF suppression. The report schema does not change.
Enabled findings keep the same rule IDs, severity handling, sorting, and format
mapping.

## Scope

When configured scope restricts a native check (DRY paths, coverage paths, or
targets), the report carries an additive top-level `scope` block:

```json
"scope": { "scanned": 42, "production_files": 45 }
```

`production_files` counts the ecosystem's source files after the conventional
non-production list defined in [Config](CONFIG.md); `scanned` counts the files
the configured scope actually covers. The text renderer prints the same
numbers. Reports without configured scope omit the block, and consumers must
treat it as optional.

When `check --baseline` is used, `ok` reflects only non-baselined findings,
and the text renderer always prints the baselined debt count. See
[Baseline](BASELINE.md).

## SARIF

SARIF output is selected with:

```sh
slophammer-go check <path> --format sarif
```

SARIF uses version `2.1.0`. Each Slophammer finding becomes one SARIF result
with:

- `ruleId` from `finding.rule_id`
- `level` mapped from severity: `error` to `error`, `warn` to `warning`
- `message.text` from `finding.message`
- `locations[0].physicalLocation.artifactLocation.uri` from `finding.path`

SARIF is an output adapter over the shared findings model. It is not a second
rule model.
