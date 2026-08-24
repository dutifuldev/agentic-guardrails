mod boundaries;
mod definitions;
mod dry;
mod evidence;
mod scope;
mod suppressions;
mod unsafe_policy;
mod workflow_binding;

use crate::config::Config;
use crate::core::{Finding, find_definition};
use crate::rule_policy::RulePolicy;
use crate::scan::Snapshot;
use definitions::definition;
pub use definitions::{default_definitions, rule_ids};
pub use dry::dry_findings;
pub use scope::scope_counts;

pub fn run_rules(snapshot: &Snapshot, config: &Config, policy: &RulePolicy<'_>) -> Vec<Finding> {
    let mut findings = Vec::new();
    for rule_id in default_definitions()
        .iter()
        .map(|item| item.id)
        .filter(|id| policy.active(id))
    {
        findings.extend(run_rule(rule_id, snapshot, config));
    }
    policy.admit_all(findings)
}

pub fn explain(rule_id: &str) -> Option<String> {
    find_definition(&default_definitions(), rule_id).map(|definition| {
        format!(
            "{}\n{}\nseverity: {}\npath: {}\n",
            definition.id, definition.description, definition.severity, definition.path
        )
    })
}

pub fn known_rule(rule_id: &str) -> bool {
    find_definition(&default_definitions(), rule_id).is_some()
}

fn run_rule(rule_id: &str, snapshot: &Snapshot, config: &Config) -> Vec<Finding> {
    match rule_id {
        rule_ids::README_REQUIRED => repo_readme(snapshot),
        rule_ids::AGENTS_REQUIRED => repo_agents(snapshot),
        rule_ids::CI_REQUIRED => repo_ci(snapshot),
        rule_ids::SLOPHAMMER_CI_REQUIRED => repo_slophammer_ci(snapshot),
        rule_ids::RUST_MANIFEST_REQUIRED => rust_manifest(snapshot),
        rule_ids::RUST_MSRV_REQUIRED => rust_msrv(snapshot),
        rule_ids::RUST_CHECK_REQUIRED => cargo_command(snapshot, rule_id, "cargo check"),
        rule_ids::RUST_FMT_REQUIRED => rust_fmt(snapshot),
        rule_ids::RUST_CLIPPY_REQUIRED => rust_clippy(snapshot),
        rule_ids::RUST_TEST_REQUIRED => cargo_command(snapshot, rule_id, "cargo test"),
        rule_ids::RUST_COVERAGE_REQUIRED => rust_coverage(snapshot, config),
        rule_ids::RUST_COMPLEXITY_REQUIRED => rust_complexity(snapshot, config),
        rule_ids::RUST_DRY_REQUIRED => rust_dry_declaration(snapshot),
        rule_ids::RUST_MUTATION_REQUIRED => rust_mutation(snapshot),
        rule_ids::RUST_UNSAFE_POLICY_REQUIRED => unsafe_policy::policy_findings(snapshot, config),
        rule_ids::RUST_DEPENDENCY_AUDIT_REQUIRED => rust_dependency_audit(snapshot),
        rule_ids::RUST_DEPENDENCY_BOUNDARIES_REQUIRED => {
            boundaries::boundary_findings(snapshot, config)
        }
        rule_ids::RUST_SUPPRESSIONS_JUSTIFIED => rust_suppressions(snapshot),
        rule_ids::RUST_SCOPE_INCOMPLETE => scope::scope_findings(snapshot, config),
        _ => Vec::new(),
    }
}

fn repo_readme(snapshot: &Snapshot) -> Vec<Finding> {
    missing(
        !snapshot.has_case_insensitive("README.md"),
        rule_ids::README_REQUIRED,
    )
}

fn repo_agents(snapshot: &Snapshot) -> Vec<Finding> {
    missing(
        !snapshot.has_case_insensitive("AGENTS.md"),
        rule_ids::AGENTS_REQUIRED,
    )
}

fn repo_ci(snapshot: &Snapshot) -> Vec<Finding> {
    missing(!snapshot.has_workflow(), rule_ids::CI_REQUIRED)
}

