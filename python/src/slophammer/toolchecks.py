"""Executable checks: run the real gates and report failures as findings.

``check --execute`` runs each gate in the target root and converts non-zero
exits into findings under the gate's rule id, mirroring the TypeScript
implementation's tool checks. Commands run through ``uv run`` when the
target is uv-managed and directly otherwise; a tool that cannot be launched
is an infrastructure error, not a finding.
"""

from __future__ import annotations

import subprocess
from collections.abc import Callable
from dataclasses import dataclass

from slophammer.config import Config
from slophammer.core import Finding
from slophammer.dry import dry_findings, max_findings
from slophammer.repo import Snapshot, has_file
from slophammer.rule_policy import RulePolicy
from slophammer.rules import coverage_threshold, evidence, python_project_present
from slophammer.rules.definitions import (
    PY_COVERAGE,
    PY_DRY,
    PY_FORMAT,
    PY_LINT,
    PY_TEST,
    PY_TYPECHECK,
    definition,
)
from slophammer.rules.toolconfig import project_dirs, typechecker_in_use

Runner = Callable[[str, list[str]], "CommandOutput"]


class ExecutionError(Exception):
    """A gate tool that cannot be launched is an infrastructure failure."""


@dataclass(frozen=True)
class CommandOutput:
    code: int
    output: str = ""
    missing: bool = False


def subprocess_runner(root: str, command: list[str]) -> CommandOutput:
    try:
        completed = subprocess.run(
            command,
            cwd=root,
            capture_output=True,
            text=True,
            check=False,
            timeout=1800,
        )
    except FileNotFoundError:
        return CommandOutput(code=127, missing=True)
    except subprocess.TimeoutExpired:
        return CommandOutput(code=124, output="timed out")
    return CommandOutput(code=completed.returncode, output=first_output(completed))


def first_output(completed: subprocess.CompletedProcess[str]) -> str:
    for stream in (completed.stderr, completed.stdout):
        for line in stream.splitlines():
            if line.strip():
                return line.strip()
    return ""


def execute_python_checks(
    snapshot: Snapshot,
    config: Config,
    policy: RulePolicy,
    runner: Runner = subprocess_runner,
) -> list[Finding]:
    if not python_project_present(snapshot):
        return []
    working_directory = execution_directory(snapshot)
    findings: list[Finding] = []
    for rule_id, label, commands in selected_gate_commands(snapshot, config, policy):
        for command in commands:
            full_command = run_prefix(snapshot) + command
            result = runner(working_directory, full_command)
            if result.missing:
                raise ExecutionError(f"cannot run {' '.join(full_command)}: command not found")
            if result.code != 0:
                findings.append(executed_finding(rule_id, label, result))
                break
    if policy.active(PY_DRY):
        findings.extend(executed_dry_findings(snapshot, config))
    return findings


# The coverage command runs the test suite, so a full run skips the bare
# test gate; a --only selection that names just the test rule still runs it.
def selected_gate_commands(
    snapshot: Snapshot, config: Config, policy: RulePolicy
) -> list[tuple[str, str, list[list[str]]]]:
    commands = [
        (rule_id, label, command)
        for rule_id, label, command in gate_commands(snapshot, config)
        if policy.active(rule_id)
    ]
    rule_ids = {rule_id for rule_id, _, _ in commands}
    if PY_COVERAGE in rule_ids:
        commands = [entry for entry in commands if entry[0] != PY_TEST]
    return commands


# Gates run in the project directory, where pyproject.toml scopes the tools,
# rather than at the scan root: a monorepo's templates and fixtures are not
# this project's gate inputs.
def execution_directory(snapshot: Snapshot) -> str:
    directories = project_dirs(snapshot)
    if not directories or directories[0] == "":
        return snapshot.root
    return f"{snapshot.root}/{directories[0]}"


# Each executed command follows the tool the static evidence credited, so
# execute mode validates the gates the repo actually declares. Mutation and
# audit gates stay declaration-only, matching the other implementations.
def gate_commands(snapshot: Snapshot, config: Config) -> list[tuple[str, str, list[list[str]]]]:
    commands = [
        (PY_FORMAT, "formatter check failed", [format_command(snapshot)]),
        (PY_LINT, "lint failed", [lint_command(snapshot)]),
        (PY_TEST, "tests failed", [test_command(snapshot)]),
        (PY_COVERAGE, "coverage gate failed", coverage_commands(snapshot, config)),
    ]
    typecheck = typecheck_command(snapshot)
    if typecheck is not None:
        commands.insert(2, (PY_TYPECHECK, "typecheck failed", [typecheck]))
    return commands


# Coverage executes the form the static evidence credited: coverage.py
# repos run collect-then-report, pytest-cov repos run the single command.
def coverage_commands(snapshot: Snapshot, config: Config) -> list[list[str]]:
    threshold = coverage_threshold(config)
    if evidence_matches(snapshot, evidence.tool_pattern("coverage") + r" report\b"):
        return [
            ["coverage", "run", "-m", "pytest"],
            ["coverage", "report", f"--fail-under={threshold}"],
        ]
    return [["pytest", f"--cov-fail-under={threshold}", "--cov", "."]]


def format_command(snapshot: Snapshot) -> list[str]:
    if evidence_matches(snapshot, evidence.tool_pattern("black")):
        return ["black", "--check", "."]
    return ["ruff", "format", "--check", "."]


def lint_command(snapshot: Snapshot) -> list[str]:
    for tool in ("flake8", "pylint"):
        if evidence_matches(snapshot, evidence.tool_pattern(tool)):
            return [tool, "."]
    return ["ruff", "check", "."]


def test_command(snapshot: Snapshot) -> list[str]:
    if evidence_matches(snapshot, evidence.tool_pattern("tox")):
        return ["tox"]
    if evidence_matches(snapshot, r"python3? -m unittest\b"):
        return ["python", "-m", "unittest", "discover"]
    return ["pytest"]


def evidence_matches(snapshot: Snapshot, pattern: str) -> bool:
    return evidence.any_segment(snapshot, pattern)


def typecheck_command(snapshot: Snapshot) -> list[str] | None:
    checker = typechecker_in_use(snapshot)
    if checker == "ty":
        return ["ty", "check"]
    if checker == "mypy":
        return ["mypy", "."]
    if checker == "pyright":
        return ["pyright"]
    return None


def run_prefix(snapshot: Snapshot) -> list[str]:
    directories = project_dirs(snapshot)
    project = directories[0] if directories else ""
    lock = f"{project}/uv.lock" if project else "uv.lock"
    return ["uv", "run", "--no-sync"] if has_file(snapshot, lock) else []


def executed_finding(rule_id: str, label: str, result: CommandOutput) -> Finding:
    template = definition(rule_id)
    detail = f": {result.output}" if result.output else ""
    return Finding(
        rule_id=rule_id,
        severity=template.severity,
        path=template.path,
        message=f"{label} (exit {result.code}){detail}",
    )


def executed_dry_findings(snapshot: Snapshot, config: Config) -> list[Finding]:
    found = dry_findings(snapshot, config)
    if len(found) <= max_findings(config):
        return []
    template = definition(PY_DRY)
    return [
        Finding(
            rule_id=PY_DRY,
            severity=template.severity,
            path=template.path,
            message=f"DRY candidates: {len(found)}; maximum: {max_findings(config)}",
        )
    ]
