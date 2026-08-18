use crate::scan::{RepoFile, Snapshot};
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};

pub const START_MARKER: &str = "<!-- slophammer:agents:start -->";
pub const END_MARKER: &str = "<!-- slophammer:agents:end -->";
pub const AGENT_RULE_IDS: &[&str] = &[
    "repo.agents-required",
    "repo.agents-empty",
    "repo.agents-commands-required",
    "repo.agents-command-invalid",
    "repo.agents-scope-required",
    "repo.agents-stale",
];

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PackageArea {
    pub path: String,
    pub manifests: Vec<String>,
    pub commands: Vec<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Evidence {
    pub global_commands: Vec<String>,
    pub packages: Vec<PackageArea>,
    pub all_commands: Vec<String>,
    pub rendered_commands: Vec<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Issue {
    pub rule_id: &'static str,
    pub path: String,
    pub message: String,
}

pub fn derive(snapshot: &Snapshot) -> Evidence {
    let global_commands = root_runner_commands(snapshot);
    let mut by_path: BTreeMap<String, PackageArea> = BTreeMap::new();
    for file in snapshot.files.values() {
        let name = base_name(&file.path);
        if !matches!(
            name,
            "Cargo.toml" | "go.mod" | "package.json" | "pyproject.toml"
        ) || ignored_manifest_path(&file.path)
        {
            continue;
        }
        let package_path = directory(&file.path);
        let area = by_path
            .entry(package_path.clone())
            .or_insert_with(|| PackageArea {
                path: package_path.clone(),
                manifests: Vec::new(),
                commands: Vec::new(),
            });
        area.manifests.push(name.to_owned());
        area.commands
            .extend(manifest_commands(snapshot, file, &package_path));
    }
    let mut packages: Vec<PackageArea> = by_path.into_values().collect();
    for area in &mut packages {
        area.manifests = unique_sorted(std::mem::take(&mut area.manifests));
        area.commands = unique(std::mem::take(&mut area.commands));
    }
    let mut all_commands = global_commands.clone();
    for area in &packages {
        all_commands.extend(area.commands.clone());
        all_commands.extend(area.commands.iter().map(|command| command_variant(command)));
    }
    let rendered_commands = if global_commands.is_empty() {
        unique(
            packages
                .iter()
                .flat_map(|area| area.commands.clone())
                .collect(),
        )
    } else {
        global_commands.clone()
    };
    Evidence {
        global_commands,
        packages,
        all_commands: unique(all_commands),
        rendered_commands,
    }
}

fn ignored_manifest_path(file_path: &str) -> bool {
    let ignored = [
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
    ];
    directory(file_path)
        .split('/')
        .any(|segment| ignored.contains(&segment) || (segment.starts_with('.') && segment != "."))
}

fn root_runner_commands(snapshot: &Snapshot) -> Vec<String> {
    unique(
        [
            ("Makefile", "make check"),
            ("makefile", "make check"),
            ("Taskfile.yml", "task check"),
            ("Taskfile.yaml", "task check"),
            ("justfile", "just check"),
        ]
        .into_iter()
        .filter(|(name, _)| {
            snapshot
                .files
                .get(*name)
                .is_some_and(|file| has_check_target(name, &file.content))
        })
        .map(|(_, command)| command.to_owned())
        .collect(),
    )
}

fn has_check_target(name: &str, content: &str) -> bool {
    content.lines().any(|line| {
        let leading = line.len() - line.trim_start().len();
        let allowed_indent = name.starts_with("Taskfile") && leading <= 2;
        let candidate = if allowed_indent {
            line.trim_start()
        } else {
            line
        };
        candidate
            .strip_prefix("check")
            .is_some_and(|rest| rest.trim_start().starts_with(':'))
    })
}

fn manifest_commands(snapshot: &Snapshot, file: &RepoFile, package_path: &str) -> Vec<String> {
    match base_name(&file.path) {
        "go.mod" => vec![scoped_command(package_path, "go test ./...")],
        "Cargo.toml" => {
            let command = if file
                .content
                .lines()
                .any(|line| line.trim() == "[workspace]")
            {
                "cargo test --workspace"
            } else {
                "cargo test"
            };
            vec![scoped_command(package_path, command)]
        }
        "package.json" => package_json_commands(snapshot, file, package_path),
        "pyproject.toml" => {
            let command = if file.content.to_ascii_lowercase().contains("pytest") {
                if adjacent_file(snapshot, package_path, "uv.lock") {
                    "uv run pytest"
                } else {
                    "python -m pytest"
                }
            } else {
                "python -m compileall ."
            };
            vec![scoped_command(package_path, command)]
        }
        _ => Vec::new(),
    }
}

fn package_json_commands(snapshot: &Snapshot, file: &RepoFile, package_path: &str) -> Vec<String> {
    let Ok(parsed) = serde_json::from_str::<Value>(&file.content) else {
        return Vec::new();
    };
    let Some(scripts) = parsed.get("scripts").and_then(Value::as_object) else {
        return Vec::new();
    };
    let Some(script) = ["check", "test", "build"]
        .into_iter()
        .find(|name| scripts.get(*name).is_some_and(Value::is_string))
    else {
        return Vec::new();
    };
    let manager = package_manager(snapshot, package_path);
    let command = if script == "test" && matches!(manager, "npm" | "yarn" | "bun") {
        format!("{manager} test")
    } else {
        format!("{manager} run {script}")
    };
    vec![scoped_command(package_path, &command)]
}

fn package_manager(snapshot: &Snapshot, package_path: &str) -> &'static str {
    for directory in ancestor_paths(package_path) {
        if let Some(manager) = declared_package_manager(snapshot, &directory) {
            return manager;
        }
        if adjacent_file(snapshot, &directory, "pnpm-lock.yaml") {
            return "pnpm";
        }
        if adjacent_file(snapshot, &directory, "yarn.lock") {
            return "yarn";
        }
        if adjacent_file(snapshot, &directory, "bun.lock")
            || adjacent_file(snapshot, &directory, "bun.lockb")
        {
            return "bun";
        }
    }
    "npm"
}

fn declared_package_manager(snapshot: &Snapshot, package_path: &str) -> Option<&'static str> {
    let file_path = if package_path == "." {
        "package.json".to_owned()
    } else {
        format!("{package_path}/package.json")
    };
    let file = snapshot.files.get(&file_path)?;
    let parsed = serde_json::from_str::<Value>(&file.content).ok()?;
    let manager = parsed.get("packageManager")?.as_str()?.split('@').next()?;
    match manager {
        "npm" => Some("npm"),
        "pnpm" => Some("pnpm"),
        "yarn" => Some("yarn"),
        "bun" => Some("bun"),
        _ => None,
    }
}

