"""Create and check repository AGENTS.md files from repository evidence."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass

from slophammer.core import Finding
from slophammer.repo import RepoFile, Snapshot

START_MARKER = "<!-- slophammer:agents:start -->"
END_MARKER = "<!-- slophammer:agents:end -->"
AGENT_RULE_IDS = (
    "repo.agents-required",
    "repo.agents-empty",
    "repo.agents-commands-required",
    "repo.agents-command-invalid",
    "repo.agents-scope-required",
    "repo.agents-stale",
)
MANIFESTS = ("Cargo.toml", "go.mod", "package.json", "pyproject.toml")


@dataclass(frozen=True)
class PackageArea:
    path: str
    manifests: tuple[str, ...]
    commands: tuple[str, ...]


@dataclass(frozen=True)
class AgentEvidence:
    global_commands: tuple[str, ...]
    packages: tuple[PackageArea, ...]

    @property
    def all_commands(self) -> tuple[str, ...]:
        commands = list(self.global_commands)
        for package in self.packages:
            commands.extend(package.commands)
            commands.extend(command_variant(command) for command in package.commands)
        return tuple(dict.fromkeys(commands))

    @property
    def rendered_commands(self) -> tuple[str, ...]:
        if self.global_commands:
            return self.global_commands
        return tuple(
            dict.fromkeys(command for package in self.packages for command in package.commands)
        )


def derive_evidence(snapshot: Snapshot) -> AgentEvidence:
    global_commands = tuple(root_runner_commands(snapshot))
    by_path: dict[str, dict[str, list[str]]] = {}
    for file in snapshot.files.values():
        name = file.path.rsplit("/", 1)[-1]
        if name not in MANIFESTS or ignored_manifest_path(file.path):
            continue
        package_path = file.path.rsplit("/", 1)[0] if "/" in file.path else "."
        area = by_path.setdefault(package_path, {"manifests": [], "commands": []})
        area["manifests"].append(name)
        area["commands"].extend(manifest_commands(snapshot, file, package_path))
    packages = tuple(
        PackageArea(
            path=package_path,
            manifests=tuple(sorted(set(values["manifests"]))),
            commands=tuple(dict.fromkeys(values["commands"])),
        )
        for package_path, values in sorted(by_path.items())
    )
    return AgentEvidence(global_commands=global_commands, packages=packages)


def ignored_manifest_path(file_path: str) -> bool:
    ignored = {
        "fixtures",
        "templates",
        "testdata",
        "tests",
        "test",
        "scripts",
        "vendor",
        "dist",
        "build",
        "target",
        "node_modules",
        ".venv",
        "coverage",
    }
    directory = file_path.rsplit("/", 1)[0] if "/" in file_path else "."
    return any(
        segment in ignored or (segment.startswith(".") and segment != ".")
        for segment in directory.split("/")
    )


def root_runner_commands(snapshot: Snapshot) -> list[str]:
    commands: list[str] = []
    for name, command, pattern in (
        ("Makefile", "make check", r"(?m)^check\s*:(?:\s|$)"),
        ("makefile", "make check", r"(?m)^check\s*:(?:\s|$)"),
        ("Taskfile.yml", "task check", r"(?m)^\s{0,2}check\s*:(?:\s|$)"),
        ("Taskfile.yaml", "task check", r"(?m)^\s{0,2}check\s*:(?:\s|$)"),
        ("justfile", "just check", r"(?m)^check\s*:(?:\s|$)"),
    ):
        file = snapshot.files.get(name)
        if file is not None and re.search(pattern, file.content):
            commands.append(command)
    return list(dict.fromkeys(commands))


def manifest_commands(snapshot: Snapshot, file: RepoFile, package_path: str) -> list[str]:
    name = file.path.rsplit("/", 1)[-1]
    if name == "go.mod":
        return [scoped_command(package_path, "go test ./...")]
    if name == "Cargo.toml":
        command = (
            "cargo test --workspace"
            if re.search(r"(?m)^\s*\[workspace\]\s*$", file.content)
            else "cargo test"
        )
        return [scoped_command(package_path, command)]
    if name == "package.json":
        return package_json_commands(snapshot, file, package_path)
    if name == "pyproject.toml":
        runner = "python -m compileall ."
        if "pytest" in file.content.lower():
            runner = (
                "uv run pytest"
                if adjacent_file(snapshot, package_path, "uv.lock")
                else "python -m pytest"
            )
        return [scoped_command(package_path, runner)]
    return []


def package_json_commands(snapshot: Snapshot, file: RepoFile, package_path: str) -> list[str]:
    try:
        parsed = json.loads(file.content)
    except json.JSONDecodeError:
        return []
    scripts = parsed.get("scripts") if isinstance(parsed, dict) else None
    if not isinstance(scripts, dict):
        return []
    script = next(
        (name for name in ("check", "test", "build") if isinstance(scripts.get(name), str)), None
    )
    if script is None:
        return []
    manager = package_manager(snapshot, package_path)
    command = (
        f"{manager} {script}"
        if script == "test" and manager in {"npm", "yarn", "bun"}
        else f"{manager} run {script}"
    )
    return [scoped_command(package_path, command)]


def package_manager(snapshot: Snapshot, package_path: str) -> str:
    if adjacent_file(snapshot, package_path, "pnpm-lock.yaml"):
        return "pnpm"
    if adjacent_file(snapshot, package_path, "yarn.lock"):
        return "yarn"
    if adjacent_file(snapshot, package_path, "bun.lock") or adjacent_file(
        snapshot, package_path, "bun.lockb"
    ):
        return "bun"
    return "npm"


def adjacent_file(snapshot: Snapshot, package_path: str, name: str) -> bool:
    path = name if package_path == "." else f"{package_path}/{name}"
    return path in snapshot.files


def command_variant(command: str) -> str:
    return (
        command.split(" && ", 1)[1] if command.startswith('cd "') and " && " in command else command
    )


def scoped_command(package_path: str, command: str) -> str:
    if package_path == ".":
        return command
    escaped = package_path.replace("\\", "\\\\").replace('"', '\\"')
    return f'cd "{escaped}" && {command}'


def render_agents(snapshot: Snapshot) -> str:
    evidence = derive_evidence(snapshot)
    return "\n".join(
        [
            "# AGENTS.md",
            "",
            "These instructions apply to this repository.",
            "",
            render_evidence_block(evidence),
            "",
            "## Working rules",
            "",
            "- Keep changes small and reviewable.",
            "- Add or update tests when behavior changes.",
            "- Run the repository checks before you finish.",
            "- Do not weaken existing checks to make a change pass.",
            "",
        ]
    )


def render_evidence_block(evidence: AgentEvidence) -> str:
    lines = [START_MARKER, "## Repository checks", ""]
    if evidence.rendered_commands:
        lines.extend(
            [
                "Run these commands before you finish:",
                "",
                "```sh",
                *evidence.rendered_commands,
                "```",
            ]
        )
    else:
        lines.append("No verification command could be derived from repository files.")
    lines.extend(["", "## Package areas", ""])
    if evidence.packages:
        for package in evidence.packages:
            manifests = ", ".join(f"`{manifest}`" for manifest in package.manifests)
            lines.append(f"- `{package.path}`: {manifests}")
    else:
        lines.append("- No package manifest was detected.")
    lines.append(END_MARKER)
    return "\n".join(lines)


def agents_findings(snapshot: Snapshot) -> list[Finding]:
    root_file = root_agents_file(snapshot)
    if root_file is None:
        return [agent_finding("repo.agents-required", "AGENTS.md", "AGENTS.md is required")]
    evidence = derive_evidence(snapshot)
    findings: list[Finding] = []
    if not useful_content(root_file.content):
        findings.append(
            agent_finding(
                "repo.agents-empty",
                "AGENTS.md",
                "AGENTS.md must contain useful repository instructions",
            )
        )
    if evidence.all_commands and not contains_any_command(root_file.content, evidence.all_commands):
        findings.append(
            agent_finding(
                "repo.agents-commands-required",
                "AGENTS.md",
                "AGENTS.md must name a verification command supported by the repository",
            )
        )
    managed = managed_block(root_file.content)
    if managed is not None:
        invalid = [
            command for command in managed_commands(managed) if command not in evidence.all_commands
        ]
        if invalid:
            findings.append(
                agent_finding(
                    "repo.agents-command-invalid",
                    "AGENTS.md",
                    "AGENTS.md contains generated commands without repository evidence: "
                    + ", ".join(invalid),
                )
            )
        if managed != render_evidence_block(evidence):
            findings.append(
                agent_finding(
                    "repo.agents-stale",
                    "AGENTS.md",
                    "The Slophammer AGENTS.md evidence block is stale",
                )
            )
    findings.extend(scope_findings(snapshot, evidence))
    return findings


def root_agents_file(snapshot: Snapshot) -> RepoFile | None:
    return next(
        (
            file
            for path, file in snapshot.files.items()
            if "/" not in path and path.lower() == "agents.md"
        ),
        None,
    )


def useful_content(content: str) -> bool:
    without_comments = re.sub(r"<!--.*?-->", "", content, flags=re.DOTALL)
    kept = [
        line
        for line in without_comments.splitlines()
        if line.strip() and not line.lstrip().startswith("#") and line.strip() != "```"
    ]
    return len(re.sub(r"[^A-Za-z0-9]+", "", " ".join(kept))) >= 20


def contains_any_command(content: str, commands: tuple[str, ...]) -> bool:
    lines = {line.strip() for line in content.splitlines()}
    return any(f"`{command}`" in content or command in lines for command in commands)


def managed_block(content: str) -> str | None:
    start = content.find(START_MARKER)
    if start < 0:
        return None
    end = content.find(END_MARKER, start)
    if end < 0:
        return content[start:].strip()
    return content[start : end + len(END_MARKER)].strip()


def managed_commands(block: str) -> tuple[str, ...]:
    match = re.search(r"```sh\n(.*?)\n```", block, flags=re.DOTALL)
    if match is None:
        return ()
    return tuple(line.strip() for line in match.group(1).splitlines() if line.strip())


def scope_findings(snapshot: Snapshot, evidence: AgentEvidence) -> list[Finding]:
    findings: list[Finding] = []
    for package in evidence.packages:
        if package.path == ".":
            continue
        acceptable = tuple(
            dict.fromkeys(
                (
                    *evidence.global_commands,
                    *package.commands,
                    *(command_variant(command) for command in package.commands),
                )
            )
        )
        if not acceptable:
            continue
        governing = governing_agents_file(snapshot, package.path)
        if governing is not None and contains_any_command(governing.content, acceptable):
            continue
        path = "AGENTS.md" if package.path == "." else f"{package.path}/AGENTS.md"
        findings.append(
            agent_finding(
                "repo.agents-scope-required",
                path,
                "Package instructions must name a verification command for this package",
            )
        )
    return findings


def governing_agents_file(snapshot: Snapshot, package_path: str) -> RepoFile | None:
    parts = [] if package_path == "." else package_path.split("/")
    for length in range(len(parts), -1, -1):
        directory = "/".join(parts[:length])
        wanted = "AGENTS.md" if directory == "" else f"{directory}/AGENTS.md"
        file = next(
            (item for path, item in snapshot.files.items() if path.lower() == wanted.lower()), None
        )
        if file is not None:
            return file
    return None


def agent_finding(rule_id: str, path: str, message: str) -> Finding:
    return Finding(rule_id=rule_id, severity="error", path=path, message=message)
