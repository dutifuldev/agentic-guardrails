package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/rules"
)

func TestWriteJSON(t *testing.T) {
	var out bytes.Buffer
	input := rules.Report{
		OK: false,
		Findings: []rules.Finding{{
			RuleID:   "repo.agents-required",
			Severity: rules.SeverityError,
			Path:     "AGENTS.md",
			Message:  "AGENTS.md is required",
		}},
	}

	if err := WriteJSON(&out, input); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	var decoded rules.Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(decoded.Findings) != 1 || decoded.Findings[0].RuleID != "repo.agents-required" {
		t.Fatalf("decoded report = %#v", decoded)
	}
}

func TestWriteText(t *testing.T) {
	var out bytes.Buffer
	input := rules.Report{
		OK: false,
		Findings: []rules.Finding{{
			RuleID:   "repo.readme-required",
			Severity: rules.SeverityError,
			Path:     "README.md",
			Message:  "README.md is required",
		}},
	}

	if err := WriteText(&out, input); err != nil {
		t.Fatalf("WriteText returned error: %v", err)
	}
	if !strings.Contains(out.String(), "repo.readme-required") {
		t.Fatalf("text output missing rule ID: %q", out.String())
	}
}

func TestWriteTextOK(t *testing.T) {
	var out bytes.Buffer
	if err := WriteText(&out, rules.Report{OK: true}); err != nil {
		t.Fatalf("WriteText returned error: %v", err)
	}
	if strings.TrimSpace(out.String()) != "OK: no findings" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestWriteTextIncludesScopeCoverage(t *testing.T) {
	var out bytes.Buffer
	input := rules.Report{OK: true, Scope: &rules.ScopeCoverage{Scanned: 42, ProductionFiles: 45}}

	if err := WriteText(&out, input); err != nil {
		t.Fatalf("WriteText returned error: %v", err)
	}
	if !strings.Contains(out.String(), "scope: scanned 42 of 45 production files\n") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestWriteJSONIncludesScopeAndBaselined(t *testing.T) {
	var out bytes.Buffer
	input := rules.Report{
		OK: true,
		Findings: []rules.Finding{{
			RuleID:    "repo.readme-required",
			Severity:  rules.SeverityError,
			Path:      "README.md",
			Message:   "README.md is required",
			Baselined: true,
		}},
		Scope: &rules.ScopeCoverage{Scanned: 1, ProductionFiles: 2},
	}

	if err := WriteJSON(&out, input); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"baselined": true`) {
		t.Fatalf("output missing baselined: %q", out.String())
	}
	if !strings.Contains(out.String(), `"scanned": 1`) || !strings.Contains(out.String(), `"production_files": 2`) {
		t.Fatalf("output missing scope: %q", out.String())
	}
}

func TestWriteJSONOmitsScopeAndBaselinedByDefault(t *testing.T) {
	var out bytes.Buffer
	input := rules.Report{
		OK: false,
		Findings: []rules.Finding{{
			RuleID:   "repo.readme-required",
			Severity: rules.SeverityError,
			Path:     "README.md",
			Message:  "README.md is required",
		}},
	}

	if err := WriteJSON(&out, input); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	if strings.Contains(out.String(), "baselined") || strings.Contains(out.String(), "scope") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestWriteSARIFMarksBaselinedFindingsSuppressed(t *testing.T) {
	var out bytes.Buffer
	input := rules.Report{
		OK: true,
		Findings: []rules.Finding{
			{RuleID: "repo.readme-required", Severity: rules.SeverityError, Path: "README.md", Message: "missing", Baselined: true},
			{RuleID: "repo.agents-required", Severity: rules.SeverityError, Path: "AGENTS.md", Message: "missing"},
		},
		Scope: &rules.ScopeCoverage{Scanned: 1, ProductionFiles: 1},
	}

	if err := WriteSARIF(&out, input); err != nil {
		t.Fatalf("WriteSARIF returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"suppressions"`) || !strings.Contains(out.String(), `"kind": "external"`) {
		t.Fatalf("SARIF output missing suppressions: %s", out.String())
	}
	if strings.Contains(out.String(), "scope") {
		t.Fatalf("SARIF output should ignore scope: %s", out.String())
	}
	if count := strings.Count(out.String(), `"suppressions"`); count != 1 {
		t.Fatalf("suppressions count = %d, want 1", count)
	}
}

func TestWriteSARIF(t *testing.T) {
	var out bytes.Buffer
	input := rules.Report{
		OK: false,
		Findings: []rules.Finding{{
			RuleID:   "repo.agents-required",
			Severity: rules.SeverityError,
			Path:     "AGENTS.md",
			Message:  "AGENTS.md is required",
		}},
	}

	if err := WriteSARIF(&out, input); err != nil {
		t.Fatalf("WriteSARIF returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if decoded["version"] != "2.1.0" {
		t.Fatalf("version = %#v, want 2.1.0", decoded["version"])
	}
	if !strings.Contains(out.String(), `"ruleId": "repo.agents-required"`) {
		t.Fatalf("SARIF output missing rule ID: %s", out.String())
	}
}
