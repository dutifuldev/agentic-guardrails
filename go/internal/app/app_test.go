package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/repo"
	"github.com/osolmaz/slophammer/go/internal/rules"
	"github.com/osolmaz/slophammer/go/internal/toolchecks"
)

func TestReplaceAgentsFileReplacesRegularFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(target, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := replaceAgentsFile(target, "new\n"); err != nil {
		t.Fatalf("replaceAgentsFile returned error: %v", err)
	}
	// #nosec G304 -- target is inside a test-owned temporary directory.
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "new\n" {
		t.Fatalf("content = %q, err=%v", content, err)
	}

	absent := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := replaceAgentsFile(absent, "created\n"); err != nil {
		t.Fatalf("replaceAgentsFile absent target returned error: %v", err)
	}
}

func TestReplaceAgentsPathWithBackup(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	temporary := filepath.Join(root, ".temporary")
	if err := os.WriteFile(target, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile target returned error: %v", err)
	}
	if err := os.WriteFile(temporary, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFile temporary returned error: %v", err)
	}
	if err := replaceAgentsPathWithBackup(temporary, target); err != nil {
		t.Fatalf("replaceAgentsPathWithBackup returned error: %v", err)
	}
	// #nosec G304 -- target is inside a test-owned temporary directory.
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "new\n" {
		t.Fatalf("content = %q, err=%v", content, err)
	}

	absentTarget := filepath.Join(root, "created.md")
	absentTemporary := filepath.Join(root, ".temporary-absent")
	if err := os.WriteFile(absentTemporary, []byte("created\n"), 0o600); err != nil {
		t.Fatalf("WriteFile absent temporary returned error: %v", err)
	}
	if err := replaceAgentsPathWithBackup(absentTemporary, absentTarget); err != nil {
		t.Fatalf("replaceAgentsPathWithBackup absent target returned error: %v", err)
	}
}

func TestReplaceAgentsFileRejectsSymlink(t *testing.T) {
	external := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(external, []byte("keep\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	target := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.Symlink(external, target); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}
	if err := replaceAgentsFile(target, "new\n"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("replaceAgentsFile error = %v", err)
	}
	// #nosec G304 -- external is inside a test-owned temporary directory.
	content, err := os.ReadFile(external)
	if err != nil || string(content) != "keep\n" {
		t.Fatalf("external content = %q, err=%v", content, err)
	}
}

func TestCheckReturnsOKForCleanRepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Test\n")
	writeFile(t, root, "AGENTS.md", "# Agents\n\nRun `go test ./...` before finishing.\n")
	writeFile(t, root, ".github/workflows/ci.yml", "name: CI\n")

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: root, Format: "json"}, &out, &errOut)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if !strings.Contains(out.String(), `"ok": true`) {
		t.Fatalf("json output = %q", out.String())
	}
}

func TestCheckMatchesSharedFixtures(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{name: "clean", code: ExitOK},
		{name: "missing-readme", code: ExitFindings},
		{name: "missing-agents", code: ExitFindings},
		{name: "missing-ci", code: ExitFindings},
		{name: "go-clean", code: ExitOK},
		{name: "go-missing-module", code: ExitFindings},
		{name: "go-missing-tests", code: ExitFindings},
		{name: "go-missing-vet", code: ExitFindings},
		{name: "go-missing-lint", code: ExitFindings},
		{name: "go-missing-coverage", code: ExitFindings},
		{name: "go-missing-complexity", code: ExitFindings},
		{name: "go-missing-dry", code: ExitFindings},
		{name: "go-missing-crap", code: ExitFindings},
		{name: "go-missing-mutation", code: ExitFindings},
		{name: "go-bad-dependency", code: ExitFindings},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkSharedFixture(t, tt.name, tt.code)
		})
	}
}

