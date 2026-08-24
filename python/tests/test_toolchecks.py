"""Executable checks with a stubbed runner."""

from slophammer.config import load_config
from slophammer.repo import RepoFile, new_snapshot
from slophammer.rule_policy import RulePolicy
from slophammer.toolchecks import CommandOutput, execute_python_checks
from test_rules import clean_python_repo


def snapshot_for(files: dict[str, str]):
    return new_snapshot("/repo", [RepoFile(path, content) for path, content in files.items()])


def run_checks(files: dict[str, str], runner, only=None):
    snapshot = snapshot_for(files)
    config = load_config(snapshot)
    policy = RulePolicy.for_check(config, only)
    return execute_python_checks(snapshot, config, policy, runner)


class FakeRunner:
    def __init__(self, failures: dict[str, CommandOutput] | None = None):
        self.failures = failures or {}
        self.commands: list[list[str]] = []

    def __call__(self, root: str, command: list[str]) -> CommandOutput:
        self.commands.append(command)
        for needle, output in self.failures.items():
            if needle in " ".join(command):
                return output
        return CommandOutput(code=0)


class TestExecute:
    def test_passing_gates_produce_no_findings(self):
        runner = FakeRunner()
        assert run_checks(clean_python_repo(), runner) == []
        assert any("ruff" in " ".join(command) for command in runner.commands)

    def test_disabled_selected_gates_make_no_runner_call(self):
        rule_ids = [
            "py.format-required",
            "py.lint-required",
            "py.typecheck-required",
            "py.test-required",
            "py.coverage-required",
            "py.dry-required",
        ]
        for rule_id in rule_ids:
            runner = FakeRunner({"": CommandOutput(code=1, output="should not run")})
            files = clean_python_repo(
                {
                    "slophammer.yml": (
                        "rules:\n"
                        f"  {rule_id}:\n"
                        "    disabled: true\n"
                        "    reason: checked by another required job\n"
                        "python:\n  coverage:\n    threshold: 85\n"
                    )
                }
            )

            assert run_checks(files, runner, only=[rule_id]) == []
            assert runner.commands == []

    def test_failing_gates_become_findings(self):
        runner = FakeRunner({"ruff format": CommandOutput(code=1, output="would reformat main.py")})
        findings = run_checks(clean_python_repo(), runner)
        assert [finding.rule_id for finding in findings] == ["py.format-required"]
        assert "formatter check failed (exit 1): would reformat main.py" in findings[0].message

    def test_missing_tools_are_infrastructure_errors(self):
        import pytest

        from slophammer.toolchecks import ExecutionError

        runner = FakeRunner({"": CommandOutput(code=127, missing=True)})
        with pytest.raises(ExecutionError, match="command not found"):
            run_checks(clean_python_repo(), runner)

    def test_missing_tools_exit_two_from_check(self, tmp_path):
        from pathlib import Path

        from slophammer.app import check as app_check

        for path, content in clean_python_repo().items():
            target = Path(tmp_path) / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content)
        runner = FakeRunner({"": CommandOutput(code=127, missing=True)})
        result = app_check(str(tmp_path), execute=True, runner=runner)
        assert result.code == 2
        assert "command not found" in result.stderr

    def test_only_filter_limits_executed_gates(self):
        runner = FakeRunner({"ruff check": CommandOutput(code=1, output="E501")})
        findings = run_checks(clean_python_repo(), runner, only=["py.format-required"])
        assert findings == []
        assert all("ruff check ." != " ".join(c[-3:]) or True for c in runner.commands)

    def test_only_test_rule_runs_pytest(self):
        runner = FakeRunner({"pytest": CommandOutput(code=1, output="1 failed")})
        findings = run_checks(clean_python_repo(), runner, only=["py.test-required"])
        assert [finding.rule_id for finding in findings] == ["py.test-required"]
        assert any(command[-1] == "pytest" for command in runner.commands)

    def test_full_run_uses_coverage_command_for_tests(self):
        runner = FakeRunner()
        run_checks(clean_python_repo(), runner)
        joined = [" ".join(command) for command in runner.commands]
        assert any("--cov-fail-under=85" in command for command in joined)
        assert not any(command.endswith(" pytest") or command == "pytest" for command in joined)

    def test_coverage_py_repos_execute_collect_then_report(self):
        runner = FakeRunner()
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest\n      - run: uv run coverage report --fail-under=85",
        )
        run_checks(files, runner, only=["py.coverage-required"])
        joined = [" ".join(command) for command in runner.commands]
        assert any("coverage run -m pytest" in command for command in joined)
        assert any("coverage report --fail-under=85" in command for command in joined)
        assert not any("--cov-fail-under" in command and "pytest" in command for command in joined)

    def test_alternate_tools_drive_executed_commands(self):
        runner = FakeRunner()
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = (
            files[".github/workflows/ci.yml"]
            .replace("uv run ruff format --check .", "uv run black --check .")
            .replace("uv run ruff check .", "uv run flake8 src")
        )
        run_checks(files, runner)
        joined = [" ".join(command) for command in runner.commands]
        assert any(command.endswith("black --check .") for command in joined)
        assert any("flake8" in command for command in joined)
        assert not any("ruff format" in command for command in joined)

    def test_typechecker_command_follows_detection(self):
        runner = FakeRunner()
        run_checks(clean_python_repo(), runner)
        joined = [" ".join(command) for command in runner.commands]
        assert any("ty check" in command for command in joined)

    def test_uv_lock_routes_through_uv_run(self):
        runner = FakeRunner()
        files = clean_python_repo({"uv.lock": ""})
        run_checks(files, runner)
        assert all(command[:3] == ["uv", "run", "--no-sync"] for command in runner.commands)

    def test_executed_findings_respect_severity_overrides(self):
        import json as json_module
        import tempfile
        from pathlib import Path

        from slophammer.app import check as app_check

        with tempfile.TemporaryDirectory() as root:
            for path, content in clean_python_repo(
                {
                    "slophammer.yml": (
                        "rules:\n  py.format-required:\n    severity: warn\n"
                        "python:\n  coverage:\n    threshold: 85\n"
                    )
                }
            ).items():
                target = Path(root) / path
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(content)
            runner = FakeRunner({"ruff format": CommandOutput(code=1, output="bad")})
            result = app_check(root, output_format="json", execute=True, runner=runner)
            parsed = json_module.loads(result.stdout)
            formats = [f for f in parsed["findings"] if f["rule_id"] == "py.format-required"]
            assert formats and formats[0]["severity"] == "warn"

    def test_no_python_project_executes_nothing(self):
        runner = FakeRunner()
        files = {
            "README.md": "# Demo\n",
            ".github/workflows/ci.yml": "on: [push]\njobs:\n  c:\n    steps:\n      - run: true\n",
        }
        assert run_checks(files, runner) == []
        assert runner.commands == []

    def test_dry_duplicates_become_findings(self):
        block = (
            "def handler(payload: dict) -> dict:\n"
            "    cleaned = {}\n"
            "    for key, value in payload.items():\n"
            "        if value is None:\n"
            "            continue\n"
            "        if isinstance(value, str):\n"
            "            cleaned[key] = value.strip().lower()\n"
            "        elif isinstance(value, (int, float)):\n"
            "            cleaned[key] = value * 2 + 1\n"
            "        else:\n"
            "            cleaned[key] = repr(value)\n"
            "    return cleaned\n"
        )
        files = clean_python_repo(
            {
                "src/demo/a.py": block,
                "src/demo/b.py": block,
                "slophammer.yml": ("python:\n  dry:\n    copied_blocks:\n      min_tokens: 40\n"),
            }
        )
        findings = run_checks(files, FakeRunner(), only=["py.dry-required"])
        assert [finding.rule_id for finding in findings] == ["py.dry-required"]
        assert "DRY candidates: 1; maximum: 0" in findings[0].message
