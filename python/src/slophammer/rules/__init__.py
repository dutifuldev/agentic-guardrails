"""Rule evaluation: the shared repo rules plus the Python rule set."""

from __future__ import annotations

from dataclasses import replace

from slophammer import agents
from slophammer.config import Config
from slophammer.core import Finding, Report, Severity, new_report
from slophammer.repo import Snapshot, has_file, workflow_files
from slophammer.rules import evidence, toolconfig
from slophammer.rules.boundaries import boundary_findings
from slophammer.rules.definitions import (
    DEFAULT_DEFINITIONS,
    PY_ABSOLUTE_IMPORTS,
    PY_AUDIT,
    PY_BOUNDARIES,
    PY_COMPLEXITY,
    PY_COVERAGE,
    PY_DRY,
    PY_FORMAT,
    PY_LINT,
    PY_MUTATION,
    PY_PROJECT,
    PY_SCOPE,
    PY_SUPPRESSIONS,
    PY_TEST,
    PY_TYPECHECK,
    PY_TYPED_MARKER,
    PY_TYPES_STRICT,
    REPO_AGENTS,
    REPO_AGENTS_COMMAND_INVALID,
    REPO_AGENTS_COMMANDS,
    REPO_AGENTS_EMPTY,
    REPO_AGENTS_SCOPE,
    REPO_AGENTS_STALE,
    REPO_CI,
    REPO_README,
    REPO_SLOPHAMMER_CI,
    Definition,
)
from slophammer.rules.imports import absolute_import_findings
from slophammer.rules.scope import production_python_files, scope_counts, scope_findings
from slophammer.rules.suppressions import suppression_findings


def run_rules(snapshot: Snapshot, config: Config, only_rule_ids: list[str] | None = None) -> Report:
    wanted = set(only_rule_ids or [])
    findings: list[Finding] = []
    for definition in DEFAULT_DEFINITIONS:
        if wanted and definition.id not in wanted:
            continue
        findings.extend(check_definition(definition, snapshot, config))
    adjusted = [
        replace(finding, severity=rule_severity(config, finding.rule_id, finding.severity))
        for finding in findings
    ]
    return new_report(adjusted, scope=scope_counts(snapshot, config))


def rule_severity(config: Config, rule_id: str, default: Severity) -> Severity:
    rule = config.rules.get(rule_id)
    if rule is not None and rule.severity in ("error", "warn"):
        return rule.severity  # type: ignore[return-value]  # ty: ignore[invalid-return-type] -- narrowed by the literal comparison above
    return default


def explain(rule_id: str) -> str | None:
    for definition in DEFAULT_DEFINITIONS:
        if definition.id == rule_id:
            return "\n".join(
                [
                    definition.id,
                    "",
                    definition.title,
                    "",
                    definition.description,
                    "",
                    f"Default severity: {definition.severity}",
                    f"Path: {definition.path}",
                ]
            )
    return None


def check_definition(definition: Definition, snapshot: Snapshot, config: Config) -> list[Finding]:
    agent_rule_ids = {
        REPO_AGENTS,
        REPO_AGENTS_EMPTY,
        REPO_AGENTS_COMMANDS,
        REPO_AGENTS_COMMAND_INVALID,
        REPO_AGENTS_SCOPE,
        REPO_AGENTS_STALE,
    }
    if definition.id in agent_rule_ids:
        return [item for item in agents.agents_findings(snapshot) if item.rule_id == definition.id]
    repo_checks = {
        REPO_README: lambda: presence_finding(definition, has_root_file(snapshot, "README.md")),
        REPO_CI: lambda: presence_finding(definition, bool(workflow_files(snapshot))),
        REPO_SLOPHAMMER_CI: lambda: slophammer_ci_findings(definition, snapshot),
    }
    check = repo_checks.get(definition.id)
    if check is not None:
        return check()
    return check_python_definition(definition, snapshot, config)


def presence_finding(definition: Definition, present: bool) -> list[Finding]:
    return [] if present else [finding(definition)]


def has_root_file(snapshot: Snapshot, name: str) -> bool:
    return any(path.lower() == name.lower() for path in snapshot.files)


def slophammer_ci_findings(definition: Definition, snapshot: Snapshot) -> list[Finding]:
    has_config = has_file(snapshot, "slophammer.yml") or has_file(snapshot, "slophammer.yaml")
    if not has_config or evidence.slophammer_invocation(evidence.command_text(snapshot)):
        return []
    return [finding(definition)]


def check_python_definition(
    definition: Definition, snapshot: Snapshot, config: Config
) -> list[Finding]:
    if definition.id == PY_SUPPRESSIONS:
        return suppression_findings(definition, snapshot)
    if definition.id == PY_ABSOLUTE_IMPORTS:
        return absolute_import_findings(definition, snapshot)
    if definition.id == PY_BOUNDARIES:
        return boundary_findings(definition, snapshot, config)
    if definition.id == PY_SCOPE:
        return scope_findings(definition, snapshot, config)
    if not python_project_present(snapshot):
        return []
    return check_python_project_definition(definition, snapshot, config)


def python_project_present(snapshot: Snapshot) -> bool:
    if toolconfig.project_dirs(snapshot) or has_file(snapshot, "setup.py"):
        return True
    return bool(production_python_files(snapshot))