fn ancestor_paths(package_path: &str) -> Vec<String> {
    let mut paths = Vec::new();
    let mut current = package_path.to_owned();
    loop {
        paths.push(current.clone());
        if current == "." {
            return paths;
        }
        current = current
            .rsplit_once('/')
            .map_or_else(|| ".".to_owned(), |(parent, _)| parent.to_owned());
    }
}

fn adjacent_file(snapshot: &Snapshot, package_path: &str, name: &str) -> bool {
    let path = if package_path == "." {
        name.to_owned()
    } else {
        format!("{package_path}/{name}")
    };
    snapshot.files.contains_key(&path)
}

fn command_variant(command: &str) -> String {
    if command.starts_with("cd \"") {
        if let Some((_, suffix)) = command.split_once(" && ") {
            return suffix.to_owned();
        }
    }
    command.to_owned()
}

fn scoped_command(package_path: &str, command: &str) -> String {
    if package_path == "." {
        return command.to_owned();
    }
    let escaped = package_path.replace('\\', "\\\\").replace('"', "\\\"");
    format!("cd \"{escaped}\" && {command}")
}

pub fn render(snapshot: &Snapshot) -> String {
    let evidence = derive(snapshot);
    [
        "# AGENTS.md".to_owned(),
        String::new(),
        "These instructions apply to this repository.".to_owned(),
        String::new(),
        render_evidence_block(&evidence),
        String::new(),
        "## Working rules".to_owned(),
        String::new(),
        "- Keep changes small and reviewable.".to_owned(),
        "- Add or update tests when behavior changes.".to_owned(),
        "- Run the repository checks before you finish.".to_owned(),
        "- Do not weaken existing checks to make a change pass.".to_owned(),
        String::new(),
    ]
    .join("\n")
}

