"""Check and dry entry points: scan, config, rules, baseline, output."""

from __future__ import annotations

import json
import os
import tempfile
from dataclasses import dataclass, replace
from pathlib import Path

from slophammer import agents
from slophammer.baseline import BaselineError, apply_baseline_check, debt_line, write_baseline
from slophammer.config import ConfigError, load_config
from slophammer.core import Report, new_report
from slophammer.dry import dry_findings, max_findings
from slophammer.report import write_json, write_sarif, write_text
from slophammer.rules import rule_severity, run_rules
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


def agents_check(root: str, output_format: str = "text") -> CommandResult:
    return check(root, output_format=output_format, only_rule_ids=list(agents.AGENT_RULE_IDS))


def agents_init(root: str, dry_run: bool = False, force: bool = False) -> CommandResult:
    try:
        snapshot = scan_repo(root)
        if agents.has_unsafe_package_path(snapshot):
            raise OSError("package paths containing newlines are not supported")
        if agents.has_reserved_marker_package_path(snapshot):
            raise OSError("package paths containing Slophammer evidence markers are not supported")
        content = agents.render_agents(snapshot)
        if dry_run:
            return CommandResult(code=0, stdout=content)
        existing = agents.root_agents_file(snapshot)
        target = existing.path if existing is not None else "AGENTS.md"
        write_agents_file(Path(snapshot.root) / target, content, force)
    except FileExistsError:
        return CommandResult(
            code=2,
            stderr="agents init failed: AGENTS.md already exists; pass --force to replace it\n",
        )
    except OSError as error:
        return CommandResult(code=2, stderr=f"agents init failed: {error}\n")
    return CommandResult(code=0, stdout="created AGENTS.md\n")


def write_agents_file(target: Path, content: str, force: bool) -> None:
    if force:
        replace_agents_file(target, content)
        return
    with target.open("x", encoding="utf-8") as file:
        file.write(content)


def replace_agents_file(target: Path, content: str) -> None:
    if target.is_symlink():
        raise OSError("AGENTS.md is a symlink; refusing forced replacement")
    if target.exists() and not target.is_file():
        raise OSError("AGENTS.md is not a regular file; refusing forced replacement")
    descriptor, temporary_name = tempfile.mkstemp(prefix=".slophammer-agents-", dir=target.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as file:
            file.write(content)
        temporary.chmod(0o644)
        os.replace(temporary, target)
    finally:
        temporary.unlink(missing_ok=True)


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
        report = run_rules(snapshot, config, only_rule_ids)
        if execute:
            executed = [
                replace(finding, severity=rule_severity(config, finding.rule_id, finding.severity))
                for finding in execute_python_checks(snapshot, config, runner, only_rule_ids)
            ]
            report = new_report([*report.findings, *executed], scope=report.scope)
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
