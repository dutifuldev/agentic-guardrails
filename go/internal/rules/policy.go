package rules

import "github.com/osolmaz/slophammer/go/internal/config"

// Policy owns rule selection, disablement, and effective severity for one check.
type Policy struct {
	config     config.Config
	selected   map[string]bool
	restricted bool
}

// NewPolicy builds the policy for check and check --execute.
func NewPolicy(cfg config.Config, onlyRuleIDs []string) Policy {
	selected := make(map[string]bool, len(onlyRuleIDs))
	for _, ruleID := range onlyRuleIDs {
		selected[ruleID] = true
	}
	return Policy{
		config:     cfg,
		selected:   selected,
		restricted: len(onlyRuleIDs) > 0,
	}
}

// Active reports whether the rule can run and emit findings.
func (p Policy) Active(ruleID string) bool {
	if p.restricted && !p.selected[ruleID] {
		return false
	}
	rule, configured := p.config.Rules[ruleID]
	return !configured || !rule.Disabled
}

// Config returns the validated config used by configured rule evaluators.
func (p Policy) Config() config.Config {
	return p.config
}

// Severity returns the configured severity for an active rule.
func (p Policy) Severity(ruleID string, fallback Severity) Severity {
	rule, configured := p.config.Rules[ruleID]
	if !configured || rule.Severity == "" {
		return fallback
	}
	return Severity(rule.Severity)
}

// Admit applies policy to one finding.
func (p Policy) Admit(finding Finding) (Finding, bool) {
	if !p.Active(finding.RuleID) {
		return Finding{}, false
	}
	finding.Severity = p.Severity(finding.RuleID, finding.Severity)
	return finding, true
}

// AdmitAll applies policy to findings without changing the input slice.
func (p Policy) AdmitAll(findings []Finding) []Finding {
	admitted := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		if effective, ok := p.Admit(finding); ok {
			admitted = append(admitted, effective)
		}
	}
	return admitted
}
