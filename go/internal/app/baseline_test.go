package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/rules"
)

func baselineFinding(ruleID string, path string) rules.Finding {
	return rules.Finding{RuleID: ruleID, Severity: rules.SeverityError, Path: path, Message: "missing"}
}

func writeBaselineFixture(t *testing.T, root string, entries ...baselineEntry) {
	t.Helper()
	content, err := json.Marshal(baselineFile{Version: 1, Findings: entries})
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, baselineFileName), content, 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
}

func TestBaselinedFindingsStopAffectingOK(t *testing.T) {
	root := t.TempDir()
	writeBaselineFixture(t, root, baselineEntry{RuleID: "repo.readme-required", Path: "README.md"})
	report := rules.NewReport([]rules.Finding{baselineFinding("repo.readme-required", "README.md")})

	if err := applyBaselineCheck(root, &report); err != nil {
		t.Fatalf("applyBaselineCheck returned error: %v", err)
	}
	if !report.OK || !report.Findings[0].Baselined {
		t.Fatalf("report = %#v", report)
	}
}

func TestNewFindingsKeepFailing(t *testing.T) {
	root := t.TempDir()
	writeBaselineFixture(t, root, baselineEntry{RuleID: "repo.readme-required", Path: "README.md"})
	report := rules.NewReport([]rules.Finding{
		baselineFinding("repo.readme-required", "README.md"),
		baselineFinding("repo.agents-required", "AGENTS.md"),
	})

	if err := applyBaselineCheck(root, &report); err != nil {
		t.Fatalf("applyBaselineCheck returned error: %v", err)
	}
	if report.OK {
		t.Fatal("report.OK = true, want false")
	}
	if got := baselineDebtLine(report); got != "1 findings baselined; 1 new\n" {
		t.Fatalf("debt line = %q", got)
	}
}

func TestStaleBaselineEntriesAreAnError(t *testing.T) {
	root := t.TempDir()
	writeBaselineFixture(t, root, baselineEntry{RuleID: "repo.readme-required", Path: "README.md"})
	report := rules.NewReport(nil)

	err := applyBaselineCheck(root, &report)

	if err == nil || !strings.Contains(err.Error(), "resolved findings") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "repo.readme-required at README.md") {
		t.Fatalf("error = %v", err)
	}
}

func TestMissingBaselineFileIsAnError(t *testing.T) {
	report := rules.NewReport(nil)

	err := applyBaselineCheck(t.TempDir(), &report)

	if err == nil || err.Error() != "baseline file slophammer-baseline.json is missing" {
		t.Fatalf("error = %v", err)
	}
}

func TestInvalidBaselineFilesAreErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "parse", content: "not json", want: "baseline parse failed"},
		{name: "version", content: `{"version": 2, "findings": []}`, want: "baseline version must be 1"},
		{name: "unknown key", content: `{"version": 1, "findings": [], "surprise": true}`, want: "baseline parse failed"},
		{name: "unknown entry key", content: `{"version": 1, "findings": [{"rule_id": "a", "path": "b", "message": "x"}]}`, want: "baseline parse failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, baselineFileName), []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write baseline: %v", err)
			}
			report := rules.NewReport(nil)

			err := applyBaselineCheck(root, &report)

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestWriteBaselineRefusesMalformedExistingBaseline(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, baselineFileName), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	report := rules.NewReport([]rules.Finding{baselineFinding("repo.readme-required", "README.md")})

	_, err := writeBaselineFile(root, report)

	if err == nil || !strings.Contains(err.Error(), "baseline parse failed") {
		t.Fatalf("error = %v, want parse failure instead of a silent rewrite", err)
	}
}

