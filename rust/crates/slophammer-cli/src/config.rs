use crate::core::Severity;
use crate::scan::Snapshot;
use serde::Deserialize;
use std::collections::BTreeMap;
use thiserror::Error;
use yaml_serde::Value;

#[derive(Clone, Debug, Default, PartialEq)]
pub struct Config {
    pub rules: BTreeMap<String, RuleConfig>,
    pub rust: Option<RustConfig>,
}

#[derive(Clone, Debug, Default, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct RuleConfig {
    pub severity: Option<RuleSeverity>,
    #[serde(default)]
    pub disabled: bool,
    pub reason: Option<String>,
    pub threshold: Option<f64>,
    pub max: Option<f64>,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum RuleSeverity {
    Error,
    Warn,
}

impl From<RuleSeverity> for Severity {
    fn from(value: RuleSeverity) -> Self {
        match value {
            RuleSeverity::Error => Self::Error,
            RuleSeverity::Warn => Self::Warn,
        }
    }
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct RustConfig {
    pub coverage: Option<RustCoverage>,
    pub complexity: Option<RustComplexity>,
    #[serde(default)]
    pub targets: Vec<String>,
    #[serde(default)]
    pub exclude: Vec<ExcludeEntry>,
    pub dry: Option<RustDry>,
    #[serde(rename = "unsafe")]
    pub unsafe_policy: Option<RustUnsafe>,
    pub mutation: Option<RustMutation>,
    #[serde(default)]
    pub dependency_boundaries: Vec<DependencyBoundary>,
}

/// An exclude entry is a plain pattern when it matches the conventional
/// non-production list, and a pattern with a reason when it carves out
/// production files.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(untagged)]
pub enum ExcludeEntry {
    Pattern(String),
    Reasoned { pattern: String, reason: String },
}

impl ExcludeEntry {
    pub fn pattern(&self) -> &str {
        match self {
            Self::Pattern(pattern) => pattern,
            Self::Reasoned { pattern, .. } => pattern,
        }
    }

    fn needs_reason(&self) -> bool {
        matches!(self, Self::Pattern(pattern) if !conventional_exclude_pattern(pattern))
    }

