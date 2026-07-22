"""Binding evidence tests, ported from typescript/tests for behavior parity."""

from slophammer.repo import (
    RepoFile,
    binding_workflow_triggers,
    command_files,
    new_snapshot,
)


def snapshot(files: dict[str, str]):
    return new_snapshot("/repo", [RepoFile(path, content) for path, content in files.items()])


def evidence_text(files: dict[str, str]) -> str:
    return "\n".join(file.content for file in command_files(snapshot(files)))


def workflow(trigger: str, steps: str) -> str:
    return f"name: CI\n{trigger}\njobs:\n  check:\n    steps:\n{steps}"


class TestBindingTriggers:
    def test_push_and_pull_request_bind(self):
        assert binding_workflow_triggers("push")
        assert binding_workflow_triggers(["pull_request"])
        assert binding_workflow_triggers({"schedule": [{"cron": "0 0 * * 1"}]})

    def test_workflow_dispatch_does_not_bind(self):
        assert not binding_workflow_triggers("workflow_dispatch")
        assert not binding_workflow_triggers(["workflow_dispatch"])

    def test_tag_only_push_does_not_bind(self):
        assert not binding_workflow_triggers({"push": {"tags": ["v*"]}})
        assert binding_workflow_triggers({"push": {"tags": ["v*"], "branches": ["main"]}})

    def test_tags_ignore_only_does_not_bind(self):
        assert not binding_workflow_triggers({"push": {"tags-ignore": ["v0*"]}})

    def test_branches_ignore_still_binds(self):
        assert binding_workflow_triggers({"push": {"branches-ignore": ["wip/*"]}})
        assert binding_workflow_triggers({"push": {"tags": ["v*"], "branches-ignore": ["wip/*"]}})

    def test_push_branch_filters(self):
        assert binding_workflow_triggers({"push": {"branches": ["main"]}})
        assert binding_workflow_triggers({"push": {"branches": ["release/*"]}})
        assert not binding_workflow_triggers({"push": {"branches": ["never-built"]}})
        assert binding_workflow_triggers({"push": None})


class TestNeutralization:
    def test_neutralized_steps_are_dropped(self):
        text = evidence_text(
            {
                ".github/workflows/ci.yml": workflow(
                    "on: [push]",
                    "      - run: pytest\n"
                    "        continue-on-error: true\n"
                    "      - run: ruff check .\n",
                )
            }
        )
        assert "pytest" not in text
        assert "ruff check ." in text

    def test_expression_literals_neutralize(self):
        text = evidence_text(
            {
                ".github/workflows/ci.yml": workflow(
                    "on: [push]",
                    "      - run: pytest\n"
                    "        continue-on-error: ${{ true }}\n"
                    "      - run: mypy src\n"
                    "        if: ${{ false }}\n"
                    "      - run: ruff check .\n",
                )
            }
        )
        assert "pytest" not in text
        assert "mypy src" not in text
        assert "ruff check ." in text

    def test_non_literal_expressions_stay_credited(self):
        text = evidence_text(
            {
                ".github/workflows/ci.yml": workflow(
                    "on: [push]",
                    "      - run: pytest\n        continue-on-error: ${{ matrix.experimental }}\n",
                )
            }
        )
        assert "pytest" in text

    def test_neutralized_jobs_drop_all_steps(self):
        content = (
            "on: [push]\n"
            "jobs:\n"
            "  skipped:\n"
            "    if: false\n"
            "    steps:\n"
            "      - run: pytest\n"
            "  live:\n"
            "    steps:\n"
            "      - run: ruff check .\n"
        )
        text = evidence_text({".github/workflows/ci.yml": content})
        assert "pytest" not in text
        assert "ruff check ." in text

    def test_non_binding_triggers_contribute_nothing(self):
        text = evidence_text(
            {
                ".github/workflows/ci.yml": workflow(
                    "on:\n  push:\n    branches: [never-built]", "      - run: pytest\n"
                )
            }
        )
        assert "pytest" not in text


class TestWorkflowShapes:
    def test_uses_steps_contribute_action_references(self):
        text = evidence_text(
            {
                ".github/workflows/ci.yml": workflow(
                    "on: [push]",
                    "      - uses: osolmaz/slophammer@v0.3.0\n",
                )
            }
        )
        assert "uses: osolmaz/slophammer@v0.3.0" in text

    def test_matrix_commands_expand(self):
        content = (
            "on: [push]\n"
            "jobs:\n"
            "  ci:\n"
            "    strategy:\n"
            "      matrix:\n"
            "        command: [pytest]\n"
            "        include:\n"
            "          - command: ruff check .\n"
            "    steps:\n"
            "      - run: ${{ matrix.command }}\n"
        )
        text = evidence_text({".github/workflows/ci.yml": content})
        assert "pytest" in text
        assert "ruff check ." in text
        assert "matrix.command" not in text

    def test_unparseable_workflows_fall_back_to_run_lines(self):
        content = "on: [push\njobs:\n  ci:\n    steps:\n      - run: pytest\n"
        text = evidence_text({".github/workflows/ci.yml": content})
        assert "pytest" in text

    def test_block_scalar_runs_are_collected(self):
        content = (
            "on: [push]\n"
            "jobs:\n"
            "  ci:\n"
            "    steps:\n"
            "      - run: |\n"
            "          pytest\n"
            "          ruff check .\n"
        )
        text = evidence_text({".github/workflows/ci.yml": content})
        assert "pytest" in text
        assert "ruff check ." in text


class TestReachability:
    def test_unreferenced_scripts_are_not_evidence(self):
        text = evidence_text(
            {
                ".github/workflows/ci.yml": workflow("on: [push]", "      - run: pytest\n"),
                "scripts/hidden.sh": "mutmut run\n",
            }
        )
        assert "mutmut" not in text

    def test_referenced_scripts_are_evidence_one_hop_deep(self):
        text = evidence_text(
            {
                ".github/workflows/ci.yml": workflow(
                    "on: [push]", "      - run: ./scripts/gate.sh\n"
                ),
                "scripts/gate.sh": "pytest\n./scripts/audit.sh\n",
                "scripts/audit.sh": "pip-audit\n",
            }
        )
        assert "pytest" in text
        assert "pip-audit" in text

    def test_makefiles_need_a_make_invocation(self):
        unreferenced = evidence_text(
            {
                ".github/workflows/ci.yml": workflow("on: [push]", "      - run: pytest\n"),
                "Makefile": "check:\n\tmutmut run\n",
            }
        )
        assert "mutmut" not in unreferenced
        referenced = evidence_text(
            {
                ".github/workflows/ci.yml": workflow("on: [push]", "      - run: make check\n"),
                "Makefile": "check:\n\tmutmut run\n",
            }
        )
        assert "mutmut" in referenced

    def test_package_scripts_credited_only_when_invoked(self):
        files = {
            ".github/workflows/ci.yml": workflow("on: [push]", "      - run: npm run gate\n"),
            "package.json": '{"scripts": {"gate": "pytest", "hidden": "mutmut run"}}',
        }
        text = evidence_text(files)
        assert "pytest" in text
        assert "mutmut" not in text

    def test_synthetic_repo_evidence_is_credited(self):
        text = evidence_text({"scripts/__repo_workflow.sh": "pytest\n"})
        assert "pytest" in text

    def test_fixture_and_template_evidence_is_ignored(self):
        text = evidence_text(
            {
                "fixtures/repos/x/.github/workflows/ci.yml": workflow(
                    "on: [push]", "      - run: pytest\n"
                )
            }
        )
        assert "pytest" not in text
