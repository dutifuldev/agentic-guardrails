"""Strict slophammer.yml parsing for the Python checker.

The ``python:`` and ``rules:`` sections are parsed into typed config; the
``go:``, ``typescript:``, and ``rust:`` sections are shape-validated against
their allowed key trees so the four checkers cross-validate each other, then
ignored. Unknown keys anywhere fail with exit code 2.
"""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field

import yaml

from slophammer.repo import Snapshot

CONFIG_FILE_NAMES = ("slophammer.yml", "slophammer.yaml")

# The conventional non-production list from specs/CONFIG.md; for Python
# paths, migrations join it (Alembic and Django migrations are as standard a
# carve-out as testdata is for Go). Classification is path-aware: whole
# segments and file-name shapes, never raw substrings, so a directory that
# merely contains "test_" in its name stays production.
CONVENTIONAL_DIR_SEGMENTS = frozenset(
    {
        "tests",
        "test",
        "fixtures",
        "templates",
        "testdata",
        "dist",
        "build",
        "coverage",
        "target",
        "node_modules",
        "vendor",
        "scripts",
        "migrations",
    }
)
CONVENTIONAL_BASENAME_MARKERS = ("_test.", ".test.", ".spec.")


class ConfigError(Exception):
    """Invalid config fails the check with exit code 2."""


@dataclass(frozen=True)
class ExcludeEntry:
    pattern: str
    reason: str | None = None


@dataclass(frozen=True)
class CoverageConfig:
    threshold: int | None = None
    paths: list[str] = field(default_factory=list)
    exclude: list[ExcludeEntry] = field(default_factory=list)


@dataclass(frozen=True)
class ComplexityConfig:
    max_complexity: int | None = None


@dataclass(frozen=True)
class CopiedBlocksConfig:
    enabled: bool = True
    min_tokens: int = 100


@dataclass(frozen=True)
class DryConfig:
    max_findings: int = 0
    paths: list[str] = field(default_factory=list)
    exclude: list[ExcludeEntry] = field(default_factory=list)
    copied_blocks: CopiedBlocksConfig = field(default_factory=CopiedBlocksConfig)


@dataclass(frozen=True)
class MutationConfig:
    targets: list[str] = field(default_factory=list)
    exclude: list[ExcludeEntry] = field(default_factory=list)


@dataclass(frozen=True)
class DependencyBoundary:
    source: str
    allow: list[str] = field(default_factory=list)


@dataclass(frozen=True)
class TypecheckDemotion:
    rule: str
    reason: str


@dataclass(frozen=True)
class TypecheckConfig:
    demotions: list[TypecheckDemotion] = field(default_factory=list)


@dataclass(frozen=True)
class PythonConfig:
    coverage: CoverageConfig | None = None
    complexity: ComplexityConfig | None = None
    dry: DryConfig | None = None
    mutation: MutationConfig | None = None
    dependency_boundaries: list[DependencyBoundary] = field(default_factory=list)
    typecheck: TypecheckConfig | None = None


@dataclass(frozen=True)
class RuleConfig:
    severity: str | None = None
    disabled: bool = False
    reason: str | None = None
    threshold: float | None = None
    max_value: float | None = None


@dataclass(frozen=True)
class Config:
    rules: dict[str, RuleConfig] = field(default_factory=dict)
    python: PythonConfig | None = None
    configured: bool = False


def load_config(snapshot: Snapshot) -> Config:
    for name in CONFIG_FILE_NAMES:
        file = snapshot.files.get(name)
        if file is not None:
            return parse_config(file.content)
    return Config()


def parse_config(content: str) -> Config:
    try:
        parsed = yaml.safe_load(content)
    except yaml.YAMLError as error:
        raise ConfigError(f"config parse failed: {error}") from error
    root = as_mapping(parsed, "root")
    assert_known_keys(root, "root", ("rules", "go", "typescript", "rust", "python"))
    for section in ("go", "typescript", "rust"):
        validate_ignored_section(root.get(section), section)
    config = Config(
        rules=parse_rules(root.get("rules")),
        python=parse_python(root.get("python")),
        configured=True,
    )
    validate(config)
    return config


