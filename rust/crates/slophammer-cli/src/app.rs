use crate::agents;
use crate::config::Config;
use crate::core::{EXIT_ERROR, EXIT_FINDINGS, EXIT_OK, Finding, Report, RuleDefinition};
use crate::exec::{RealRunner, Runner};
use crate::report::{new_report, write_json, write_sarif, write_text};
use crate::scan::{Snapshot, scan_repo, scan_repo_unignored};
use std::fmt;
use std::fs::{self, OpenOptions};
use std::io::{self, Write};
use std::path::Path;
use thiserror::Error;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum OutputFormat {
    Text,
    Json,
    Sarif,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CheckOptions {
    pub root: String,
    pub format: OutputFormat,
    pub execute: bool,
    pub only_rule_ids: Vec<String>,
    pub baseline: crate::baseline::BaselineMode,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AgentsInitOptions {
    pub root: String,
    pub dry_run: bool,
    pub force: bool,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct DirectOptions {
    pub root: String,
    pub format: OutputFormat,
    pub max_findings: Option<usize>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AppResult {
    pub code: i32,
    pub stdout: String,
    pub stderr: String,
}

#[derive(Debug, Error)]
pub enum AppError {
    #[error("scan failed: {0}")]
    Scan(#[from] crate::scan::ScanError),
    #[error("config failed: {0}")]
    Config(#[from] crate::config::ConfigError),
    #[error("report failed: {0}")]
    Report(#[from] crate::report::ReportError),
    #[error("unknown rule: {0}")]
    UnknownRule(String),
    #[error("--only requires a rule id")]
    EmptyOnlyRule,
    #[error(transparent)]
    Baseline(#[from] crate::baseline::BaselineError),
}

pub fn agents_check(root: String, format: OutputFormat) -> AppResult {
    check(CheckOptions {
        root,
        format,
        execute: false,
        only_rule_ids: agents::AGENT_RULE_IDS
            .iter()
            .map(|rule_id| (*rule_id).to_owned())
            .collect(),
        baseline: crate::baseline::BaselineMode::Off,
    })
}

pub fn agents_init(options: AgentsInitOptions) -> AppResult {
    let snapshot = match scan_repo_unignored(&options.root) {
        Ok(snapshot) => snapshot,
        Err(error) => return agents_error(error.to_string()),
    };
    if agents::has_unsafe_package_path(&snapshot) {
        return agents_error("package paths containing newlines are not supported".to_owned());
    }
    if agents::has_reserved_marker_package_path(&snapshot) {
        return agents_error(
            "package paths containing Slophammer evidence markers are not supported".to_owned(),
        );
    }
    let content = agents::render(&snapshot);
    if options.dry_run {
        return AppResult {
            code: EXIT_OK,
            stdout: content,
            stderr: String::new(),
        };
    }
    let target = snapshot
        .root
        .join(agents::root_agents_file(&snapshot).map_or("AGENTS.md", |file| file.path.as_str()));
    let result = if options.force {
        replace_agents_file(&target, content.as_bytes())
    } else {
        write_new_file(&target, content.as_bytes())
    };
    if let Err(error) = result {
        let message = if error.kind() == io::ErrorKind::AlreadyExists {
            "AGENTS.md already exists; pass --force to replace it".to_owned()
        } else {
            error.to_string()
        };
        return agents_error(message);
    }
    AppResult {
        code: EXIT_OK,
        stdout: "created AGENTS.md\n".to_owned(),
        stderr: String::new(),
    }
}

fn write_new_file(path: &Path, content: &[u8]) -> io::Result<()> {
    let mut file = OpenOptions::new().write(true).create_new(true).open(path)?;
    file.write_all(content)
}

fn replace_agents_file(path: &Path, content: &[u8]) -> io::Result<()> {
    reject_nonregular_agents_target(path)?;
    let (temporary, mut file) = new_agents_temporary(path)?;
    let write_result = file.write_all(content).and_then(|()| file.sync_all());
    drop(file);
    if let Err(error) = write_result {
        let _ = fs::remove_file(&temporary);
        return Err(error);
    }
    let result = fs::rename(&temporary, path);
    if result.is_err() {
        let _ = fs::remove_file(&temporary);
    }
    result
}

fn reject_nonregular_agents_target(path: &Path) -> io::Result<()> {
    let metadata = match fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == io::ErrorKind::NotFound => return Ok(()),
        Err(error) => return Err(error),
    };
    if metadata.file_type().is_symlink() {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "AGENTS.md is a symlink; refusing forced replacement",
        ));
    }
    if !metadata.file_type().is_file() {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "AGENTS.md is not a regular file; refusing forced replacement",
        ));
    }
    Ok(())
}

fn new_agents_temporary(path: &Path) -> io::Result<(std::path::PathBuf, fs::File)> {
    let parent = path.parent().unwrap_or_else(|| Path::new("."));
    for attempt in 0..100 {
        let temporary = parent.join(format!(
            ".slophammer-agents-{}-{attempt}",
            std::process::id()
        ));
        match OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&temporary)
        {
            Ok(file) => return Ok((temporary, file)),
            Err(error) if error.kind() == io::ErrorKind::AlreadyExists => {}
            Err(error) => return Err(error),
        }
    }
    Err(io::Error::new(
        io::ErrorKind::AlreadyExists,
        "could not create a temporary AGENTS.md file",
    ))
}

fn agents_error(message: String) -> AppResult {
    AppResult {
        code: EXIT_ERROR,
        stdout: String::new(),
        stderr: format!("agents init failed: {message}\n"),
    }
}

pub fn check(options: CheckOptions) -> AppResult {
    check_with_runner(options, &RealRunner)
}

pub fn check_with_runner(options: CheckOptions, runner: &impl Runner) -> AppResult {
    match check_inner(options, runner) {
        Ok(result) => result,
        Err(error) => AppResult {
            code: EXIT_ERROR,
            stdout: String::new(),
            stderr: format!("check failed: {error}\n"),
        },
    }
}

pub fn dry(options: DirectOptions) -> AppResult {
    direct(options, |snapshot, config, max_findings| {
        crate::rust_rules::dry_findings(snapshot, config, max_findings)
    })
}

pub fn boundaries(options: DirectOptions) -> AppResult {
    direct(options, |snapshot, config, _| {
        crate::rust_rules::run_rules(
            snapshot,
            config,
            &[crate::rust_rules::rule_ids::RUST_DEPENDENCY_BOUNDARIES_REQUIRED.to_owned()],
        )
    })
}

pub fn unsafe_policy(options: DirectOptions) -> AppResult {
    direct(options, |snapshot, config, _| {
        crate::rust_rules::run_rules(
            snapshot,
            config,
            &[crate::rust_rules::rule_ids::RUST_UNSAFE_POLICY_REQUIRED.to_owned()],
        )
    })
}

pub fn explain(rule_id: &str) -> AppResult {
    match crate::rust_rules::explain(rule_id) {
        Some(text) => AppResult {
            code: EXIT_OK,
            stdout: text,
            stderr: String::new(),
        },
        None => AppResult {
            code: EXIT_ERROR,
            stdout: String::new(),
            stderr: format!("unknown rule: {rule_id}\n"),
        },
    }
}

pub fn rules(format: OutputFormat) -> AppResult {
    let definitions = crate::rust_rules::default_definitions();
    match render_rules(format, &definitions) {
        Ok(stdout) => AppResult {
            code: EXIT_OK,
            stdout,
            stderr: String::new(),
        },
        Err(error) => AppResult {
            code: EXIT_ERROR,
            stdout: String::new(),
            stderr: format!("rules failed: {error}\n"),
        },
    }
}

fn check_inner(options: CheckOptions, runner: &impl Runner) -> Result<AppResult, AppError> {
    let only_rule_ids = expand_only_rule_ids(&options.only_rule_ids)?;
    let snapshot = scan_repo(command_root(&options.root))?;
    let config = crate::config::load(&snapshot)?;
    let mut findings = static_findings(
        command_root(&options.root),
        &snapshot,
        &config,
        &only_rule_ids,
    )?;
    if options.execute {
        findings.extend(crate::exec::execute_rust_checks(
            &snapshot,
            &config,
            &only_rule_ids,
            runner,
        ));
    }
    crate::config::apply_rule_config(&config, &mut findings);
    let mut report = new_report(findings);
    report.scope =
        crate::rust_rules::scope_counts(&snapshot, &config).map(|(scanned, production_files)| {
            crate::core::ScopeCoverage {
                scanned,
                production_files,
            }
        });
    finish_check(options, &snapshot, report)
}

fn static_findings(
    root: &str,
    snapshot: &Snapshot,
    config: &Config,
    only_rule_ids: &[String],
) -> Result<Vec<Finding>, AppError> {
    let selected = if only_rule_ids.is_empty() {
        crate::rust_rules::default_definitions()
            .into_iter()
            .map(|definition| definition.id.to_owned())
            .collect()
    } else {
        only_rule_ids.to_vec()
    };
    let (agent_rule_ids, other_rule_ids): (Vec<_>, Vec<_>) = selected
        .into_iter()
        .partition(|rule_id| agents::AGENT_RULE_IDS.contains(&rule_id.as_str()));
    let mut findings = if other_rule_ids.is_empty() {
        Vec::new()
    } else {
        crate::rust_rules::run_rules(snapshot, config, &other_rule_ids)
    };
    if !agent_rule_ids.is_empty() {
        let agent_snapshot = scan_repo_unignored(root)?;
        findings.extend(crate::rust_rules::run_rules(
            &agent_snapshot,
            config,
            &agent_rule_ids,
        ));
    }
    Ok(findings)
}

fn finish_check(
    options: CheckOptions,
    snapshot: &crate::scan::Snapshot,
    mut report: Report,
) -> Result<AppResult, AppError> {
    use crate::baseline::BaselineMode;
    let mut trailer = String::new();
    match options.baseline {
        BaselineMode::Off => {}
        BaselineMode::Check => {
            crate::baseline::apply_check(&snapshot.root, &mut report)?;
            trailer = crate::baseline::debt_line(&report);
        }
        BaselineMode::Write => {
            let summary = crate::baseline::write(&snapshot.root, &report)?;
            return Ok(AppResult {
                code: EXIT_OK,
                stdout: summary,
                stderr: String::new(),
            });
        }
    }
    let mut stdout = render_report(options.format, &report)?;
    if options.format == OutputFormat::Text {
        stdout.push_str(&trailer);
    }
    Ok(AppResult {
        code: if report.ok { EXIT_OK } else { EXIT_FINDINGS },
        stdout,
        stderr: String::new(),
    })
}

fn direct(
    options: DirectOptions,
    check: impl FnOnce(&Snapshot, &Config, usize) -> Vec<Finding>,
) -> AppResult {
    match direct_inner(options, check) {
        Ok(result) => result,
        Err(error) => AppResult {
            code: EXIT_ERROR,
            stdout: String::new(),
            stderr: format!("check failed: {error}\n"),
        },
    }
}

fn direct_inner(
    options: DirectOptions,
    check: impl FnOnce(&Snapshot, &Config, usize) -> Vec<Finding>,
) -> Result<AppResult, AppError> {
    let snapshot = scan_repo(command_root(&options.root))?;
    let config = crate::config::load(&snapshot)?;
    let mut findings = check(&snapshot, &config, options.max_findings.unwrap_or(0));
    crate::config::apply_rule_config(&config, &mut findings);
    let report = new_report(findings);
    let stdout = render_report(options.format, &report)?;
    Ok(AppResult {
        code: if report.ok { EXIT_OK } else { EXIT_FINDINGS },
        stdout,
        stderr: String::new(),
    })
}

fn expand_only_rule_ids(raw: &[String]) -> Result<Vec<String>, AppError> {
    let expanded: Vec<String> = raw
        .iter()
        .flat_map(|value| value.split(','))
        .map(str::trim)
        .filter(|rule_id| !rule_id.is_empty())
        .map(str::to_owned)
        .collect();
    if !raw.is_empty() && expanded.is_empty() {
        return Err(AppError::EmptyOnlyRule);
    }
    let unknown: Vec<&str> = expanded
        .iter()
        .filter(|rule_id| !crate::rust_rules::known_rule(rule_id))
        .map(String::as_str)
        .collect();
    if !unknown.is_empty() {
        return Err(AppError::UnknownRule(unknown.join(", ")));
    }
    Ok(expanded)
}

fn command_root(root: &str) -> &str {
    if root.is_empty() { "." } else { root }
}

fn render_report(format: OutputFormat, report: &Report) -> Result<String, AppError> {
    match format {
        OutputFormat::Text => Ok(write_text(report)),
        OutputFormat::Json => Ok(write_json(report)?),
        OutputFormat::Sarif => Ok(write_sarif(report)?),
    }
}

fn render_rules(
    format: OutputFormat,
    definitions: &[RuleDefinition],
) -> Result<String, serde_json::Error> {
    match format {
        OutputFormat::Text | OutputFormat::Sarif => Ok(rules_text(definitions)),
        OutputFormat::Json => Ok(format!("{}\n", serde_json::to_string_pretty(definitions)?)),
    }
}

fn rules_text(definitions: &[RuleDefinition]) -> String {
    let mut output = String::from("RULE ID\tCATEGORY\tSEVERITY\tSTATUS\tTOOL\n");
    for definition in definitions {
        output.push_str(&format!(
            "{}\t{}\t{}\t{}\t{}\n",
            definition.id,
            definition.category,
            definition.severity,
            definition.status,
            definition.tool.unwrap_or("")
        ));
    }
    output
}

impl fmt::Display for OutputFormat {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Text => formatter.write_str("text"),
            Self::Json => formatter.write_str("json"),
            Self::Sarif => formatter.write_str("sarif"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::core::{EXIT_FINDINGS, EXIT_OK};

    mod fixtures {
        include!(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/tests/support/fixtures.rs"
        ));
    }
    use fixtures::{fixture, fixture_path, temp_root, write_file};

    #[test]
    fn unknown_only_rule_is_error() {
        let result = check(CheckOptions {
            root: ".".to_owned(),
            format: OutputFormat::Json,
            execute: false,
            only_rule_ids: vec!["missing.rule".to_owned(), "other.rule".to_owned()],
            baseline: crate::baseline::BaselineMode::Off,
        });
        assert_eq!(result.code, EXIT_ERROR);
        assert!(
            result
                .stderr
                .contains("unknown rule: missing.rule, other.rule")
        );
    }

    #[test]
    fn only_rules_accept_comma_separated_ids() {
        let fixture = fixture("rust-clean");
        let result = check(CheckOptions {
            root: fixture_path(&fixture),
            format: OutputFormat::Json,
            execute: false,
            only_rule_ids: vec!["repo.readme-required, repo.agents-required".to_owned()],
            baseline: crate::baseline::BaselineMode::Off,
        });
        assert_eq!(result.code, EXIT_OK, "{}", result.stderr);
    }

    #[test]
    fn empty_only_rules_are_an_error() {
        let result = check(CheckOptions {
            root: ".".to_owned(),
            format: OutputFormat::Json,
            execute: false,
            only_rule_ids: vec![" , ".to_owned()],
            baseline: crate::baseline::BaselineMode::Off,
        });
        assert_eq!(result.code, EXIT_ERROR);
        assert!(result.stderr.contains("--only requires a rule id"));
    }

    #[test]
    fn clean_fixture_has_no_findings() {
        let fixture = fixture("rust-clean");
        let result = check(CheckOptions {
            root: fixture_path(&fixture),
            format: OutputFormat::Json,
            execute: false,
            only_rule_ids: Vec::new(),
            baseline: crate::baseline::BaselineMode::Off,
        });
        assert_eq!(result.code, EXIT_OK);
        assert!(result.stdout.contains("\"ok\": true"));
    }

    #[test]
    fn rust_failure_fixtures_report_findings() {
        for fixture_name in [
            "rust-bad-dependency",
            "rust-missing-audit",
            "rust-missing-ci",
            "rust-missing-clippy",
            "rust-missing-coverage",
            "rust-missing-dry",
            "rust-missing-fmt",
            "rust-missing-msrv",
            "rust-missing-mutation",
            "rust-missing-tests",
            "rust-unsafe",
        ] {
            let fixture = fixture(fixture_name);
            let result = check(CheckOptions {
                root: fixture_path(&fixture),
                format: OutputFormat::Json,
                execute: false,
                only_rule_ids: Vec::new(),
                baseline: crate::baseline::BaselineMode::Off,
            });
            assert_eq!(result.code, EXIT_FINDINGS, "{fixture_name}");
            assert!(result.stdout.contains("\"ok\": false"), "{fixture_name}");
        }
    }

    #[test]
    fn direct_commands_use_report_contract() {
        let clean_fixture = fixture("rust-clean");
        let dry = dry(DirectOptions {
            root: fixture_path(&clean_fixture),
            format: OutputFormat::Text,
            max_findings: None,
        });
        assert_eq!(dry.code, EXIT_OK);
        assert!(dry.stdout.contains("OK: no findings"));

        let bad_dependency_fixture = fixture("rust-bad-dependency");
        let boundaries = boundaries(DirectOptions {
            root: fixture_path(&bad_dependency_fixture),
            format: OutputFormat::Sarif,
            max_findings: None,
        });
        assert_eq!(boundaries.code, EXIT_FINDINGS);
        assert!(
            boundaries
                .stdout
                .contains("rust.dependency-boundaries-required")
        );

        let unsafe_fixture = fixture("rust-unsafe");
        let unsafe_result = unsafe_policy(DirectOptions {
            root: fixture_path(&unsafe_fixture),
            format: OutputFormat::Json,
            max_findings: None,
        });
        assert_eq!(unsafe_result.code, EXIT_FINDINGS);
        assert!(unsafe_result.stdout.contains("rust.unsafe-policy-required"));
    }

    #[test]
    fn rules_and_explain_are_available() {
        let catalog = rules(OutputFormat::Json);
        assert_eq!(catalog.code, EXIT_OK);
        assert!(catalog.stdout.contains("rust.check-required"));

        let explanation = explain("rust.check-required");
        assert_eq!(explanation.code, EXIT_OK);
        assert!(
            explanation
                .stdout
                .contains("Rust projects should declare cargo check")
        );
    }

    #[test]
    fn check_applies_rule_severity_overrides() {
        let root = temp_root("rule-severity");
        write_file(root.path(), "AGENTS.md", "# Agents\n");
        write_file(
            root.path(),
            ".github/workflows/ci.yml",
            "jobs:\n  ci:\n    steps:\n      - run: echo ok\n",
        );
        write_file(
            root.path(),
            "slophammer.yml",
            "rules:\n  repo.readme-required:\n    severity: warn\n",
        );

        let result = check(CheckOptions {
            root: fixture_path(&root),
            format: OutputFormat::Json,
            execute: false,
            only_rule_ids: vec!["repo.readme-required".to_owned()],
            baseline: crate::baseline::BaselineMode::Off,
        });

        assert_eq!(result.code, EXIT_FINDINGS);
        assert!(
            result
                .stdout
                .contains("\"rule_id\": \"repo.readme-required\"")
        );
        assert!(result.stdout.contains("\"severity\": \"warn\""));
    }

    #[test]
    fn config_error_fixtures_return_error_code() {
        for fixture_name in ["rust-invalid-config", "rust-unknown-config"] {
            let fixture = fixture(fixture_name);
            let result = check(CheckOptions {
                root: fixture_path(&fixture),
                format: OutputFormat::Json,
                execute: false,
                only_rule_ids: Vec::new(),
                baseline: crate::baseline::BaselineMode::Off,
            });
            assert_eq!(result.code, EXIT_ERROR, "{fixture_name}");
            assert!(result.stderr.contains("config failed"), "{fixture_name}");
        }
    }
}