func TestWriteBaselineRefusesSupersets(t *testing.T) {
	root := t.TempDir()
	writeBaselineFixture(t, root, baselineEntry{RuleID: "repo.readme-required", Path: "README.md"})
	report := rules.NewReport([]rules.Finding{
		baselineFinding("repo.readme-required", "README.md"),
		baselineFinding("repo.agents-required", "AGENTS.md"),
	})

	_, err := writeBaselineFile(root, report)

	if err == nil || !strings.Contains(err.Error(), "grow the baseline") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteBaselineRecordsAndShrinks(t *testing.T) {
	root := t.TempDir()
	first := rules.NewReport([]rules.Finding{
		baselineFinding("repo.readme-required", "README.md"),
		baselineFinding("repo.agents-required", "AGENTS.md"),
		baselineFinding("repo.agents-required", "AGENTS.md"),
	})

	summary, err := writeBaselineFile(root, first)
	if err != nil {
		t.Fatalf("writeBaselineFile returned error: %v", err)
	}
	if !strings.Contains(summary, "baseline written: 2 finding(s)\n") ||
		!strings.Contains(summary, "added: repo.agents-required at AGENTS.md\n") {
		t.Fatalf("summary = %q", summary)
	}
	assertBaselineFileContent(t, root)

	second := rules.NewReport([]rules.Finding{baselineFinding("repo.agents-required", "AGENTS.md")})
	summary, err = writeBaselineFile(root, second)
	if err != nil {
		t.Fatalf("writeBaselineFile returned error: %v", err)
	}
	if !strings.Contains(summary, "removed: repo.readme-required at README.md\n") {
		t.Fatalf("summary = %q", summary)
	}
}

func assertBaselineFileContent(t *testing.T, root string) {
	t.Helper()
	// #nosec G304 -- the test reads the baseline from its own temp directory.
	content, err := os.ReadFile(filepath.Join(root, baselineFileName))
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	if !strings.HasSuffix(string(content), "\n") {
		t.Fatal("baseline file is missing the trailing newline")
	}
	var file baselineFile
	if err := json.Unmarshal(content, &file); err != nil {
		t.Fatalf("unmarshal baseline: %v", err)
	}
	want := []baselineEntry{
		{RuleID: "repo.agents-required", Path: "AGENTS.md"},
		{RuleID: "repo.readme-required", Path: "README.md"},
	}
	if file.Version != 1 || len(file.Findings) != len(want) {
		t.Fatalf("baseline = %#v", file)
	}
	for i, entry := range want {
		if file.Findings[i] != entry {
			t.Fatalf("baseline entry[%d] = %#v, want %#v", i, file.Findings[i], entry)
		}
	}
}

func TestCheckBaselineModeMarksFindingsAndExitsOK(t *testing.T) {
	root := t.TempDir()
	writeBaselineFixture(t, root,
		baselineEntry{RuleID: "repo.readme-required", Path: "README.md"},
		baselineEntry{RuleID: "repo.agents-required", Path: "AGENTS.md"},
		baselineEntry{RuleID: "repo.ci-required", Path: ".github/workflows"},
	)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: root, Format: "text", Baseline: BaselineCheck}, &out, &errOut)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if !strings.Contains(out.String(), "3 findings baselined; 0 new\n") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckBaselineModeKeepsFailingOnNewFindings(t *testing.T) {
	root := t.TempDir()
	writeBaselineFixture(t, root,
		baselineEntry{RuleID: "repo.readme-required", Path: "README.md"},
		baselineEntry{RuleID: "repo.agents-required", Path: "AGENTS.md"},
	)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: root, Format: "json", Baseline: BaselineCheck}, &out, &errOut)

	if code != ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	if !strings.Contains(out.String(), `"baselined": true`) {
		t.Fatalf("stdout = %q", out.String())
	}
	if strings.Contains(out.String(), "findings baselined;") {
		t.Fatalf("json output should not carry the text debt line: %q", out.String())
	}
}

func TestCheckBaselineModeFailsWithoutBaselineFile(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: t.TempDir(), Format: "text", Baseline: BaselineCheck}, &out, &errOut)

	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "check failed: baseline file slophammer-baseline.json is missing") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckBaselineWriteModeWritesFile(t *testing.T) {
	root := t.TempDir()

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: root, Format: "text", Baseline: BaselineWrite}, &out, &errOut)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if !strings.Contains(out.String(), "baseline written: 3 finding(s)\n") {
		t.Fatalf("stdout = %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, baselineFileName)); err != nil {
		t.Fatalf("baseline file missing: %v", err)
	}
}

func TestCheckBaselineWriteModeRefusesSupersets(t *testing.T) {
	root := t.TempDir()
	writeBaselineFixture(t, root, baselineEntry{RuleID: "repo.readme-required", Path: "README.md"})

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: root, Format: "text", Baseline: BaselineWrite}, &out, &errOut)

	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "grow the baseline") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestWriteBaselineReportsUnwritableRoot(t *testing.T) {
	report := rules.NewReport([]rules.Finding{baselineFinding("repo.readme-required", "README.md")})

	_, err := writeBaselineFile(filepath.Join(t.TempDir(), "missing"), report)

	if err == nil || !strings.Contains(err.Error(), "baseline write failed") {
		t.Fatalf("error = %v", err)
	}
}
