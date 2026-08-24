# Baseline

Slophammer thresholds are absolute and config cannot weaken them, so a
repository that is not already clean has no incremental path to adoption.
The baseline is that path: it grandfathers existing findings while keeping
every threshold non-negotiable for new findings, and it can only shrink.

## File

The baseline lives in a checked-in `slophammer-baseline.json` at the
repository root:

```json
{
  "version": 1,
  "findings": [
    { "rule_id": "go.coverage-required", "path": ".github/workflows" }
  ]
}
```

`version` must be `1`. Each entry carries `rule_id` and `path`. Matching is
on the pair; `message` is deliberately excluded so message rewording across
checker versions does not invalidate baselines. Duplicate pairs collapse to
one entry. Unknown keys fail strict validation, like `slophammer.yml`.

## Commands

`check --baseline` reads `slophammer-baseline.json` after config has removed
disabled rules and after `--only` filtering:

- exit `0` when every current finding matches a baseline entry
- exit `1` when any finding does not match the baseline
- exit `2` when the baseline file is missing, invalid, or stale

Baselined findings stay in the report, marked `"baselined": true`. The text
renderer always prints the debt — `12 findings baselined; 0 new` — so the
debt is never silent. SARIF output maps baselined findings to SARIF
`suppressions`, which code scanning understands natively.

A stale baseline — an entry whose finding no longer occurs — is an error
(`baseline contains resolved findings; rewrite it`, exit `2`). This is what
makes the ratchet shrink-only: fixing a finding forces the baseline to
shrink, and the entry cannot quietly return.

A disabled rule needs no baseline entry. When a project disables a rule that
already has an entry, that entry becomes stale because the finding no longer
exists. The project must review and shrink the baseline with
`check --baseline-write`; Slophammer does not silently ignore the old entry.

`check --baseline-write` writes the file from current findings. It refuses
to write a superset of an existing baseline (exit `2`) and prints the full
added and removed entries when it writes, so a reviewer sees exactly what
debt is being recorded.

The baseline is only honored behind the explicit `--baseline` flag. The
plain `check` invocation never reads the file, so a committed baseline
cannot silently mute findings for callers that did not opt in.

## Limits

Deleting the baseline file and writing a fresh one can launder new findings
into "initial" debt; a static checker cannot distinguish a first adoption
from a rewrite. The mitigation is procedural and required: protect the
baseline file with review (CODEOWNERS or equivalent), and rely on the
write-time diff output to make a laundering rewrite obvious.