def check_python_project_definition(
    definition: Definition, snapshot: Snapshot, config: Config
) -> list[Finding]:
    checks = {
        PY_PROJECT: lambda: project_findings(definition, snapshot),
        PY_TYPECHECK: lambda: presence_finding(
            definition, evidence.has_typecheck_command(snapshot)
        ),
        PY_TYPES_STRICT: lambda: types_strict_findings(definition, snapshot, config),
        PY_LINT: lambda: presence_finding(definition, evidence.has_lint_command(snapshot)),
        PY_FORMAT: lambda: presence_finding(definition, evidence.has_format_command(snapshot)),
        PY_TEST: lambda: presence_finding(definition, evidence.has_test_command(snapshot)),
        PY_COVERAGE: lambda: presence_finding(
            definition, evidence.has_coverage_command(snapshot, coverage_threshold(config))
        ),
        PY_COMPLEXITY: lambda: complexity_findings(definition, snapshot, config),
        PY_DRY: lambda: presence_finding(definition, evidence.has_dry_command(snapshot)),
        PY_MUTATION: lambda: presence_finding(definition, evidence.has_mutation_command(snapshot)),
        PY_AUDIT: lambda: presence_finding(definition, evidence.has_audit_command(snapshot)),
        PY_TYPED_MARKER: lambda: typed_marker_findings(definition, snapshot),
    }
    check = checks.get(definition.id)
    return check() if check is not None else []


def project_findings(definition: Definition, snapshot: Snapshot) -> list[Finding]:
    if not production_python_files(snapshot):
        return []
    return presence_finding(definition, bool(toolconfig.project_dirs(snapshot)))


def coverage_threshold(config: Config) -> int:
    if config.python is not None and config.python.coverage is not None:
        threshold = config.python.coverage.threshold
        if threshold is not None:
            return threshold
    return 85


def complexity_limit(config: Config) -> int:
    if config.python is not None and config.python.complexity is not None:
        limit = config.python.complexity.max_complexity
        if limit is not None:
            return limit
    return 8


def complexity_findings(
    definition: Definition, snapshot: Snapshot, config: Config
) -> list[Finding]:
    if evidence.has_complexity_command(snapshot):
        return []
    limit = toolconfig.ruff_complexity_limit(snapshot)
    enforced = (
        toolconfig.ruff_selects(snapshot, "C901")
        and limit is not None
        and limit <= complexity_limit(config)
    )
    return presence_finding(definition, enforced)


def typed_marker_findings(definition: Definition, snapshot: Snapshot) -> list[Finding]:
    return [
        Finding(
            rule_id=definition.id,
            severity=definition.severity,
            path=pyproject_path,
            message=definition.message,
        )
        for pyproject_path in toolconfig.packaged_dirs_without_typed_marker(snapshot)
    ]


def types_strict_findings(
    definition: Definition, snapshot: Snapshot, config: Config
) -> list[Finding]:
    checker = toolconfig.typechecker_in_use(snapshot)
    if checker is None:
        return []
    details = typechecker_details(checker, snapshot, config)
    if not toolconfig.ruff_selects(snapshot, "ANN"):
        details.append("Ruff must select the ANN rules so every signature is annotated")
    if not details:
        return []
    path = "ty.toml" if has_file(snapshot, "ty.toml") and checker == "ty" else definition.path
    return [
        Finding(
            rule_id=definition.id,
            severity=definition.severity,
            path=path,
            message=f"{definition.message}: {'; '.join(details)}",
        )
    ]


def typechecker_details(checker: str, snapshot: Snapshot, config: Config) -> list[str]:
    if checker == "ty":
        return ty_contract_details(snapshot, config)
    if checker == "mypy":
        return mypy_details(snapshot)
    if not toolconfig.pyright_strict(snapshot):
        return ["pyright must set typeCheckingMode to strict"]
    return []


def ty_contract_details(snapshot: Snapshot, config: Config) -> list[str]:
    contract = toolconfig.ty_contract(snapshot)
    details: list[str] = []
    demoted = toolconfig.demoted_stable_rules(contract, config)
    if demoted:
        details.append(
            "default-error ty rules demoted without a reasoned override: " + ", ".join(demoted)
        )
    if not contract.error_on_warning:
        details.append("ty must set error-on-warning so warn-tier rules block")
    missing = toolconfig.missing_promotions(contract)
    if missing:
        details.append("ty rules that must be promoted to error: " + ", ".join(missing))
    if contract.respects_type_ignore:
        details.append(
            "ty must set respect-type-ignore-comments to false so only coded suppressions work"
        )
    return details


def mypy_details(snapshot: Snapshot) -> list[str]:
    details: list[str] = []
    if not toolconfig.mypy_strict(snapshot):
        details.append("mypy must run with strict or disallow_untyped_defs")
    if toolconfig.pydantic_dependency(snapshot) and not toolconfig.mypy_pydantic_plugin(snapshot):
        details.append("mypy needs the pydantic plugin when pydantic is a dependency")
    return details


def finding(definition: Definition) -> Finding:
    return Finding(
        rule_id=definition.id,
        severity=definition.severity,
        path=definition.path,
        message=definition.message,
    )
