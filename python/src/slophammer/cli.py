"""slophammer-py command line interface."""

from __future__ import annotations

import argparse
import json
import sys
from importlib import metadata

from slophammer.app import CommandResult, agents_check, agents_init, check, dry
from slophammer.rules import explain
from slophammer.rules.definitions import DEFAULT_DEFINITIONS

FORMATS = ("text", "json", "sarif")


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    arguments = parser.parse_args(argv)
    result = dispatch(arguments)
    if result.stdout:
        sys.stdout.write(result.stdout)
    if result.stderr:
        sys.stderr.write(result.stderr)
    return result.code


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="slophammer-py")
    parser.add_argument("--version", action="version", version=package_version())
    commands = parser.add_subparsers(dest="command", required=True)

    check_parser = commands.add_parser("check", help="Check a repository against the rules")
    check_parser.add_argument("path")
    check_parser.add_argument("--format", choices=FORMATS, default="text")
    check_parser.add_argument(
        "--only",
        action="append",
        help="Comma-separated rule ids to run; repeatable",
    )
    check_parser.add_argument(
        "--execute",
        action="store_true",
        help=(
            "Run the executable gates (format, lint, typecheck, tests, coverage, dry) "
            "and report failures; mutation and audit stay declaration-only"
        ),
    )
    baseline = check_parser.add_mutually_exclusive_group()
    baseline.add_argument("--baseline", action="store_true", help="Apply the checked-in baseline")
    baseline.add_argument(
        "--baseline-write", action="store_true", help="Record current findings as the baseline"
    )

    agents_parser = commands.add_parser("agents", help="Create or check AGENTS.md")
    agents_commands = agents_parser.add_subparsers(dest="agents_command", required=True)
    agents_init = agents_commands.add_parser("init", help="Create a root AGENTS.md")
    agents_init.add_argument("path", nargs="?", default=".")
    agents_init.add_argument("--dry-run", action="store_true")
    agents_init.add_argument("--force", action="store_true")
    agents_check = agents_commands.add_parser("check", help="Check AGENTS.md rules")
    agents_check.add_argument("path", nargs="?", default=".")
    agents_check.add_argument("--format", choices=FORMATS, default="text")

    dry_parser = commands.add_parser("dry", help="Run the copied-block DRY check")
    dry_parser.add_argument("path")
    dry_parser.add_argument("--format", choices=("text", "json"), default="text")

    explain_parser = commands.add_parser("explain", help="Explain a rule")
    explain_parser.add_argument("rule_id")

    rules_parser = commands.add_parser("rules", help="List rule ids")
    rules_parser.add_argument("--format", choices=("text", "json"), default="text")
    return parser


def dispatch(arguments: argparse.Namespace) -> CommandResult:
    if arguments.command == "check":
        return run_check(arguments)
    if arguments.command == "agents":
        return run_agents(arguments)
    if arguments.command == "dry":
        return dry(arguments.path, output_format=arguments.format)
    if arguments.command == "explain":
        return run_explain(arguments.rule_id)
    return run_rules_catalog(arguments.format)


def run_agents(arguments: argparse.Namespace) -> CommandResult:
    if arguments.agents_command == "check":
        return agents_check(arguments.path, output_format=arguments.format)
    return agents_init(arguments.path, dry_run=arguments.dry_run, force=arguments.force)


def run_rules_catalog(output_format: str) -> CommandResult:
    if output_format == "json":
        catalog = [
            {
                "id": definition.id,
                "title": definition.title,
                "severity": definition.severity,
                "path": definition.path,
                "message": definition.message,
                "description": definition.description,
            }
            for definition in DEFAULT_DEFINITIONS
        ]
        return CommandResult(code=0, stdout=json.dumps(catalog, indent=2) + "\n")
    return CommandResult(
        code=0, stdout="".join(f"{definition.id}\n" for definition in DEFAULT_DEFINITIONS)
    )


def run_explain(rule_id: str) -> CommandResult:
    text = explain(rule_id)
    if text is None:
        return CommandResult(code=2, stderr=f"explain failed: unknown rule {rule_id}\n")
    return CommandResult(code=0, stdout=text + "\n")


def run_check(arguments: argparse.Namespace) -> CommandResult:
    only, error = parse_only(arguments.only)
    if error is not None:
        return CommandResult(code=2, stderr=error)
    baseline = "write" if arguments.baseline_write else "check" if arguments.baseline else "off"
    return check(
        arguments.path,
        output_format=arguments.format,
        only_rule_ids=only,
        baseline=baseline,
        execute=arguments.execute,
    )


def parse_only(values: list[str] | None) -> tuple[list[str] | None, str | None]:
    if values is None:
        return None, None
    known = {definition.id for definition in DEFAULT_DEFINITIONS}
    rule_ids = [rule_id.strip() for value in values for rule_id in value.split(",")]
    if not any(rule_ids) or "" in rule_ids:
        return None, "check failed: --only requires rule ids\n"
    unknown = [rule_id for rule_id in rule_ids if rule_id not in known]
    if unknown:
        return None, f"check failed: unknown rule ids: {', '.join(unknown)}\n"
    return rule_ids, None


def package_version() -> str:
    try:
        return metadata.version("slophammer-py")
    except metadata.PackageNotFoundError:
        return "0.0.0"


if __name__ == "__main__":
    raise SystemExit(main())