pub fn render_evidence_block(evidence: &Evidence) -> String {
    let mut lines = vec![
        START_MARKER.to_owned(),
        "## Repository checks".to_owned(),
        String::new(),
    ];
    if evidence.rendered_commands.is_empty() {
        lines.push("No verification command could be derived from repository files.".to_owned());
    } else {
        lines.extend([
            "Run these commands before you finish:".to_owned(),
            String::new(),
            "```sh".to_owned(),
        ]);
        lines.extend(evidence.rendered_commands.clone());
        lines.push("```".to_owned());
    }
    lines.extend([String::new(), "## Package areas".to_owned(), String::new()]);
    if evidence.packages.is_empty() {
        lines.push("- No package manifest was detected.".to_owned());
    } else {
        for area in &evidence.packages {
            let manifests = area
                .manifests
                .iter()
                .map(|manifest| format!("`{manifest}`"))
                .collect::<Vec<_>>()
                .join(", ");
            lines.push(format!("- `{}`: {manifests}", area.path));
        }
    }
    lines.push(END_MARKER.to_owned());
    lines.join("\n")
}

pub fn evaluate(snapshot: &Snapshot) -> Vec<Issue> {
    let Some(root_file) = root_agents_file(snapshot) else {
        return vec![Issue {
            rule_id: "repo.agents-required",
            path: "AGENTS.md".to_owned(),
            message: "AGENTS.md is required".to_owned(),
        }];
    };
    let evidence = derive(snapshot);
    let mut issues = Vec::new();
    if !useful_content(&root_file.content)
        && !contains_any_command(&root_file.content, &evidence.all_commands)
    {
        issues.push(Issue {
            rule_id: "repo.agents-empty",
            path: "AGENTS.md".to_owned(),
            message: "AGENTS.md must contain useful repository instructions".to_owned(),
        });
    }
    if !evidence.all_commands.is_empty()
        && !contains_any_command(&root_file.content, &evidence.all_commands)
    {
        issues.push(Issue {
            rule_id: "repo.agents-commands-required",
            path: "AGENTS.md".to_owned(),
            message: "AGENTS.md must name a verification command supported by the repository"
                .to_owned(),
        });
    }
    if let Some(managed) = managed_block(&root_file.content) {
        let invalid = managed_commands(&managed)
            .into_iter()
            .filter(|command| !evidence.all_commands.contains(command))
            .collect::<Vec<_>>();
        if !invalid.is_empty() {
            issues.push(Issue {
                rule_id: "repo.agents-command-invalid",
                path: "AGENTS.md".to_owned(),
                message: format!(
                    "AGENTS.md contains generated commands without repository evidence: {}",
                    invalid.join(", ")
                ),
            });
        }
        if managed != render_evidence_block(&evidence) {
            issues.push(Issue {
                rule_id: "repo.agents-stale",
                path: "AGENTS.md".to_owned(),
                message: "The Slophammer AGENTS.md evidence block is stale".to_owned(),
            });
        }
    }
    issues.extend(scope_issues(snapshot, &evidence));
    issues
}

pub fn root_agents_file(snapshot: &Snapshot) -> Option<&RepoFile> {
    snapshot
        .files
        .values()
        .find(|file| !file.path.contains('/') && file.path.eq_ignore_ascii_case("AGENTS.md"))
}

fn useful_content(content: &str) -> bool {
    let without_comments = remove_html_comments(content);
    let kept = without_comments
        .lines()
        .filter(|line| {
            let trimmed = line.trim();
            !trimmed.is_empty() && !trimmed.starts_with('#') && trimmed != "```"
        })
        .collect::<Vec<_>>()
        .join(" ");
    kept.chars()
        .filter(|character| character.is_ascii_alphanumeric())
        .count()
        >= 20
}

fn remove_html_comments(content: &str) -> String {
    let mut output = String::new();
    let mut remaining = content;
    loop {
        let Some(start) = remaining.find("<!--") else {
            output.push_str(remaining);
            break;
        };
        output.push_str(&remaining[..start]);
        let Some(end) = remaining[start + 4..].find("-->") else {
            output.push_str(&remaining[start..]);
            break;
        };
        remaining = &remaining[start + 4 + end + 3..];
    }
    output
}