func TestCheckReportsScopeCoverageWhenScopeConfigured(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Test\n")
	writeFile(t, root, "AGENTS.md", "# Agents\n\nRun `go test ./...` before finishing.\n")
	writeFile(t, root, ".github/workflows/ci.yml", "name: CI\non: [push]\njobs:\n  ci:\n    steps:\n      - run: slophammer-go check .\n")
	writeFile(t, root, "internal/app.go", "package app\n")
	writeFile(t, root, "tools/extra.go", "package tools\n")
	writeFile(t, root, "slophammer.yml", "go:\n  targets:\n    - internal\n")

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: root, Format: "json"}, &out, &errOut)

	if code != ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	report := unmarshalReport(t, out.Bytes(), "scope")
	if report.Scope == nil || report.Scope.Scanned != 1 || report.Scope.ProductionFiles != 2 {
		t.Fatalf("scope = %#v", report.Scope)
	}
	assertFinding(t, report, rules.GoScopeIncompleteRuleID)

	var text bytes.Buffer
	if code := Check(context.Background(), CheckOptions{Root: root, Format: "text"}, &text, &errOut); code != ExitFindings {
		t.Fatalf("text code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	if !strings.Contains(text.String(), "scope: scanned 1 of 2 production files\n") {
		t.Fatalf("text output = %q", text.String())
	}
}

func TestCheckOmitsScopeCoverageWithoutConfiguredScope(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: t.TempDir(), Format: "json"}, &out, &errOut)

	if code != ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	if strings.Contains(out.String(), `"scope"`) {
		t.Fatalf("json output = %q", out.String())
	}
}

func TestCheckReturnsFindingsForMissingFiles(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: t.TempDir(), Format: "text"}, &out, &errOut)

	if code != ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	if !strings.Contains(out.String(), "repo.agents-required") {
		t.Fatalf("text output = %q", out.String())
	}
}

func TestCheckOnlyFiltersFindings(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	options := CheckOptions{Root: t.TempDir(), Format: "json", OnlyRuleIDs: []string{"repo.readme-required"}}
	code := Check(context.Background(), options, &out, &errOut)

	if code != ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	if !strings.Contains(out.String(), "repo.readme-required") {
		t.Fatalf("json output = %q", out.String())
	}
	if strings.Contains(out.String(), "repo.agents-required") {
		t.Fatalf("json output should not include filtered rules: %q", out.String())
	}
}

func TestCheckOnlyPassesWhenFocusedRuleIsClean(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Test\n")

	var out bytes.Buffer
	var errOut bytes.Buffer
	options := CheckOptions{Root: root, Format: "json", OnlyRuleIDs: []string{"repo.readme-required"}}
	code := Check(context.Background(), options, &out, &errOut)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
}

func TestCheckRejectsUnknownOnlyRule(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	options := CheckOptions{Root: t.TempDir(), Format: "json", OnlyRuleIDs: []string{"repo.not-a-rule"}}
	code := Check(context.Background(), options, &out, &errOut)

	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "unknown rule: repo.not-a-rule") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckRejectsUnknownFormat(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: t.TempDir(), Format: "xml"}, &out, &errOut)

	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "unsupported format") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckWritesSARIF(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: t.TempDir(), Format: "sarif"}, &out, &errOut)

	if code != ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	if !strings.Contains(out.String(), `"version": "2.1.0"`) ||
		!strings.Contains(out.String(), `"ruleId": "repo.agents-required"`) {
		t.Fatalf("SARIF output = %q", out.String())
	}
}

func TestCheckRejectsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "slophammer.yml", "rules:\n  repo.readme-required:\n    severity: info\n")

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: root, Format: "text"}, &out, &errOut)

	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "config failed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckExecuteAddsToolFindings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Test\n")
	writeFile(t, root, "AGENTS.md", "# Agents\n\nRun `go test ./...` before finishing.\n")
	writeFile(t, root, ".github/workflows/ci.yml", "name: CI\n")
	writeFile(t, root, "left.go", duplicateGoSource("Left"))
	writeFile(t, root, "right.go", duplicateGoSource("Right"))
	writeFile(t, root, "internal/example.go", "package internal\n")
	writeFile(t, root, "slophammer.yml", strings.Join([]string{
		"go:",
		"  coverage:",
		"    threshold: 85",
		"  dry:",
		"    max_findings: 0",
		"  crap:",
		"    max_score: 8",
		"  mutation:",
		"    targets:",
		"      - internal/example.go",
		"",
	}, "\n"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := check(context.Background(), CheckOptions{Root: root, Format: "json", Execute: true}, &out, &errOut, executeFakeRunner{})

	if code != ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	report := unmarshalReport(t, out.Bytes(), "execute")
	assertFinding(t, report, rules.GoDryRequiredRuleID)
	assertFinding(t, report, rules.GoCoverageRequiredRuleID)
	assertFinding(t, report, rules.GoCRAPRequiredRuleID)
	assertFinding(t, report, rules.GoMutationRequiredRuleID)
}

func TestCheckExecuteOnlyRunsSelectedToolChecks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Test\n")
	writeFile(t, root, "AGENTS.md", "# Agents\n\nRun `go test ./...` before finishing.\n")
	writeFile(t, root, ".github/workflows/ci.yml", "name: CI\n")
	writeFile(t, root, "left.go", duplicateGoSource("Left"))
	writeFile(t, root, "right.go", duplicateGoSource("Right"))
	writeFile(t, root, "internal/example.go", "package internal\n")
	writeFile(t, root, "slophammer.yml", strings.Join([]string{
		"go:",
		"  coverage:",
		"    threshold: 85",
		"  dry:",
		"    max_findings: 0",
		"  crap:",
		"    max_score: 8",
		"  mutation:",
		"    targets:",
		"      - internal/example.go",
		"",
	}, "\n"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	options := CheckOptions{
		Root:        root,
		Format:      "json",
		Execute:     true,
		OnlyRuleIDs: []string{rules.GoDryRequiredRuleID},
	}
	code := check(context.Background(), options, &out, &errOut, executeFakeRunner{})

	if code != ExitFindings {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitFindings, errOut.String())
	}
	report := unmarshalReport(t, out.Bytes(), "execute only")
	assertFinding(t, report, rules.GoDryRequiredRuleID)
	for _, finding := range report.Findings {
		if finding.RuleID != rules.GoDryRequiredRuleID {
			t.Fatalf("unexpected finding %q in --only run", finding.RuleID)
		}
	}
}

func TestCheckExecuteReusesConfiguredCoverageProfile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/example.go:2.1,2.2 10 1\n")
	snapshot := repo.NewSnapshot(root, map[string]repo.File{
		"internal/example.go": {Path: "internal/example.go"},
	})
	cfg := config.Config{Go: config.GoConfig{
		CoverageThreshold: 85,
		CoverageProfile:   "coverage.out",
		Targets:           []string{"internal/example.go"},
		CRAPMaxScore:      8,
	}}
	runner := &coverageProfileExecuteRunner{}

	findings := executeGoChecks(context.Background(), snapshot, CheckOptions{Root: root, Execute: true}, cfg, runner)

	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
	coverCalls := 0
	for _, call := range runner.calls {
		got := strings.Join(call.args, " ")
		if strings.HasPrefix(got, "test ") {
			t.Fatalf("unexpected go test coverage generation call: %#v", call)
		}
		if strings.HasPrefix(got, "tool cover -func=") {
			coverCalls++
			if !filepath.IsAbs(strings.TrimPrefix(got, "tool cover -func=")) {
				t.Fatalf("cover args = %q, want absolute profile path", got)
			}
		}
	}
	if coverCalls != 2 {
		t.Fatalf("coverCalls = %d, want 2; calls = %#v", coverCalls, runner.calls)
	}
}

// The static go.mutation-required rule credits `check --execute` as
// mutation evidence, so the execute path must run the survivor-failing
// gate instead of a scan.
func TestCheckExecuteRunsSurvivorFailingMutationGate(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go.mod":              {Path: "go.mod"},
		"internal/example.go": {Path: "internal/example.go"},
	})
	cfg := config.Config{Go: config.GoConfig{Targets: []string{"internal"}}}
	runner := &survivorMutationRunner{}

	findings := executeGoChecks(context.Background(), snapshot, CheckOptions{Root: "/repo", Execute: true}, cfg, runner)

	if len(findings) != 1 || findings[0].RuleID != rules.GoMutationRequiredRuleID {
		t.Fatalf("findings = %#v, want one go.mutation-required finding", findings)
	}
	if !strings.Contains(findings[0].Message, "mutation gate failed") {
		t.Fatalf("message = %q, want gate failure message", findings[0].Message)
	}
	if len(runner.calls) == 0 {
		t.Fatal("no mutation command ran")
	}
	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call.args, " "), "--scan") {
			t.Fatalf("call = %#v, want an executing run without --scan", call)
		}
	}
}

