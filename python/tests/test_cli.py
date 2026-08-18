"""CLI surface tests: commands, formats, flags, and exit codes."""

import json
from pathlib import Path

import pytest

from slophammer.cli import main


def write_repo(root: Path, files: dict[str, str]) -> None:
    for path, content in files.items():
        target = root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)


CLEAN = {
    "README.md": "# Demo\n",
    "AGENTS.md": "# Agents\n\nFollow the repository checks before finishing.\n",
    ".github/workflows/ci.yml": (
        "name: CI\non: [push]\njobs:\n  c:\n    steps:\n      - run: true\n"
    ),
}


class TestCheck:
    def test_clean_repo_exits_zero(self, tmp_path: Path, capsys):
        write_repo(tmp_path, CLEAN)
        assert main(["check", str(tmp_path)]) == 0
        assert "OK: no findings" in capsys.readouterr().out

    def test_findings_exit_one_with_text_output(self, tmp_path: Path, capsys):
        write_repo(
            tmp_path, {"AGENTS.md": "# Agents\n\nFollow the repository checks before finishing.\n"}
        )
        assert main(["check", str(tmp_path)]) == 1
        out = capsys.readouterr().out
        assert "error repo.readme-required README.md" in out
        assert "finding(s)" in out

    def test_json_format(self, tmp_path: Path, capsys):
        write_repo(tmp_path, CLEAN)
        assert main(["check", str(tmp_path), "--format", "json"]) == 0
        parsed = json.loads(capsys.readouterr().out)
        assert parsed == {"ok": True, "findings": []}

    def test_sarif_format(self, tmp_path: Path, capsys):
        write_repo(tmp_path, {})
        assert main(["check", str(tmp_path), "--format", "sarif"]) == 1
        parsed = json.loads(capsys.readouterr().out)
        assert parsed["version"] == "2.1.0"
        assert parsed["runs"][0]["tool"]["driver"]["name"] == "slophammer"

    def test_only_filters_rules(self, tmp_path: Path, capsys):
        write_repo(tmp_path, {})
        assert main(["check", str(tmp_path), "--only", "repo.readme-required"]) == 1
        out = capsys.readouterr().out
        assert "repo.readme-required" in out
        assert "repo.agents-required" not in out

    def test_unknown_only_rule_is_an_error(self, tmp_path: Path, capsys):
        write_repo(tmp_path, CLEAN)
        assert main(["check", str(tmp_path), "--only", "no.such-rule"]) == 2
        assert "unknown rule ids" in capsys.readouterr().err

    def test_empty_only_is_an_error(self, tmp_path: Path, capsys):
        write_repo(tmp_path, CLEAN)
        assert main(["check", str(tmp_path), "--only", ""]) == 2
        assert "requires rule ids" in capsys.readouterr().err

    def test_invalid_config_exits_two(self, tmp_path: Path, capsys):
        write_repo(tmp_path, {**CLEAN, "slophammer.yml": "made_up: true\n"})
        assert main(["check", str(tmp_path)]) == 2
        assert "root.made_up is not supported" in capsys.readouterr().err

    def test_weak_threshold_exits_two(self, tmp_path: Path, capsys):
        write_repo(
            tmp_path, {**CLEAN, "slophammer.yml": "python:\n  coverage:\n    threshold: 84\n"}
        )
        assert main(["check", str(tmp_path)]) == 2
        assert "at least 85" in capsys.readouterr().err

    def test_missing_root_is_a_usage_error(self, tmp_path: Path, capsys):
        assert main(["check", str(tmp_path / "missing")]) == 2
        assert "is not a directory" in capsys.readouterr().err
        assert main(["dry", str(tmp_path / "missing")]) == 2

    def test_baseline_flags_conflict(self, tmp_path: Path):
        write_repo(tmp_path, CLEAN)
        with pytest.raises(SystemExit):
            main(["check", str(tmp_path), "--baseline", "--baseline-write"])


class TestOtherCommands:
    def test_dry_reports_candidates(self, tmp_path: Path, capsys):
        write_repo(tmp_path, CLEAN)
        assert main(["dry", str(tmp_path)]) == 0
        assert "DRY candidates: 0; maximum: 0" in capsys.readouterr().out

    def test_explain_known_rule(self, capsys):
        assert main(["explain", "repo.readme-required"]) == 0
        assert "README required" in capsys.readouterr().out

    def test_explain_unknown_rule(self, capsys):
        assert main(["explain", "no.such-rule"]) == 2
        assert "unknown rule" in capsys.readouterr().err

    def test_repeated_only_flags_accumulate(self, tmp_path: Path, capsys):
        write_repo(tmp_path, {})
        code = main(
            [
                "check",
                str(tmp_path),
                "--only",
                "repo.readme-required",
                "--only",
                "repo.ci-required",
            ]
        )
        assert code == 1
        out = capsys.readouterr().out
        assert "repo.readme-required" in out
        assert "repo.ci-required" in out

    def test_rules_catalog_as_json(self, capsys):
        assert main(["rules", "--format", "json"]) == 0
        catalog = json.loads(capsys.readouterr().out)
        assert {"id", "title", "severity", "path", "message", "description"} <= set(catalog[0])
        assert any(entry["id"] == "py.types-strict-required" for entry in catalog)

    def test_rules_lists_all_ids(self, capsys):
        assert main(["rules"]) == 0
        out = capsys.readouterr().out
        assert "repo.readme-required" in out
        assert "py.types-strict-required" in out
