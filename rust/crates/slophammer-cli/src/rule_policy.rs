use crate::config::Config;
use crate::core::{Finding, Severity};
use std::collections::BTreeSet;

/// Owns rule selection, disablement, and effective severity for one check.
pub struct RulePolicy<'a> {
    config: &'a Config,
    selected: Option<BTreeSet<String>>,
    honor_disabled: bool,
}

impl<'a> RulePolicy<'a> {
    pub fn for_check(config: &'a Config, only_rule_ids: &[String]) -> Self {
        Self::new(config, only_rule_ids, true)
    }

    pub fn for_direct(config: &'a Config, rule_ids: &[String]) -> Self {
        Self::new(config, rule_ids, false)
    }

    fn new(config: &'a Config, rule_ids: &[String], honor_disabled: bool) -> Self {
        Self {
            config,
            selected: (!rule_ids.is_empty()).then(|| rule_ids.iter().cloned().collect()),
            honor_disabled,
        }
    }

    pub fn active(&self, rule_id: &str) -> bool {
        let selected = self
            .selected
            .as_ref()
            .is_none_or(|selected| selected.contains(rule_id));
        let enabled = !self.honor_disabled
            || self
                .config
                .rules
                .get(rule_id)
                .is_none_or(|rule| !rule.disabled);
        selected && enabled
    }

    pub fn severity(&self, rule_id: &str, fallback: Severity) -> Severity {
        self.config
            .rules
            .get(rule_id)
            .and_then(|rule| rule.severity)
            .map(Severity::from)
            .unwrap_or(fallback)
    }

    pub fn admit(&self, mut finding: Finding) -> Option<Finding> {
        if !self.active(&finding.rule_id) {
            return None;
        }
        finding.severity = self.severity(&finding.rule_id, finding.severity);
        Some(finding)
    }

    pub fn admit_all(&self, findings: Vec<Finding>) -> Vec<Finding> {
        findings
            .into_iter()
            .filter_map(|finding| self.admit(finding))
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{RuleConfig, RuleSeverity};
    use std::collections::BTreeMap;

    const README_RULE: &str = "repo.readme-required";

    #[test]
    fn absent_and_false_disablement_stay_active() {
        assert!(RulePolicy::for_check(&Config::default(), &[]).active(README_RULE));
        let config = config_with_rules([(README_RULE, RuleConfig::default())]);
        assert!(RulePolicy::for_check(&config, &[]).active(README_RULE));
    }

    #[test]
    fn selection_intersects_with_disablement() {
        let config = config_with_rules([(
            README_RULE,
            RuleConfig {
                disabled: true,
                reason: Some("not required here".to_owned()),
                ..RuleConfig::default()
            },
        )]);
        let selected = [README_RULE.to_owned()];
        let policy = RulePolicy::for_check(&config, &selected);
        assert!(!policy.active(README_RULE));
        assert!(!policy.active("repo.agents-required"));
    }

    #[test]
    fn admission_drops_disabled_and_applies_enabled_severity() {
        let config = config_with_rules([
            (
                README_RULE,
                RuleConfig {
                    disabled: true,
                    reason: Some("not required here".to_owned()),
                    severity: Some(RuleSeverity::Warn),
                    ..RuleConfig::default()
                },
            ),
            (
                "repo.agents-required",
                RuleConfig {
                    severity: Some(RuleSeverity::Warn),
                    ..RuleConfig::default()
                },
            ),
        ]);
        let admitted = RulePolicy::for_check(&config, &[])
            .admit_all(vec![finding(README_RULE), finding("repo.agents-required")]);
        assert_eq!(admitted.len(), 1);
        assert_eq!(admitted[0].rule_id, "repo.agents-required");
        assert_eq!(admitted[0].severity, Severity::Warn);
    }

    fn config_with_rules<const N: usize>(rules: [(&str, RuleConfig); N]) -> Config {
        Config {
            rules: rules
                .into_iter()
                .map(|(rule_id, config)| (rule_id.to_owned(), config))
                .collect::<BTreeMap<_, _>>(),
            rust: None,
        }
    }

    fn finding(rule_id: &str) -> Finding {
        Finding {
            rule_id: rule_id.to_owned(),
            severity: Severity::Error,
            path: "README.md".to_owned(),
            message: "missing".to_owned(),
            baselined: None,
        }
    }
}