type survivorMutationRunner struct {
	calls []recordedCall
}

func (r *survivorMutationRunner) Run(_ context.Context, dir string, _ string, args ...string) (toolchecks.CommandResult, error) {
	r.calls = append(r.calls, recordedCall{dir: dir, args: append([]string(nil), args...)})
	return toolchecks.CommandResult{Stdout: []byte("Killed: 1\nSurvived: 1\n")}, nil
}

func TestApplyCommandConfigUsesConfiguredDefaults(t *testing.T) {
	cfg := config.Config{Go: config.GoConfig{
		DRYMaxCandidates:    0,
		DRYMaxCandidatesSet: true,
		DRYPaths:            []string{"go/cmd", "go/internal"},
		DRYExclude:          []string{"**/*_test.go"},
		CRAPMaxScore:        8,
		CoverageProfile:     "coverage.out",
		Targets:             []string{"go"},
		Exclude:             []string{"go/generated/**"},
	}}

	dry := toolchecks.DryOptions{}
	applyDryConfig(&dry, cfg)
	if dry.MaximumCandidates != 0 || !dry.MaximumSet {
		t.Fatalf("dry = %#v", dry)
	}
	if !reflect.DeepEqual(dry.Paths, []string{"go/cmd", "go/internal"}) || !reflect.DeepEqual(dry.Exclude, []string{"**/*_test.go"}) {
		t.Fatalf("dry paths = %#v excludes = %#v", dry.Paths, dry.Exclude)
	}

	crap := toolchecks.CRAPOptions{}
	applyCRAPConfig(&crap, cfg)
	if crap.MaximumScore != 8 || !crap.MaximumSet {
		t.Fatalf("crap = %#v", crap)
	}
	if crap.CoverageProfile != "coverage.out" {
		t.Fatalf("crap coverage profile = %q", crap.CoverageProfile)
	}

	mutation := toolchecks.MutationOptions{}
	applyMutationConfig(&mutation, cfg)
	if !reflect.DeepEqual(mutation.Targets, []string{"go"}) || !reflect.DeepEqual(mutation.Exclude, []string{"go/generated/**"}) {
		t.Fatalf("mutation = %#v", mutation)
	}
}

func TestApplyCoverageConfigUsesConfiguredScope(t *testing.T) {
	cfg := config.Config{Go: config.GoConfig{
		CoverageThreshold: 85,
		CoverageProfile:   "coverage.out",
		Targets:           []string{"go"},
		Exclude:           []string{"go/generated/**"},
	}}

	coverage := toolchecks.CoverageOptions{}
	applyCoverageConfig(&coverage, cfg)

	if coverage.Threshold != 85 || !coverage.ThresholdSet {
		t.Fatalf("coverage = %#v", coverage)
	}
	if coverage.CoverageProfile != "coverage.out" {
		t.Fatalf("coverage profile = %q", coverage.CoverageProfile)
	}
	if !reflect.DeepEqual(coverage.Targets, []string{"go"}) || !reflect.DeepEqual(coverage.Exclude, []string{"go/generated/**"}) {
		t.Fatalf("coverage targets = %#v excludes = %#v", coverage.Targets, coverage.Exclude)
	}
}

func TestApplyDryConfigFallsBackToSharedGoScope(t *testing.T) {
	cfg := config.Config{Go: config.GoConfig{
		DRYMaxCandidates:    0,
		DRYMaxCandidatesSet: true,
		Targets:             []string{"go/cmd", "go/internal"},
		Exclude:             []string{"**/*_test.go", "go/internal/testutil/**"},
	}}

	dry := toolchecks.DryOptions{}
	applyDryConfig(&dry, cfg)

	if !reflect.DeepEqual(dry.Paths, []string{"go/cmd", "go/internal"}) {
		t.Fatalf("dry paths = %#v", dry.Paths)
	}
	if !reflect.DeepEqual(dry.Exclude, []string{"**/*_test.go", "go/internal/testutil/**"}) {
		t.Fatalf("dry excludes = %#v", dry.Exclude)
	}
}