/// Config without enforcement is decoration: when slophammer.yml is present,
/// binding CI evidence must invoke a Slophammer checker.
fn repo_slophammer_ci(snapshot: &Snapshot) -> Vec<Finding> {
    let has_config =
        snapshot.file("slophammer.yml").is_some() || snapshot.file("slophammer.yaml").is_some();
    missing(
        has_config && !slophammer_invocation(&evidence::command_text(snapshot)),
        rule_ids::SLOPHAMMER_CI_REQUIRED,
    )
}

fn slophammer_invocation(evidence: &str) -> bool {
    if evidence.contains("uses: osolmaz/slophammer@") {
        return true;
    }
    [
        "slophammer-go",
        "slophammer-ts",
        "slophammer-rs",
        "slophammer-py",
    ]
    .iter()
    .any(|binary| invocation_with_check(evidence, binary))
}

fn invocation_with_check(evidence: &str, binary: &str) -> bool {
    evidence.match_indices(binary).any(|(index, _)| {
        let window_end = (index + 160).min(evidence.len());
        evidence[index..window_end].contains(" check")
    })
}

fn rust_suppressions(snapshot: &Snapshot) -> Vec<Finding> {
    if !is_rust_project(snapshot) {
        return Vec::new();
    }
    suppressions::suppression_findings(snapshot)
}

fn rust_manifest(snapshot: &Snapshot) -> Vec<Finding> {
    missing(
        is_rust_project(snapshot) && rust_manifests(snapshot).is_empty(),
        rule_ids::RUST_MANIFEST_REQUIRED,
    )
}

fn rust_msrv(snapshot: &Snapshot) -> Vec<Finding> {
    missing(
        is_rust_project(snapshot) && !has_msrv(snapshot),
        rule_ids::RUST_MSRV_REQUIRED,
    )
}

fn cargo_command(snapshot: &Snapshot, rule_id: &str, command: &str) -> Vec<Finding> {
    missing(
        is_rust_project(snapshot) && !evidence::command_text(snapshot).contains(command),
        rule_id,
    )
}

/// `cargo mutants --list` enumerates mutants and `--check` only builds
/// them; neither runs tests, so neither can fail on a survivor. Executing
/// forms (including `--in-diff`) count.
fn rust_mutation(snapshot: &Snapshot) -> Vec<Finding> {
    let executing = evidence::command_segments(snapshot).iter().any(|segment| {
        segment.contains("cargo mutants")
            && !segment.contains("--list")
            && !segment.contains("--check")
    });
    missing(
        is_rust_project(snapshot) && !executing,
        rule_ids::RUST_MUTATION_REQUIRED,
    )
}

fn rust_fmt(snapshot: &Snapshot) -> Vec<Finding> {
    let evidence = evidence::command_text(snapshot);
    missing(
        is_rust_project(snapshot)
            && !(evidence.contains("cargo fmt") && evidence.contains("--check")),
        rule_ids::RUST_FMT_REQUIRED,
    )
}

fn rust_clippy(snapshot: &Snapshot) -> Vec<Finding> {
    let evidence = evidence::command_text(snapshot);
    missing(
        is_rust_project(snapshot)
            && !(evidence.contains("cargo clippy")
                && (evidence.contains("-d warnings") || evidence.contains("--deny warnings"))),
        rule_ids::RUST_CLIPPY_REQUIRED,
    )
}

fn rust_coverage(snapshot: &Snapshot, config: &Config) -> Vec<Finding> {
    let threshold = crate::config::rust_coverage_threshold(config);
    missing(
        is_rust_project(snapshot) && !has_coverage_gate(snapshot, threshold),
        rule_ids::RUST_COVERAGE_REQUIRED,
    )
}

fn rust_complexity(snapshot: &Snapshot, config: &Config) -> Vec<Finding> {
    missing(
        is_rust_project(snapshot) && !has_complexity_limit(snapshot, config),
        rule_ids::RUST_COMPLEXITY_REQUIRED,
    )
}

