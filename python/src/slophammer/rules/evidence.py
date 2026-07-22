"""Command evidence matchers over binding CI evidence.

Matchers are uv-native: tools count when invoked bare, via ``uv run``,
``uvx``, ``uv run --directory <dir>``, or ``python -m``. Segments split on
newlines, ``&&``, and ``;``; segments containing ``||`` are discredited the
same way the other implementations discredit fallback chains.
"""

from __future__ import annotations

import re

from slophammer.repo import Snapshot, command_files

CHECK_INVOCATION_WINDOW = 160
CHECKER_BINARIES = (
    "slophammer",
    "slophammer-go",
    "slophammer-ts",
    "slophammer-rs",
    "slophammer-py",
)


def command_text(snapshot: Snapshot) -> str:
    return "\n".join(file.content for file in command_files(snapshot))


def command_segments(content: str) -> list[str]:
    segments = re.split(r"\n|&&|;", content.replace("\\\n", " "))
    normalized = (normalize_command_content(segment).strip() for segment in segments)
    return [segment for segment in normalized if segment and "||" not in segment]


def normalize_command_content(content: str) -> str:
    return re.sub(r"\s+", " ", content.replace("\\\n", " ")).lower()


def snapshot_segments(snapshot: Snapshot) -> list[str]:
    return [
        segment for file in command_files(snapshot) for segment in command_segments(file.content)
    ]


def any_segment(snapshot: Snapshot, pattern: str) -> bool:
    compiled = re.compile(pattern)
    return any(compiled.search(segment) for segment in snapshot_segments(snapshot))


# A tool invocation anchored to command position, so package names inside
# `pip install pytest mypy` segments are not evidence. Tools count bare, via
# `uv run [flags] tool`, `uvx [flags] tool`, or `python -m tool`, optionally
# behind environment-variable assignments.
ENV_PREFIX = r"(?:[A-Za-z_][A-Za-z0-9_]*=\S+ )*"
RUNNER_PREFIX = (
    r"(?:uv run(?: -[\w-]+(?: [\w./=-]+)?)* |uvx(?: -[\w-]+(?: [\w./=-]+)?)* |python3? -m )?"
)


def tool_pattern(tool: str) -> str:
    return rf"^{ENV_PREFIX}{RUNNER_PREFIX}{tool}\b"


def has_typecheck_command(snapshot: Snapshot) -> bool:
    return (
        any_segment(snapshot, tool_pattern("ty") + r"(?: [\w./=-]+)* check\b")
        or any_segment(snapshot, tool_pattern("mypy"))
        or any_segment(snapshot, tool_pattern("pyright"))
    )


def has_lint_command(snapshot: Snapshot) -> bool:
    return (
        any_segment(snapshot, tool_pattern("ruff") + r" check\b")
        or any_segment(snapshot, tool_pattern("flake8"))
        or any_segment(snapshot, tool_pattern("pylint"))
    )


def has_format_command(snapshot: Snapshot) -> bool:
    return any_segment(snapshot, tool_pattern("ruff") + r" format\b[^\n]*--check\b") or any_segment(
        snapshot, tool_pattern("black") + r"\b[^\n]*--check\b"
    )


def has_test_command(snapshot: Snapshot) -> bool:
    return (
        any_segment(snapshot, tool_pattern("pytest"))
        or any_segment(snapshot, rf"^{ENV_PREFIX}python3? -m unittest\b")
        or any_segment(snapshot, tool_pattern("tox"))
        or any_segment(snapshot, tool_pattern("coverage") + r" run\b[^\n]*\b(?:pytest|unittest)\b")
    )


COV_FAIL_UNDER_FLAG = re.compile(r"--cov-fail-under[ =](\d+(?:\.\d+)?)")
FAIL_UNDER_FLAG = re.compile(r"--fail-under[ =](\d+(?:\.\d+)?)")


# Threshold flags only count on commands that evaluate them: pytest-cov
# collection and coverage report. An explicit below-threshold value is a
# weakened gate and fails the rule instead of falling back to config.
def has_coverage_command(snapshot: Snapshot, threshold: int) -> bool:
    values = coverage_flag_values(snapshot)
    if values:
        return min(values) >= threshold
    return has_coverage_run(snapshot) and has_coverage_config_threshold(snapshot, threshold)


def coverage_flag_values(snapshot: Snapshot) -> list[float]:
    values: list[float] = []
    for segment in snapshot_segments(snapshot):
        if pytest_cov_segment(segment):
            values.extend(float(match.group(1)) for match in COV_FAIL_UNDER_FLAG.finditer(segment))
        elif coverage_report_segment(segment):
            values.extend(float(match.group(1)) for match in FAIL_UNDER_FLAG.finditer(segment))
    return values


def pytest_cov_segment(segment: str) -> bool:
    return (
        re.search(tool_pattern("pytest"), segment) is not None
        and re.search(r"--cov(?:[= ]|$)", segment) is not None
    )


def coverage_report_segment(segment: str) -> bool:
    return re.search(tool_pattern("coverage") + r" report\b", segment) is not None