def parse_rules(value: object) -> dict[str, RuleConfig]:
    if value is None:
        return {}
    section = as_mapping(value, "rules")
    rules: dict[str, RuleConfig] = {}
    for rule_id, raw in section.items():
        entry = as_mapping(raw, f"rules.{rule_id}")
        assert_known_keys(
            entry, f"rules.{rule_id}", ("severity", "disabled", "reason", "threshold", "max")
        )
        rules[str(rule_id)] = RuleConfig(
            severity=rule_severity_value(entry.get("severity"), f"rules.{rule_id}.severity"),
            disabled=optional_bool(entry.get("disabled"), f"rules.{rule_id}.disabled"),
            reason=optional_str(entry.get("reason"), f"rules.{rule_id}.reason"),
            threshold=optional_number(entry.get("threshold"), f"rules.{rule_id}.threshold"),
            max_value=optional_number(entry.get("max"), f"rules.{rule_id}.max"),
        )
    return rules


def parse_python(value: object) -> PythonConfig | None:
    if value is None:
        return None
    root = as_mapping(value, "python")
    assert_known_keys(
        root,
        "python",
        ("coverage", "complexity", "dry", "mutation", "dependency_boundaries", "typecheck"),
    )
    return PythonConfig(
        coverage=parse_coverage(root.get("coverage")),
        complexity=parse_complexity(root.get("complexity")),
        dry=parse_dry(root.get("dry")),
        mutation=parse_mutation(root.get("mutation")),
        dependency_boundaries=parse_boundaries(root.get("dependency_boundaries")),
        typecheck=parse_typecheck(root.get("typecheck")),
    )


def parse_coverage(value: object) -> CoverageConfig | None:
    if value is None:
        return None
    section = as_mapping(value, "python.coverage")
    assert_known_keys(section, "python.coverage", ("threshold", "paths", "exclude"))
    return CoverageConfig(
        threshold=optional_int(section.get("threshold"), "python.coverage.threshold"),
        paths=string_list(section.get("paths"), "python.coverage.paths"),
        exclude=exclude_entries(section.get("exclude"), "python.coverage.exclude"),
    )


def parse_complexity(value: object) -> ComplexityConfig | None:
    if value is None:
        return None
    section = as_mapping(value, "python.complexity")
    assert_known_keys(section, "python.complexity", ("max",))
    return ComplexityConfig(
        max_complexity=optional_int(section.get("max"), "python.complexity.max")
    )


def parse_dry(value: object) -> DryConfig | None:
    if value is None:
        return None
    section = as_mapping(value, "python.dry")
    assert_known_keys(section, "python.dry", ("max_findings", "paths", "exclude", "copied_blocks"))
    max_findings = optional_int(section.get("max_findings"), "python.dry.max_findings")
    return DryConfig(
        max_findings=0 if max_findings is None else max_findings,
        paths=string_list(section.get("paths"), "python.dry.paths"),
        exclude=exclude_entries(section.get("exclude"), "python.dry.exclude"),
        copied_blocks=parse_copied_blocks(section.get("copied_blocks")),
    )


def parse_copied_blocks(value: object) -> CopiedBlocksConfig:
    if value is None:
        return CopiedBlocksConfig()
    section = as_mapping(value, "python.dry.copied_blocks")
    assert_known_keys(section, "python.dry.copied_blocks", ("enabled", "min_tokens"))
    enabled = section.get("enabled", True)
    if not isinstance(enabled, bool):
        raise ConfigError("python.dry.copied_blocks.enabled must be a boolean")
    min_tokens = optional_int(section.get("min_tokens"), "python.dry.copied_blocks.min_tokens")
    return CopiedBlocksConfig(enabled=enabled, min_tokens=100 if min_tokens is None else min_tokens)


def parse_mutation(value: object) -> MutationConfig | None:
    if value is None:
        return None
    section = as_mapping(value, "python.mutation")
    assert_known_keys(section, "python.mutation", ("targets", "exclude"))
    return MutationConfig(
        targets=string_list(section.get("targets"), "python.mutation.targets"),
        exclude=exclude_entries(section.get("exclude"), "python.mutation.exclude"),
    )


def parse_boundaries(value: object) -> list[DependencyBoundary]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise ConfigError("python.dependency_boundaries must be a list")
    boundaries: list[DependencyBoundary] = []
    for index, item in enumerate(value):
        field_name = f"python.dependency_boundaries[{index}]"
        entry = as_mapping(item, field_name)
        assert_known_keys(entry, field_name, ("from", "allow"))
        boundaries.append(
            DependencyBoundary(
                source=require_str(entry.get("from"), f"{field_name}.from"),
                allow=string_list(entry.get("allow"), f"{field_name}.allow"),
            )
        )
    return boundaries


