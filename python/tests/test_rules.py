"""Rule evaluation tests over synthetic repositories."""

from slophammer.config import Config, load_config, parse_config
from slophammer.repo import RepoFile, new_snapshot
from slophammer.rule_policy import RulePolicy
from slophammer.rules import explain, run_rules

GATE_STEPS = """\
      - run: uv run ty check src --error-on-warning
      - run: uv run ruff check .
      - run: uv run ruff format --check .
      - run: uv run pytest --cov=src --cov-fail-under=85
      - run: uv run slophammer-py dry .
      - run: uv run mutmut run
      - run: uv run python scripts/check-mutation.py --min-kill-rate 74
      - run: uv run pip-audit
      - run: uvx slophammer-py check .
"""

STRICT_TY_TOML = """\
[terminal]
error-on-warning = true

[analysis]
respect-type-ignore-comments = false

[rules]
missing-type-argument = "error"
possibly-missing-attribute = "error"
possibly-unresolved-reference = "error"
possibly-missing-import = "error"
"""

STRICT_PYPROJECT = """\
[project]
name = "demo"
version = "0.0.0"

[tool.ruff.lint]
select = ["E", "F", "ANN", "C90"]

[tool.ruff.lint.mccabe]
max-complexity = 8
"""


def clean_python_repo(overrides: dict[str, str] | None = None) -> dict[str, str]:
    files = {
        "README.md": "# Demo\n",
        "AGENTS.md": "# Agents\n",
        ".github/workflows/ci.yml": (
            f"name: CI\non: [push]\njobs:\n  check:\n    steps:\n{GATE_STEPS}"
        ),
        "pyproject.toml": STRICT_PYPROJECT,
        "ty.toml": STRICT_TY_TOML,
        "src/demo/__init__.py": "",
        "src/demo/main.py": "def run() -> int:\n    return 0\n",
        "slophammer.yml": "python:\n  coverage:\n    threshold: 85\n",
    }
    files.update(overrides or {})
    return files


def report_for(files: dict[str, str], only: list[str] | None = None):
    snapshot = new_snapshot("/repo", [RepoFile(path, content) for path, content in files.items()])
    config = load_config(snapshot)
    return run_rules(snapshot, config, RulePolicy.for_check(config, only))


def rule_ids(report) -> list[str]:
    return [finding.rule_id for finding in report.findings]


class TestCleanRepo:
    def test_clean_python_repo_passes(self):
        report = report_for(clean_python_repo())
        assert report.findings == []
        assert report.ok

    def test_conventional_pyprojects_are_not_python_projects(self):
        report = report_for(
            {
                "README.md": "# Demo\n",
                "AGENTS.md": "# Agents\n",
                ".github/workflows/ci.yml": (
                    "name: CI\non: [push]\njobs:\n  c:\n    steps:\n      - run: true\n"
                ),
                "tests/pyproject.toml": '[project]\nname = "fixture"\nversion = "0"\n',
                "scripts/pyproject.toml": '[project]\nname = "tooling"\nversion = "0"\n',
            }
        )
        assert report.findings == []

    def test_repo_without_python_skips_python_rules(self):
        report = report_for(
            {
                "README.md": "# Demo\n",
                "AGENTS.md": "# Agents\n",
                ".github/workflows/ci.yml": (
                    "name: CI\non: [push]\njobs:\n  c:\n    steps:\n      - run: true\n"
                ),
            }
        )
        assert report.findings == []


class TestRepoRules:
    def test_missing_readme_and_agents(self):
        files = clean_python_repo()
        del files["README.md"]
        del files["AGENTS.md"]
        ids = rule_ids(report_for(files))
        assert "repo.readme-required" in ids
        assert "repo.agents-required" in ids

    def test_missing_ci(self):
        files = clean_python_repo()
        del files[".github/workflows/ci.yml"]
        ids = rule_ids(report_for(files))
        assert "repo.ci-required" in ids

    def test_unenforced_config(self):
        files = clean_python_repo(
            {
                ".github/workflows/ci.yml": (
                    "name: CI\non: [push]\njobs:\n  check:\n    steps:\n"
                    "      - run: uv run pytest\n"
                )
            }
        )
        ids = rule_ids(report_for(files, only=["repo.slophammer-ci-required"]))
        assert ids == ["repo.slophammer-ci-required"]

    def test_action_reference_satisfies_slophammer_ci(self):
        files = clean_python_repo(
            {
                ".github/workflows/ci.yml": (
                    "name: CI\non: [push]\njobs:\n  check:\n    steps:\n"
                    "      - uses: osolmaz/slophammer@v0.3.0\n"
                )
            }
        )
        ids = rule_ids(report_for(files, only=["repo.slophammer-ci-required"]))
        assert ids == []

    def test_bare_slophammer_satisfies_slophammer_ci(self):
        files = clean_python_repo(
            {
                ".github/workflows/ci.yml": (
                    "name: CI\non: [push]\njobs:\n  check:\n    steps:\n"
                    "      - run: uvx slophammer check .\n"
                )
            }
        )
        ids = rule_ids(report_for(files, only=["repo.slophammer-ci-required"]))
        assert ids == []