# Only commands that evaluate the threshold count: pytest-cov collection and
# `coverage report` honor fail_under; a bare `coverage run` never does, and
# `--cov-report` is output selection, not collection.
def has_coverage_run(snapshot: Snapshot) -> bool:
    if any(pytest_cov_segment(segment) for segment in snapshot_segments(snapshot)):
        return True
    return any_segment(snapshot, tool_pattern("coverage") + r" report\b")


def has_coverage_config_threshold(snapshot: Snapshot, threshold: int) -> bool:
    from slophammer.rules.toolconfig import coverage_fail_under

    configured = coverage_fail_under(snapshot)
    return configured is not None and configured >= threshold


# radon cc is report-only and exits zero on complex code, so it is not a
# gate; xenon gates, but only with a strict absolute threshold — grade A or
# B keeps cyclomatic complexity within the shared bound, anything looser is
# a weakened gate.
def has_complexity_command(snapshot: Snapshot) -> bool:
    return any_segment(snapshot, tool_pattern("xenon") + r"\b[^\n]*--max-absolute[= ][ab]\b")


def has_dry_command(snapshot: Snapshot) -> bool:
    return (
        any_segment(snapshot, rf"^{ENV_PREFIX}{RUNNER_PREFIX}slophammer-py(?:@\S+)? dry\b")
        or any_segment(snapshot, rf"^{ENV_PREFIX}{RUNNER_PREFIX}slophammer(?:@\S+)? dry\b")
        or any_segment(snapshot, rf"^{ENV_PREFIX}{RUNNER_PREFIX}slophammer(?:@\S+)? python dry\b")
        or any_segment(snapshot, tool_pattern("jscpd"))
        or any_segment(snapshot, tool_pattern("pylint") + r"[^\n]*duplicate-code")
    )


# mutmut and cosmic-ray exec both exit zero when mutants survive, so an
# executing run alone is not a gate. Evidence is the gate that can fail:
# a kill-rate check for mutmut (which may also run mutmut itself) or
# cr-rate --fail-over reading a session that cosmic-ray exec produced.
def has_mutation_command(snapshot: Snapshot) -> bool:
    if has_kill_rate_gate(snapshot):
        return True
    return has_gated_cosmic_ray(snapshot)


KILL_RATE_FLOOR = re.compile(r"--min-kill-rate[= ]+(\d+(?:\.\d+)?)")

# A mutation gate in command position: a script or module whose name
# mentions mutation/mutmut, optionally behind a runner and interpreter.
# Echoed or logged text never sits in command position, so it never
# counts.
KILL_RATE_GATE_COMMAND = re.compile(
    rf"^{ENV_PREFIX}{RUNNER_PREFIX}(?:python3? )?\S*(?:mutation|mutmut)\S*(?:\s|$)"
)


# The flag only gates when it rides a mutation command and the floor can
# actually fail: an unrelated command mentioning the flag, or a floor of
# zero, is not evidence. A --stats-file invocation only parses a log, so
# it counts only beside an executing mutmut run that produced the log.
def has_kill_rate_gate(snapshot: Snapshot) -> bool:
    for segment in snapshot_segments(snapshot):
        if KILL_RATE_GATE_COMMAND.search(segment) is None:
            continue
        match = KILL_RATE_FLOOR.search(segment)
        if match is None or float(match.group(1)) <= 0:
            continue
        if "--stats-file" in segment and not has_executing_mutmut_run(snapshot):
            continue
        return True
    return False


def has_executing_mutmut_run(snapshot: Snapshot) -> bool:
    cannot_fail = re.compile(r"--(?:list|dry-?run)")
    run = re.compile(tool_pattern("mutmut") + r" run\b")
    return any(
        run.search(segment) and not cannot_fail.search(segment)
        for segment in snapshot_segments(snapshot)
    )


def has_gated_cosmic_ray(snapshot: Snapshot) -> bool:
    cannot_fail = re.compile(r"--(?:list|dry-?run)")
    executing = re.compile(tool_pattern("cosmic-ray") + r" exec\b")
    if not any(
        executing.search(segment) and not cannot_fail.search(segment)
        for segment in snapshot_segments(snapshot)
    ):
        return False
    # cr-rate fails when the survival rate exceeds the maximum, and the
    # rate cannot exceed 100, so a threshold of 100 or more never fails.
    gate = re.compile(tool_pattern("cr-rate") + r"[^\n]*--fail-over[= ]+(\d+(?:\.\d+)?)")
    return any(
        (match := gate.search(segment)) is not None and float(match.group(1)) < 100
        for segment in snapshot_segments(snapshot)
    )


def has_audit_command(snapshot: Snapshot) -> bool:
    return any_segment(snapshot, tool_pattern("pip[-_]audit")) or any_segment(
        snapshot, rf"^{ENV_PREFIX}uvx? (?:tool run )?(?:pip-)?audit\b"
    )


def slophammer_invocation(evidence: str) -> bool:
    if "uses: osolmaz/slophammer@" in evidence:
        return True
    return any(invocation_with_check(evidence, binary) for binary in CHECKER_BINARIES)


def invocation_with_check(evidence: str, binary: str) -> bool:
    index = evidence.find(binary)
    while index >= 0:
        if " check" in evidence[index : index + CHECK_INVOCATION_WINDOW]:
            return True
        index = evidence.find(binary, index + 1)
    return False