def parse_typecheck(value: object) -> TypecheckConfig | None:
    if value is None:
        return None
    section = as_mapping(value, "python.typecheck")
    assert_known_keys(section, "python.typecheck", ("demotions",))
    demotions = section.get("demotions")
    if demotions is None:
        return TypecheckConfig()
    if not isinstance(demotions, list):
        raise ConfigError("python.typecheck.demotions must be a list")
    parsed: list[TypecheckDemotion] = []
    for index, item in enumerate(demotions):
        field_name = f"python.typecheck.demotions[{index}]"
        entry = as_mapping(item, field_name)
        assert_known_keys(entry, field_name, ("rule", "reason"))
        parsed.append(
            TypecheckDemotion(
                rule=require_str(entry.get("rule"), f"{field_name}.rule"),
                reason=require_str(entry.get("reason"), f"{field_name}.reason"),
            )
        )
    return TypecheckConfig(demotions=parsed)


def validate(config: Config) -> None:
    for rule_id, rule in config.rules.items():
        if rule.disabled and (rule.reason is None or rule.reason.strip() == ""):
            raise ConfigError(f"rules.{rule_id}.reason is required when disabled is true")
    python = config.python
    if python is None:
        return
    validate_thresholds(python)
    validate_excludes(python)
    for boundary in python.dependency_boundaries:
        if boundary.source.strip() == "":
            raise ConfigError("python dependency boundaries require from")
    for demotion in python.typecheck.demotions if python.typecheck else []:
        if demotion.rule.strip() == "" or demotion.reason.strip() == "":
            raise ConfigError("python.typecheck.demotions entries require rule and reason")


def validate_thresholds(python: PythonConfig) -> None:
    if python.coverage is not None and python.coverage.threshold is not None:
        if python.coverage.threshold < 85:
            raise ConfigError("python coverage threshold must be at least 85")
    if python.complexity is not None and python.complexity.max_complexity is not None:
        if python.complexity.max_complexity > 8:
            raise ConfigError("python complexity max must be at most 8")
    if python.dry is not None:
        if python.dry.max_findings != 0:
            raise ConfigError("python dry max_findings must be 0 for production code")
        if python.dry.copied_blocks.min_tokens <= 0:
            raise ConfigError("python dry copied_blocks min_tokens must be positive")


def validate_excludes(python: PythonConfig) -> None:
    sections = [
        ("python.coverage.exclude", python.coverage.exclude if python.coverage else []),
        ("python.dry.exclude", python.dry.exclude if python.dry else []),
        ("python.mutation.exclude", python.mutation.exclude if python.mutation else []),
    ]
    for section, entries in sections:
        for entry in entries:
            if entry.reason is not None and entry.reason.strip() == "":
                raise ConfigError(f"{section} reasons must be non-empty")
            if entry.reason is None and not conventional_exclude_pattern(entry.pattern):
                raise ConfigError(f"{section} requires a reason for production paths")


def conventional_exclude_pattern(pattern: str) -> bool:
    if "generated" in pattern:
        return True
    segments = [segment for segment in pattern.split("/") if segment not in ("", "*", "**")]
    if not segments:
        return False
    if any(segment in CONVENTIONAL_DIR_SEGMENTS for segment in segments):
        return True
    return conventional_file_pattern(segments[-1])


def conventional_file_pattern(basename: str) -> bool:
    name = basename.lstrip("*")
    if name.startswith("test_") or name == "conftest.py":
        return True
    if name.endswith("_test.py") or basename.endswith("_test.py"):
        return True
    return any(marker in basename for marker in CONVENTIONAL_BASENAME_MARKERS)


def exclude_entries(value: object, section: str) -> list[ExcludeEntry]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise ConfigError(f"{section} must be a list")
    entries: list[ExcludeEntry] = []
    for index, item in enumerate(value):
        if isinstance(item, str):
            entries.append(ExcludeEntry(pattern=item))
            continue
        field_name = f"{section}[{index}]"
        entry = as_mapping(item, field_name)
        assert_known_keys(entry, field_name, ("pattern", "reason"))
        entries.append(
            ExcludeEntry(
                pattern=require_str(entry.get("pattern"), f"{field_name}.pattern"),
                reason=require_str(entry.get("reason"), f"{field_name}.reason"),
            )
        )
    return entries


