"""AGENTS.md evidence, findings, and CLI tests."""

from pathlib import Path

from slophammer.agents import agents_findings, derive_evidence, render_agents
from slophammer.cli import main
from slophammer.repo import RepoFile, new_snapshot


def snapshot(files: dict[str, str]):
    return new_snapshot("/repo", [RepoFile(path, content) for path, content in files.items()])


def test_derives_runner_and_package_commands():
    evidence = derive_evidence(
        snapshot(
            {
                "Makefile": "check:\n\t@true\n",
                "go/go.mod": "module example.com/demo\n",
                "rust/Cargo.toml": "[workspace]\nmembers = []\n",
                "python/pyproject.toml": '[dependency-groups]\ndev = ["pytest"]\n',
                "python/uv.lock": "version = 1\n",
                "web/package.json": '{"scripts":{"check":"true"}}',
                "web/pnpm-lock.yaml": "lockfileVersion: 9\n",
                "fixtures/demo/go.mod": "module example.com/ignored\n",
            }
        )
    )
    assert evidence.global_commands == ("make check",)
    for command in ("go test ./...", "cargo test --workspace", "uv run pytest", "pnpm run check"):
        assert command in evidence.all_commands
    assert "fixtures/demo" not in {package.path for package in evidence.packages}
    assert "No package manifest was detected" in render_agents(snapshot({}))


def test_uses_workspace_package_managers():
    lock_evidence = derive_evidence(
        snapshot(
            {
                "pnpm-lock.yaml": "lockfileVersion: 9\n",
                "packages/app/package.json": '{"scripts":{"check":"true"}}',
            }
        )
    )
    assert "pnpm run check" in lock_evidence.all_commands

    metadata_evidence = derive_evidence(
        snapshot(
            {
                "package.json": '{"packageManager":"yarn@4.1.0"}',
                "packages/app/package.json": '{"scripts":{"build":"true"}}',
            }
        )
    )
    assert "yarn run build" in metadata_evidence.all_commands


def test_reports_agent_instruction_failures():
    assert [item.rule_id for item in agents_findings(snapshot({}))] == ["repo.agents-required"]
    assert [item.rule_id for item in agents_findings(snapshot({"AGENTS.md": "# Agents\n"}))] == [
        "repo.agents-empty"
    ]
    missing_command = agents_findings(
        snapshot(
            {
                "AGENTS.md": "# Agents\n\nKeep changes small and reviewable.\n",
                "package.json": '{"scripts":{"check":"true"}}',
            }
        )
    )
    assert [item.rule_id for item in missing_command] == ["repo.agents-commands-required"]
    missing_scope = agents_findings(
        snapshot(
            {
                "AGENTS.md": "# Agents\n\nRun `npm test` before finishing.\n",
                "packages/app/package.json": '{"scripts":{"test":"true"}}',
                "packages/worker/package.json": '{"scripts":{"build":"true"}}',
            }
        )
    )
    assert any(
        item.rule_id == "repo.agents-scope-required" and item.path == "packages/worker/AGENTS.md"
        for item in missing_scope
    )

    stale = """\
# AGENTS.md

Keep changes small and reviewable.

<!-- slophammer:agents:start -->
## Repository checks

```sh
npm test
```
<!-- slophammer:agents:end -->
"""
    assert [item.rule_id for item in agents_findings(snapshot({"AGENTS.md": stale}))] == [
        "repo.agents-command-invalid",
        "repo.agents-stale",
    ]


def test_treats_supported_command_as_useful_content():
    assert (
        agents_findings(
            snapshot(
                {
                    "AGENTS.md": "```sh\ngo test ./...\n```\n",
                    "go.mod": "module example.com/demo\n",
                }
            )
        )
        == []
    )


def test_accepts_generated_evidence():
    files = {"package.json": '{"scripts":{"check":"true"}}'}
    generated = render_agents(snapshot(files))
    assert agents_findings(snapshot({"AGENTS.md": generated, **files})) == []


def test_agents_cli_rejects_newline_package_path(tmp_path: Path, capsys):
    package_root = tmp_path / "line\nbreak"
    package_root.mkdir()
    (package_root / "go.mod").write_text("module example.com/unsafe\n")

    assert main(["agents", "init", str(tmp_path)]) == 2
    assert "newlines" in capsys.readouterr().err


def test_agents_cli_rejects_forced_symlink_replacement(tmp_path: Path, capsys):
    external = tmp_path.parent / f"{tmp_path.name}-external.md"
    external.write_text("keep\n")
    (tmp_path / "AGENTS.md").symlink_to(external)

    assert main(["agents", "init", str(tmp_path), "--force"]) == 2
    assert "symlink" in capsys.readouterr().err
    assert external.read_text() == "keep\n"


def test_agents_cli_writes_refuses_forces_previews_and_checks(tmp_path: Path, capsys):
    (tmp_path / "Makefile").write_text("check:\n\t@true\n")

    assert main(["agents", "init", str(tmp_path), "--dry-run"]) == 0
    assert "make check" in capsys.readouterr().out
    assert not (tmp_path / "AGENTS.md").exists()

    assert main(["agents", "init", str(tmp_path)]) == 0
    capsys.readouterr()
    assert main(["agents", "init", str(tmp_path)]) == 2
    assert "--force" in capsys.readouterr().err

    (tmp_path / "AGENTS.md").write_text("old\n")
    assert main(["agents", "init", str(tmp_path), "--force"]) == 0
    capsys.readouterr()
    assert "make check" in (tmp_path / "AGENTS.md").read_text()

    assert main(["agents", "check", str(tmp_path), "--format", "json"]) == 0
    assert '"ok": true' in capsys.readouterr().out