func TestApplyCRAPConfigUsesConfiguredScope(t *testing.T) {
	cfg := config.Config{Go: config.GoConfig{
		CRAPMaxScore: 8,
		Targets:      []string{"go"},
		Exclude:      []string{"go/generated/**"},
	}}

	crap := toolchecks.CRAPOptions{}
	applyCRAPConfig(&crap, cfg)

	if !reflect.DeepEqual(crap.Targets, []string{"go"}) || !reflect.DeepEqual(crap.Exclude, []string{"go/generated/**"}) {
		t.Fatalf("crap targets = %#v excludes = %#v", crap.Targets, crap.Exclude)
	}
}

func TestApplyCommandConfigKeepsExplicitValues(t *testing.T) {
	cfg := config.Config{Go: config.GoConfig{
		DRYMaxCandidates:    7,
		DRYMaxCandidatesSet: true,
		DRYPaths:            []string{"go/internal"},
		CRAPMaxScore:        8,
		Exclude:             []string{"generated/**"},
		Mutation: config.MutationConfig{
			Targets: []string{"configured.go"},
			Exclude: []string{"mutation_generated/**"},
		},
	}}

	dry := toolchecks.DryOptions{MaximumCandidates: 3, MaximumSet: true}
	applyDryConfig(&dry, cfg)
	if dry.MaximumCandidates != 3 {
		t.Fatalf("dry = %#v", dry)
	}

	crap := toolchecks.CRAPOptions{MaximumScore: 4, MaximumSet: true}
	applyCRAPConfig(&crap, cfg)
	if crap.MaximumScore != 4 {
		t.Fatalf("crap = %#v", crap)
	}
	if !reflect.DeepEqual(crap.Exclude, []string{"generated/**"}) {
		t.Fatalf("crap excludes = %#v", crap.Exclude)
	}

	mutation := toolchecks.MutationOptions{Target: "explicit.go"}
	applyMutationConfig(&mutation, cfg)
	if mutation.Target != "explicit.go" || len(mutation.Targets) != 0 || !reflect.DeepEqual(mutation.Exclude, []string{"mutation_generated/**"}) {
		t.Fatalf("mutation = %#v", mutation)
	}
}

func TestExplicitMutationTargetUsesConfiguredExcludes(t *testing.T) {
	cfg := config.Config{Go: config.GoConfig{
		Targets: []string{"internal"},
		Exclude: []string{"internal/generated/**"},
	}}
	options := toolchecks.MutationOptions{
		Root:   ".",
		Target: "internal",
		Scan:   true,
	}
	applyMutationConfig(&options, cfg)
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go.mod":                          {Path: "go.mod"},
		"internal/example.go":             {Path: "internal/example.go"},
		"internal/generated/generated.go": {Path: "internal/generated/generated.go"},
	})
	runner := &recordingRunner{}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := checkMutationInModules(context.Background(), snapshot, options, &out, &errOut, runner)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	if got := strings.Join(runner.calls[0].args, " "); !strings.Contains(got, "internal/example.go --scan") || strings.Contains(got, "generated.go") {
		t.Fatalf("args = %q", got)
	}
}

func TestCheckMutationInModulesRunsResolvedTargetsFromNestedModule(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":              {Path: "go/go.mod"},
		"go/internal/example.go": {Path: "go/internal/example.go"},
	})
	runner := &recordingRunner{}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := checkMutationInModules(context.Background(), snapshot, toolchecks.MutationOptions{
		Root:    ".",
		Targets: []string{"go/internal"},
		Scan:    true,
	}, &out, &errOut, runner)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	call := runner.calls[0]
	if call.dir != "go" {
		t.Fatalf("dir = %q, want go", call.dir)
	}
	if got := strings.Join(call.args, " "); !strings.Contains(got, "internal/example.go --scan") {
		t.Fatalf("args = %q", got)
	}
}

