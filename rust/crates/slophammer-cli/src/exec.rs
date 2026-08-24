use crate::config::Config;
use crate::core::{Finding, Severity};
use crate::rule_policy::RulePolicy;
use crate::scan::Snapshot;
use std::path::Path;
use std::process::Command;
use thiserror::Error;

pub trait Runner {
    fn run(&self, cwd: &Path, command: &str, args: &[&str]) -> Result<CommandOutput, ExecError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CommandOutput {
    pub status: i32,
    pub stdout: String,
    pub stderr: String,
}

#[derive(Debug, Error)]
pub enum ExecError {
    #[error("command failed to start: {0}")]
    Start(String),
}

#[derive(Default)]
pub struct RealRunner;

impl Runner for RealRunner {
    fn run(&self, cwd: &Path, command: &str, args: &[&str]) -> Result<CommandOutput, ExecError> {
        let output = Command::new(command)
            .args(args)
            .current_dir(cwd)
            .output()
            .map_err(|error| ExecError::Start(error.to_string()))?;
        Ok(CommandOutput {
            status: output.status.code().unwrap_or(1),
            stdout: String::from_utf8_lossy(&output.stdout).into_owned(),
            stderr: String::from_utf8_lossy(&output.stderr).into_owned(),
        })
    }
}

pub fn execute_rust_checks(
    snapshot: &Snapshot,
    config: &Config,
    policy: &RulePolicy<'_>,
    runner: &impl Runner,
) -> Vec<Finding> {
    rust_workspace_roots(snapshot)
        .into_iter()
        .flat_map(|root| execute_workspace(snapshot, config, policy, runner, root))
        .collect()
}

fn execute_workspace(
    snapshot: &Snapshot,
    config: &Config,
    policy: &RulePolicy<'_>,
    runner: &impl Runner,
    workspace_root: String,
) -> Vec<Finding> {
    let cwd = snapshot.root.join(&workspace_root);
    let threshold = crate::config::rust_coverage_threshold(config).to_string();
    let mut checks = vec![
        ExecutableCheck {
            rule_id: "rust.fmt-required",
            path: ".github/workflows",
            command: "cargo",
            args: vec!["fmt", "--check"],
            message: "cargo fmt --check failed",
        },
        ExecutableCheck {
            rule_id: "rust.clippy-required",
            path: ".github/workflows",
            command: "cargo",
            args: vec![
                "clippy",
                "--workspace",
                "--all-targets",
                "--",
                "-D",
                "warnings",
            ],
            message: "cargo clippy failed",
        },
        ExecutableCheck {
            rule_id: "rust.test-required",
            path: ".github/workflows",
            command: "cargo",
            args: vec!["test", "--workspace", "--all-targets"],
            message: "cargo test failed",
        },
        ExecutableCheck {
            rule_id: "rust.coverage-required",
            path: ".github/workflows",
            command: "cargo",
            args: vec!["llvm-cov", "--workspace", "--fail-under-lines", &threshold],
            message: "cargo llvm-cov failed",
        },
        ExecutableCheck {
            rule_id: "rust.dependency-audit-required",
            path: ".github/workflows",
            command: "cargo",
            args: vec!["audit"],
            message: "cargo audit failed",
        },
    ];
    if config
        .rust
        .as_ref()
        .and_then(|rust| rust.mutation.as_ref())
        .is_some_and(|mutation| !mutation.targets.is_empty())
    {
        checks.push(ExecutableCheck {
            rule_id: "rust.mutation-required",
            path: ".github/workflows",
            command: "cargo",
            args: vec!["mutants", "--workspace"],
            message: "cargo mutants failed",
        });
    }
    policy.admit_all(
        checks
            .into_iter()
            .filter(|check| policy.active(check.rule_id))
            .filter_map(|check| run_check(&cwd, &workspace_root, runner, check))
            .collect(),
    )
}

fn run_check(
    cwd: &Path,
    workspace_root: &str,
    runner: &impl Runner,
    check: ExecutableCheck<'_>,
) -> Option<Finding> {
    match runner.run(cwd, check.command, &check.args) {
        Ok(output) if output.status == 0 => None,
        Ok(output) => Some(Finding {
            rule_id: check.rule_id.to_owned(),
            severity: Severity::Error,
            path: prefixed_path(workspace_root, check.path),
            message: format!("{}: {}", check.message, compact_output(&output)),
            baselined: None,
        }),
        Err(error) => Some(Finding {
            rule_id: check.rule_id.to_owned(),
            severity: Severity::Error,
            path: prefixed_path(workspace_root, check.path),
            message: format!("{}: {error}", check.message),
            baselined: None,
        }),
    }
}

fn compact_output(output: &CommandOutput) -> String {
    let combined = format!("{}{}", output.stderr, output.stdout);
    combined
        .lines()
        .map(str::trim)
        .find(|line| !line.is_empty())
        .unwrap_or("command exited non-zero")
        .chars()
        .take(240)
        .collect()
}

fn rust_workspace_roots(snapshot: &Snapshot) -> Vec<String> {
    let mut roots: Vec<String> = snapshot
        .files
        .keys()
        .filter(|path| path.ends_with("Cargo.toml"))
        .filter(|path| !path.starts_with("fixtures/") && !path.starts_with("templates/"))
        .filter_map(|path| {
            path.strip_suffix("/Cargo.toml")
                .or_else(|| (path == "Cargo.toml").then_some(""))
        })
        .map(str::to_owned)
        .collect();
    roots.sort();
    roots
        .iter()
        .filter(|root| !has_parent_root(root, &roots))
        .cloned()
        .collect()
}

fn has_parent_root(root: &str, roots: &[String]) -> bool {
    !root.is_empty()
        && roots.iter().any(|candidate| {
            candidate != root
                && (candidate.is_empty() || root.starts_with(&format!("{candidate}/")))
        })
}

fn prefixed_path(root: &str, path: &str) -> String {
    if root.is_empty() {
        path.to_owned()
    } else {
        format!("{root}/{path}")
    }
}

struct ExecutableCheck<'a> {
    rule_id: &'static str,
    path: &'static str,
    command: &'static str,
    args: Vec<&'a str>,
    message: &'static str,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::scan::{RepoFile, Snapshot};
    use std::cell::Cell;
    use std::collections::BTreeMap;
    use std::path::PathBuf;