# Allowed key trees for the sections this checker does not enforce. A tuple
# marks nested mappings; EXCLUDES and BOUNDARIES mark the two shared list
# shapes that still need their entry keys checked.
EXCLUDES = "<excludes>"
BOUNDARIES = "<boundaries>"
IGNORED_SECTIONS: dict[str, dict[str, object]] = {
    "go": {
        "coverage": {"threshold": None, "profile": None},
        "targets": None,
        "exclude": EXCLUDES,
        "dry": {
            "max_findings": None,
            "paths": None,
            "exclude": EXCLUDES,
            "structural": {
                "enabled": None,
                "threshold": None,
                "min_lines": None,
                "min_nodes": None,
            },
            "copied_blocks": {"enabled": None, "min_tokens": None},
        },
        "crap": {"max_score": None},
        "mutation": {"targets": None, "exclude": EXCLUDES},
        "dependency_boundaries": BOUNDARIES,
    },
    "typescript": {
        "coverage": {"threshold": None, "paths": None, "exclude": EXCLUDES},
        "complexity": {"max": None},
        "dry": {
            "max_findings": None,
            "paths": None,
            "exclude": EXCLUDES,
            "copied_blocks": {"enabled": None, "min_tokens": None},
        },
        "mutation": {"targets": None},
        "dependency_boundaries": BOUNDARIES,
    },
    "rust": {
        "coverage": {"threshold": None, "paths": None, "exclude": EXCLUDES},
        "complexity": {"cognitive_max": None},
        "targets": None,
        "exclude": EXCLUDES,
        "dry": {
            "max_findings": None,
            "paths": None,
            "exclude": EXCLUDES,
            "copied_blocks": {"enabled": None, "min_tokens": None},
        },
        "unsafe": {"policy": None, "allow": "<unsafe-allow>"},
        "mutation": {"targets": None, "exclude": EXCLUDES},
        "dependency_boundaries": BOUNDARIES,
    },
}


def validate_ignored_section(value: object, section: str) -> None:
    if value is None:
        return
    validate_ignored_tree(value, IGNORED_SECTIONS[section], section)


def validate_ignored_tree(value: object, tree: Mapping[str, object], field_name: str) -> None:
    mapping = as_mapping(value, field_name)
    assert_known_keys(mapping, field_name, tuple(tree))
    for key, subtree in tree.items():
        if key not in mapping or mapping[key] is None:
            continue
        validate_ignored_value(mapping[key], subtree, f"{field_name}.{key}")


def validate_ignored_value(value: object, subtree: object, field_name: str) -> None:
    if isinstance(subtree, dict):
        validate_ignored_tree(value, {str(key): item for key, item in subtree.items()}, field_name)
    elif subtree == EXCLUDES:
        exclude_entries(value, field_name)
    elif subtree == BOUNDARIES:
        validate_ignored_entries(value, field_name, ("from", "allow"))
    elif subtree == "<unsafe-allow>":
        validate_ignored_entries(value, field_name, ("path", "reason"))


def validate_ignored_entries(value: object, field_name: str, allowed: tuple[str, ...]) -> None:
    if not isinstance(value, list):
        raise ConfigError(f"{field_name} must be a list")
    for index, item in enumerate(value):
        entry = as_mapping(item, f"{field_name}[{index}]")
        assert_known_keys(entry, f"{field_name}[{index}]", allowed)


def as_mapping(value: object, field_name: str) -> Mapping[str, object]:
    if value is None:
        return {}
    if not isinstance(value, dict):
        raise ConfigError(f"{field_name} must be a mapping")
    return {str(key): item for key, item in value.items()}


def assert_known_keys(
    mapping: Mapping[str, object], field_name: str, allowed: tuple[str, ...]
) -> None:
    if field_name == "rules":
        return
    for key in mapping:
        if key not in allowed:
            raise ConfigError(f"{field_name}.{key} is not supported")


def rule_severity_value(value: object, field_name: str) -> str | None:
    severity = optional_str(value, field_name)
    if severity is not None and severity not in ("error", "warn"):
        raise ConfigError(f"{field_name} must be error or warn")
    return severity


def optional_str(value: object, field_name: str) -> str | None:
    if value is None:
        return None
    if not isinstance(value, str):
        raise ConfigError(f"{field_name} must be a string")
    return value


def require_str(value: object, field_name: str) -> str:
    if not isinstance(value, str) or value.strip() == "":
        raise ConfigError(f"{field_name} must be a non-empty string")
    return value


def optional_bool(value: object, field_name: str) -> bool:
    if value is None:
        return False
    if not isinstance(value, bool):
        raise ConfigError(f"{field_name} must be a boolean")
    return value


def optional_int(value: object, field_name: str) -> int | None:
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, int):
        raise ConfigError(f"{field_name} must be an integer")
    return value


def optional_number(value: object, field_name: str) -> float | None:
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{field_name} must be a number")
    return float(value)


def string_list(value: object, field_name: str) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise ConfigError(f"{field_name} must be a list of strings")
    items: list[str] = []
    for item in value:
        if not isinstance(item, str):
            raise ConfigError(f"{field_name} must be a list of strings")
        items.append(item)
    return items
