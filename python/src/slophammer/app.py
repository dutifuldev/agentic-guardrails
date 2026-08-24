"""Check and dry entry points: scan, config, rules, baseline, output."""

from __future__ import annotations

import json
from dataclasses import dataclass

from slophammer.baseline import BaselineError, apply_baseline_check, debt_line, write_baseline
from slophammer.config import ConfigError, load_config
from slophammer.core import Report, new_report
from slophammer.dry import dry_findings, max_findings
from slophammer.report import write_json, write_sarif, write_text
from slophammer.rule_policy import RulePolicy
from slophammer.rules import run_rules
from slophammer.scan import scan_repo
from slophammer.toolchecks import (
    ExecutionError,
    Runner,
    execute_python_checks,
    subprocess_runner,
)


@dataclass(frozen=True)
class CommandResult:
    code: int
    stdout: str = ""
    stderr: str = ""


def check(
    root: str,
    output_format: str = "text",
    only_rule_ids: list[str] | None = None,
    baseline: str = "off",
    execute: bool = False,
    runner: Runner = subprocess_runner,
) -> CommandResult:
    try:
        snapshot = scan_repo(root)
        config = load_config(snapshot)
        policy = RulePolicy.for_check(config, only_rule_ids)
        report = run_rules(snapshot, config, policy)
        findings = list(report.findings)
        if execute:
            findings.extend(execute_python_checks(snapshot, config, policy, runner))
        report = new_report(policy.admit_all(findings), scope=report.scope)
        return finish_check(snapshot.root, report, output_format, baseline)
    except (ConfigError, BaselineError, ExecutionError, OSError) as error:
        return CommandResult(code=2, stderr=f"check failed: {error}\n")


def finish_check(root: str, report: Report, output_format: str, baseline: str) -> CommandResult:
    if baseline == "write":
        summary = write_baseline(root, report)
        report = apply_baseline_check(root, report)
        return CommandResult(code=0 if report.ok else 1, stdout=summary)
    if baseline == "check":
        report = apply_baseline_check(root, report)
    body = format_report(report, output_format)
    if baseline == "check" and output_format == "text":
        body += debt_line(report)
    return CommandResult(code=0 if report.ok else 1, stdout=body)


def format_report(report: Report, output_format: str) -> str:
    if output_format == "json":
        return write_json(report)
    if output_format == "sarif":
        return write_sarif(report)
    return write_text(report)


def dry(root: str, output_format: str = "text") -> CommandResult:
    try:
        snapshot = scan_repo(root)
        config = load_config(snapshot)
    except (ConfigError, OSError) as error:
        return CommandResult(code=2, stderr=f"dry failed: {error}\n")
    findings = dry_findings(snapshot, config)
    allowed = max_findings(config)
    code = 0 if len(findings) <= allowed else 1
    if output_format == "json":
        body = (
            json.dumps(
                {
                    "ok": code == 0,
                    "findings": [finding.json_value() for finding in findings],
                },
                indent=2,
            )
            + "\n"
        )
    else:
        body = f"DRY candidates: {len(findings)}; maximum: {allowed}\n"
    return CommandResult(code=code, stdout=body)