class TestGateRules:
    def test_disabled_mutation_rule_emits_no_static_finding(self):
        steps = "\n".join(line for line in GATE_STEPS.split("\n") if "min-kill-rate" not in line)
        files = clean_python_repo(
            {
                ".github/workflows/ci.yml": (
                    f"name: CI\non: [push]\njobs:\n  check:\n    steps:\n{steps}\n"
                ),
                "slophammer.yml": (
                    "rules:\n"
                    "  py.mutation-required:\n"
                    "    disabled: true\n"
                    "    reason: checked by another required job\n"
                    "python:\n  coverage:\n    threshold: 85\n"
                ),
            }
        )

        assert rule_ids(report_for(files, only=["py.mutation-required"])) == []

    def test_each_missing_gate_fires_its_rule(self):
        cases = {
            "ty check": "py.typecheck-required",
            "ruff check .": "py.lint-required",
            "ruff format --check .": "py.format-required",
            "pytest --cov=src --cov-fail-under=85": "py.coverage-required",
            "slophammer-py dry .": "py.dry-required",
            "min-kill-rate": "py.mutation-required",
            "pip-audit": "py.dependency-audit-required",
        }
        for needle, rule in cases.items():
            steps = "\n".join(line for line in GATE_STEPS.split("\n") if needle not in line)
            files = clean_python_repo(
                {
                    ".github/workflows/ci.yml": (
                        f"name: CI\non: [push]\njobs:\n  check:\n    steps:\n{steps}\n"
                    )
                }
            )
            ids = rule_ids(report_for(files, only=[rule]))
            assert ids == [rule], f"{rule} should fire when {needle!r} is removed"

    def test_install_steps_are_not_tool_evidence(self):
        files = clean_python_repo(
            {
                ".github/workflows/ci.yml": (
                    "name: CI\non: [push]\njobs:\n  check:\n    steps:\n"
                    "      - run: pip install pytest mypy mutmut pip-audit ruff\n"
                    "      - run: uv pip install pytest-cov\n"
                )
            }
        )
        report = report_for(
            files,
            only=[
                "py.test-required",
                "py.typecheck-required",
                "py.mutation-required",
                "py.dependency-audit-required",
            ],
        )
        assert sorted(rule_ids(report)) == [
            "py.dependency-audit-required",
            "py.mutation-required",
            "py.test-required",
            "py.typecheck-required",
        ]

    def test_bare_slophammer_satisfies_dry_gate(self):
        steps = GATE_STEPS.replace("uv run slophammer-py dry .", "uvx slophammer dry .")
        files = clean_python_repo(
            {
                ".github/workflows/ci.yml": (
                    f"name: CI\non: [push]\njobs:\n  check:\n    steps:\n{steps}\n"
                )
            }
        )
        ids = rule_ids(report_for(files, only=["py.dry-required"]))
        assert ids == []

    def test_lowered_coverage_flag_fails_the_gate(self):
        files = clean_python_repo(
            {
                "pyproject.toml": STRICT_PYPROJECT + "\n[tool.coverage.report]\nfail_under = 85\n",
            }
        )
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "--cov-fail-under=85", "--cov-fail-under=50"
        )
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == ["py.coverage-required"]

    def test_env_prefixed_commands_are_evidence(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest", "CI=1 uv run pytest"
        )
        ids = rule_ids(report_for(files, only=["py.test-required", "py.coverage-required"]))
        assert ids == []

    def test_coverage_run_pytest_satisfies_tests(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest",
        )
        ids = rule_ids(report_for(files, only=["py.test-required"]))
        assert ids == []

    def test_bare_coverage_run_is_not_a_gate(self):
        files = clean_python_repo(
            {
                "pyproject.toml": STRICT_PYPROJECT + "\n[tool.coverage.report]\nfail_under = 85\n",
            }
        )
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest",
        )
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == ["py.coverage-required"]

    def test_coverage_report_with_config_threshold_is_a_gate(self):
        files = clean_python_repo(
            {
                "pyproject.toml": STRICT_PYPROJECT + "\n[tool.coverage.report]\nfail_under = 85\n",
            }
        )
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest\n      - run: uv run coverage report",
        )
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == []

    def test_detached_fail_under_flag_is_not_a_gate(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "echo --cov-fail-under=85\n      - run: uv run pytest",
        )
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == ["py.coverage-required"]

    def test_fail_under_without_cov_collection_is_not_a_gate(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run pytest --cov-fail-under=85",
        )
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == ["py.coverage-required"]

    def test_commented_coveragerc_fail_under_is_not_a_gate(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest\n      - run: uv run coverage report",
        )
        files[".coveragerc"] = "[report]\n# fail_under = 85\n"
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == ["py.coverage-required"]

    def test_weakened_coverage_report_flag_fails(self):
        files = clean_python_repo(
            {
                "pyproject.toml": STRICT_PYPROJECT + "\n[tool.coverage.report]\nfail_under = 85\n",
            }
        )
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest\n      - run: uv run coverage report --fail-under=50",
        )
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == ["py.coverage-required"]

    def test_coverage_report_flag_at_threshold_is_a_gate(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest\n      - run: uv run coverage report --fail-under=85",
        )
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == []

    def test_non_numeric_coveragerc_fail_under_is_not_a_gate(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest\n      - run: uv run coverage report",
        )
        files[".coveragerc"] = "[report]\nfail_under = lots\n"
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == ["py.coverage-required"]

    def test_active_coveragerc_fail_under_is_a_gate(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run coverage run -m pytest\n      - run: uv run coverage report",
        )
        files[".coveragerc"] = "[report]\nfail_under = 85\n"
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == []

    def test_cov_report_flag_is_not_collection(self):
        files = clean_python_repo(
            {
                "pyproject.toml": STRICT_PYPROJECT + "\n[tool.coverage.report]\nfail_under = 85\n",
            }
        )
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pytest --cov=src --cov-fail-under=85",
            "uv run pytest --cov-report=xml",
        )
        ids = rule_ids(report_for(files, only=["py.coverage-required"]))
        assert ids == ["py.coverage-required"]

    def test_cannot_fail_mutation_forms_are_not_evidence(self):
        weak_forms = (
            "uv run mutmut run",
            "uv run mutmut results",
            "uv run mutmut run --dry-run",
            "uv run cosmic-ray init config.toml session.sqlite",
            "uv run cosmic-ray report session.sqlite",
            "uv run cosmic-ray exec config.toml session.sqlite",
        )
        for weak in weak_forms:
            files = clean_python_repo()
            files[".github/workflows/ci.yml"] = (
                files[".github/workflows/ci.yml"]
                .replace("uv run mutmut run", weak)
                .replace(
                    "      - run: uv run python scripts/check-mutation.py --min-kill-rate 74\n",
                    "",
                )
            )
            ids = rule_ids(report_for(files, only=["py.mutation-required"]))
            assert ids == ["py.mutation-required"], weak

    def test_unanchored_or_zero_kill_rate_flags_are_not_evidence(self):
        weak_forms = (
            "echo --min-kill-rate 74",
            "echo mutation --min-kill-rate 74",
            "echo check-mutation.py --min-kill-rate 74",
            "uv run python scripts/check-mutation.py --min-kill-rate 0",
        )
        for weak in weak_forms:
            files = clean_python_repo()
            files[".github/workflows/ci.yml"] = (
                files[".github/workflows/ci.yml"]
                .replace("      - run: uv run mutmut run\n", "")
                .replace("uv run python scripts/check-mutation.py --min-kill-rate 74", weak)
            )
            ids = rule_ids(report_for(files, only=["py.mutation-required"]))
            assert ids == ["py.mutation-required"], weak

    def test_stats_file_gate_requires_an_executing_run(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = (
            files[".github/workflows/ci.yml"]
            .replace("      - run: uv run mutmut run\n", "")
            .replace(
                "uv run python scripts/check-mutation.py --min-kill-rate 74",
                "uv run python scripts/check-mutation.py --stats-file /tmp/mutmut.log"
                " --min-kill-rate 74",
            )
        )
        ids = rule_ids(report_for(files, only=["py.mutation-required"]))
        assert ids == ["py.mutation-required"]

    def test_stats_file_gate_beside_executing_run_counts(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run python scripts/check-mutation.py --min-kill-rate 74",
            "uv run python scripts/check-mutation.py --stats-file /tmp/mutmut.log"
            " --min-kill-rate 74",
        )
        ids = rule_ids(report_for(files, only=["py.mutation-required"]))
        assert ids == []

    def test_kill_rate_gate_alone_counts(self):
        # check-mutation.py runs mutmut itself when no stats file is given,
        # so the gate line is complete evidence on its own.
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "      - run: uv run mutmut run\n", ""
        )
        ids = rule_ids(report_for(files, only=["py.mutation-required"]))
        assert ids == []

    def test_gated_cosmic_ray_counts(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = (
            files[".github/workflows/ci.yml"]
            .replace("uv run mutmut run", "uv run cosmic-ray exec config.toml session.sqlite")
            .replace(
                "uv run python scripts/check-mutation.py --min-kill-rate 74",
                "uv run cr-rate --fail-over 74 session.sqlite",
            )
        )
        ids = rule_ids(report_for(files, only=["py.mutation-required"]))
        assert ids == []

    def test_cosmic_ray_fail_over_at_hundred_is_not_evidence(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = (
            files[".github/workflows/ci.yml"]
            .replace("uv run mutmut run", "uv run cosmic-ray exec config.toml session.sqlite")
            .replace(
                "uv run python scripts/check-mutation.py --min-kill-rate 74",
                "uv run cr-rate --fail-over 100 session.sqlite",
            )
        )
        ids = rule_ids(report_for(files, only=["py.mutation-required"]))
        assert ids == ["py.mutation-required"]

    def test_cosmic_ray_exec_without_rate_gate_is_not_evidence(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = (
            files[".github/workflows/ci.yml"]
            .replace("uv run mutmut run", "uv run cosmic-ray exec config.toml session.sqlite")
            .replace(
                "      - run: uv run python scripts/check-mutation.py --min-kill-rate 74\n",
                "",
            )
        )
        ids = rule_ids(report_for(files, only=["py.mutation-required"]))
        assert ids == ["py.mutation-required"]

    def test_module_spelling_pip_audit_counts(self):
        files = clean_python_repo()
        files[".github/workflows/ci.yml"] = files[".github/workflows/ci.yml"].replace(
            "uv run pip-audit", "python -m pip_audit"
        )
        ids = rule_ids(report_for(files, only=["py.dependency-audit-required"]))
        assert ids == []

    def test_pytest_without_coverage_still_satisfies_tests(self):
        files = clean_python_repo()
        ids = rule_ids(report_for(files, only=["py.test-required"]))
        assert ids == []

    def test_missing_pyproject_fires_project_rule(self):
        files = clean_python_repo()
        del files["pyproject.toml"]
        ids = rule_ids(report_for(files, only=["py.project-required"]))
        assert ids == ["py.project-required"]

    def test_exact_c901_selection_counts(self):
        files = clean_python_repo(
            {
                "pyproject.toml": STRICT_PYPROJECT.replace(
                    'select = ["E", "F", "ANN", "C90"]', 'select = ["E", "F", "ANN", "C901"]'
                )
            }
        )
        ids = rule_ids(report_for(files, only=["py.complexity-required"]))
        assert ids == []

    def test_complexity_via_xenon_command(self):
        files = clean_python_repo({"pyproject.toml": '[project]\nname = "demo"\nversion = "0"\n'})
        files[".github/workflows/ci.yml"] += "      - run: uv run xenon --max-absolute B src\n"
        ids = rule_ids(report_for(files, only=["py.complexity-required"]))
        assert ids == []

    def test_permissive_xenon_threshold_is_not_a_gate(self):
        files = clean_python_repo({"pyproject.toml": '[project]\nname = "demo"\nversion = "0"\n'})
        files[".github/workflows/ci.yml"] += "      - run: uv run xenon --max-absolute F src\n"
        ids = rule_ids(report_for(files, only=["py.complexity-required"]))
        assert ids == ["py.complexity-required"]

    def test_report_only_radon_is_not_a_gate(self):
        files = clean_python_repo({"pyproject.toml": '[project]\nname = "demo"\nversion = "0"\n'})
        files[".github/workflows/ci.yml"] += "      - run: uv run radon cc src\n"
        ids = rule_ids(report_for(files, only=["py.complexity-required"]))
        assert ids == ["py.complexity-required"]

    def test_complexity_missing_when_no_mccabe(self):
        files = clean_python_repo({"pyproject.toml": '[project]\nname = "demo"\nversion = "0"\n'})
        ids = rule_ids(report_for(files, only=["py.complexity-required"]))
        assert ids == ["py.complexity-required"]


class TestTypedMarker:
    def test_published_package_needs_marker(self):
        files = clean_python_repo(
            {
                "pyproject.toml": STRICT_PYPROJECT
                + '\n[build-system]\nrequires = ["hatchling"]\nbuild-backend = "hatchling.build"\n'
            }
        )
        ids = rule_ids(report_for(files, only=["py.typed-marker-required"]))
        assert ids == ["py.typed-marker-required"]
        files["src/demo/py.typed"] = ""
        ids = rule_ids(report_for(files, only=["py.typed-marker-required"]))
        assert ids == []

    def test_applications_are_exempt(self):
        ids = rule_ids(report_for(clean_python_repo(), only=["py.typed-marker-required"]))
        assert ids == []

    def test_marker_under_tests_does_not_count(self):
        build_system = (
            '[build-system]\nrequires = ["hatchling"]\nbuild-backend = "hatchling.build"\n'
        )
        files = clean_python_repo({"pyproject.toml": STRICT_PYPROJECT + "\n" + build_system})
        files["tests/py.typed"] = ""
        ids = rule_ids(report_for(files, only=["py.typed-marker-required"]))
        assert ids == ["py.typed-marker-required"]

    def test_sibling_package_marker_does_not_type_the_other(self):
        build_system = (
            '[build-system]\nrequires = ["hatchling"]\nbuild-backend = "hatchling.build"\n'
        )
        files = clean_python_repo({"pyproject.toml": STRICT_PYPROJECT + "\n" + build_system})
        files["src/bar/__init__.py"] = ""
        files["src/bar/py.typed"] = ""
        ids = rule_ids(report_for(files, only=["py.typed-marker-required"]))
        assert ids == ["py.typed-marker-required"]
        files["src/demo/py.typed"] = ""
        ids = rule_ids(report_for(files, only=["py.typed-marker-required"]))
        assert ids == []

    def test_each_package_needs_its_own_marker(self):
        packaged = (
            '[project]\nname = "pkg"\nversion = "0"\n'
            '[build-system]\nrequires = ["hatchling"]\nbuild-backend = "hatchling.build"\n'
        )
        files = clean_python_repo(
            {
                "packages/one/pyproject.toml": packaged,
                "packages/one/src/one/__init__.py": "",
                "packages/one/src/one/py.typed": "",
                "packages/two/pyproject.toml": packaged,
                "packages/two/src/two/__init__.py": "",
            }
        )
        report = report_for(files, only=["py.typed-marker-required"])
        assert [finding.path for finding in report.findings] == ["packages/two/pyproject.toml"]


class TestSeverityOverrides:
    def test_rule_severity_override_applies(self):
        files = clean_python_repo(
            {"slophammer.yml": "rules:\n  repo.readme-required:\n    severity: warn\n"}
        )
        del files["README.md"]
        report = report_for(files, only=["repo.readme-required"])
        assert report.findings[0].severity == "warn"


class TestExplain:
    def test_explains_known_rules(self):
        text = explain("py.types-strict-required")
        assert text is not None
        assert "py.types-strict-required" in text
        assert explain("no.such-rule") is None


class TestScopeCoverage:
    def test_report_carries_scope_counts(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n  coverage:\n    threshold: 85\n    paths:\n      - src\n"
                ),
            }
        )
        report = report_for(files)
        assert report.scope is not None
        assert report.scope.scanned == report.scope.production_files

    def test_carved_scope_fires_scope_incomplete(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n  coverage:\n    threshold: 85\n    paths:\n      - src/demo\n"
                ),
                "corner/extra.py": "def hidden() -> int:\n    return 1\n",
            }
        )
        ids = rule_ids(report_for(files, only=["py.scope-incomplete"]))
        assert ids == ["py.scope-incomplete"]

    def test_single_star_does_not_cross_directories(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n"
                    "  coverage:\n"
                    "    threshold: 85\n"
                    "    paths:\n"
                    "      - src/demo\n"
                    "    exclude:\n"
                    "      - pattern: corner/*.py\n"
                    "        reason: top-level corner prototypes only\n"
                ),
                "corner/top.py": "def top() -> int:\n    return 1\n",
                "corner/nested/hidden.py": "def hidden() -> int:\n    return 1\n",
            }
        )
        report = report_for(files, only=["py.scope-incomplete"])
        assert len(report.findings) == 1
        assert "corner/nested" in report.findings[0].message

    def test_reasoned_exclude_covers_carved_scope(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n"
                    "  coverage:\n"
                    "    threshold: 85\n"
                    "    paths:\n"
                    "      - src/demo\n"
                    "    exclude:\n"
                    "      - pattern: corner/**\n"
                    "        reason: prototype corner kept out of every gate\n"
                ),
                "corner/extra.py": "def hidden() -> int:\n    return 1\n",
            }
        )
        ids = rule_ids(report_for(files, only=["py.scope-incomplete"]))
        assert ids == []