    struct FailingRunner;
    struct PassingRunner;
    struct RecordingRunner {
        calls: Cell<usize>,
    }

    impl Runner for FailingRunner {
        fn run(
            &self,
            _cwd: &Path,
            _command: &str,
            _args: &[&str],
        ) -> Result<CommandOutput, ExecError> {
            Ok(CommandOutput {
                status: 1,
                stdout: String::new(),
                stderr: "failed".to_owned(),
            })
        }
    }

    impl Runner for PassingRunner {
        fn run(
            &self,
            _cwd: &Path,
            _command: &str,
            _args: &[&str],
        ) -> Result<CommandOutput, ExecError> {
            Ok(CommandOutput {
                status: 0,
                stdout: String::new(),
                stderr: String::new(),
            })
        }
    }

    impl Runner for RecordingRunner {
        fn run(
            &self,
            _cwd: &Path,
            _command: &str,
            _args: &[&str],
        ) -> Result<CommandOutput, ExecError> {
            self.calls.set(self.calls.get() + 1);
            Ok(CommandOutput {
                status: 0,
                stdout: String::new(),
                stderr: String::new(),
            })
        }
    }

    #[test]
    fn execute_failures_become_findings() {
        let snapshot = Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([(
                "Cargo.toml".to_owned(),
                RepoFile {
                    path: "Cargo.toml".to_owned(),
                    content: String::new(),
                },
            )]),
        };
        let config = Config::default();
        let policy = RulePolicy::for_check(&config, &[]);
        let findings = execute_rust_checks(&snapshot, &config, &policy, &FailingRunner);
        assert!(
            findings
                .iter()
                .any(|finding| finding.rule_id == "rust.test-required")
        );
    }

    #[test]
    fn passing_execute_checks_are_clean() {
        let snapshot = Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([(
                "Cargo.toml".to_owned(),
                RepoFile {
                    path: "Cargo.toml".to_owned(),
                    content: String::new(),
                },
            )]),
        };
        let config = Config::default();
        let policy = RulePolicy::for_check(&config, &[]);
        let findings = execute_rust_checks(&snapshot, &config, &policy, &PassingRunner);
        assert!(findings.is_empty());
    }

    #[test]
    fn disabled_mutation_never_reaches_the_runner() {
        let snapshot = Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([(
                "Cargo.toml".to_owned(),
                RepoFile {
                    path: "Cargo.toml".to_owned(),
                    content: String::new(),
                },
            )]),
        };
        let config = crate::config::parse(
            "rules:\n  rust.mutation-required:\n    disabled: true\n    reason: checked elsewhere\nrust:\n  mutation:\n    targets:\n      - src\n",
        )
        .expect("config");
        let selected = ["rust.mutation-required".to_owned()];
        let policy = RulePolicy::for_check(&config, &selected);
        let runner = RecordingRunner {
            calls: Cell::new(0),
        };

        let findings = execute_rust_checks(&snapshot, &config, &policy, &runner);

        assert!(findings.is_empty());
        assert_eq!(runner.calls.get(), 0);
    }

    #[test]
    fn only_rule_filter_limits_execute_checks() {
        let snapshot = Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([(
                "Cargo.toml".to_owned(),
                RepoFile {
                    path: "Cargo.toml".to_owned(),
                    content: String::new(),
                },
            )]),
        };
        let config = Config::default();
        let only = ["rust.test-required".to_owned()];
        let policy = RulePolicy::for_check(&config, &only);
        let findings = execute_rust_checks(&snapshot, &config, &policy, &FailingRunner);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].rule_id, "rust.test-required");
    }
}
