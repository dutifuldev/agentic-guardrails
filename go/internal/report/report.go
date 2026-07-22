package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/osolmaz/slophammer/go/internal/rules"
)

func WriteJSON(out io.Writer, report rules.Report) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteSARIF(out io.Writer, report rules.Report) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(sarifReport(report))
}

func WriteText(out io.Writer, report rules.Report) error {
	if err := writeTextFindings(out, report); err != nil {
		return err
	}
	if report.Scope == nil {
		return nil
	}
	_, err := fmt.Fprintf(out, "scope: scanned %d of %d production files\n", report.Scope.Scanned, report.Scope.ProductionFiles)
	return err
}

func writeTextFindings(out io.Writer, report rules.Report) error {
	if report.OK {
		_, err := fmt.Fprintln(out, "OK: no findings")
		return err
	}
	for _, finding := range report.Findings {
		if _, err := fmt.Fprintf(out, "%s %s %s: %s\n", finding.Severity, finding.RuleID, finding.Path, finding.Message); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "\n%d finding(s)\n", len(report.Findings))
	return err
}

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name  string      `json:"name"`
	Rules []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	ShortDescription sarifMessage `json:"shortDescription"`
}

type sarifResult struct {
	RuleID       string             `json:"ruleId"`
	Level        string             `json:"level"`
	Message      sarifMessage       `json:"message"`
	Locations    []sarifLocation    `json:"locations,omitempty"`
	Suppressions []sarifSuppression `json:"suppressions,omitempty"`
}

type sarifSuppression struct {
	Kind string `json:"kind"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

func sarifReport(report rules.Report) sarifLog {
	return sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:  "slophammer",
					Rules: sarifRules(report.Findings),
				},
			},
			Results: sarifResults(report.Findings),
		}},
	}
}

func sarifRules(findings []rules.Finding) []sarifRule {
	seen := map[string]struct{}{}
	out := make([]sarifRule, 0)
	for _, finding := range findings {
		if _, ok := seen[finding.RuleID]; ok {
			continue
		}
		seen[finding.RuleID] = struct{}{}
		out = append(out, sarifRule{
			ID:               finding.RuleID,
			ShortDescription: sarifMessage{Text: finding.Message},
		})
	}
	return out
}

func sarifResults(findings []rules.Finding) []sarifResult {
	results := make([]sarifResult, 0, len(findings))
	for _, finding := range findings {
		results = append(results, sarifResult{
			RuleID:       finding.RuleID,
			Level:        sarifLevel(finding.Severity),
			Message:      sarifMessage{Text: finding.Message},
			Locations:    sarifLocations(finding.Path),
			Suppressions: sarifSuppressions(finding.Baselined),
		})
	}
	return results
}

func sarifLevel(severity rules.Severity) string {
	switch severity {
	case rules.SeverityWarn:
		return "warning"
	default:
		return "error"
	}
}

// sarifSuppressions maps baselined findings to SARIF suppressions, which
// code scanning understands natively.
func sarifSuppressions(baselined bool) []sarifSuppression {
	if !baselined {
		return nil
	}
	return []sarifSuppression{{Kind: "external"}}
}

func sarifLocations(filePath string) []sarifLocation {
	if filePath == "" {
		return nil
	}
	return []sarifLocation{{
		PhysicalLocation: sarifPhysicalLocation{
			ArtifactLocation: sarifArtifactLocation{URI: filePath},
			Region:           sarifRegion{StartLine: 1},
		},
	}}
}