func TestCheckCRAPInModulesUsesConfiguredGoTargets(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"backend/go.mod":                           {Path: "backend/go.mod"},
		"backend/cmd/server/main.go":               {Path: "backend/cmd/server/main.go"},
		"backend/internal/service/service.go":      {Path: "backend/internal/service/service.go"},
		"backend/internal/testutil/testutil.go":    {Path: "backend/internal/testutil/testutil.go"},
		"backend/internal/service/service_test.go": {Path: "backend/internal/service/service_test.go"},
		"engine/go.mod":                            {Path: "engine/go.mod"},
		"engine/main.go":                           {Path: "engine/main.go"},
	})
	runner := &recordingRunner{}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := checkCRAPInModules(context.Background(), snapshot, toolchecks.CRAPOptions{
		Root:         ".",
		MaximumScore: 8,
		MaximumSet:   true,
		Targets:      []string{"backend/cmd", "backend/internal"},
		Exclude:      []string{"**/*_test.go", "backend/internal/testutil/**"},
	}, &out, &errOut, runner)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if len(runner.calls) != 7 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	call := runner.calls[3]
	if call.dir != "backend" {
		t.Fatalf("dir = %q, want backend", call.dir)
	}
	got := strings.Join(call.args, " ")
	if !strings.Contains(got, "gocyclo") ||
		!strings.Contains(got, "cmd/server/main.go") ||
		!strings.Contains(got, "internal/service/service.go") {
		t.Fatalf("args = %q", got)
	}
	if strings.Contains(got, "testutil") || strings.Contains(got, "service_test.go") || strings.Contains(got, "engine") {
		t.Fatalf("args = %q", got)
	}
}

func TestCheckCoverageInModulesUsesConfiguredGoTargets(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"backend/go.mod":                           {Path: "backend/go.mod"},
		"backend/cmd/server/main.go":               {Path: "backend/cmd/server/main.go"},
		"backend/internal/service/service.go":      {Path: "backend/internal/service/service.go"},
		"backend/internal/testutil/testutil.go":    {Path: "backend/internal/testutil/testutil.go"},
		"backend/internal/service/service_test.go": {Path: "backend/internal/service/service_test.go"},
		"engine/go.mod":                            {Path: "engine/go.mod"},
		"engine/main.go":                           {Path: "engine/main.go"},
	})
	runner := &recordingRunner{}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := checkCoverageInModules(context.Background(), snapshot, toolchecks.CoverageOptions{
		Root:         ".",
		Threshold:    85,
		ThresholdSet: true,
		Targets:      []string{"backend/cmd", "backend/internal"},
		Exclude:      []string{"**/*_test.go", "backend/internal/testutil/**"},
	}, &out, &errOut, runner)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if len(runner.calls) != 5 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	call := runner.calls[3]
	if call.dir != "backend" {
		t.Fatalf("dir = %q, want backend", call.dir)
	}
	got := strings.Join(call.args, " ")
	if !strings.Contains(got, "-coverpkg=example.test/backend/cmd/server,example.test/backend/internal/service") {
		t.Fatalf("args = %q", got)
	}
	if strings.Contains(got, "testutil") || strings.Contains(got, "service_test.go") || strings.Contains(got, "engine") {
		t.Fatalf("args = %q", got)
	}
}

func TestCheckMutationInModulesRebasesExcludesInSingleModuleFallback(t *testing.T) {
	for _, exclude := range [][]string{
		{"generated/**"},
		{"internal/generated/**"},
	} {
		t.Run(strings.Join(exclude, ","), func(t *testing.T) {
			snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
				"go/go.mod":                      {Path: "go/go.mod"},
				"go/internal/example.go":         {Path: "go/internal/example.go"},
				"go/internal/generated/model.go": {Path: "go/internal/generated/model.go"},
			})
			runner := &recordingRunner{}

			var out bytes.Buffer
			var errOut bytes.Buffer
			code := checkMutationInModules(context.Background(), snapshot, toolchecks.MutationOptions{
				Root:    ".",
				Targets: []string{"internal"},
				Exclude: exclude,
				Scan:    true,
			}, &out, &errOut, runner)

			if code != ExitOK {
				t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
			}
			if len(runner.calls) != 1 {
				t.Fatalf("calls = %#v", runner.calls)
			}
			if got := strings.Join(runner.calls[0].args, " "); !strings.Contains(got, "internal/example.go --scan") || strings.Contains(got, "generated/model.go") {
				t.Fatalf("args = %q", got)
			}
		})
	}
}

func TestRunWithCommandConfigLoadsRepoConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "slophammer.yml", "go:\n  crap:\n    max_score: 8\n")

	var errOut bytes.Buffer
	code := runWithCommandConfig(root, &errOut, func(snapshot repo.Snapshot, cfg config.Config) int {
		if snapshot.Root != root {
			t.Fatalf("snapshot.Root = %q, want %q", snapshot.Root, root)
		}
		if cfg.Go.CRAPMaxScore != 8 {
			t.Fatalf("CRAPMaxScore = %v, want 8", cfg.Go.CRAPMaxScore)
		}
		return ExitOK
	})

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
}

func TestRulesWritesRuleCatalog(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := Rules(RulesOptions{}, &out, &errOut)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if !strings.Contains(out.String(), "repo.readme-required") ||
		!strings.Contains(out.String(), "go.dry-required") {
		t.Fatalf("rules output = %q", out.String())
	}
}

func TestRulesWritesJSONRuleCatalog(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := Rules(RulesOptions{Format: "json"}, &out, &errOut)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	var definitions []rules.Definition
	if err := json.Unmarshal(out.Bytes(), &definitions); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(definitions) == 0 || definitions[0].ID != rules.ReadmeRequiredRuleID {
		t.Fatalf("definitions = %#v", definitions)
	}
}

func TestRunWithCommandConfigRejectsInvalidRoot(t *testing.T) {
	var errOut bytes.Buffer
	code := runWithCommandConfig(filepath.Join(t.TempDir(), "missing"), &errOut, func(repo.Snapshot, config.Config) int {
		t.Fatal("run should not be called")
		return ExitOK
	})

	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "config failed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestExplain(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Explain("repo.readme-required", &out, &errOut)

	if code != ExitOK {
		t.Fatalf("code = %d, want %d; stderr=%q", code, ExitOK, errOut.String())
	}
	if !strings.Contains(out.String(), "repo.readme-required") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestExplainRejectsUnknownRule(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Explain("missing", &out, &errOut)

	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(errOut.String(), "unknown rule") {
		t.Fatalf("stderr = %q", errOut.String())
	}
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

func duplicateGoSource(name string) string {
	return `package sample

func ` + name + `(items []int) []int {
	var kept []int
	for _, item := range items {
		if item%2 == 0 {
			kept = append(kept, item+1)
		}
	}
	return kept
}
`
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller returned ok=false")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func checkSharedFixture(t *testing.T, name string, wantCode int) {
	t.Helper()
	root := repoRoot(t)
	fixtureRoot := filepath.Join(root, "fixtures", "repos", name)
	expectedPath := filepath.Join(root, "fixtures", "expected", name+".json")

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Check(context.Background(), CheckOptions{Root: fixtureRoot, Format: "json"}, &out, &errOut)

	if code != wantCode {
		t.Fatalf("code = %d, want %d; stderr=%q", code, wantCode, errOut.String())
	}

	got := unmarshalReport(t, out.Bytes(), "actual")
	// #nosec G304 -- test fixtures are read from a path derived from the test name table.
	expectedContent, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected report: %v", err)
	}
	want := unmarshalReport(t, expectedContent, "expected")

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("report mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func unmarshalReport(t *testing.T, content []byte, label string) rules.Report {
	t.Helper()
	var report rules.Report
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("unmarshal %s report: %v\n%s", label, err, string(content))
	}
	return report
}

func assertFinding(t *testing.T, report rules.Report, ruleID string) {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.RuleID == ruleID {
			return
		}
	}
	t.Fatalf("missing finding %s in %#v", ruleID, report.Findings)
}

type executeFakeRunner struct{}