class TestBoundaries:
    def test_boundary_violation_is_a_finding(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n  dependency_boundaries:\n    - from: src/demo\n      allow: []\n"
                ),
                "src/demo/main.py": "from src.helpers import util\n",
                "src/helpers/__init__.py": "",
                "src/helpers/util.py": "VALUE = 1\n",
            }
        )
        ids = rule_ids(report_for(files, only=["py.dependency-boundaries-required"]))
        assert ids == ["py.dependency-boundaries-required"]

    def test_allowed_imports_pass(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n"
                    "  dependency_boundaries:\n"
                    "    - from: src/demo\n"
                    "      allow:\n"
                    "        - src/helpers\n"
                ),
                "src/demo/main.py": "from src.helpers import util\n",
                "src/helpers/__init__.py": "",
                "src/helpers/util.py": "VALUE = 1\n",
            }
        )
        ids = rule_ids(report_for(files, only=["py.dependency-boundaries-required"]))
        assert ids == []

    def test_alias_only_relative_imports_resolve_to_modules(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n"
                    "  dependency_boundaries:\n"
                    "    - from: src/demo\n"
                    "      allow:\n"
                    "        - src/config\n"
                ),
                "src/__init__.py": "",
                "src/demo/main.py": "from .. import config\n",
                "src/config/__init__.py": "",
            }
        )
        ids = rule_ids(report_for(files, only=["py.dependency-boundaries-required"]))
        assert ids == []

    def test_from_import_alias_resolving_to_allowed_submodule(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n"
                    "  dependency_boundaries:\n"
                    "    - from: src/demo/feature\n"
                    "      allow:\n"
                    "        - src/demo/config\n"
                ),
                "src/demo/feature/__init__.py": "",
                "src/demo/feature/x.py": "from demo import config\n",
                "src/demo/config/__init__.py": "",
            }
        )
        ids = rule_ids(report_for(files, only=["py.dependency-boundaries-required"]))
        assert ids == []

    def test_third_party_imports_are_ignored(self):
        files = clean_python_repo(
            {
                "slophammer.yml": (
                    "python:\n  dependency_boundaries:\n    - from: src/demo\n      allow: []\n"
                ),
                "src/demo/main.py": "import json\nimport yaml\n",
            }
        )
        ids = rule_ids(report_for(files, only=["py.dependency-boundaries-required"]))
        assert ids == []


def test_invalid_rule_severity_is_a_config_error():
    import pytest

    from slophammer.config import ConfigError

    with pytest.raises(ConfigError, match="severity must be error or warn"):
        parse_config("rules:\n  repo.readme-required:\n    severity: warning\n")


def test_parse_config_rejects_unknown_keys():
    import pytest

    from slophammer.config import ConfigError

    with pytest.raises(ConfigError, match=r"python\.made_up is not supported"):
        parse_config("python:\n  made_up: true\n")


def test_default_config_when_missing():
    snapshot = new_snapshot("/repo", [])
    assert load_config(snapshot) == Config()