fn rust_dry_declaration(snapshot: &Snapshot) -> Vec<Finding> {
    missing(
        is_rust_project(snapshot)
            && !evidence::command_text(snapshot).contains("slophammer-rs dry"),
        rule_ids::RUST_DRY_REQUIRED,
    )
}

fn rust_dependency_audit(snapshot: &Snapshot) -> Vec<Finding> {
    let evidence = evidence::command_text(snapshot);
    missing(
        is_rust_project(snapshot)
            && !(evidence.contains("cargo audit") || evidence.contains("cargo deny")),
        rule_ids::RUST_DEPENDENCY_AUDIT_REQUIRED,
    )
}

fn missing(condition: bool, rule_id: &str) -> Vec<Finding> {
    if condition {
        vec![Finding::new(definition(rule_id))]
    } else {
        Vec::new()
    }
}

pub fn is_rust_project(snapshot: &Snapshot) -> bool {
    !rust_manifests(snapshot).is_empty()
        || snapshot
            .files
            .keys()
            .any(|path| rust_source_path(path) && !ignored_project_path(path))
        || evidence::command_text(snapshot).contains("cargo ")
}

fn rust_manifests(snapshot: &Snapshot) -> Vec<&str> {
    snapshot
        .files
        .keys()
        .map(String::as_str)
        .filter(|path| path.ends_with("Cargo.toml") && !ignored_project_path(path))
        .collect()
}

fn has_msrv(snapshot: &Snapshot) -> bool {
    snapshot
        .files
        .values()
        .filter(|file| file.path.ends_with("Cargo.toml") && !ignored_project_path(&file.path))
        .any(|file| file.content.contains("rust-version"))
        || snapshot
            .file("rust-toolchain.toml")
            .is_some_and(|file| toolchain_declares_version(&file.content))
        || snapshot
            .file("rust-toolchain")
            .is_some_and(|file| toolchain_declares_version(&file.content))
}

fn toolchain_declares_version(content: &str) -> bool {
    content
        .lines()
        .map(strip_inline_comment)
        .map(str::trim)
        .filter(|line| !line.is_empty())
        .any(|line| {
            if let Some((key, value)) = line.split_once('=') {
                key.trim() == "channel" && value.chars().any(|ch| ch.is_ascii_digit())
            } else {
                line.chars().any(|ch| ch.is_ascii_digit())
            }
        })
}

fn has_coverage_gate(snapshot: &Snapshot, minimum: u32) -> bool {
    let evidence = evidence::command_text(snapshot);
    evidence.contains("cargo llvm-cov")
        && coverage_thresholds(&evidence)
            .into_iter()
            .any(|threshold| threshold >= f64::from(minimum))
}

fn coverage_thresholds(content: &str) -> Vec<f64> {
    let tokens = content.split_whitespace().collect::<Vec<_>>();
    let mut thresholds = Vec::new();
    for (index, token) in tokens.iter().enumerate() {
        if !token.starts_with("--fail-under") {
            continue;
        }
        if let Some((_, value)) = token.split_once('=') {
            if let Some(threshold) = numeric_prefix(value) {
                thresholds.push(threshold);
            }
        } else if let Some(next) = tokens
            .get(index + 1)
            .and_then(|value| numeric_prefix(value))
        {
            thresholds.push(next);
        }
    }
    thresholds
}

fn numeric_prefix(value: &str) -> Option<f64> {
    let number = value
        .trim_matches(|ch: char| ch == '"' || ch == '\'')
        .chars()
        .take_while(|ch| ch.is_ascii_digit() || *ch == '.')
        .collect::<String>();
    if number.is_empty() {
        None
    } else {
        number.parse::<f64>().ok()
    }
}

fn has_complexity_limit(snapshot: &Snapshot, config: &Config) -> bool {
    config
        .rust
        .as_ref()
        .and_then(|rust| rust.complexity.as_ref())
        .is_some_and(|complexity| complexity.cognitive_max <= 8)
        || has_clippy_complexity_limit(snapshot)
}