func (executeFakeRunner) Run(_ context.Context, _ string, _ string, args ...string) (toolchecks.CommandResult, error) {
	command := strings.Join(args, " ")
	switch {
	case strings.Contains(command, "dry4go"):
		return toolchecks.CommandResult{Stdout: []byte(`{"candidates":[{},{}]}`)}, nil
	case command == "list -m":
		return toolchecks.CommandResult{Stdout: []byte("example.test/backend\n")}, nil
	case strings.HasPrefix(command, "list "):
		return toolchecks.CommandResult{Stdout: []byte("example.test/backend/internal/example\n")}, nil
	case strings.HasPrefix(command, "test "):
		return toolchecks.CommandResult{}, nil
	case strings.HasPrefix(command, "tool cover -func="):
		return toolchecks.CommandResult{Stdout: []byte("total:\t(statements)\t84.9%\n")}, nil
	case strings.Contains(command, "crap4go"):
		return toolchecks.CommandResult{Stdout: []byte("pkg.Func 1 2 3 10.1\n")}, nil
	case strings.Contains(command, "mutate4go"):
		return toolchecks.CommandResult{}, errors.New("boom")
	default:
		return toolchecks.CommandResult{}, errors.New("unexpected command")
	}
}

type coverageProfileExecuteRunner struct {
	calls []recordedCall
}

func (r *coverageProfileExecuteRunner) Run(_ context.Context, dir string, _ string, args ...string) (toolchecks.CommandResult, error) {
	r.calls = append(r.calls, recordedCall{dir: dir, args: append([]string(nil), args...)})
	command := strings.Join(args, " ")
	switch {
	case command == "list -m":
		return toolchecks.CommandResult{Stdout: []byte("example.test/backend\n")}, nil
	case strings.HasPrefix(command, "list -f "):
		return toolchecks.CommandResult{Stdout: []byte(filepath.Join(dir, "internal") + "|example.go\n")}, nil
	case strings.HasPrefix(command, "list ./internal"):
		return toolchecks.CommandResult{Stdout: []byte("example.test/backend/internal\n")}, nil
	case strings.HasPrefix(command, "tool cover -func="):
		return toolchecks.CommandResult{Stdout: []byte("example.test/backend/internal/example.go:2:\tRun\t100.0%\ntotal:\t(statements)\t100.0%\n")}, nil
	case strings.Contains(command, "gocyclo"):
		return toolchecks.CommandResult{Stdout: []byte("1 internal Run internal/example.go:2:1\n")}, nil
	case strings.Contains(command, "mutate4go"):
		return toolchecks.CommandResult{}, nil
	default:
		return toolchecks.CommandResult{}, errors.New("unexpected command: " + command)
	}
}

type recordingRunner struct {
	calls []recordedCall
}

type recordedCall struct {
	dir  string
	args []string
}

func (r *recordingRunner) Run(_ context.Context, dir string, _ string, args ...string) (toolchecks.CommandResult, error) {
	r.calls = append(r.calls, recordedCall{dir: dir, args: append([]string(nil), args...)})
	command := strings.Join(args, " ")
	switch {
	case command == "list -m":
		return toolchecks.CommandResult{Stdout: []byte("example.test/backend\n")}, nil
	case strings.HasPrefix(command, "list -f "):
		root, _ := filepath.Abs(dir)
		return toolchecks.CommandResult{Stdout: []byte(
			filepath.Join(root, "cmd/server") + "|main.go\n" +
				filepath.Join(root, "internal/service") + "|service.go\n",
		)}, nil
	case strings.HasPrefix(command, "list ./"):
		return toolchecks.CommandResult{Stdout: []byte("example.test/backend/cmd/server\nexample.test/backend/internal/service\n")}, nil
	case strings.HasPrefix(command, "test "):
		return toolchecks.CommandResult{}, nil
	case strings.Contains(command, "gocyclo"):
		return toolchecks.CommandResult{Stdout: []byte("1 service Run internal/service/service.go:12:1\n")}, nil
	case strings.HasPrefix(command, "tool cover -func="):
		return toolchecks.CommandResult{Stdout: []byte("example.test/backend/internal/service/service.go:12:\tRun\t85.7%\ntotal:\t(statements)\t85.7%\n")}, nil
	}
	return toolchecks.CommandResult{}, nil
}
