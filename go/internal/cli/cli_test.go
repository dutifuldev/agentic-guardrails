package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/app"
)

func TestRunHelp(t *testing.T) {
	result := runCLI(t, "help")

	if result.code != app.ExitOK {
		t.Fatalf("code = %d, want %d", result.code, app.ExitOK)
	}
	if !strings.Contains(result.stdout, "slophammer-go check") {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func TestRunCheckParsesFormatAfterPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Test\n")
	writeFile(t, root, "AGENTS.md", "# Agents\n\nRun `go test ./...` before finishing.\n")
	writeFile(t, root, ".github/workflows/ci.yml", "name: CI\n")

	result := runCLI(t, "check", root, "--format", "json")

	if result.code != app.ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", result.code, app.ExitOK, result.stderr)
	}
	if !strings.Contains(result.stdout, `"ok": true`) {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func TestRunCheckParsesSARIFFormat(t *testing.T) {
	result := runCLI(t, "check", t.TempDir(), "--format", "sarif")

	if result.code != app.ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", result.code, app.ExitFindings, result.stderr)
	}
	if !strings.Contains(result.stdout, `"version": "2.1.0"`) {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func TestParseCheckArgsAllowsExecute(t *testing.T) {
	var errOut bytes.Buffer
	options, ok := parseCheckArgs([]string{"/repo", "--format", "json", "--execute", "--coverage-profile", "coverage.out"}, &errOut)

	if !ok {
		t.Fatalf("ok = false; stderr=%q", errOut.String())
	}
	if options.Root != "/repo" || options.Format != "json" || !options.Execute || options.CoverageProfile != "coverage.out" {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseCheckArgsAllowsJSONShorthand(t *testing.T) {
	var errOut bytes.Buffer
	options, ok := parseCheckArgs([]string{"/repo", "--json"}, &errOut)

	if !ok {
		t.Fatalf("ok = false; stderr=%q", errOut.String())
	}
	if options.Format != "json" {
		t.Fatalf("options.Format = %q, want json", options.Format)
	}
}

func TestParseCheckArgsParsesBaselineModes(t *testing.T) {
	tests := []struct {
		name string
		flag string
		want app.BaselineMode
	}{
		{name: "check", flag: "--baseline", want: app.BaselineCheck},
		{name: "write", flag: "--baseline-write", want: app.BaselineWrite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			options, ok := parseCheckArgs([]string{"/repo", tt.flag}, &errOut)

			if !ok {
				t.Fatalf("ok = false; stderr=%q", errOut.String())
			}
			if options.Baseline != tt.want {
				t.Fatalf("options.Baseline = %v, want %v", options.Baseline, tt.want)
			}
		})
	}
}

func TestParseCheckArgsRejectsCombinedBaselineFlags(t *testing.T) {
	assertCLIError(t, []string{"check", ".", "--baseline", "--baseline-write"}, "mutually exclusive")
	assertCLIError(t, []string{"check", ".", "--baseline-write", "--baseline"}, "mutually exclusive")
}

func TestRunCheckBaselineWriteRoundTrip(t *testing.T) {
	root := t.TempDir()

	written := runCLI(t, "check", root, "--baseline-write")
	if written.code != app.ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", written.code, app.ExitOK, written.stderr)
	}
	if !strings.Contains(written.stdout, "baseline written: 3 finding(s)") {
		t.Fatalf("stdout = %q", written.stdout)
	}

	checked := runCLI(t, "check", root, "--baseline")
	if checked.code != app.ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", checked.code, app.ExitOK, checked.stderr)
	}
	if !strings.Contains(checked.stdout, "3 findings baselined; 0 new") {
		t.Fatalf("stdout = %q", checked.stdout)
	}
}

func TestRunExplain(t *testing.T) {
	result := runCLI(t, "explain", "repo.ci-required")

	if result.code != app.ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", result.code, app.ExitOK, result.stderr)
	}
	if !strings.Contains(result.stdout, "repo.ci-required") {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func TestRunExplainRejectsWrongArity(t *testing.T) {
	assertCLIError(t, []string{"explain"}, "usage: slophammer-go explain")
}

func TestRunRules(t *testing.T) {
	result := runCLI(t, "rules")

	if result.code != app.ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", result.code, app.ExitOK, result.stderr)
	}
	if !strings.Contains(result.stdout, "repo.readme-required") ||
		!strings.Contains(result.stdout, "go.dry-required") {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func TestRunRulesJSON(t *testing.T) {
	result := runCLI(t, "rules", "--format", "json")

	if result.code != app.ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", result.code, app.ExitOK, result.stderr)
	}
	if !strings.Contains(result.stdout, `"id": "repo.readme-required"`) ||
		!strings.Contains(result.stdout, `"id": "go.dry-required"`) {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func TestRunRulesRejectsArgs(t *testing.T) {
	assertCLIError(t, []string{"rules", "repo.readme-required"}, "usage: slophammer-go rules")
}

func TestRunGoRejectsMissingSubcommand(t *testing.T) {
	assertCLIError(t, []string{"go"}, "slophammer-go dry")
}

func TestRunGoRejectsUnknownSubcommand(t *testing.T) {
	assertCLIError(t, []string{"go", "wat"}, "unknown go command")
}

func TestParseGoDryArgs(t *testing.T) {
	var errOut bytes.Buffer
	options, ok := parseGoDryArgs([]string{"/repo", "--max-candidates", "12", "--show-report", "--format", "text"}, &errOut)

	if !ok {
		t.Fatalf("ok = false; stderr=%q", errOut.String())
	}
	if options.Root != "/repo" || options.MaximumCandidates != 12 || !options.MaximumSet || !options.ShowReport || options.Format != "text" {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseGoDryArgsRejectsInvalidFormat(t *testing.T) {
	var errOut bytes.Buffer

	_, ok := parseGoDryArgs([]string{"/repo", "--format", "xml"}, &errOut)

	if ok {
		t.Fatal("ok = true, want false")
	}
	if !strings.Contains(errOut.String(), "unsupported go dry format") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestParseGoCRAPArgs(t *testing.T) {
	var errOut bytes.Buffer
	options, ok := parseGoCRAPArgs([]string{"/repo", "--max-score", "25.5", "--coverage-profile", "coverage.out"}, &errOut)

	if !ok {
		t.Fatalf("ok = false; stderr=%q", errOut.String())
	}
	if options.Root != "/repo" || options.MaximumScore != 25.5 || !options.MaximumSet || options.CoverageProfile != "coverage.out" {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseGoCoverageArgs(t *testing.T) {
	var errOut bytes.Buffer
	options, ok := parseGoCoverageArgs([]string{"/repo", "--threshold", "85.5", "--profile", "coverage.out"}, &errOut)

	if !ok {
		t.Fatalf("ok = false; stderr=%q", errOut.String())
	}
	if options.Root != "/repo" || options.Threshold != 85.5 || !options.ThresholdSet || options.CoverageProfile != "coverage.out" {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseGoMutationArgs(t *testing.T) {
	var errOut bytes.Buffer
	options, ok := parseGoMutationArgs([]string{"/repo", "--target", "main.go", "--scan"}, &errOut)

	if !ok {
		t.Fatalf("ok = false; stderr=%q", errOut.String())
	}
	if options.Root != "/repo" || options.Target != "main.go" || !options.Scan {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseGoMutationArgsAllowsConfigTarget(t *testing.T) {
	var errOut bytes.Buffer
	options, ok := parseGoMutationArgs([]string{"/repo", "--scan"}, &errOut)

	if !ok {
		t.Fatalf("ok = false; stderr=%q", errOut.String())
	}
	if options.Root != "/repo" || options.Target != "" || !options.Scan {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseGoMutationArgsRejectsFlagTarget(t *testing.T) {
	var errOut bytes.Buffer
	_, ok := parseGoMutationArgs([]string{"/repo", "--target", "--scan"}, &errOut)

	if ok {
		t.Fatal("ok = true, want false")
	}
	if !strings.Contains(errOut.String(), "--target requires a file value") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestParseGoToolArgsRejectInvalidNumbers(t *testing.T) {
	tests := []struct {
		name string
		run  func(io.Writer) bool
		want string
	}{
		{
			name: "dry",
			run: func(errOut io.Writer) bool {
				_, ok := parseGoDryArgs([]string{"--max-candidates", "x"}, errOut)
				return ok
			},
			want: "--max-candidates must be a non-negative integer",
		},
		{
			name: "coverage",
			run: func(errOut io.Writer) bool {
				_, ok := parseGoCoverageArgs([]string{"--threshold", "x"}, errOut)
				return ok
			},
			want: "--threshold must be a non-negative number",
		},
		{
			name: "crap",
			run: func(errOut io.Writer) bool {
				_, ok := parseGoCRAPArgs([]string{"--max-score", "-1"}, errOut)
				return ok
			},
			want: "--max-score must be a non-negative number",
		},
		{
			name: "crap NaN",
			run: func(errOut io.Writer) bool {
				_, ok := parseGoCRAPArgs([]string{"--max-score", "NaN"}, errOut)
				return ok
			},
			want: "--max-score must be a non-negative number",
		},
		{
			name: "crap infinity",
			run: func(errOut io.Writer) bool {
				_, ok := parseGoCRAPArgs([]string{"--max-score", "+Inf"}, errOut)
				return ok
			},
			want: "--max-score must be a non-negative number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			if tt.run(&errOut) {
				t.Fatal("ok = true, want false")
			}
			if !strings.Contains(errOut.String(), tt.want) {
				t.Fatalf("stderr = %q", errOut.String())
			}
		})
	}
}

func TestRunCheckRejectsMissingFormatValue(t *testing.T) {
	assertCLIError(t, []string{"check", ".", "--format"}, "--format requires a value")
}

func TestParseCheckArgsAllowsOnly(t *testing.T) {
	var errOut bytes.Buffer
	options, ok := parseCheckArgs([]string{"/repo", "--only", "repo.readme-required", "--only", "repo.ci-required, repo.agents-required"}, &errOut)

	if !ok {
		t.Fatalf("ok = false; stderr=%q", errOut.String())
	}
	want := []string{"repo.readme-required", "repo.ci-required", "repo.agents-required"}
	if len(options.OnlyRuleIDs) != len(want) {
		t.Fatalf("OnlyRuleIDs = %#v, want %#v", options.OnlyRuleIDs, want)
	}
	for i, ruleID := range want {
		if options.OnlyRuleIDs[i] != ruleID {
			t.Fatalf("OnlyRuleIDs = %#v, want %#v", options.OnlyRuleIDs, want)
		}
	}
}

func TestRunCheckRejectsMissingOnlyValue(t *testing.T) {
	assertCLIError(t, []string{"check", ".", "--only"}, "--only requires a value")
}

func TestRunCheckRejectsEmptyOnlyValue(t *testing.T) {
	assertCLIError(t, []string{"check", ".", "--only", " , "}, "--only requires a rule id")
}

func TestRunCheckRejectsUnknownOption(t *testing.T) {
	assertCLIError(t, []string{"check", "--wat", "."}, "unknown check option")
}

func TestRunCheckRejectsDuplicatePath(t *testing.T) {
	assertCLIError(t, []string{"check", ".", ".."}, "exactly one path")
}

func TestRunCheckRejectsMissingPath(t *testing.T) {
	assertCLIError(t, []string{"check"}, "usage: slophammer-go check")
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	assertCLIError(t, []string{"wat"}, "unknown command")
}

func TestRunAcceptsPublicGoSubcommands(t *testing.T) {
	root := t.TempDir()

	result := runCLI(t, "dry", root)

	if result.code != app.ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", result.code, app.ExitOK, result.stderr)
	}
	if !strings.Contains(result.stdout, "DRY candidates: 0") {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func TestRunAgentsInitWritesRefusesAndForces(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "Makefile", "check:\n\t@true\n")

	dryRun := runCLI(t, "agents", "init", root, "--dry-run")
	if dryRun.code != app.ExitOK || !strings.Contains(dryRun.stdout, "make check") {
		t.Fatalf("dry run = %#v", dryRun)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote AGENTS.md: %v", err)
	}

	created := runCLI(t, "agents", "init", root)
	if created.code != app.ExitOK {
		t.Fatalf("created = %#v", created)
	}
	refused := runCLI(t, "agents", "init", root)
	if refused.code != app.ExitError || !strings.Contains(refused.stderr, "--force") {
		t.Fatalf("refused = %#v", refused)
	}
	writeFile(t, root, "AGENTS.md", "old\n")
	forced := runCLI(t, "agents", "init", root, "--force")
	if forced.code != app.ExitOK {
		t.Fatalf("forced = %#v", forced)
	}
	// #nosec G304 -- the path is inside the test-owned temporary directory.
	content, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil || !strings.Contains(string(content), "make check") {
		t.Fatalf("AGENTS.md = %q, err=%v", content, err)
	}
}

func TestRunAgentsCheckAndUsageErrors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Agents\n")
	result := runCLI(t, "agents", "check", root, "--format", "json")
	if result.code != app.ExitFindings || !strings.Contains(result.stdout, "repo.agents-empty") {
		t.Fatalf("result = %#v", result)
	}
	assertCLIError(t, []string{"agents"}, "usage:")
	assertCLIError(t, []string{"agents", "wat"}, "unknown agents command")
	assertCLIError(t, []string{"agents", "check", "--wat"}, "unknown agents check option")
	assertCLIError(t, []string{"agents", "check", ".", ".."}, "exactly one path")
	assertCLIError(t, []string{"agents", "init", "--wat"}, "unknown agents init option")
	assertCLIError(t, []string{"agents", "init", ".", ".."}, "exactly one path")
}

func assertCLIError(t *testing.T, args []string, stderr string) {
	t.Helper()
	result := runCLI(t, args...)
	if result.code != app.ExitError {
		t.Fatalf("code = %d, want %d", result.code, app.ExitError)
	}
	if !strings.Contains(result.stderr, stderr) {
		t.Fatalf("stderr = %q", result.stderr)
	}
}

func runCLI(t *testing.T, args ...string) cliResult {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run(context.Background(), args, &out, &errOut)
	return cliResult{code: code, stdout: out.String(), stderr: errOut.String()}
}

type cliResult struct {
	code   int
	stdout string
	stderr string
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