fn has_clippy_complexity_limit(snapshot: &Snapshot) -> bool {
    snapshot
        .files
        .values()
        .filter(|file| {
            file.path.ends_with("clippy.toml")
                || file.path.ends_with(".clippy.toml")
                || file.path.ends_with("rust/clippy.toml")
        })
        .any(|file| {
            let normalized = file.content.to_ascii_lowercase();
            normalized.contains("cognitive-complexity-threshold")
                && numeric_limit_at_most(&normalized, 8)
        })
        || snapshot.files.values().any(|file| {
            rust_source_path(&file.path)
                && file
                    .content
                    .to_ascii_lowercase()
                    .contains("clippy::cognitive_complexity")
        })
}

fn strip_inline_comment(line: &str) -> &str {
    line.split('#').next().unwrap_or(line)
}

fn numeric_limit_at_most(content: &str, maximum: u32) -> bool {
    content
        .split(|ch: char| !ch.is_ascii_digit())
        .filter(|part| !part.is_empty())
        .filter_map(|part| part.parse::<u32>().ok())
        .any(|value| value <= maximum)
}

fn rust_source_path(path: &str) -> bool {
    path.ends_with(".rs")
}

fn ignored_project_path(path: &str) -> bool {
    path.starts_with("fixtures/")
        || path.starts_with("templates/")
        || path.contains("/fixtures/")
        || path.contains("/target/")
        || path.contains("/node_modules/")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::scan::{RepoFile, Snapshot};
    use std::collections::BTreeMap;
    use std::path::PathBuf;

    #[test]
    fn slophammer_invocation_rejects_unrelated_evidence() {
        assert!(!slophammer_invocation("cargo test --workspace"));
        assert!(slophammer_invocation("uses: osolmaz/slophammer@v0.4.1"));
    }

    #[test]
    fn run_rule_dispatches_mutation_and_dry_rules() {
        let snapshot = Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([(
                "Cargo.toml".to_owned(),
                RepoFile {
                    path: "Cargo.toml".to_owned(),
                    content: "[package]\nname = \"x\"\n".to_owned(),
                },
            )]),
        };
        let config = Config::default();
        let mutation = run_rule(rule_ids::RUST_MUTATION_REQUIRED, &snapshot, &config);
        assert_eq!(
            mutation.len(),
            1,
            "mutation rule must dispatch through run_rule"
        );
        assert_eq!(mutation[0].rule_id, rule_ids::RUST_MUTATION_REQUIRED);
        let dry = run_rule(rule_ids::RUST_DRY_REQUIRED, &snapshot, &config);
        assert_eq!(dry.len(), 1, "dry rule must dispatch through run_rule");
        assert_eq!(dry[0].rule_id, rule_ids::RUST_DRY_REQUIRED);
    }

    fn mutation_workflow_snapshot(commands: &[&str]) -> Snapshot {
        let steps: String = commands
            .iter()
            .map(|command| format!("      - run: {command}\n"))
            .collect();
        let workflow = format!("name: CI\non: [push]\njobs:\n  ci:\n    steps:\n{steps}");
        Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([
                (
                    "Cargo.toml".to_owned(),
                    RepoFile {
                        path: "Cargo.toml".to_owned(),
                        content: "[package]\nname = \"x\"\n".to_owned(),
                    },
                ),
                (
                    ".github/workflows/ci.yml".to_owned(),
                    RepoFile {
                        path: ".github/workflows/ci.yml".to_owned(),
                        content: workflow,
                    },
                ),
            ]),
        }
    }

    #[test]
    fn list_command_beside_executing_gate_stays_credited() {
        let snapshot = mutation_workflow_snapshot(&[
            "cargo mutants --workspace --list",
            "cargo mutants --workspace --in-diff pr.diff",
        ]);
        assert!(rust_mutation(&snapshot).is_empty());
    }

    #[test]
    fn list_only_mutation_commands_are_not_evidence() {
        let findings = rust_mutation(&mutation_workflow_snapshot(&[
            "cargo mutants --workspace --list",
        ]));
        assert_eq!(findings.len(), 1, "list-only run must not satisfy the rule");

        let executing = mutation_workflow_snapshot(&["cargo mutants --workspace"]);
        assert!(rust_mutation(&executing).is_empty());
    }

    #[test]
    fn check_only_mutation_commands_are_not_evidence() {
        let findings = rust_mutation(&mutation_workflow_snapshot(&[
            "cargo mutants --workspace --check",
        ]));
        assert_eq!(
            findings.len(),
            1,
            "a build-only --check run cannot fail on a survivor"
        );

        let snapshot = mutation_workflow_snapshot(&[
            "cargo mutants --workspace --check",
            "cargo mutants --workspace --in-diff pr.diff",
        ]);
        assert!(rust_mutation(&snapshot).is_empty());
    }

    #[test]
    fn missing_rust_checks_are_reported() {
        let snapshot = Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([
                (
                    "README.md".to_owned(),
                    RepoFile {
                        path: "README.md".to_owned(),
                        content: String::new(),
                    },
                ),
                (
                    "AGENTS.md".to_owned(),
                    RepoFile {
                        path: "AGENTS.md".to_owned(),
                        content: String::new(),
                    },
                ),
                (
                    "Cargo.toml".to_owned(),
                    RepoFile {
                        path: "Cargo.toml".to_owned(),
                        content: "[package]\nname = \"x\"\nrust-version = \"1.86\"\n".to_owned(),
                    },
                ),
            ]),
        };
        let config = Config::default();
        let policy = RulePolicy::for_check(&config, &[]);
        let findings = run_rules(&snapshot, &config, &policy);
        assert!(
            findings
                .iter()
                .any(|finding| finding.rule_id == rule_ids::RUST_CHECK_REQUIRED)
        );
    }

    #[test]
    fn accepts_stricter_coverage_thresholds() {
        let snapshot = rust_snapshot_with_workflow(
            "cargo llvm-cov --workspace --fail-under-lines 90",
            "[package]\nname = \"x\"\nrust-version = \"1.86\"\n",
        );
        let config =
            crate::config::parse("rust:\n  coverage:\n    threshold: 85\n").expect("config");
        assert!(rust_coverage(&snapshot, &config).is_empty());
    }

    #[test]
    fn configured_complexity_policy_satisfies_complexity_rule() {
        let snapshot = rust_snapshot_with_workflow(
            "cargo check --workspace",
            "[package]\nname = \"x\"\nrust-version = \"1.86\"\n",
        );
        let config =
            crate::config::parse("rust:\n  complexity:\n    cognitive_max: 8\n").expect("config");
        assert!(rust_complexity(&snapshot, &config).is_empty());
    }

    #[test]
    fn plain_rust_toolchain_pin_satisfies_msrv_rule() {
        let mut snapshot = rust_snapshot_with_workflow(
            "cargo check --workspace",
            "[package]\nname = \"x\"\nedition = \"2024\"\n",
        );
        snapshot.files.insert(
            "rust-toolchain".to_owned(),
            RepoFile {
                path: "rust-toolchain".to_owned(),
                content: "1.86.0\n".to_owned(),
            },
        );
        assert!(rust_msrv(&snapshot).is_empty());
    }

    fn rust_snapshot_with_workflow(command: &str, manifest: &str) -> Snapshot {
        Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([
                (
                    "Cargo.toml".to_owned(),
                    RepoFile {
                        path: "Cargo.toml".to_owned(),
                        content: manifest.to_owned(),
                    },
                ),
                (
                    "src/lib.rs".to_owned(),
                    RepoFile {
                        path: "src/lib.rs".to_owned(),
                        content: "pub fn demo() {}\n".to_owned(),
                    },
                ),
                (
                    ".github/workflows/ci.yml".to_owned(),
                    RepoFile {
                        path: ".github/workflows/ci.yml".to_owned(),
                        content: format!(
                            "on: [push]\njobs:\n  ci:\n    steps:\n      - run: {command}\n"
                        ),
                    },
                ),
            ]),
        }
    }
}