fn contains_any_command(content: &str, commands: &[String]) -> bool {
    let lines = content.lines().map(str::trim).collect::<BTreeSet<_>>();
    commands.iter().any(|command| {
        content.contains(&format!("`{command}`")) || lines.contains(command.as_str())
    })
}

fn managed_block(content: &str) -> Option<String> {
    let start = content.find(START_MARKER)?;
    let remaining = &content[start..];
    let end = remaining
        .find(END_MARKER)
        .map_or(content.len(), |offset| start + offset + END_MARKER.len());
    Some(content[start..end].trim().to_owned())
}

fn managed_commands(block: &str) -> Vec<String> {
    let Some(start) = block.find("```sh\n") else {
        return Vec::new();
    };
    let commands = &block[start + 6..];
    let Some(end) = commands.find("\n```") else {
        return Vec::new();
    };
    commands[..end]
        .lines()
        .map(str::trim)
        .filter(|line| !line.is_empty())
        .map(str::to_owned)
        .collect()
}

fn scope_issues(snapshot: &Snapshot, evidence: &Evidence) -> Vec<Issue> {
    evidence
        .packages
        .iter()
        .filter_map(|area| {
            if area.path == "." {
                return None;
            }
            let acceptable = unique(
                evidence
                    .global_commands
                    .iter()
                    .cloned()
                    .chain(area.commands.iter().cloned())
                    .chain(area.commands.iter().map(|command| command_variant(command)))
                    .collect(),
            );
            if acceptable.is_empty() {
                return None;
            }
            if governing_agents_file(snapshot, &area.path)
                .is_some_and(|file| contains_any_command(&file.content, &acceptable))
            {
                return None;
            }
            Some(Issue {
                rule_id: "repo.agents-scope-required",
                path: if area.path == "." {
                    "AGENTS.md".to_owned()
                } else {
                    format!("{}/AGENTS.md", area.path)
                },
                message: "Package instructions must name a verification command for this package"
                    .to_owned(),
            })
        })
        .collect()
}

fn governing_agents_file<'a>(snapshot: &'a Snapshot, package_path: &str) -> Option<&'a RepoFile> {
    let parts = if package_path == "." {
        Vec::new()
    } else {
        package_path.split('/').collect::<Vec<_>>()
    };
    for length in (0..=parts.len()).rev() {
        let directory = parts[..length].join("/");
        let wanted = if directory.is_empty() {
            "AGENTS.md".to_owned()
        } else {
            format!("{directory}/AGENTS.md")
        };
        if let Some(file) = snapshot
            .files
            .values()
            .find(|file| file.path.eq_ignore_ascii_case(&wanted))
        {
            return Some(file);
        }
    }
    None
}

fn base_name(path: &str) -> &str {
    path.rsplit('/').next().unwrap_or(path)
}

fn directory(path: &str) -> String {
    path.rsplit_once('/')
        .map_or_else(|| ".".to_owned(), |(directory, _)| directory.to_owned())
}

fn unique(values: Vec<String>) -> Vec<String> {
    let mut seen = BTreeSet::new();
    values
        .into_iter()
        .filter(|value| seen.insert(value.clone()))
        .collect()
}