    fn empty_reason(&self) -> bool {
        matches!(self, Self::Reasoned { reason, .. } if reason.trim().is_empty())
    }
}

/// The conventional non-production list from specs/CONFIG.md: patterns that
/// scope may exclude without a reason.
pub fn conventional_exclude_pattern(pattern: &str) -> bool {
    const CONVENTIONAL: [&str; 15] = [
        "_test.",
        ".test.",
        ".spec.",
        "tests/",
        "fixtures/",
        "templates/",
        "testdata/",
        "dist/",
        "build/",
        "coverage/",
        "target/",
        "node_modules/",
        "vendor/",
        "generated",
        "scripts/",
    ];
    CONVENTIONAL.iter().any(|marker| pattern.contains(marker))
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct RustCoverage {
    pub threshold: u32,
    #[serde(default)]
    pub paths: Vec<String>,
    #[serde(default)]
    pub exclude: Vec<ExcludeEntry>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct RustComplexity {
    pub cognitive_max: u32,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct RustDry {
    pub max_findings: usize,
    #[serde(default)]
    pub paths: Vec<String>,
    #[serde(default)]
    pub exclude: Vec<ExcludeEntry>,
    pub copied_blocks: Option<CopiedBlocks>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct CopiedBlocks {
    pub enabled: bool,
    pub min_tokens: usize,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct RustUnsafe {
    pub policy: UnsafePolicy,
    #[serde(default)]
    pub allow: Vec<UnsafeAllow>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "snake_case")]
pub enum UnsafePolicy {
    Forbid,
    AllowDocumented,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct UnsafeAllow {
    pub path: String,
    pub reason: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct RustMutation {
    #[serde(default)]
    pub targets: Vec<String>,
    #[serde(default)]
    pub exclude: Vec<ExcludeEntry>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct DependencyBoundary {
    pub from: String,
    #[serde(default)]
    pub allow: Vec<String>,
}

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("parse failed: {0}")]
    Parse(#[from] yaml_serde::Error),
    #[error("{0}")]
    Validation(String),
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct RootConfig {
    #[serde(default)]
    rules: BTreeMap<String, RuleConfig>,
    #[serde(default)]
    go: Option<Value>,
    #[serde(default)]
    typescript: Option<Value>,
    #[serde(default)]
    python: Option<Value>,
    #[serde(default)]
    rust: Option<RustConfig>,
}

pub fn load(snapshot: &Snapshot) -> Result<Config, ConfigError> {
    let Some(content) = snapshot
        .file("slophammer.yml")
        .or_else(|| snapshot.file("slophammer.yaml"))
        .map(|file| file.content.as_str())
    else {
        return Ok(Config::default());
    };
    parse(content)
}

pub fn parse(content: &str) -> Result<Config, ConfigError> {
    let root: RootConfig = yaml_serde::from_str(content)?;
    let _known_sections = (&root.go, &root.typescript);
    if let Some(python) = &root.python {
        validate_python_section(python)?;
    }
    let config = Config {
        rules: root.rules,
        rust: root.rust,
    };
    validate(&config)?;
    Ok(config)
}

/// The shared python section is enforced by the Python checker but
/// shape-validated by every implementation, so typos in shared config fail
/// everywhere. Key trees mirror specs/CONFIG.md.
fn validate_python_section(value: &Value) -> Result<(), ConfigError> {
    let allowed = [
        "coverage",
        "complexity",
        "dry",
        "mutation",
        "dependency_boundaries",
        "typecheck",
    ];
    validate_value_keys(value, "python", &allowed)?;
    let Some(mapping) = value.as_mapping() else {
        return Ok(());
    };
    if let Some(coverage) = mapping.get("coverage") {
        validate_value_keys(
            coverage,
            "python.coverage",
            &["threshold", "paths", "exclude"],
        )?;
        validate_value_excludes(coverage.get("exclude"), "python.coverage.exclude")?;
    }
    if let Some(complexity) = mapping.get("complexity") {
        validate_value_keys(complexity, "python.complexity", &["max"])?;
    }
    if let Some(dry) = mapping.get("dry") {
        validate_value_keys(
            dry,
            "python.dry",
            &["max_findings", "paths", "exclude", "copied_blocks"],
        )?;
        validate_value_excludes(dry.get("exclude"), "python.dry.exclude")?;
        if let Some(copied) = dry.get("copied_blocks") {
            validate_value_keys(
                copied,
                "python.dry.copied_blocks",
                &["enabled", "min_tokens"],
            )?;
        }
    }
    if let Some(mutation) = mapping.get("mutation") {
        validate_value_keys(mutation, "python.mutation", &["targets", "exclude"])?;
        validate_value_excludes(mutation.get("exclude"), "python.mutation.exclude")?;
    }
    validate_value_entries(
        mapping.get("dependency_boundaries"),
        "python.dependency_boundaries",
        &["from", "allow"],
    )?;
    if let Some(typecheck) = mapping.get("typecheck") {
        validate_value_keys(typecheck, "python.typecheck", &["demotions"])?;
        validate_value_entries(
            typecheck.get("demotions"),
            "python.typecheck.demotions",
            &["rule", "reason"],
        )?;
    }
    Ok(())
}

fn validate_value_keys(value: &Value, field: &str, allowed: &[&str]) -> Result<(), ConfigError> {
    let Some(mapping) = value.as_mapping() else {
        if matches!(value, Value::Null) {
            return Ok(());
        }
        return Err(ConfigError::Validation(format!(
            "{field} must be a mapping"
        )));
    };
    for (key, _) in mapping {
        let name = key.as_str().unwrap_or_default();
        if !allowed.contains(&name) {
            return Err(ConfigError::Validation(format!(
                "{field}.{name} is not supported"
            )));
        }
    }
    Ok(())
}

fn validate_value_entries(
    value: Option<&Value>,
    field: &str,
    allowed: &[&str],
) -> Result<(), ConfigError> {
    let entries = match value_sequence(value, field)? {
        Some(entries) => entries,
        None => return Ok(()),
    };
    for (index, entry) in entries.iter().enumerate() {
        validate_value_keys(entry, &format!("{field}[{index}]"), allowed)?;
    }
    Ok(())
}

/// A present but non-sequence value is a shape error, matching the other
/// implementations; absent and null values are fine.
fn value_sequence<'a>(
    value: Option<&'a Value>,
    field: &str,
) -> Result<Option<&'a Vec<Value>>, ConfigError> {
    let Some(value) = value else {
        return Ok(None);
    };
    if matches!(value, Value::Null) {
        return Ok(None);
    }
    value
        .as_sequence()
        .map(Some)
        .ok_or_else(|| ConfigError::Validation(format!("{field} must be a sequence")))
}

fn validate_value_excludes(value: Option<&Value>, field: &str) -> Result<(), ConfigError> {
    let entries = match value_sequence(value, field)? {
        Some(entries) => entries,
        None => return Ok(()),
    };
    for (index, entry) in entries.iter().enumerate() {
        if entry.as_str().is_some() {
            continue;
        }
        validate_value_keys(entry, &format!("{field}[{index}]"), &["pattern", "reason"])?;
    }
    Ok(())
}

fn validate(config: &Config) -> Result<(), ConfigError> {
    for (rule_id, rule) in &config.rules {
        if rule.disabled && rule.reason.as_deref().unwrap_or("").trim().is_empty() {
            return Err(ConfigError::Validation(format!(
                "rules.{rule_id}.reason is required when disabled is true"
            )));
        }
    }
    let Some(rust) = &config.rust else {
        return Ok(());
    };
    validate_excludes(rust)?;
    let coverage_threshold = rust.coverage.as_ref().map(|coverage| coverage.threshold);
    if let Some(threshold) = coverage_threshold {
        if threshold < 85 {
            return Err(ConfigError::Validation(
                "rust coverage threshold must be at least 85".to_owned(),
            ));
        }
    }
    if let Some(complexity) = &rust.complexity {
        if complexity.cognitive_max > 8 {
            return Err(ConfigError::Validation(
                "rust complexity cognitive_max must be at most 8".to_owned(),
            ));
        }
    }
    if let Some(dry) = &rust.dry {
        if dry.max_findings != 0 {
            return Err(ConfigError::Validation(
                "rust dry max_findings must be 0 for production code".to_owned(),
            ));
        }
        if let Some(copied_blocks) = &dry.copied_blocks {
            if copied_blocks.min_tokens == 0 {
                return Err(ConfigError::Validation(
                    "rust dry copied_blocks min_tokens must be positive".to_owned(),
                ));
            }
        }
    }
    if let Some(unsafe_policy) = &rust.unsafe_policy {
        for allow in &unsafe_policy.allow {
            if allow.path.trim().is_empty() || allow.reason.trim().is_empty() {
                return Err(ConfigError::Validation(
                    "rust unsafe allow entries require path and reason".to_owned(),
                ));
            }
        }
    }
    for boundary in &rust.dependency_boundaries {
        if boundary.from.trim().is_empty() {
            return Err(ConfigError::Validation(
                "rust dependency boundaries require from".to_owned(),
            ));
        }
    }
    Ok(())
}

/// A string exclude may only name conventional non-production patterns;
/// excludes that carve out production files must carry a reason.
fn validate_excludes(rust: &RustConfig) -> Result<(), ConfigError> {
    let sections: [(&str, &[ExcludeEntry]); 4] = [
        ("rust.exclude", &rust.exclude),
        (
            "rust.coverage.exclude",
            rust.coverage.as_ref().map_or(&[], |c| &c.exclude),
        ),
        (
            "rust.dry.exclude",
            rust.dry.as_ref().map_or(&[], |d| &d.exclude),
        ),
        (
            "rust.mutation.exclude",
            rust.mutation.as_ref().map_or(&[], |m| &m.exclude),
        ),
    ];
    for (section, entries) in sections {
        validate_exclude_entries(section, entries)?;
    }
    Ok(())
}

fn validate_exclude_entries(section: &str, entries: &[ExcludeEntry]) -> Result<(), ConfigError> {
    for entry in entries {
        if entry.needs_reason() {
            return Err(ConfigError::Validation(format!(
                "{section} requires a reason for production paths"
            )));
        }
        if entry.empty_reason() {
            return Err(ConfigError::Validation(format!(
                "{section} reasons must not be empty"
            )));
        }
    }
    Ok(())
}

pub fn rust_targets(config: &Config) -> Vec<String> {
    config
        .rust
        .as_ref()
        .map(|rust| {
            if rust.targets.is_empty() {
                vec![".".to_owned()]
            } else {
                rust.targets.clone()
            }
        })
        .unwrap_or_else(|| vec![".".to_owned()])
}

pub fn rust_exclude(config: &Config) -> Vec<String> {
    config
        .rust
        .as_ref()
        .map(|rust| exclude_patterns(&rust.exclude))
        .unwrap_or_default()
}

pub fn exclude_patterns(entries: &[ExcludeEntry]) -> Vec<String> {
    entries
        .iter()
        .map(|entry| entry.pattern().to_owned())
        .collect()
}

pub fn rust_coverage_threshold(config: &Config) -> u32 {
    config
        .rust
        .as_ref()
        .and_then(|rust| rust.coverage.as_ref().map(|coverage| coverage.threshold))
        .unwrap_or(85)
}

pub fn rust_dry_paths(config: &Config) -> Vec<String> {
    config
        .rust
        .as_ref()
        .and_then(|rust| rust.dry.as_ref())
        .map(|dry| {
            if dry.paths.is_empty() {
                rust_targets(config)
            } else {
                dry.paths.clone()
            }
        })
        .unwrap_or_else(|| rust_targets(config))
}

pub fn rust_dry_exclude(config: &Config) -> Vec<String> {
    config
        .rust
        .as_ref()
        .and_then(|rust| rust.dry.as_ref())
        .map(|dry| exclude_patterns(&dry.exclude))
        .unwrap_or_else(|| rust_exclude(config))
}

pub fn rust_dry_min_tokens(config: &Config) -> usize {
    config
        .rust
        .as_ref()
        .and_then(|rust| rust.dry.as_ref())
        .and_then(|dry| dry.copied_blocks.as_ref())
        .map(|copied_blocks| copied_blocks.min_tokens)
        .unwrap_or(100)
}

pub fn rust_dry_copied_blocks_enabled(config: &Config) -> bool {
    config
        .rust
        .as_ref()
        .and_then(|rust| rust.dry.as_ref())
        .and_then(|dry| dry.copied_blocks.as_ref())
        .map(|copied_blocks| copied_blocks.enabled)
        .unwrap_or(true)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_weak_rust_coverage() {
        let error = parse("rust:\n  coverage:\n    threshold: 40\n").unwrap_err();
        assert!(error.to_string().contains("at least 85"));
    }

    #[test]
    fn rejects_unknown_rust_keys() {
        let error = parse("rust:\n  surprise: true\n").unwrap_err();
        assert!(error.to_string().contains("unknown field"));
    }

    #[test]
    fn rejects_malformed_python_shapes() {
        let scalar = parse("python: 8\n").unwrap_err();
        assert!(scalar.to_string().contains("python must be a mapping"));
        let entries = parse("python:\n  typecheck:\n    demotions: yes\n").unwrap_err();
        assert!(
            entries
                .to_string()
                .contains("python.typecheck.demotions must be a sequence")
        );
    }

    #[test]
    fn rejects_unknown_python_keys() {
        let error = parse("python:\n  made_up: true\n").unwrap_err();
        assert!(
            error
                .to_string()
                .contains("python.made_up is not supported")
        );
        let nested = parse("python:\n  typecheck:\n    demotions:\n      - rule: deprecated\n        made_up: true\n")
            .unwrap_err();
        assert!(
            nested
                .to_string()
                .contains("python.typecheck.demotions[0].made_up is not supported")
        );
        let exclude = parse(
            "python:\n  coverage:\n    exclude:\n      - pattern: generated/**\n        made_up: true\n",
        )
        .unwrap_err();
        assert!(
            exclude
                .to_string()
                .contains("python.coverage.exclude[0].made_up is not supported")
        );
    }

    #[test]
    fn parses_full_rust_config() {
        let config = parse(
            r#"
go:
  ignored: true
typescript:
  ignored: true
python:
  complexity:
    max: 8
rules:
  repo.readme-required:
    severity: warn
rust:
  coverage:
    threshold: 90
    paths:
      - crates
    exclude:
      - target/**
  complexity:
    cognitive_max: 7
  targets:
    - crates
  exclude:
    - generated/**
  dry:
    max_findings: 0
    paths:
      - crates/slophammer-cli/src/rust_rules
    exclude:
      - tests/**
    copied_blocks:
      enabled: true
      min_tokens: 50
  unsafe:
    policy: allow_documented
    allow:
      - path: crates/slophammer-cli/src/rust_rules/ffi.rs
        reason: ffi boundary
  mutation:
    targets:
      - crates/slophammer-cli/src/rust_rules
    exclude:
      - generated/**
  dependency_boundaries:
    - from: crates/slophammer-cli
      allow: []
"#,
        )
        .expect("config");
        assert_eq!(rust_coverage_threshold(&config), 90);
        assert_eq!(rust_targets(&config), vec!["crates"]);
        assert_eq!(
            rust_dry_paths(&config),
            vec!["crates/slophammer-cli/src/rust_rules".to_owned()]
        );
        assert_eq!(rust_dry_min_tokens(&config), 50);
        assert_eq!(
            config
                .rules
                .get("repo.readme-required")
                .and_then(|rule| rule.severity),
            Some(RuleSeverity::Warn)
        );
    }

    #[test]
    fn rejects_weak_complexity_and_dry_budget() {
        let complexity = parse("rust:\n  complexity:\n    cognitive_max: 9\n").unwrap_err();
        assert!(complexity.to_string().contains("at most 8"));

        let dry = parse("rust:\n  dry:\n    max_findings: 1\n").unwrap_err();
        assert!(dry.to_string().contains("must be 0"));
    }

    #[test]
    fn rejects_unknown_rule_config_keys() {
        let error = parse(
            r#"
rules:
  repo.readme-required:
    surprise: true
"#,
        )
        .unwrap_err();
        assert!(error.to_string().contains("unknown field"));
    }

    #[test]
    fn disabled_rules_require_a_reason() {
        for content in [
            "rules:\n  repo.readme-required:\n    disabled: true\n",
            "rules:\n  repo.readme-required:\n    disabled: true\n    reason: ' '\n",
        ] {
            let error = parse(content).unwrap_err();
            assert!(error.to_string().contains("reason is required"));
        }
    }

    #[test]
    fn disabled_rules_require_a_boolean() {
        let error =
            parse("rules:\n  repo.readme-required:\n    disabled: 'true'\n    reason: test\n")
                .unwrap_err();
        assert!(error.to_string().contains("invalid type"));
    }
}
