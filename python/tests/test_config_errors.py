"""Config validation error branches."""

import pytest

from slophammer.config import ConfigError, parse_config


def rejects(content: str, message: str) -> None:
    with pytest.raises(ConfigError, match=message):
        parse_config(content)


class TestShapeErrors:
    def test_root_must_be_mapping(self):
        rejects("- a\n- b\n", "root must be a mapping")

    def test_unparseable_yaml(self):
        rejects(": [", "config parse failed")

    def test_boundaries_must_be_a_list(self):
        rejects("python:\n  dependency_boundaries: yes\n", "must be a list")

    def test_boundary_from_required(self):
        rejects(
            "python:\n  dependency_boundaries:\n    - allow: []\n",
            "from must be a non-empty string",
        )

    def test_demotions_must_be_a_list(self):
        rejects("python:\n  typecheck:\n    demotions: yes\n", "must be a list")

    def test_demotion_reason_required(self):
        rejects(
            "python:\n  typecheck:\n    demotions:\n      - rule: deprecated\n",
            "reason must be a non-empty string",
        )

    def test_exclude_must_be_a_list(self):
        rejects("python:\n  coverage:\n    exclude: src\n", "must be a list")

    def test_exclude_entry_unknown_key(self):
        rejects(
            "python:\n  coverage:\n    exclude:\n      - pattern: x/**\n        surprise: true\n",
            "is not supported",
        )

    def test_paths_must_be_strings(self):
        rejects("python:\n  coverage:\n    paths:\n      - 1\n", "list of strings")

    def test_threshold_must_be_integer(self):
        rejects("python:\n  coverage:\n    threshold: high\n", "must be an integer")

    def test_copied_blocks_enabled_must_be_boolean(self):
        rejects(
            'python:\n  dry:\n    copied_blocks:\n      enabled: "yes"\n',
            "must be a boolean",
        )


class TestPolicyErrors:
    def test_complexity_cannot_be_weakened(self):
        rejects("python:\n  complexity:\n    max: 9\n", "at most 8")

    def test_dry_max_findings_must_be_zero(self):
        rejects("python:\n  dry:\n    max_findings: 3\n", "must be 0")

    def test_min_tokens_must_be_positive(self):
        rejects(
            "python:\n  dry:\n    copied_blocks:\n      min_tokens: 0\n",
            "must be positive",
        )

    def test_disabled_rule_needs_reason(self):
        rejects(
            "rules:\n  repo.readme-required:\n    disabled: true\n",
            "reason is required",
        )

    def test_disabled_rule_rejects_blank_reason(self):
        rejects(
            'rules:\n  repo.readme-required:\n    disabled: true\n    reason: " "\n',
            "reason is required",
        )

    def test_disabled_rule_must_be_boolean(self):
        rejects(
            'rules:\n  repo.readme-required:\n    disabled: "true"\n',
            "disabled must be a boolean",
        )

    def test_substring_lookalikes_are_production(self):
        rejects(
            "python:\n  coverage:\n    exclude:\n      - src/latest_feature/**\n",
            "requires a reason for production paths",
        )

    def test_conftest_and_test_dir_are_conventional(self):
        parse_config(
            "python:\n  coverage:\n    exclude:\n      - conftest.py\n      - test/**\n"
            '      - "**/test_*.py"\n'
        )

    def test_production_exclude_needs_reason(self):
        rejects(
            "python:\n  coverage:\n    exclude:\n      - src/core/**\n",
            "requires a reason for production paths",
        )

    def test_empty_exclude_reason_rejected(self):
        rejects(
            "python:\n  coverage:\n    exclude:\n      - pattern: src/core/**\n"
            '        reason: " "\n',
            "must be a non-empty string",
        )


class TestIgnoredSections:
    def test_go_structural_unknown_key(self):
        rejects("go:\n  dry:\n    structural:\n      made_up: true\n", "is not supported")

    def test_rust_unsafe_allow_unknown_key(self):
        rejects(
            "rust:\n  unsafe:\n    allow:\n      - path: src/lib.rs\n        made_up: true\n",
            "is not supported",
        )

    def test_typescript_boundary_unknown_key(self):
        rejects(
            "typescript:\n  dependency_boundaries:\n    - from: src/app\n      made_up: true\n",
            "is not supported",
        )

    def test_go_exclude_entries_validated(self):
        rejects("go:\n  exclude: nope\n", "must be a list")