fn unique_sorted(values: Vec<String>) -> Vec<String> {
    values
        .into_iter()
        .collect::<BTreeSet<_>>()
        .into_iter()
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    #[test]
    fn derives_runner_and_package_commands() {
        let evidence = derive(&snapshot(&[
            ("Makefile", "check:\n\t@true\n"),
            ("go/go.mod", "module example.com/demo\n"),
            ("rust/Cargo.toml", "[workspace]\nmembers = []\n"),
            (
                "python/pyproject.toml",
                "[dependency-groups]\ndev = [\"pytest\"]\n",
            ),
            ("python/uv.lock", "version = 1\n"),
            ("web/package.json", "{\"scripts\":{\"check\":\"true\"}}"),
            ("web/pnpm-lock.yaml", "lockfileVersion: 9\n"),
            ("fixtures/demo/go.mod", "module example.com/ignored\n"),
        ]));
        assert_eq!(evidence.global_commands, ["make check"]);
        for command in [
            "go test ./...",
            "cargo test --workspace",
            "uv run pytest",
            "pnpm run check",
        ] {
            assert!(evidence.all_commands.iter().any(|item| item == command));
        }
        assert!(
            !evidence
                .packages
                .iter()
                .any(|area| area.path == "fixtures/demo")
        );
        assert!(render(&snapshot(&[])).contains("No package manifest was detected"));
    }

    #[test]
    fn uses_workspace_package_managers() {
        let lock_evidence = derive(&snapshot(&[
            ("pnpm-lock.yaml", "lockfileVersion: 9\n"),
            (
                "packages/app/package.json",
                "{\"scripts\":{\"check\":\"true\"}}",
            ),
        ]));
        assert!(
            lock_evidence
                .all_commands
                .iter()
                .any(|command| command == "pnpm run check")
        );

        let metadata_evidence = derive(&snapshot(&[
            ("package.json", "{\"packageManager\":\"yarn@4.1.0\"}"),
            (
                "packages/app/package.json",
                "{\"scripts\":{\"build\":\"true\"}}",
            ),
        ]));
        assert!(
            metadata_evidence
                .all_commands
                .iter()
                .any(|command| command == "yarn run build")
        );
    }

    #[test]
    fn reports_agent_instruction_failures() {
        assert_eq!(
            rule_ids(&evaluate(&snapshot(&[]))),
            ["repo.agents-required"]
        );
        assert_eq!(
            rule_ids(&evaluate(&snapshot(&[("AGENTS.md", "# Agents\n")]))),
            ["repo.agents-empty"]
        );
        assert_eq!(
            rule_ids(&evaluate(&snapshot(&[
                (
                    "AGENTS.md",
                    "# Agents\n\nKeep changes small and reviewable.\n",
                ),
                ("package.json", "{\"scripts\":{\"check\":\"true\"}}",),
            ]))),
            ["repo.agents-commands-required"]
        );
        let scoped = evaluate(&snapshot(&[
            (
                "AGENTS.md",
                "# Agents\n\nRun `npm test` before finishing.\n",
            ),
            (
                "packages/app/package.json",
                "{\"scripts\":{\"test\":\"true\"}}",
            ),
            (
                "packages/worker/package.json",
                "{\"scripts\":{\"build\":\"true\"}}",
            ),
        ]));
        assert!(scoped.iter().any(|issue| {
            issue.rule_id == "repo.agents-scope-required"
                && issue.path == "packages/worker/AGENTS.md"
        }));

        let stale = format!(
            "# AGENTS.md\n\nKeep changes small and reviewable.\n\n{START_MARKER}\n## Repository checks\n\n```sh\nnpm test\n```\n{END_MARKER}\n"
        );
        assert_eq!(
            rule_ids(&evaluate(&snapshot(&[("AGENTS.md", &stale)]))),
            ["repo.agents-command-invalid", "repo.agents-stale"]
        );
    }

    #[test]
    fn treats_supported_command_as_useful_content() {
        let checked = snapshot(&[
            ("AGENTS.md", "```sh\ngo test ./...\n```\n"),
            ("go.mod", "module example.com/demo\n"),
        ]);
        assert!(evaluate(&checked).is_empty());
    }

    #[test]
    fn accepts_generated_evidence() {
        let initial = snapshot(&[("package.json", "{\"scripts\":{\"check\":\"true\"}}")]);
        let generated = render(&initial);
        let checked = snapshot(&[
            ("AGENTS.md", &generated),
            ("package.json", "{\"scripts\":{\"check\":\"true\"}}"),
        ]);
        assert!(evaluate(&checked).is_empty());
    }

    fn snapshot(files: &[(&str, &str)]) -> Snapshot {
        Snapshot {
            root: PathBuf::from("/repo"),
            files: files
                .iter()
                .map(|(path, content)| {
                    (
                        (*path).to_owned(),
                        RepoFile {
                            path: (*path).to_owned(),
                            content: (*content).to_owned(),
                        },
                    )
                })
                .collect(),
        }
    }

    fn rule_ids(issues: &[Issue]) -> Vec<&str> {
        issues.iter().map(|issue| issue.rule_id).collect()
    }
}
